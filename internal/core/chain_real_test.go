package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

// TestRealChainRoutesThroughEverySyntheticHop drives a real pinned-core chain
// through two loopback SOCKS fixtures. If dialer-proxy were ignored, the first
// fixture would never be dialled; if the chain were downgraded to a direct
// outbound, neither fixture would see the connection. The target is only the
// final origin, never a public proxy.
func TestRealChainRoutesThroughEverySyntheticHop(t *testing.T) {
	m := realManager(t)
	firstAddress, firstHits := localSOCKSFixture(t)
	secondAddress, secondHits := localSOCKSFixture(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Millisecond)
		_, _ = io.WriteString(w, "chain-real-core-ok")
	}))
	defer origin.Close()

	nodes := make([]model.Node, 0, 2)
	for index, address := range []string{firstAddress, secondAddress} {
		host, portText, _ := net.SplitHostPort(address)
		port, _ := strconv.Atoi(portText)
		node := testNode("chain-hop-"+strconv.Itoa(index+1), true)
		node.Config = map[string]any{"type": "socks5", "server": host, "port": port, "udp": false}
		node.Server = host
		node.Port = port
		nodes = append(nodes, node)
	}
	state := model.DefaultState()
	state.Revision = 1
	state.Nodes = nodes
	state.Chains = []model.Chain{{ID: "real-chain", Name: "real chain", NodeIDs: []string{nodes[0].ID, nodes[1].ID}, Enabled: true}}
	state.Listeners = []model.Listener{{ID: "chain-entry", Name: "chain entry", Protocol: "socks", Kind: "chain", TargetID: "real-chain", Host: "127.0.0.1", Port: freePort(t), Enabled: true, Users: []model.Credential{{Username: "client", Password: "credential"}}}}
	applyReal(t, m, state)

	result, err := m.CheckChain(context.Background(), "real-chain", origin.URL, 5*time.Second)
	if err != nil || !result.OK || result.ChainID != "real-chain" || result.LatencyMS == nil {
		t.Fatalf("real chain URL-test: %+v %v", result, err)
	}
	conn := socksConnect(t, state.Listeners[0], strings.TrimPrefix(origin.URL, "http://"))
	defer conn.Close()
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: fixture\r\nConnection: close\r\n\r\n")
	reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(reply.Body)
	reply.Body.Close()
	if readErr != nil || reply.StatusCode != 200 || !strings.Contains(string(body), "chain-real-core-ok") {
		t.Fatalf("chain entry did not proxy body: status=%d body=%q err=%v", reply.StatusCode, body, readErr)
	}
	if firstHits.Load() == 0 || secondHits.Load() == 0 {
		t.Fatalf("chain skipped a hop: first=%d second=%d", firstHits.Load(), secondHits.Load())
	}
}
