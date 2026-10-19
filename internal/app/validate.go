package app

import (
	"encoding/json"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/model"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var idPattern = regexp.MustCompile("^[a-zA-Z0-9_-]{1,80}$")
var colorPattern = regexp.MustCompile("^#[0-9a-fA-F]{6}$")

func control(v string) bool {
	for _, c := range v {
		if c < 32 || c == 127 {
			return true
		}
	}
	return false
}
func nameOK(v string) bool {
	return len(strings.TrimSpace(v)) > 0 && utf8.RuneCountInString(v) <= 160 && !control(v)
}
func loopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }
func validURL(v string) bool {
	u, e := url.Parse(v)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func ValidateState(s model.State, adminPort int) error {
	encoded, err := json.Marshal(s)
	if err != nil || len(encoded) > 32*1024*1024 {
		return bad("配置快照超过32 MiB限制；请减少节点/私钥数据")
	}
	if len(s.Nodes) > 5000 || len(s.Pools) > 500 || len(s.Listeners) > 2000 || len(s.Subscriptions) > 200 || len(s.Groups) > 1000 {
		return bad("配置数量超过安全限制")
	}
	ids := map[string]bool{}
	checkID := func(id string) error {
		if !idPattern.MatchString(id) || ids[id] {
			return bad("对象 ID 无效或重复")
		}
		ids[id] = true
		return nil
	}
	groups := map[string]bool{}
	groupNames := map[string]bool{}
	for _, g := range s.Groups {
		if e := checkID(g.ID); e != nil {
			return e
		}
		if !nameOK(g.Name) {
			return bad("分组名称无效")
		}
		if groupNames[g.Name] {
			return bad("分组名称已存在")
		}
		groupNames[g.Name] = true
		groups[g.ID] = true
		if g.Color != "" && !colorPattern.MatchString(g.Color) {
			return bad("分组颜色须为 #RRGGBB")
		}
	}
	checkGroups := func(gs []string) error {
		if len(gs) > 1000 {
			return bad("分组数量过多")
		}
		for _, g := range gs {
			if !groups[g] {
				return bad("引用的分组不存在")
			}
		}
		return nil
	}
	if err := model.ValidateRelayGraph(s.Nodes); err != nil {
		return bad(err.Error())
	}
	nodes := map[string]model.Node{}
	for _, n := range s.Nodes {
		if e := checkID(n.ID); e != nil {
			return e
		}
		if !nameOK(n.Name) || n.Port < 1 || n.Port > 65535 || n.Server == "" || control(n.Server) || strings.ContainsAny(n.Server, " /") || strings.ContainsRune(n.Server, 92) {
			return bad("节点名称、服务器或端口无效")
		}
		if !contains([]string{"vless", "vmess", "trojan", "ss", "hysteria", "hysteria2", "tuic", "ssh", "socks5", "http"}, n.Protocol) {
			return bad("不支持的节点协议")
		}
		if n.DialerIdentity != "" && (!validRelayIdentity(n.DialerIdentity) || n.DialerID == "") {
			return bad("中转身份指纹无效")
		}
		if n.DialerID != "" && !idPattern.MatchString(n.DialerID) {
			return bad("节点中转 ID 无效")
		}
		if n.DialerProxy != "" || n.DialerFingerprint != "" {
			return bad("未解析的中转引用不能保存")
		}
		if n.Config == nil {
			return bad("节点配置缺失")
		}
		if e := checkGroups(n.GroupIDs); e != nil {
			return e
		}
		if e := checkGroups(n.ManualGroupIDs); e != nil {
			return e
		}
		for _, gs := range n.SourceGroups {
			if e := checkGroups(gs); e != nil {
				return e
			}
		}
		nodes[n.ID] = n
	}
	pools := map[string]model.Pool{}
	for _, p := range s.Pools {
		if e := checkID(p.ID); e != nil {
			return e
		}
		if !nameOK(p.Name) {
			return bad("代理池名称无效")
		}
		if !contains([]string{"url-test", "round-robin", "consistent-hashing", "sticky-sessions"}, p.Strategy) {
			return bad("不支持的调度策略")
		}
		if p.HashKey != "" && (p.HashKey != "in-user" || p.Strategy == "round-robin" || p.Strategy == "url-test") {
			return bad("当前策略不支持此哈希键")
		}
		if p.GroupMode != "any" && p.GroupMode != "all" {
			return bad("分组匹配方式无效")
		}
		if e := checkGroups(p.GroupIDs); e != nil {
			return e
		}
		if e := checkGroups(p.ExcludeGroupIDs); e != nil {
			return e
		}
		for _, id := range union(p.NodeIDs, p.ExcludeNodeIDs) {
			if _, ok := nodes[id]; !ok {
				return bad("代理池引用不存在的节点")
			}
		}
		if !validURL(p.TestURL) || p.IntervalSeconds < 10 || p.IntervalSeconds > 86400 || p.TimeoutMS < 500 || p.TimeoutMS > 30000 || p.ToleranceMS < 0 || p.ToleranceMS > 10000 {
			return bad("代理池检测 URL、间隔、超时或容差无效")
		}
		pools[p.ID] = p
	}
	for i, l := range s.Listeners {
		if e := checkID(l.ID); e != nil {
			return e
		}
		if !nameOK(l.Name) || net.ParseIP(l.Host) == nil || l.Port < 1024 || l.Port > 65535 {
			return bad("监听名称、IP 或端口无效（端口范围 1024–65535）")
		}
		if l.Port == adminPort {
			return bad("代理端口不能与管理端口相同")
		}
		if !contains([]string{"http", "socks", "mixed"}, l.Protocol) {
			return bad("入口协议无效")
		}
		if l.Protocol == "http" && l.UDP {
			return bad("HTTP 入口不支持 SOCKS5 UDP")
		}
		if l.Kind != "node" && l.Kind != "pool" {
			return bad("入口绑定类型无效")
		}
		if !loopback(l.Host) && len(l.Users) == 0 {
			return bad("非本机监听必须配置代理认证")
		}
		if l.UDP && !loopback(l.Host) {
			return bad("当前 Mihomo 原生 UDP 不受用户名/密码保护，仅允许回环 UDP 监听")
		}
		names := map[string]bool{}
		for _, u := range l.Users {
			if u.Username == "" || len(u.Username) > 255 || strings.Contains(u.Username, ":") || control(u.Username) || u.Password == "" || len(u.Password) > 255 || control(u.Password) {
				return bad("代理用户名/密码不能为空且不能超过 255 字节")
			}
			if names[u.Username] {
				return bad("代理用户名重复")
			}
			names[u.Username] = true
		}
		if l.Enabled {
			for j := 0; j < i; j++ {
				o := s.Listeners[j]
				if o.Enabled && l.Port == o.Port && (l.Host == o.Host || l.Host == "0.0.0.0" || l.Host == "::" || o.Host == "0.0.0.0" || o.Host == "::") {
					return bad(fmt.Sprintf("端口 %d 被多个入口使用", l.Port))
				}
			}
		}
		if l.Kind == "node" {
			n, ok := nodes[l.TargetID]
			if l.Enabled && !ok {
				return bad("独享入口目标节点不存在")
			}
			if l.UDP && ok && !n.UDP {
				return bad("节点不支持 UDP")
			}
		} else {
			p, ok := pools[l.TargetID]
			if l.Enabled && !ok {
				return bad("入口目标代理池不存在")
			}
			if l.UDP && ok && !p.UDP {
				return bad("请先启用代理池 UDP 能力")
			}
			if ok && p.HashKey == "in-user" && l.UDP {
				return bad("原生 UDP 不支持按认证用户名哈希，请关闭 UDP 或更换哈希键")
			}
			if ok && p.HashKey == "in-user" && l.Enabled && len(l.Users) == 0 {
				return bad("按用户名分配的代理池入口必须配置认证")
			}
		}
	}
	sources := map[string]bool{}
	for _, sub := range s.Subscriptions {
		if e := checkID(sub.ID); e != nil {
			return e
		}
		if !nameOK(sub.Name) || !validURL(sub.URL) || sub.IntervalMinutes < 5 || sub.IntervalMinutes > 10080 {
			return bad("订阅名称、URL 或更新间隔无效（5–10080 分钟）")
		}
		if e := checkGroups(sub.GroupIDs); e != nil {
			return e
		}
		sources[sub.ID] = true
	}
	for _, n := range s.Nodes {
		for _, id := range n.SourceIDs {
			if !sources[id] {
				return bad("节点来源订阅不存在")
			}
		}
		for id := range n.SourceGroups {
			if !sources[id] {
				return bad("节点来源分组引用无效")
			}
		}
	}
	v := s.Settings
	if !validURL(v.TestURL) || !validURL(v.SpeedTestURL) || v.TestTimeoutMS < 500 || v.TestTimeoutMS > 30000 || v.TestConcurrency < 1 || v.TestConcurrency > 32 || v.SpeedTestBytes < 1024 || v.SpeedTestBytes > 100*1024*1024 || v.HealthIntervalSeconds < 30 || v.HealthIntervalSeconds > 86400 || v.PortRangeStart < 1024 || v.PortRangeEnd > 65535 || v.PortRangeEnd < v.PortRangeStart {
		return bad("检测设置或端口分配范围无效")
	}
	return nil
}
