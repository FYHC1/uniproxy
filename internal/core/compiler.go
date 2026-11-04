// Package core compiles and supervises the pinned, external Mihomo process.
// It does not implement proxy selection in Go: selection belongs to native groups.
package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/uniproxy/uniproxy/internal/coreinstall"
	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

// PinnedVersion is the version whose native hash-key, listeners and reload APIs
// have been verified. In particular older cores can silently ignore hash-key.
const PinnedVersion = coreinstall.PinnedVersion

const probeName = "uniproxy-probe"
const defaultTestURL = "https://www.gstatic.com/generate_204"

// BuildOptions are private management endpoints, never public proxy listeners.
// Both addresses must contain a literal loopback IP and a nonzero port. Secrets
// must be nonempty. Manager generates independent random 256-bit secrets.
type BuildOptions struct {
	ControllerAddress string
	ControllerSecret  string
	ProbeAddress      string
	ProbeSecret       string
}

type compiledConfig struct {
	data      []byte
	revision  int64
	nodes     map[string]struct{}
	nodeTags  map[string]string
	chainTags map[string]string
	chainHops map[string][]string
	listeners []model.Listener
	probe     model.Listener
	runtime   []model.ListenerRuntime
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Build returns deterministic YAML without mutating state or its native maps.
// Names are presentation only; all routing identities derive from stable IDs.
func Build(state model.State, opts BuildOptions) ([]byte, error) {
	cfg, err := compile(state, opts)
	if err != nil {
		return nil, err
	}
	return cfg.data, nil
}

func compile(state model.State, opts BuildOptions) (*compiledConfig, error) {
	controllerHost, controllerPort, err := privateAddress(opts.ControllerAddress)
	if err != nil {
		return nil, errors.New("controller must use a literal loopback IP and nonzero port")
	}
	probeHost, probePort, err := privateAddress(opts.ProbeAddress)
	if err != nil {
		return nil, errors.New("probe must use a literal loopback IP and nonzero port")
	}
	if opts.ControllerSecret == "" || opts.ProbeSecret == "" || strings.ContainsAny(opts.ControllerSecret, "\r\n") {
		return nil, errors.New("controller and probe authentication secrets are required")
	}
	if addressesOverlap(controllerHost, controllerPort, probeHost, probePort) {
		return nil, errors.New("controller and probe addresses conflict")
	}
	cfg := &compiledConfig{revision: state.Revision, nodes: map[string]struct{}{}, nodeTags: map[string]string{}, chainTags: map[string]string{}, chainHops: map[string][]string{}, listeners: []model.Listener{}, runtime: []model.ListenerRuntime{}}
	nodes := append([]model.Node(nil), state.Nodes...)
	pools := append([]model.Pool(nil), state.Pools...)
	chains := append([]model.Chain(nil), state.Chains...)
	listeners := append([]model.Listener(nil), state.Listeners...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(pools, func(i, j int) bool { return pools[i].ID < pools[j].ID })
	sort.Slice(chains, func(i, j int) bool { return chains[i].ID < chains[j].ID })
	sort.Slice(listeners, func(i, j int) bool { return listeners[i].ID < listeners[j].ID })
	if err := model.ValidateRelayGraph(nodes); err != nil {
		return nil, err
	}
	blocked := model.RelayErrors(nodes)
	nodeByID := map[string]model.Node{}
	poolByID := map[string]model.Pool{}
	chainByID := map[string]model.Chain{}
	proxies := []map[string]any{}
	groups := []map[string]any{}
	inbounds := []map[string]any{}
	probeUsers := []map[string]string{}
	probeRules := []string{}

	// Resolve every node before compiling so a relay may refer to a node that
	// appears later in the persisted slice. The compiled tag must use the same
	// proxyTag mapping as the referenced outbound (sidecar nodes expose x-<ID>,
	// native nodes expose n-<ID>); otherwise Mihomo refers to a missing outbound.
	for _, node := range nodes {
		if !safeID.MatchString(node.ID) {
			return nil, errors.New("node has an invalid stable ID")
		}
		if _, ok := nodeByID[node.ID]; ok {
			return nil, errors.New("duplicate node ID")
		}
		nodeByID[node.ID] = node
	}
	for _, node := range nodes {
		if !node.Enabled || blocked[node.ID] != "" {
			continue
		}
		if node.IsSidecar() && node.DialerID != "" {
			return nil, errors.New("sidecar 节点不能同时设置中转")
		}
		proxy, err := outboundProxy(node)
		if err != nil {
			return nil, fmt.Errorf("node %s: %w", node.ID, err)
		}
		if node.DialerID != "" {
			if !safeID.MatchString(node.DialerID) {
				return nil, errors.New("invalid stable relay ID")
			}
			dialer, ok := nodeByID[node.DialerID]
			if !ok {
				return nil, errors.New("中转节点已删除或缺失，禁止直连回退")
			}
			proxy["dialer-proxy"] = proxyTag(dialer)
		}
		proxies = append(proxies, proxy)
		cfg.nodes[node.ID] = struct{}{}
		cfg.nodeTags[node.ID] = proxyTag(node)
		username := nodeTag(node.ID)
		probeUsers = append(probeUsers, map[string]string{"username": username, "password": probePassword(opts.ProbeSecret, node.ID)})
		probeRules = append(probeRules, "IN-USER,"+username+","+proxyTag(node))
	}

	for _, chain := range chains {
		if !safeID.MatchString(chain.ID) {
			return nil, errors.New("chain has an invalid stable ID")
		}
		if _, ok := chainByID[chain.ID]; ok {
			return nil, errors.New("duplicate chain ID")
		}
		chainByID[chain.ID] = chain
		if !chain.Enabled || len(chain.NodeIDs) < 2 || len(chain.NodeIDs) > 16 {
			continue
		}
		tags := make([]string, 0, len(chain.NodeIDs))
		chainProxies := make([]map[string]any, 0, len(chain.NodeIDs))
		seen := make(map[string]struct{}, len(chain.NodeIDs))
		valid := true
		for index, id := range chain.NodeIDs {
			n, ok := nodeByID[id]
			if !ok || !n.Enabled || blocked[id] != "" || n.DialerID != "" {
				valid = false
				break
			}
			if _, duplicate := seen[id]; duplicate {
				valid = false
				break
			}
			seen[id] = struct{}{}
			if index > 0 && n.IsSidecar() {
				return nil, model.ErrSidecarMustBeChainHead
			}
			proxy, err := outboundProxy(n)
			if err != nil {
				valid = false
				break
			}
			tag := chainHopTag(chain.ID, index+1)
			proxy["name"] = tag
			if index > 0 {
				proxy["dialer-proxy"] = tags[index-1]
			}
			chainProxies = append(chainProxies, proxy)
			tags = append(tags, tag)
		}
		if valid && len(tags) == len(chain.NodeIDs) {
			proxies = append(proxies, chainProxies...)
			cfg.chainTags[chain.ID] = tags[len(tags)-1]
			cfg.chainHops[chain.ID] = append([]string(nil), tags...)
		}
	}

	for _, pool := range pools {
		if !safeID.MatchString(pool.ID) {
			return nil, errors.New("pool has an invalid stable ID")
		}
		if _, ok := poolByID[pool.ID]; ok {
			return nil, errors.New("duplicate pool ID")
		}
		poolByID[pool.ID] = pool
		if pool.GroupMode != "" && pool.GroupMode != "any" && pool.GroupMode != "all" {
			return nil, errors.New("invalid pool group mode")
		}
		switch pool.Strategy {
		case "url-test", "round-robin", "consistent-hashing", "sticky-sessions":
		default:
			return nil, errors.New("unsupported native pool strategy")
		}
		if pool.HashKey != "" && (pool.HashKey != "in-user" || pool.Strategy == "url-test" || pool.Strategy == "round-robin") {
			return nil, errors.New("hash-key in-user requires consistent-hashing or sticky-sessions")
		}
		members := model.PoolMembers(state, pool)
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		tags := make([]string, 0, len(members))
		for _, n := range members {
			tags = append(tags, proxyTag(n))
		}
		if len(tags) == 0 {
			tags = append(tags, "REJECT")
		}
		testURL := pool.TestURL
		if testURL == "" {
			testURL = state.Settings.TestURL
		}
		if testURL == "" {
			testURL = defaultTestURL
		}
		if err := validTestURL(testURL); err != nil {
			return nil, errors.New("pool test URL must be an HTTP(S) URL without credentials")
		}
		interval, timeout := pool.IntervalSeconds, pool.TimeoutMS
		if interval == 0 {
			interval = 300
		}
		if timeout == 0 {
			timeout = 8000
		}
		if interval < 1 || interval > 86400 || timeout < 1 || timeout > 600000 || pool.ToleranceMS < 0 || pool.ToleranceMS > 65535 {
			return nil, errors.New("pool health-check bounds are invalid")
		}
		group := map[string]any{"name": poolTag(pool.ID), "proxies": tags, "url": testURL, "interval": interval, "timeout": timeout, "lazy": false, "expected-status": "200-399", "disable-udp": !pool.UDP, "empty-fallback": "REJECT"}
		if pool.Strategy == "url-test" {
			group["type"] = "url-test"
			group["tolerance"] = pool.ToleranceMS
		} else {
			group["type"] = "load-balance"
			group["strategy"] = pool.Strategy
			if pool.HashKey != "" {
				group["hash-key"] = pool.HashKey
			}
		}
		groups = append(groups, group)
	}

	listenerIDs := map[string]bool{}
	for _, listener := range listeners {
		if !safeID.MatchString(listener.ID) {
			return nil, errors.New("listener has an invalid stable ID")
		}
		if listenerIDs[listener.ID] {
			return nil, errors.New("duplicate listener ID")
		}
		listenerIDs[listener.ID] = true
		runtime := model.ListenerRuntime{ID: listener.ID}
		if !listener.Enabled {
			cfg.runtime = append(cfg.runtime, runtime)
			continue
		}
		switch listener.Protocol {
		case "http", "socks", "mixed":
		default:
			return nil, errors.New("unsupported listener protocol")
		}
		if listener.Protocol == "http" && listener.UDP {
			return nil, errors.New("HTTP listeners do not support UDP")
		}
		host, err := listenerHost(listener.Host)
		if err != nil || listener.Port < 1 || listener.Port > 65535 {
			return nil, errors.New("listener must have a literal IP address and port 1-65535")
		}
		listener.Host = host
		ip, _ := netip.ParseAddr(host)
		if listener.UDP && !ip.IsLoopback() {
			return nil, errors.New("Mihomo SOCKS UDP does not authenticate packets; UDP listeners must bind loopback")
		}
		if err := validUsers(listener.Users, host); err != nil {
			return nil, err
		}
		if addressesOverlap(host, listener.Port, controllerHost, controllerPort) || addressesOverlap(host, listener.Port, probeHost, probePort) {
			return nil, errors.New("listener conflicts with a private management endpoint")
		}
		for _, prev := range cfg.listeners {
			if addressesOverlap(host, listener.Port, prev.Host, prev.Port) {
				return nil, errors.New("enabled listener bind addresses conflict")
			}
		}
		target := "REJECT"
		switch listener.Kind {
		case "node":
			if n, ok := nodeByID[listener.TargetID]; ok && n.Enabled && blocked[n.ID] == "" {
				if listener.UDP && !n.UDP {
					return nil, errors.New("UDP listener targets a TCP-only node")
				}
				target = proxyTag(n)
			}
		case "pool":
			if p, ok := poolByID[listener.TargetID]; ok && p.Enabled {
				if listener.UDP && !p.UDP {
					return nil, errors.New("UDP listener requires a UDP-capable pool")
				}
				if p.HashKey == "in-user" && listener.UDP {
					return nil, errors.New("Mihomo UDP packets have no inbound username; in-user pools require TCP-only listeners")
				}
				if p.HashKey == "in-user" && len(listener.Users) == 0 {
					return nil, errors.New("in-user pools require authenticated listener users")
				}
				target = poolTag(p.ID)
				if len(model.PoolMembers(state, p)) == 0 {
					runtime.Error = "pool has no eligible members; traffic is rejected"
				}
			}
		case "chain":
			if c, ok := chainByID[listener.TargetID]; ok && c.Enabled {
				if tag, live := cfg.chainTags[c.ID]; live {
					if listener.UDP {
						for _, id := range c.NodeIDs {
							if n, exists := nodeByID[id]; !exists || !n.UDP {
								return nil, errors.New("UDP listener targets a chain with a TCP-only node")
							}
						}
					}
					target = tag
				}
			}
		default:
			return nil, errors.New("listener target kind must be node, pool or chain")
		}
		if target == "REJECT" {
			runtime.Error = "target or relay dependency is absent or disabled; traffic is rejected"
		}
		users := make([]map[string]string, 0, len(listener.Users))
		for _, user := range listener.Users {
			users = append(users, map[string]string{"username": user.Username, "password": user.Password})
		}
		inbounds = append(inbounds, map[string]any{"name": listenerTag(listener.ID), "type": listener.Protocol, "listen": host, "port": listener.Port, "udp": listener.UDP, "users": users, "proxy": target})
		listener.Users = append([]model.Credential(nil), listener.Users...)
		cfg.listeners = append(cfg.listeners, listener)
		cfg.runtime = append(cfg.runtime, runtime)
	}
	// Never emit users: [] on the private probe: that explicitly disables auth.
	if len(probeUsers) == 0 {
		probeUsers = append(probeUsers, map[string]string{"username": "blocked", "password": probePassword(opts.ProbeSecret, "")})
	}
	probeRules = append(probeRules, "MATCH,REJECT")
	inbounds = append(inbounds, map[string]any{"name": probeName, "type": "mixed", "listen": probeHost, "port": probePort, "udp": false, "users": probeUsers, "rule": probeName})
	cfg.probe = model.Listener{ID: probeName, Protocol: "mixed", Host: probeHost, Port: probePort, Enabled: true, Users: []model.Credential{{Username: probeUsers[0]["username"], Password: probeUsers[0]["password"]}}}
	document := map[string]any{
		"mode": "rule", "log-level": "warning", "ipv6": true, "allow-lan": false, "bind-address": "127.0.0.1",
		"port": 0, "socks-port": 0, "mixed-port": 0, "redir-port": 0, "tproxy-port": 0,
		"external-controller": opts.ControllerAddress, "secret": opts.ControllerSecret,
		"external-controller-cors": map[string]any{"allow-origins": []string{}, "allow-private-network": false},
		"authentication":           []string{}, "skip-auth-prefixes": []string{},
		"find-process-mode": "off", "geo-auto-update": false,
		"dns": map[string]any{"enable": false}, "tun": map[string]any{"enable": false}, "sniffer": map[string]any{"enable": false},
		"profile": map[string]any{"store-selected": false, "store-fake-ip": false},
		"proxies": proxies, "proxy-groups": groups, "listeners": inbounds,
		"sub-rules": map[string]any{probeName: probeRules}, "rules": []string{"MATCH,REJECT"},
	}
	if name := strings.TrimSpace(state.Settings.OutboundInterface); name != "" {
		// Administrator-only global pin. Per-node interface-name remains blocked
		// by safeNativeValue so an imported subscription can never choose its own
		// source interface.
		document["interface-name"] = name
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return nil, errors.New("could not encode native core configuration")
	}
	cfg.data = data
	return cfg, nil
}

func nodeTag(id string) string    { return "n-" + id }
func sidecarTag(id string) string { return "x-" + id }
func proxyTag(node model.Node) string {
	if node.IsSidecar() {
		return sidecarTag(node.ID)
	}
	return nodeTag(node.ID)
}
func poolTag(id string) string                { return "p-" + id }
func chainHopTag(id string, index int) string { return "c-" + id + "-" + strconv.Itoa(index) }
func listenerTag(id string) string            { return "l-" + id }
func probePassword(secret, id string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte("uniproxy-probe:" + id))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func privateAddress(address string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return "", 0, errors.New("not loopback")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", 0, errors.New("invalid port")
	}
	return ip.String(), p, nil
}
func listenerHost(host string) (string, error) {
	if host == "" || host == "localhost" {
		return "127.0.0.1", nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return "", errors.New("not a literal IP")
	}
	return ip.Unmap().String(), nil
}
func addressesOverlap(a string, ap int, b string, bp int) bool {
	if ap != bp {
		return false
	}
	ai, _ := netip.ParseAddr(a)
	bi, _ := netip.ParseAddr(b)
	if ai.Unmap() == bi.Unmap() {
		return true
	}
	// Treat an IPv6 wildcard as dual-stack. Rejecting an ambiguous bind is safer
	// than accepting a platform-dependent collision.
	if ai.IsUnspecified() || bi.IsUnspecified() {
		return ai.Is6() || bi.Is6() || ai.Is4() == bi.Is4()
	}
	return false
}
func validUsers(users []model.Credential, host string) error {
	ip, _ := netip.ParseAddr(host)
	if !ip.IsLoopback() && len(users) == 0 {
		return errors.New("non-loopback listeners require authentication")
	}
	seen := map[string]bool{}
	for _, user := range users {
		if len(user.Username) == 0 || len(user.Username) > 255 || strings.ContainsAny(user.Username, ":\r\n\x00") || len(user.Password) == 0 || len(user.Password) > 255 || strings.ContainsAny(user.Password, "\r\n\x00") {
			return errors.New("listener credentials must be nonempty, valid HTTP/SOCKS credentials of at most 255 bytes")
		}
		if seen[user.Username] {
			return errors.New("duplicate listener username")
		}
		seen[user.Username] = true
	}
	return nil
}
func validTestURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 8192 {
		return errors.New("test URL must be an HTTP(S) URL without credentials or a fragment")
	}
	return nil
}

// outboundProxy chooses the native Mihomo adapter or the loopback SOCKS
// adapter that represents a sidecar node. The real remote address is never
// copied into the Mihomo document for a sidecar node.
func outboundProxy(node model.Node) (map[string]any, error) {
	if node.IsSidecar() {
		return sidecarProxy(node)
	}
	return nativeProxy(node)
}

func sidecarProxy(node model.Node) (map[string]any, error) {
	endpoint := node.SidecarEndpoint
	if endpoint == nil {
		return nil, errors.New("sidecar 节点缺少本地端点")
	}
	ip, err := netip.ParseAddr(endpoint.Host)
	if err != nil || !ip.IsLoopback() || ip.String() != "127.0.0.1" {
		return nil, errors.New("sidecar 端点必须监听 127.0.0.1")
	}
	if endpoint.Port < 1024 || endpoint.Port > 65535 {
		return nil, errors.New("sidecar endpoint port is invalid")
	}
	if endpoint.Username == "" || len(endpoint.Username) > 255 || strings.ContainsAny(endpoint.Username, ":\r\n\x00") || endpoint.Password == "" || len(endpoint.Password) > 255 || strings.ContainsAny(endpoint.Password, "\r\n\x00") {
		return nil, errors.New("sidecar endpoint credentials are invalid")
	}
	proxy := map[string]any{
		"name": sidecarTag(node.ID), "type": "socks5", "server": "127.0.0.1", "port": float64(endpoint.Port),
		"username": endpoint.Username, "password": endpoint.Password, "udp": node.UDP && node.Protocol != "http",
	}
	return proxy, nil
}

func nativeProxy(node model.Node) (map[string]any, error) {
	// JSON cloning accepts the importer's native maps, but not arbitrary YAML
	// tags, functions, or pointer graphs. It also detects cyclic backup content.
	encoded, err := json.Marshal(node.Config)
	if err != nil || len(encoded) > 1<<20 {
		return nil, errors.New("native outbound configuration is invalid or too large")
	}
	var native map[string]any
	if err = json.Unmarshal(encoded, &native); err != nil || native == nil {
		return nil, errors.New("native outbound configuration is required")
	}
	typ, ok := native["type"].(string)
	if !ok || typ != node.Protocol {
		return nil, errors.New("outbound type must use the canonical native Mihomo protocol")
	}
	switch typ {
	case "vless", "vmess", "trojan", "ss", "hysteria", "hysteria2", "tuic", "ssh", "socks5", "http":
	default:
		return nil, errors.New("unsupported outbound protocol")
	}
	if err := safeNativeValue(native, 0); err != nil {
		return nil, err
	}
	server, ok := native["server"].(string)
	if !ok || server == "" || strings.ContainsAny(server, "\r\n\x00") {
		return nil, errors.New("native outbound server is required")
	}
	port, ok := native["port"].(float64)
	if !ok || port < 1 || port > 65535 || port != float64(int(port)) {
		return nil, errors.New("native outbound port is invalid")
	}
	if node.UDP && (typ == "http" || typ == "ssh") {
		return nil, errors.New("TCP-only outbound cannot advertise UDP")
	}
	// These native adapters have an explicit UDP opt-in. Metadata is canonical;
	// do not infer UDP from aliases or from a protocol display label.
	switch typ {
	case "vless", "vmess", "trojan", "ss", "socks5":
		native["udp"] = node.UDP
	}
	native["name"] = nodeTag(node.ID)
	return native, nil
}
func safeNativeValue(value any, depth int) error {
	if depth > 32 {
		return errors.New("native outbound nesting is too deep")
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch strings.ToLower(key) {
			case "dialer-proxy", "detour", "script", "scripts", "interface-name", "routing-mark", "private-key-path", "certificate-path", "ca-path", "plugin-path":
				return errors.New("native outbound contains a forbidden routing or file option")
			case "private-key", "certificate", "ca":
				if text, ok := child.(string); ok && text != "" && !strings.HasPrefix(strings.TrimSpace(text), "-----BEGIN ") {
					return errors.New("native outbound certificate/key must be inline PEM, not a file path")
				}
			}
			if err := safeNativeValue(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := safeNativeValue(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
