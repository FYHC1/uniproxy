package app

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

// Only editable listener fields are accepted here. Identity and the pool binding
// are generated together on the server, never merged from client input.
type poolListenerInput struct {
	Name     string             `json:"name"`
	Protocol string             `json:"protocol"`
	Host     string             `json:"host"`
	Port     int                `json:"port"`
	UDP      bool               `json:"udp"`
	Enabled  *bool              `json:"enabled"`
	Users    []model.Credential `json:"users"`
}

func (a *App) createPoolWithListener(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Pool     map[string]json.RawMessage `json:"pool"`
		Listener *poolListenerInput         `json:"listener"`
	}
	if err := decode(w, r, &body); err != nil {
		return nil, err
	}
	if body.Pool == nil || body.Listener == nil {
		return nil, bad("请同时提供代理池和监听配置")
	}
	if err := filterPatch(body.Pool, "name", "strategy", "hashKey", "nodeIds", "groupIds", "groupMode", "excludeNodeIds", "excludeGroupIds", "enabled", "udp", "testUrl", "intervalSeconds", "timeoutMs", "toleranceMs"); err != nil {
		return nil, err
	}
	pool := model.Pool{Enabled: true, Strategy: "url-test", GroupMode: "any", TestURL: "https://www.gstatic.com/generate_204", IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50}
	if err := jsonMerge(&pool, body.Pool); err != nil {
		return nil, bad("代理池字段格式无效")
	}
	input := body.Listener
	listener := model.Listener{Name: input.Name, Protocol: input.Protocol, Host: input.Host, Port: input.Port, UDP: input.UDP, Enabled: true, Users: append([]model.Credential{}, input.Users...)}
	if listener.Name == "" {
		listener.Name = pool.Name
	}
	if listener.Protocol == "" {
		listener.Protocol = "mixed"
	}
	if listener.Host == "" {
		listener.Host = "127.0.0.1"
	}
	if input.Enabled != nil {
		listener.Enabled = *input.Enabled
	}
	for i := range listener.Users {
		listener.Users[i].HasPassword = false
	}
	// Reject hostnames and unsafe transports before any OS bind/allocation. No
	// DNS resolution is needed to create a local listener.
	if net.ParseIP(listener.Host) == nil {
		return nil, bad("监听地址必须是 IP，不能使用域名")
	}
	if !contains([]string{"mixed", "http", "socks"}, listener.Protocol) || (listener.UDP && listener.Protocol == "http") {
		return nil, bad("监听协议/UDP 组合无效")
	}
	if listener.UDP && !loopback(listener.Host) {
		return nil, bad("原生 SOCKS5 UDP 仅允许本机回环监听")
	}
	if !loopback(listener.Host) && len(listener.Users) == 0 {
		return nil, bad("对外监听必须配置认证")
	}
	if listener.Port != 0 && (listener.Port < 1024 || listener.Port > 65535) {
		return nil, bad("监听端口范围必须为 1024–65535，或使用 0 自动分配")
	}

	return a.mutate(r.Context(), "创建代理池及共享入口", func(s *model.State) (any, error) {
		used := reservedListenerPorts(*s, a.opts.AdminPort)
		if listener.Port == 0 {
			port, err := allocateListenerPort(r.Context(), s.Settings, listener.Host, listener.UDP, used)
			if err != nil {
				return nil, err
			}
			listener.Port = port
		} else {
			if used[listener.Port] {
				return nil, bad("代理端口已被管理服务或其他入口配置使用")
			}
			if r.Context().Err() != nil {
				return nil, bad("端口分配请求已取消")
			}
			available, err := listenerPortAvailable(listener.Host, listener.Port, listener.UDP)
			if err != nil {
				return nil, err
			}
			if !available {
				return nil, bad("指定的代理端口已被其他程序占用")
			}
		}
		if r.Context().Err() != nil {
			return nil, bad("端口分配请求已取消")
		}
		now := model.Now()
		pool.ID, pool.CreatedAt, pool.UpdatedAt = ID(), now, now
		listener.ID, listener.CreatedAt, listener.UpdatedAt = ID(), now, now
		listener.Kind, listener.TargetID = "pool", pool.ID
		s.Pools = append(s.Pools, pool)
		s.Listeners = append(s.Listeners, listener)
		// Keep response collections normalized as well as the saved snapshot.
		store.Normalize(s)
		return map[string]any{"pool": s.Pools[len(s.Pools)-1], "listener": publicListener(listener)}, nil
	})
}

func (a *App) deletePoolMember(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.mutate(r.Context(), "移除代理池成员", func(s *model.State) (any, error) {
		i := poolIndex(s, r.PathValue("id"))
		nodeID := r.PathValue("nodeId")
		if i < 0 || nodeIndex(s, nodeID) < 0 {
			return nil, missing()
		}
		pool := &s.Pools[i]
		pool.NodeIDs = without(pool.NodeIDs, []string{nodeID})
		// Membership is a union with dynamic groups. An exclusion prevents a
		// group-selected node from immediately reappearing, including on retry.
		pool.ExcludeNodeIDs = union(pool.ExcludeNodeIDs, []string{nodeID})
		pool.UpdatedAt = model.Now()
		return *pool, nil
	})
}
