package core

import (
	"bytes"
	"context"
	"github.com/uniproxy/uniproxy/internal/model"
	"gopkg.in/yaml.v3"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestCompileStableRelayReferencesAndNoDirectFallback(t *testing.T) {
	s := model.DefaultState()
	relay := testNode("relay", true)
	leaf := testNode("leaf", true)
	leaf.DialerID = "relay"
	s.Nodes = []model.Node{leaf, relay}
	s.Listeners = []model.Listener{{ID: "in", Kind: "node", TargetID: "leaf", Host: "127.0.0.1", Port: 21088, Protocol: "mixed", Enabled: true}}
	opts := BuildOptions{ControllerAddress: "127.0.0.1:29090", ControllerSecret: "secret", ProbeAddress: "127.0.0.1:29091", ProbeSecret: "secret-two"}
	raw, e := Build(s, opts)
	if e != nil {
		t.Fatal(e)
	}
	var cfg map[string]any
	if e = yaml.Unmarshal(raw, &cfg); e != nil {
		t.Fatal(e)
	}
	var found bool
	for _, v := range cfg["proxies"].([]any) {
		p := v.(map[string]any)
		if p["name"] == "n-leaf" {
			found = true
			if p["dialer-proxy"] != "n-relay" {
				t.Fatal("dependency not remapped to stable ID")
			}
		}
	}
	if !found {
		t.Fatal("leaf missing")
	}
	if _, ok := s.Nodes[0].Config["dialer-proxy"]; ok {
		t.Fatal("input mutated")
	}
	s.Nodes[1].Enabled = false
	raw, e = Build(s, opts)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte("name: n-leaf")) {
		t.Fatal("leaf bypasses disabled relay")
	}
	if !bytes.Contains(raw, []byte("proxy: REJECT")) {
		t.Fatal("listener not fail-closed")
	}
	s.Nodes = s.Nodes[:1]
	raw, e = Build(s, opts)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(raw, []byte("proxy: REJECT")) {
		t.Fatal("deleted relay bypassed")
	}
	s.Nodes = append(s.Nodes, relay)
	s.Nodes[1].DialerID = "leaf"
	if _, e = Build(s, opts); e == nil {
		t.Fatal("cycle accepted")
	}
}
func TestRealDialerChainTraversesRelayAndBlocksWhenDisabled(t *testing.T) {
	m := realManager(t)
	relayAddr, relayCount := localSOCKSFixture(t)
	exitAddr, exitCount := localSOCKSFixture(t)
	makeNode := func(id, addr string) model.Node {
		host, p, _ := net.SplitHostPort(addr)
		port, _ := strconv.Atoi(p)
		n := testNode(id, true)
		n.Server = host
		n.Port = port
		n.Config = map[string]any{"type": "socks5", "server": host, "port": port, "udp": true}
		return n
	}
	relay := makeNode("chain-relay", relayAddr)
	leaf := makeNode("chain-exit", exitAddr)
	leaf.DialerID = relay.ID
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.Write([]byte("chained-origin"))
	}))
	defer origin.Close()
	s := model.DefaultState()
	s.Nodes = []model.Node{leaf, relay}
	s.Revision = 1
	l := model.Listener{ID: "chain-in", Kind: "node", TargetID: leaf.ID, Host: "127.0.0.1", Port: freePort(t), Protocol: "mixed", Enabled: true}
	s.Listeners = []model.Listener{l}
	applyReal(t, m, s)
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort(l.Host, strconv.Itoa(l.Port))}
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: 3 * time.Second}
	res, e := client.Get(origin.URL)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || string(body) != "chained-origin" || relayCount.Load() == 0 || exitCount.Load() == 0 {
		t.Fatal("did not traverse both actual proxy hops")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if result, e := m.Check(ctx, leaf.ID, origin.URL, 3*time.Second); e != nil || !result.OK {
		t.Fatalf("chained URL test: %v", e)
	}
	s.Nodes[1].Enabled = false
	s.Revision++
	applyReal(t, m, s)
	before := exitCount.Load()
	res, e = client.Get(origin.URL)
	if e == nil {
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("disabled relay caused direct exit dial")
		}
	}
	if exitCount.Load() != before {
		t.Fatal("exit dial bypassed relay")
	}
	s.Nodes[1].Enabled = true
	s.Revision++
	applyReal(t, m, s)
	res, e = client.Get(origin.URL)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("restored relay did not restore chain")
	}
}
