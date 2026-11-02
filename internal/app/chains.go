package app

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

func chainList(s model.State) []model.Chain {
	if s.Chains == nil {
		return []model.Chain{}
	}
	return append([]model.Chain{}, s.Chains...)
}

func (a *App) putChain(w http.ResponseWriter, r *http.Request) (any, error) {
	c := model.Chain{Enabled: true, NodeIDs: []string{}}
	if e := decode(w, r, &c); e != nil {
		return nil, e
	}
	return a.mutate(r.Context(), "保存代理链", func(s *model.State) (any, error) {
		c.UpdatedAt = model.Now()
		if r.Method == http.MethodPost {
			c.ID = ID()
			c.CreatedAt = c.UpdatedAt
			s.Chains = append(s.Chains, c)
		} else {
			i := chainIndex(s, r.PathValue("id"))
			if i < 0 {
				return nil, missing()
			}
			c.ID = s.Chains[i].ID
			c.CreatedAt = s.Chains[i].CreatedAt
			s.Chains[i] = c
		}
		return c, nil
	})
}

// createChainWithListener creates the ordered path and its dedicated entry in
// one mutation, so an entry can never be saved pointing at a half-created chain.
func (a *App) createChainWithListener(w http.ResponseWriter, r *http.Request) (any, error) {
	req := struct {
		Chain    model.Chain        `json:"chain"`
		Listener *poolListenerInput `json:"listener"`
	}{Chain: model.Chain{Enabled: true, NodeIDs: []string{}}}
	if e := decode(w, r, &req); e != nil {
		return nil, e
	}
	var listener model.Listener
	if req.Listener != nil {
		input := req.Listener
		listener = model.Listener{Name: input.Name, Protocol: input.Protocol, Host: input.Host, Port: input.Port, UDP: input.UDP, Enabled: true, Users: append([]model.Credential{}, input.Users...)}
		if listener.Name == "" {
			listener.Name = strings.TrimSpace(req.Chain.Name)
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
	}
	return a.mutate(r.Context(), "创建代理链", func(s *model.State) (any, error) {
		now := model.Now()
		chain := req.Chain
		chain.ID = ID()
		chain.CreatedAt = now
		chain.UpdatedAt = now
		if chain.NodeIDs == nil {
			chain.NodeIDs = []string{}
		}
		s.Chains = append(s.Chains, chain)
		var created *model.Listener
		if req.Listener != nil {
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
				available, err := listenerPortAvailable(listener.Host, listener.Port, listener.UDP)
				if err != nil {
					return nil, err
				}
				if !available {
					return nil, bad("指定的代理端口已被其他程序占用")
				}
			}
			listener.ID = ID()
			listener.Kind = "chain"
			listener.TargetID = chain.ID
			listener.CreatedAt = now
			listener.UpdatedAt = now
			s.Listeners = append(s.Listeners, listener)
			out := publicListener(listener)
			created = &out
		}
		return map[string]any{"chain": chain, "listener": created}, nil
	})
}

func (a *App) deleteChain(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.mutate(r.Context(), "删除代理链", func(s *model.State) (any, error) {
		id := r.PathValue("id")
		i := chainIndex(s, id)
		if i < 0 {
			return nil, missing()
		}
		s.Chains = append(s.Chains[:i], s.Chains[i+1:]...)
		for j := range s.Listeners {
			if s.Listeners[j].Kind == "chain" && s.Listeners[j].TargetID == id {
				s.Listeners[j].Enabled = false
			}
		}
		return map[string]bool{"deleted": true}, nil
	})
}

// testChain runs one end-to-end URL test through the final synthetic outbound.
// It is intentionally synchronous and bounded by the configured test timeout;
// it does not persist latency or mutate node health.
func (a *App) testChain(w http.ResponseWriter, r *http.Request) (any, error) {
	s := a.store.Snapshot()
	id := r.PathValue("id")
	if chainIndex(&s, id) < 0 {
		return nil, missing()
	}
	timeout := time.Duration(s.Settings.TestTimeoutMS) * time.Millisecond
	result, err := a.core.CheckChain(r.Context(), id, s.Settings.TestURL, timeout)
	if err != nil {
		if result.ChainID == "" {
			return nil, err
		}
		// A URL test that did not pass is a valid check result, not an API
		// failure: keep the per-chain error visible to the UI without a 500.
		return result, nil
	}
	return result, nil
}
