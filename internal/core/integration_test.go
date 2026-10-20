package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

// These tests never use public proxy servers. All TCP origins, UDP echo servers,
// and upstream HTTP/SOCKS proxies bind loopback. Opt in with a verified v1.19.32:
// UNIPROXY_TEST_CORE_PATH=/absolute/path/to/mihomo go test ./internal/core -run Real
func realManager(t *testing.T) *Manager {
	t.Helper()
	path := os.Getenv("UNIPROXY_TEST_CORE_PATH")
	if path == "" {
		t.Skip("set UNIPROXY_TEST_CORE_PATH to a verified Mihomo v" + PinnedVersion)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{BinaryPath: absolute, DataDir: t.TempDir(), Log: func(level, message string) { t.Log(level + ": " + message) }})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}
func applyReal(t *testing.T, m *Manager, state model.State) {
	t.Helper()
	if err := m.Apply(context.Background(), state); err != nil {
		t.Fatalf("Apply: %v\nSynthetic-fixture core log:\n%s", err, m.logs.text())
	}
}

func TestRealHTTPAndSOCKSDelaySpeedAndTCPPreservation(t *testing.T) {
	m := realManager(t)
	proxyAddress, proxied := localSOCKSFixture(t)
	host, portText, _ := net.SplitHostPort(proxyAddress)
	port, _ := strconv.Atoi(portText)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if r.Method == http.MethodHead {
			// An in-memory origin can finish below the pinned core's millisecond
			// resolution, which its URL-test API treats as a failed zero delay.
			// Add real fixture response time, never fabricate a measured delay.
			time.Sleep(3 * time.Millisecond)
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, strings.Repeat("actual-download-", 16384))
	}))
	defer origin.Close()
	state := model.DefaultState()
	node := testNode("fixture", true)
	node.Config = map[string]any{"type": "socks5", "server": host, "port": port, "udp": true}
	node.Server = host
	node.Port = port
	state.Nodes = []model.Node{node}
	state.Revision = 1
	for i, protocol := range []string{"http", "socks", "mixed"} {
		state.Listeners = append(state.Listeners, model.Listener{ID: "inbound-" + protocol, Protocol: protocol, Kind: "node", TargetID: node.ID, Host: "127.0.0.1", Port: freePort(t), Enabled: true, UDP: i > 0, Users: []model.Credential{{Username: "client", Password: "credential"}}})
	}
	applyReal(t, m, state)
	initialPID := m.process.cmd.Process.Pid
	for _, l := range state.Listeners {
		if l.Protocol == "socks" {
			conn := socksConnect(t, l, strings.TrimPrefix(origin.URL, "http://"))
			fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: fixture\r\nConnection: close\r\n\r\n")
			reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				conn.Close()
				t.Fatal(err)
			}
			body, err := io.ReadAll(reply.Body)
			reply.Body.Close()
			conn.Close()
			if err != nil || reply.StatusCode != 200 || len(body) == 0 {
				t.Fatal("SOCKS listener did not proxy real data")
			}
			continue
		}
		proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort(l.Host, strconv.Itoa(l.Port)), User: url.UserPassword("client", "credential")}
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
		response, err := client.Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		count, err := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		transport.CloseIdleConnections()
		if err != nil || response.StatusCode != 200 || count == 0 {
			t.Fatal("HTTP/mixed listener did not proxy actual data")
		}
	}
	checked, err := m.Check(context.Background(), node.ID, origin.URL, 3*time.Second)
	if err != nil || !checked.OK || checked.LatencyMS == nil {
		t.Fatalf("real URL-test failed: %+v %v", checked, err)
	}
	result, err := m.Speed(context.Background(), node.ID, origin.URL, 5*time.Second, 32<<10)
	if err != nil || !result.OK || result.Bytes != 32<<10 || result.SpeedMbps == nil {
		t.Fatalf("real speed failed: %+v %v", result, err)
	}
	if proxied.Load() < 5 {
		t.Fatalf("fixture saw too little proxy traffic: %d", proxied.Load())
	}
	echo := localTCPEcho(t)
	stream := socksConnect(t, state.Listeners[1], echo)
	defer stream.Close()
	roundTripEcho(t, stream, "before-update")
	// A real native group changes the compiled proxy graph but not the existing
	// bound SOCKS listener. There must be neither process nor stream restart.
	state.Pools = []model.Pool{{ID: "native-pool", Enabled: true, UDP: true, Strategy: "round-robin", NodeIDs: []string{node.ID}, TestURL: origin.URL, IntervalSeconds: 300, TimeoutMS: 1000}}
	state.Listeners[0].Kind = "pool"
	state.Listeners[0].TargetID = "native-pool"
	state.Revision++
	applyReal(t, m, state)
	if m.process.cmd.Process.Pid != initialPID {
		t.Fatal("normal real update restarted the core")
	}
	roundTripEcho(t, stream, "after-update")
	// Change hash strategies natively on the pinned binary, including in-user.
	for _, strategy := range []string{"url-test", "consistent-hashing", "sticky-sessions"} {
		state.Pools[0].Strategy = strategy
		state.Pools[0].HashKey = ""
		if strategy != "url-test" {
			state.Pools[0].HashKey = "in-user"
		}
		state.Revision++
		applyReal(t, m, state)
		roundTripEcho(t, stream, "after-"+strategy)
		proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Listeners[0].Port)), User: url.UserPassword("client", "credential")}
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
		response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		transport.CloseIdleConnections()
		if response.StatusCode != 200 {
			t.Fatalf("native strategy %s did not route: %d", strategy, response.StatusCode)
		}
	}
	// Swapping identities across existing ports needs a staged native reload.
	state.Listeners[0].Port, state.Listeners[2].Port = state.Listeners[2].Port, state.Listeners[0].Port
	state.Revision++
	applyReal(t, m, state)
	roundTripEcho(t, stream, "after-port-swap")
	state.Nodes = nil
	state.Pools = nil
	state.Revision++
	applyReal(t, m, state)
	before := proxied.Load()
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Listeners[0].Port)), User: url.UserPassword("client", "credential")}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: time.Second}).Get(origin.URL)
	if err == nil {
		response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			t.Fatal("deleted target fell back to direct")
		}
	}
	if proxied.Load() != before {
		t.Fatal("deleted target still used an outbound")
	}
}

func TestRealUDPDatagramsAndFailClosedEmptyPool(t *testing.T) {
	m := realManager(t)
	proxyAddress, _ := localSOCKSFixture(t)
	host, portText, _ := net.SplitHostPort(proxyAddress)
	port, _ := strconv.Atoi(portText)
	echo := localUDPEcho(t)
	node := testNode("udp-fixture", true)
	node.Config = map[string]any{"type": "socks5", "server": host, "port": port, "udp": true}
	node.Server = host
	node.Port = port
	state := model.DefaultState()
	state.Nodes = []model.Node{node}
	state.Pools = []model.Pool{{ID: "udp-pool", Enabled: true, UDP: true, Strategy: "round-robin", NodeIDs: []string{node.ID}, TestURL: "http://127.0.0.1:9", IntervalSeconds: 300, TimeoutMS: 50}}
	state.Listeners = []model.Listener{{ID: "udp", Enabled: true, Protocol: "mixed", Host: "127.0.0.1", Port: freePort(t), UDP: true, Kind: "pool", TargetID: "udp-pool"}}
	state.Revision = 1
	applyReal(t, m, state)
	client, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Listeners[0].Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	packet := append([]byte{0, 0, 0}, socksAddress(t, echo)...)
	packet = append(packet, []byte("actual-udp-datagram")...)
	if _, err = client.Write(packet); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65536)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("real UDP failed: %v; logs %s", err, m.logs.text())
	}
	_, payload, err := decodeUDP(buf[:n])
	if err != nil || string(payload) != "actual-udp-datagram" {
		t.Fatalf("UDP payload %q error %v", payload, err)
	}
	state.Pools[0].NodeIDs = nil
	state.Revision++
	applyReal(t, m, state)
	// Reload may retain established UDP mappings, so use a fresh source socket to
	// verify fail-closed behavior of NEW flows after the empty-pool mutation.
	blocked, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Listeners[0].Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Close()
	_ = blocked.SetDeadline(time.Now().Add(400 * time.Millisecond))
	blocked.Write(packet)
	if _, err = blocked.Read(buf); err == nil {
		t.Fatal("empty pool sent UDP via direct fallback")
	}
}

func TestRealNativeHTTPOutbound(t *testing.T) {
	m := realManager(t)
	var through atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, strings.Repeat("bounded-data", 1024))
	}))
	defer origin.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "Basic dXBzdHJlYW06cHJpdmF0ZQ==" {
			w.WriteHeader(407)
			return
		}
		through.Add(1)
		if r.Method == http.MethodConnect {
			destination, err := net.DialTimeout("tcp", r.Host, time.Second)
			if err != nil {
				w.WriteHeader(502)
				return
			}
			defer destination.Close()
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer client.Close()
			io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n")
			done := make(chan struct{})
			go func() { io.Copy(destination, buffer); close(done) }()
			io.Copy(client, destination)
			client.Close()
			<-done
			return
		}
		request := r.Clone(r.Context())
		request.RequestURI = ""
		request.Header.Del("Proxy-Authorization")
		transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		reply, err := transport.RoundTrip(request)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer reply.Body.Close()
		for k, v := range reply.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(reply.StatusCode)
		io.Copy(w, reply.Body)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	host, portText, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portText)
	state := model.DefaultState()
	state.Nodes = []model.Node{{ID: "http-upstream", Enabled: true, ParsedNode: model.ParsedNode{Protocol: "http", Server: host, Port: port, Config: map[string]any{"type": "http", "server": host, "port": port, "username": "upstream", "password": "private"}}}}
	state.Revision = 1
	applyReal(t, m, state)
	if result, err := m.Check(context.Background(), "http-upstream", origin.URL, 3*time.Second); err != nil || !result.OK {
		t.Fatalf("native HTTP delay: %+v %v", result, err)
	}
	if result, err := m.Speed(context.Background(), "http-upstream", origin.URL, 3*time.Second, 4096); err != nil || !result.OK || result.Bytes != 4096 {
		t.Fatalf("native HTTP speed: %+v %v", result, err)
	}
	if through.Load() < 2 {
		t.Fatal("native HTTP outbound bypassed upstream")
	}
}

func localTCPEcho(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); io.Copy(conn, conn) }()
		}
	}()
	return listener.Addr().String()
}
func localUDPEcho(t *testing.T) string {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	go func() {
		buf := make([]byte, 65536)
		for {
			n, addr, err := socket.ReadFrom(buf)
			if err != nil {
				return
			}
			socket.WriteTo(buf[:n], addr)
		}
	}()
	return socket.LocalAddr().String()
}
func roundTripEcho(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, payload); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != payload {
		t.Fatal("established stream corrupted")
	}
}

func socksConnect(t *testing.T, l model.Listener, target string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(l.Host, strconv.Itoa(l.Port)), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	method := byte(0)
	if len(l.Users) > 0 {
		method = 2
	}
	conn.Write([]byte{5, 1, method})
	reply := make([]byte, 2)
	if _, err = io.ReadFull(conn, reply); err != nil || reply[1] != method {
		conn.Close()
		t.Fatalf("SOCKS negotiate: %v %v", reply, err)
	}
	if method == 2 {
		u := l.Users[0]
		auth := []byte{1, byte(len(u.Username))}
		auth = append(auth, []byte(u.Username)...)
		auth = append(auth, byte(len(u.Password)))
		auth = append(auth, []byte(u.Password)...)
		conn.Write(auth)
		if _, err = io.ReadFull(conn, reply); err != nil || reply[1] != 0 {
			conn.Close()
			t.Fatal("SOCKS authentication failed")
		}
	}
	request := append([]byte{5, 1, 0}, socksAddress(t, target)...)
	conn.Write(request)
	header := make([]byte, 3)
	if _, err = io.ReadFull(conn, header); err != nil || header[1] != 0 {
		conn.Close()
		t.Fatalf("SOCKS connect failed %v %v", header, err)
	}
	if _, err = readSOCKSAddress(conn); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}
func socksAddress(t *testing.T, address string) []byte {
	t.Helper()
	data, err := encodeSOCKSAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func encodeSOCKSAddress(address string) ([]byte, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	var encoded []byte
	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		encoded = append([]byte{1}, ip4...)
	} else if ip != nil {
		encoded = append([]byte{4}, ip.To16()...)
	} else {
		if len(host) > 255 {
			return nil, fmt.Errorf("host too long")
		}
		encoded = append([]byte{3, byte(len(host))}, []byte(host)...)
	}
	return append(encoded, byte(port>>8), byte(port)), nil
}
func readSOCKSAddress(reader io.Reader) (string, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(reader, atyp[:]); err != nil {
		return "", err
	}
	var host string
	switch atyp[0] {
	case 1, 4:
		size := 4
		if atyp[0] == 4 {
			size = 16
		}
		ip := make([]byte, size)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(reader, n[:]); err != nil {
			return "", err
		}
		name := make([]byte, int(n[0]))
		if _, err := io.ReadFull(reader, name); err != nil {
			return "", err
		}
		host = string(name)
	default:
		return "", fmt.Errorf("unknown SOCKS address type")
	}
	var port [2]byte
	if _, err := io.ReadFull(reader, port[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))), nil
}
func decodeUDP(packet []byte) (string, []byte, error) {
	if len(packet) < 4 || packet[0] != 0 || packet[1] != 0 || packet[2] != 0 {
		return "", nil, fmt.Errorf("invalid SOCKS UDP header")
	}
	reader := strings.NewReader(string(packet[3:]))
	address, err := readSOCKSAddress(reader)
	if err != nil {
		return "", nil, err
	}
	payload := packet[len(packet)-reader.Len():]
	return address, payload, nil
}

func localSOCKSFixture(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	count := &atomic.Int32{}
	var mu sync.Mutex
	conns := map[net.Conn]bool{}
	t.Cleanup(func() {
		listener.Close()
		udp.Close()
		mu.Lock()
		for c := range conns {
			c.Close()
		}
		mu.Unlock()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns[conn] = true
			mu.Unlock()
			go func(c net.Conn) {
				defer func() { c.Close(); mu.Lock(); delete(conns, c); mu.Unlock() }()
				_ = c.SetDeadline(time.Now().Add(30 * time.Second))
				var head [2]byte
				if _, err := io.ReadFull(c, head[:]); err != nil {
					return
				}
				methods := make([]byte, int(head[1]))
				if _, err := io.ReadFull(c, methods); err != nil {
					return
				}
				c.Write([]byte{5, 0})
				var request [3]byte
				if _, err := io.ReadFull(c, request[:]); err != nil {
					return
				}
				target, err := readSOCKSAddress(c)
				if err != nil {
					return
				}
				switch request[1] {
				case 1:
					upstream, err := net.DialTimeout("tcp", target, 2*time.Second)
					if err != nil {
						c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
						return
					}
					defer upstream.Close()
					count.Add(1)
					c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
					done := make(chan struct{})
					go func() { io.Copy(upstream, c); close(done) }()
					io.Copy(c, upstream)
					c.Close()
					<-done
				case 3:
					address, _ := encodeSOCKSAddress(udp.LocalAddr().String())
					c.Write(append([]byte{5, 0, 0}, address...))
					io.Copy(io.Discard, c)
				}
			}(conn)
		}
	}()
	go func() {
		buf := make([]byte, 65536)
		for {
			n, peer, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			packet := append([]byte(nil), buf[:n]...)
			go func() {
				target, payload, err := decodeUDP(packet)
				if err != nil {
					return
				}
				conn, err := net.DialTimeout("udp", target, time.Second)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				conn.Write(payload)
				reply := make([]byte, 65536)
				n, err := conn.Read(reply)
				if err != nil {
					return
				}
				count.Add(1)
				address, _ := encodeSOCKSAddress(target)
				response := append([]byte{0, 0, 0}, address...)
				response = append(response, reply[:n]...)
				udp.WriteTo(response, peer)
			}()
		}
	}()
	return listener.Addr().String(), count
}
