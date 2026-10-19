package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
)

type endpoint func(http.ResponseWriter, *http.Request) (any, error)

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func wrap(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, e := fn(w, r)
		if e != nil {
			status, message, code := errorStatus(e)
			respond(w, status, map[string]any{"error": message, "code": code})
			return
		}
		if value != nil {
			respond(w, 200, value)
		}
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return &apiError{415, "请求必须使用 application/json", "content_type"}
	}
	limit := int64(12 * 1024 * 1024)
	if r.URL.Path == "/api/import/backup" {
		limit = 40 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return bad("JSON 格式无效、字段不匹配或内容过大")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return bad("请求只能包含一个 JSON 对象")
	}
	return nil
}
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		host := r.Host
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if a.opts.Token == "" && host != "localhost" && !loopback(host) {
			respond(w, 403, map[string]string{"error": "不允许的 Host，默认仅允许本机访问"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			ok := contains(a.opts.AllowedOrigins, origin)
			u, e := url.Parse(origin)
			if e == nil && u.User == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host {
				ok = true
			}
			if e == nil && a.opts.Token == "" && (u.Hostname() == "localhost" || loopback(u.Hostname())) && (u.Port() == "5173" || u.Port() == strconv.Itoa(a.opts.AdminPort)) {
				ok = true
			}
			if !ok {
				respond(w, 403, map[string]string{"error": "不允许的跨站请求来源"})
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			respond(w, 403, map[string]string{"error": "拒绝跨站 API 请求"})
			return
		}
		if a.opts.Token != "" && r.URL.Path != "/api/health" {
			actual := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
			expected := sha256.Sum256([]byte(a.opts.Token))
			if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
				respond(w, 401, map[string]string{"error": "请输入有效的管理员 API Token"})
				return
			}
		}
		defer func() {
			if recover() != nil {
				respond(w, 500, map[string]string{"error": "内部处理错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, fn endpoint) { mux.HandleFunc(pattern, wrap(fn)) }
	route("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"ok": true, "version": "0.1.0"}, nil
	})
	route("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.bootstrap(), nil })
	route("GET /api/runtime", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.Runtime(), nil })
	route("POST /api/runtime/apply", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.core.Inspect(r.Context())
		a.manuallyStopped = false
		a.applyLocked(a.store.Snapshot())
		return a.runtime(), nil
	})
	route("POST /api/runtime/stop", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if e := a.core.Stop(r.Context()); e != nil {
			return nil, bad(e.Error())
		}
		a.manuallyStopped = true
		a.applyError = "代理内核已手动停止"
		a.store.Activity("warning", "手动停止代理内核")
		return a.runtime(), nil
	})
	route("POST /api/import", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body importRequest
		if e := decode(w, r, &body); e != nil {
			return nil, e
		}
		return a.importNodes(r.Context(), body)
	})
	route("PATCH /api/nodes/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p map[string]json.RawMessage
		if e := decode(w, r, &p); e != nil {
			return nil, e
		}
		if e := filterPatch(p, "name", "groupIds", "enabled"); e != nil {
			return nil, e
		}
		return a.mutate(r.Context(), "修改节点", func(s *model.State) (any, error) {
			i := nodeIndex(s, r.PathValue("id"))
			if i < 0 {
				return nil, missing()
			}
			n := &s.Nodes[i]
			if e := jsonMerge(n, p); e != nil {
				return nil, bad("节点字段格式无效")
			}
			if _, ok := p["name"]; ok {
				n.CustomName = true
			}
			if _, ok := p["groupIds"]; ok {
				setNodeGroups(n, n.GroupIDs)
			}
			n.UpdatedAt = model.Now()
			annotateRelays(s)
			out := *n
			out.Config = nil
			out.Fingerprint = ""
			out.DialerIdentity = ""
			out.SourceGroups = nil
			out.ManualGroupIDs = nil
			return out, nil
		})
	})
	route("POST /api/nodes/batch", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var b struct {
			IDs      []string `json:"ids"`
			Action   string   `json:"action"`
			GroupIDs []string `json:"groupIds"`
		}
		if e := decode(w, r, &b); e != nil {
			return nil, e
		}
		return a.mutate(r.Context(), "批量管理节点", func(s *model.State) (any, error) {
			b.IDs = union(nil, b.IDs)
			if len(b.IDs) == 0 {
				return nil, bad("请选择节点")
			}
			for _, id := range b.IDs {
				if nodeIndex(s, id) < 0 {
					return nil, missing()
				}
			}
			if !contains([]string{"enable", "disable", "delete", "add_groups", "remove_groups"}, b.Action) {
				return nil, bad("批量操作无效")
			}
			if b.Action == "delete" {
				pruneNodes(s, b.IDs)
			} else {
				for i := range s.Nodes {
					n := &s.Nodes[i]
					if !contains(b.IDs, n.ID) {
						continue
					}
					switch b.Action {
					case "enable":
						n.Enabled = true
					case "disable":
						n.Enabled = false
					case "add_groups":
						n.ManualGroupIDs = union(n.ManualGroupIDs, b.GroupIDs)
						groupsFor(n)
					case "remove_groups":
						n.ManualGroupIDs = without(n.ManualGroupIDs, b.GroupIDs)
						for k, gs := range n.SourceGroups {
							n.SourceGroups[k] = without(gs, b.GroupIDs)
						}
						groupsFor(n)
					}
					n.UpdatedAt = model.Now()
				}
			}
			return map[string]int{"affected": len(b.IDs)}, nil
		})
	})
	route("POST /api/groups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var g model.Group
		if e := decode(w, r, &g); e != nil {
			return nil, e
		}
		return a.mutate(r.Context(), "创建分组", func(s *model.State) (any, error) {
			g.ID = ID()
			g.Name = strings.TrimSpace(g.Name)
			g.CreatedAt = model.Now()
			if g.Color == "" {
				g.Color = "#0d9488"
			}
			s.Groups = append(s.Groups, g)
			return g, nil
		})
	})
	route("PATCH /api/groups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p map[string]json.RawMessage
		if e := decode(w, r, &p); e != nil {
			return nil, e
		}
		if e := filterPatch(p, "name", "color"); e != nil {
			return nil, e
		}
		return a.mutate(r.Context(), "更新分组", func(s *model.State) (any, error) {
			for i := range s.Groups {
				if s.Groups[i].ID == r.PathValue("id") {
					if e := jsonMerge(&s.Groups[i], p); e != nil {
						return nil, bad("分组格式无效")
					}
					return s.Groups[i], nil
				}
			}
			return nil, missing()
		})
	})
	route("DELETE /api/groups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.mutate(r.Context(), "删除分组", func(s *model.State) (any, error) {
			id := r.PathValue("id")
			found := false
			gs := []model.Group{}
			for _, g := range s.Groups {
				if g.ID == id {
					found = true
				} else {
					gs = append(gs, g)
				}
			}
			if !found {
				return nil, missing()
			}
			s.Groups = gs
			for i := range s.Nodes {
				n := &s.Nodes[i]
				n.ManualGroupIDs = without(n.ManualGroupIDs, []string{id})
				for k, v := range n.SourceGroups {
					n.SourceGroups[k] = without(v, []string{id})
				}
				groupsFor(n)
			}
			for i := range s.Pools {
				s.Pools[i].GroupIDs = without(s.Pools[i].GroupIDs, []string{id})
				s.Pools[i].ExcludeGroupIDs = without(s.Pools[i].ExcludeGroupIDs, []string{id})
			}
			for i := range s.Subscriptions {
				s.Subscriptions[i].GroupIDs = without(s.Subscriptions[i].GroupIDs, []string{id})
			}
			return map[string]bool{"deleted": true}, nil
		})
	})
	route("POST /api/pools/with-listener", a.createPoolWithListener)
	route("DELETE /api/pools/{id}/members/{nodeId}", a.deletePoolMember)
	for _, method := range []string{"POST /api/pools", "PUT /api/pools/{id}"} {
		route(method, a.putPool)
	}
	route("DELETE /api/pools/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.mutate(r.Context(), "删除代理池", func(s *model.State) (any, error) {
			id := r.PathValue("id")
			i := poolIndex(s, id)
			if i < 0 {
				return nil, missing()
			}
			s.Pools = append(s.Pools[:i], s.Pools[i+1:]...)
			for j := range s.Listeners {
				if s.Listeners[j].Kind == "pool" && s.Listeners[j].TargetID == id {
					s.Listeners[j].Enabled = false
				}
			}
			return map[string]bool{"deleted": true}, nil
		})
	})
	for _, method := range []string{"POST /api/listeners", "PUT /api/listeners/{id}"} {
		route(method, a.putListener)
	}
	route("POST /api/listeners/batch", a.batchListeners)
	route("DELETE /api/listeners/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.mutate(r.Context(), "删除代理入口", func(s *model.State) (any, error) {
			for i, l := range s.Listeners {
				if l.ID == r.PathValue("id") {
					s.Listeners = append(s.Listeners[:i], s.Listeners[i+1:]...)
					return map[string]bool{"deleted": true}, nil
				}
			}
			return nil, missing()
		})
	})
	route("POST /api/subscriptions/{id}/refresh", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var b struct {
			Force bool `json:"force"`
		}
		if r.ContentLength != 0 {
			if e := decode(w, r, &b); e != nil {
				return nil, e
			}
		}
		return a.refresh(r.Context(), r.PathValue("id"), b.Force)
	})
	route("PATCH /api/subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p map[string]json.RawMessage
		if e := decode(w, r, &p); e != nil {
			return nil, e
		}
		return a.subscriptionPatch(r.Context(), r.PathValue("id"), p)
	})
	route("DELETE /api/subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.mutate(r.Context(), "删除订阅及孤立节点", func(s *model.State) (any, error) {
			id := r.PathValue("id")
			found := false
			ss := []model.Subscription{}
			for _, sub := range s.Subscriptions {
				if sub.ID == id {
					found = true
				} else {
					ss = append(ss, sub)
				}
			}
			if !found {
				return nil, missing()
			}
			s.Subscriptions = ss
			orphans := []string{}
			for i := range s.Nodes {
				n := &s.Nodes[i]
				n.SourceIDs = without(n.SourceIDs, []string{id})
				delete(n.SourceGroups, id)
				groupsFor(n)
				if !n.Manual && len(n.SourceIDs) == 0 {
					orphans = append(orphans, n.ID)
				}
			}
			pruneNodes(s, orphans)
			return map[string]int{"removed": len(orphans)}, nil
		})
	})
	route("PATCH /api/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p map[string]json.RawMessage
		if e := decode(w, r, &p); e != nil {
			return nil, e
		}
		if e := filterPatch(p, "testUrl", "speedTestUrl", "testTimeoutMs", "testConcurrency", "speedTestBytes", "healthIntervalSeconds", "portRangeStart", "portRangeEnd"); e != nil {
			return nil, e
		}
		return a.mutate(r.Context(), "更新全局设置", func(s *model.State) (any, error) {
			if e := jsonMerge(&s.Settings, p); e != nil {
				return nil, bad("设置字段格式无效")
			}
			return s.Settings, nil
		})
	})
	route("POST /api/tests", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var b struct {
			NodeIDs []string `json:"nodeIds"`
			Kind    string   `json:"kind"`
		}
		if e := decode(w, r, &b); e != nil {
			return nil, e
		}
		j, e := a.startJob(b.NodeIDs, b.Kind)
		if e != nil {
			return nil, e
		}
		respond(w, 202, j)
		return nil, nil
	})
	route("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.getJob(r.PathValue("id")) })
	route("POST /api/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.cancelJob(r.PathValue("id")) })
	route("GET /api/export/nodes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		s := a.store.Snapshot()
		ps := exportNativeNodes(s)
		b, e := yaml.Marshal(map[string]any{"proxies": ps})
		if e != nil {
			return nil, e
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Content-Disposition", `attachment; filename="uniproxy-nodes.yaml"`)
		_, e = w.Write(b)
		return nil, e
	})
	route("GET /api/export/backup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		w.Header().Set("Content-Disposition", `attachment; filename="uniproxy-backup.json"`)
		return map[string]any{"schemaVersion": 1, "state": a.store.Snapshot()}, nil
	})
	route("POST /api/import/backup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			SchemaVersion int         `json:"schemaVersion"`
			State         model.State `json:"state"`
		}
		if e := decode(w, r, &body); e != nil {
			return nil, e
		}
		if body.SchemaVersion != 0 && body.SchemaVersion != 1 {
			return nil, bad("不支持的备份版本")
		}
		return a.mutate(r.Context(), "恢复完整备份", func(s *model.State) (any, error) {
			rev := s.Revision
			if err := recomputeFingerprints(&body.State); err != nil {
				return nil, err
			}
			for i := range body.State.Subscriptions {
				body.State.Subscriptions[i].UpdatedAt = model.Now()
			}
			*s = body.State
			s.Revision = rev
			return map[string]bool{"restored": true}, nil
		})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 404, map[string]string{"error": "API 不存在"})
	})
	return a.security(mux)
}
func (a *App) putPool(w http.ResponseWriter, r *http.Request) (any, error) {
	p := model.Pool{Enabled: true, Strategy: "url-test", GroupMode: "any", TestURL: "https://www.gstatic.com/generate_204", IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50}
	if e := decode(w, r, &p); e != nil {
		return nil, e
	}
	return a.mutate(r.Context(), "保存代理池", func(s *model.State) (any, error) {
		p.UpdatedAt = model.Now()
		if r.Method == http.MethodPost {
			p.ID = ID()
			p.CreatedAt = p.UpdatedAt
			s.Pools = append(s.Pools, p)
		} else {
			i := poolIndex(s, r.PathValue("id"))
			if i < 0 {
				return nil, missing()
			}
			p.ID = s.Pools[i].ID
			p.CreatedAt = s.Pools[i].CreatedAt
			s.Pools[i] = p
		}
		return p, nil
	})
}
func (a *App) putListener(w http.ResponseWriter, r *http.Request) (any, error) {
	l := model.Listener{Enabled: true, Protocol: "mixed", Host: "127.0.0.1", Users: []model.Credential{}}
	if e := decode(w, r, &l); e != nil {
		return nil, e
	}
	return a.mutate(r.Context(), "保存代理入口", func(s *model.State) (any, error) {
		l.UpdatedAt = model.Now()
		if r.Method == http.MethodPost {
			l.ID = ID()
			l.CreatedAt = l.UpdatedAt
			s.Listeners = append(s.Listeners, l)
		} else {
			idx := -1
			for i, v := range s.Listeners {
				if v.ID == r.PathValue("id") {
					idx = i
					break
				}
			}
			if idx < 0 {
				return nil, missing()
			}
			old := s.Listeners[idx]
			for i := range l.Users {
				if l.Users[i].Password == "" {
					for _, u := range old.Users {
						if u.Username == l.Users[i].Username {
							l.Users[i].Password = u.Password
						}
					}
				}
				l.Users[i].HasPassword = false
			}
			l.ID = old.ID
			l.CreatedAt = old.CreatedAt
			s.Listeners[idx] = l
		}
		return publicListener(l), nil
	})
}
func (a *App) batchListeners(w http.ResponseWriter, r *http.Request) (any, error) {
	var b struct {
		NodeIDs  []string           `json:"nodeIds"`
		Protocol string             `json:"protocol"`
		UDP      bool               `json:"udp"`
		Host     string             `json:"host"`
		Users    []model.Credential `json:"users"`
	}
	if e := decode(w, r, &b); e != nil {
		return nil, e
	}
	if b.Host == "" {
		b.Host = "127.0.0.1"
	}
	if b.Protocol == "" {
		b.Protocol = "mixed"
	}
	if net.ParseIP(b.Host) == nil {
		return nil, bad("监听地址必须是 IP，不能使用域名")
	}
	if !contains([]string{"mixed", "http", "socks"}, b.Protocol) || (b.UDP && b.Protocol == "http") {
		return nil, bad("监听协议/UDP 组合无效")
	}
	if b.UDP && !loopback(b.Host) {
		return nil, bad("原生 SOCKS5 UDP 仅允许本机回环监听")
	}
	if len(b.NodeIDs) == 0 || len(b.NodeIDs) > 2000 {
		return nil, bad("单次请选择1–2000个节点分配端口")
	}
	if !loopback(b.Host) && len(b.Users) == 0 {
		return nil, bad("对外监听必须配置认证")
	}
	return a.mutate(r.Context(), "批量创建节点独享入口", func(s *model.State) (any, error) {
		if len(b.NodeIDs) == 0 {
			return nil, bad("请选择节点")
		}
		out := []model.Listener{}
		used := reservedListenerPorts(*s, a.opts.AdminPort)
		for _, id := range union(nil, b.NodeIDs) {
			idx := nodeIndex(s, id)
			if idx < 0 {
				return nil, missing()
			}
			port, err := allocateListenerPort(r.Context(), s.Settings, b.Host, b.UDP, used)
			if err != nil {
				return nil, err
			}
			used[port] = true
			now := model.Now()
			// A node label may already sit at the name ceiling, so the suffix is
			// appended through derivedName instead of producing an invalid listener.
			l := model.Listener{ID: ID(), Name: derivedName(s.Nodes[idx].Name, " · 独享"), Kind: "node", TargetID: id, Protocol: b.Protocol, Host: b.Host, Port: port, UDP: b.UDP, Enabled: true, Users: b.Users, CreatedAt: now, UpdatedAt: now}
			s.Listeners = append(s.Listeners, l)
			out = append(out, publicListener(l))
		}
		return out, nil
	})
}
