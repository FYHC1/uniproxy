// Package sidecar generates and supervises local proxy-engine processes that
// expose one authenticated SOCKS5 inbound per node. Mihomo remains the only
// public listener and the only scheduler; a sidecar is an outbound provider on
// 127.0.0.1. Real remote node addresses and credentials never leave the
// sidecar configuration.
package sidecar

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/sidecarinstall"
)

const (
	// XrayVersion is the pinned sidecar version whose config schema was tested.
	// The installer owns the single version constant so download pins and the
	// generated schema cannot drift.
	XrayVersion = sidecarinstall.PinnedVersion
	// MaxSidecarNodes is the first-stage limit for one engine process.
	MaxSidecarNodes = 200
)

var sidecarNodeID = regexp.MustCompile("^[A-Za-z0-9_-]{1,128}$")

type XrayConfigOptions struct {
	LogLevel string
}

// BuildXrayConfig renders one Xray document with one local SOCKS inbound and
// one remote outbound per node. Each node's Config["xray"] holds its Xray
// outbound template. The template's first vnext/server entry receives the
// authoritative Node.Server and Node.Port values.
func BuildXrayConfig(nodes []model.Node) ([]byte, error) {
	return BuildXrayConfigWithOptions(nodes, XrayConfigOptions{})
}

func BuildXrayConfigWithOptions(nodes []model.Node, opts XrayConfigOptions) ([]byte, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no Xray sidecar nodes")
	}
	if len(nodes) > MaxSidecarNodes {
		return nil, nodeLimitError(len(nodes))
	}
	logLevel := opts.LogLevel
	if logLevel == "" {
		logLevel = "warning"
	}
	switch logLevel {
	case "debug", "info", "warning", "error", "none":
	default:
		return nil, errors.New("invalid Xray log level")
	}

	prepared := make([]model.Node, 0, len(nodes))
	seenIDs := map[string]bool{}
	seenPorts := map[int]bool{}
	seenUsers := map[string]bool{}
	for _, node := range nodes {
		if node.EngineName() != model.EngineXray {
			return nil, errors.New("Xray sidecar node has a non-Xray engine")
		}
		if !sidecarNodeID.MatchString(node.ID) {
			return nil, errors.New("Xray sidecar node has an invalid stable ID")
		}
		if seenIDs[node.ID] {
			return nil, errors.New("duplicate Xray sidecar node ID")
		}
		seenIDs[node.ID] = true
		if node.Server == "" || node.Port < 1 || node.Port > 65535 {
			return nil, fmt.Errorf("node %s has an invalid remote address", node.ID)
		}
		endpoint := node.SidecarEndpoint
		if endpoint == nil {
			return nil, fmt.Errorf("node %s has no sidecar endpoint", node.ID)
		}
		ip, err := netip.ParseAddr(endpoint.Host)
		if err != nil || !ip.IsLoopback() || ip.String() != "127.0.0.1" {
			return nil, fmt.Errorf("node %s sidecar endpoint must listen on 127.0.0.1", node.ID)
		}
		if endpoint.Port < 1024 || endpoint.Port > 65535 {
			return nil, fmt.Errorf("node %s sidecar endpoint port is invalid", node.ID)
		}
		if seenPorts[endpoint.Port] {
			return nil, errors.New("duplicate sidecar endpoint port")
		}
		seenPorts[endpoint.Port] = true
		if endpoint.Username == "" || len(endpoint.Username) > 255 || strings.ContainsAny(endpoint.Username, ":\r\n\x00") {
			return nil, fmt.Errorf("node %s sidecar username is invalid", node.ID)
		}
		if endpoint.Password == "" || len(endpoint.Password) > 255 || strings.ContainsAny(endpoint.Password, "\r\n\x00") {
			return nil, fmt.Errorf("node %s sidecar password is invalid", node.ID)
		}
		if seenUsers[endpoint.Username] {
			return nil, errors.New("duplicate sidecar username")
		}
		seenUsers[endpoint.Username] = true
		prepared = append(prepared, node)
	}

	sort.Slice(prepared, func(i, j int) bool { return prepared[i].ID < prepared[j].ID })

	inbounds := make([]map[string]any, 0, len(prepared))
	outbounds := make([]map[string]any, 0, len(prepared)+1)
	rules := make([]map[string]any, 0, len(prepared)+1)
	for _, node := range prepared {
		inboundTag := "in-" + node.ID
		outboundTag := "out-" + node.ID
		inbounds = append(inbounds, map[string]any{
			"tag": inboundTag, "listen": "127.0.0.1", "port": node.SidecarEndpoint.Port,
			"protocol": "socks",
			"settings": map[string]any{
				"auth":     "password",
				"accounts": []map[string]string{{"user": node.SidecarEndpoint.Username, "pass": node.SidecarEndpoint.Password}},
				"udp":      node.UDP,
			},
		})
		outbound, err := xrayOutbound(node)
		if err != nil {
			return nil, fmt.Errorf("node %s: %w", node.ID, err)
		}
		outbound["tag"] = outboundTag
		outbounds = append(outbounds, outbound)
		rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{inboundTag}, "outboundTag": outboundTag})
	}
	outbounds = append(outbounds, map[string]any{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}})
	rules = append(rules, map[string]any{"type": "field", "network": "tcp,udp", "outboundTag": "blocked"})

	document := map[string]any{
		"log":       map[string]any{"loglevel": logLevel},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   map[string]any{"domainStrategy": "AsIs", "rules": rules},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, errors.New("could not serialize Xray configuration")
	}
	return append(data, '\n'), nil
}

func xrayOutbound(node model.Node) (map[string]any, error) {
	if node.Config == nil {
		return nil, errors.New("node config is missing")
	}
	var template any
	if value, ok := node.Config["xray"]; ok {
		template = value
	} else if _, ok := node.Config["protocol"]; ok {
		template = node.Config
	} else {
		return nil, errors.New("node config has no Xray outbound template")
	}
	data, err := json.Marshal(template)
	if err != nil {
		return nil, errors.New("Xray outbound template is not serializable")
	}
	var outbound map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&outbound); err != nil {
		return nil, errors.New("Xray outbound template must be an object")
	}
	protocol, _ := outbound["protocol"].(string)
	if protocol == "" || protocol != xrayProtocol(node.Protocol) {
		return nil, errors.New("Xray outbound protocol does not match node protocol")
	}
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http":
	default:
		return nil, errors.New("unsupported Xray outbound protocol")
	}
	settings, ok := outbound["settings"].(map[string]any)
	if !ok {
		return nil, errors.New("Xray outbound settings must be an object")
	}
	switch protocol {
	case "vless", "vmess":
		if err := setServerEntry(settings, "vnext", node); err != nil {
			return nil, err
		}
	case "trojan", "shadowsocks", "socks", "http":
		if err := setServerEntry(settings, "servers", node); err != nil {
			return nil, err
		}
	}
	if stream, ok := outbound["streamSettings"]; ok {
		if _, ok := stream.(map[string]any); !ok {
			return nil, errors.New("Xray streamSettings must be an object")
		}
	}
	if err := validateXrayPaths(outbound); err != nil {
		return nil, err
	}
	delete(outbound, "tag")
	return outbound, nil
}

// xrayProtocol maps the application's short protocol names to Xray's
// outbound protocol names. The two differ for Shadowsocks and SOCKS5.
func xrayProtocol(nodeProtocol string) string {
	switch nodeProtocol {
	case "ss":
		return "shadowsocks"
	case "socks5":
		return "socks"
	default:
		return nodeProtocol
	}
}

func setServerEntry(settings map[string]any, key string, node model.Node) error {
	entries, ok := settings[key].([]any)
	if !ok || len(entries) != 1 {
		return fmt.Errorf("Xray outbound settings.%s must contain exactly one server", key)
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		return fmt.Errorf("Xray outbound settings.%s[0] must be an object", key)
	}
	first["address"] = node.Server
	first["port"] = node.Port
	return nil
}

var unsafeXrayFileKeys = map[string]bool{
	"certificatefile":       true,
	"keyfile":               true,
	"cafile":                true,
	"clientcertificatefile": true,
	"clientkeyfile":         true,
}

func validateXrayPaths(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if unsafeXrayFileKeys[strings.ToLower(key)] {
				return errors.New("Xray configuration may not reference local credential files")
			}
			if err := validateXrayPaths(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateXrayPaths(child); err != nil {
				return err
			}
		}
	}
	return nil
}
