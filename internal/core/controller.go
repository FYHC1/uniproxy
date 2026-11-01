package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

const maxControllerResponse = 8 << 20

func (m *Manager) controller(ctx context.Context, method, path string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return errors.New("could not encode core controller request")
		}
		body = bytes.NewReader(data)
	}
	m.mu.RLock()
	address, secret := m.buildOpts.ControllerAddress, m.buildOpts.ControllerSecret
	m.mu.RUnlock()
	if _, _, err := privateAddress(address); err != nil || secret == "" {
		return errors.New("private core controller is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+address+path, body)
	if err != nil {
		return errors.New("could not create core controller request")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("authenticated core controller request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Core error JSON can contain native outbound secrets. Do not propagate it.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("core controller returned HTTP %d", resp.StatusCode)
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if err != nil {
			return errors.New("core controller response was interrupted")
		}
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxControllerResponse+1))
	if err != nil || len(data) > maxControllerResponse {
		return errors.New("core controller response is invalid or too large")
	}
	if err = json.Unmarshal(data, output); err != nil {
		return errors.New("core controller returned invalid JSON")
	}
	return nil
}
func (m *Manager) controllerReady(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var reply struct {
		Version string `json:"version"`
	}
	if err := m.controller(requestCtx, http.MethodGet, "/version", nil, &reply); err != nil {
		return err
	}
	if reply.Version != PinnedVersion && reply.Version != "v"+PinnedVersion {
		return errors.New("core controller version does not match the pinned binary")
	}
	return nil
}
func (m *Manager) reload(ctx context.Context, data []byte) error {
	updateCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return m.controller(updateCtx, http.MethodPut, "/configs?force=true", map[string]string{"payload": string(data)}, nil)
}

func (m *Manager) verify(ctx context.Context, cfg *compiledConfig) error {
	if err := m.controllerReady(ctx); err != nil {
		return err
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := m.verifyProxies(verifyCtx, cfg); err != nil {
		return err
	}
	if err := verifyListener(verifyCtx, cfg.probe); err != nil {
		return errors.New("private authenticated core probe did not become ready")
	}
	// Bounded workers avoid O(N) sequential network latency when applying large
	// batches, without opening thousands of simultaneous probe connections.
	var workers sync.WaitGroup
	checks := make(chan model.Listener)
	results := make(chan error, len(cfg.listeners))
	workerCount := min(16, len(cfg.listeners))
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for listener := range checks {
				if err := verifyListener(verifyCtx, listener); err != nil {
					results <- fmt.Errorf("listener %s failed its protocol/authentication bind check", listener.ID)
				}
			}
		}()
	}
	for _, listener := range cfg.listeners {
		checks <- listener
	}
	close(checks)
	workers.Wait()
	close(results)
	for err := range results {
		return err
	}
	return verifyCtx.Err()
}

const (
	proxyVerifyTimeout           = 3 * time.Second
	proxyVerifyRetryInterval     = 100 * time.Millisecond
	listenerVerifyTimeout        = 3 * time.Second
	listenerVerifyAttemptTimeout = time.Second
	listenerVerifyRetryInterval  = 100 * time.Millisecond
)

// verifyProxies confirms the native proxy graph, not simply a listening
// unrelated process. A fresh core can answer its controller version before the
// submitted configuration is visible there, so a bounded retry avoids treating
// that startup race as a bad configuration.
func (m *Manager) verifyProxies(ctx context.Context, cfg *compiledConfig) error {
	waitCtx, cancel := context.WithTimeout(ctx, proxyVerifyTimeout)
	defer cancel()
	var lastErr error
	for {
		var reply struct {
			Proxies map[string]json.RawMessage `json:"proxies"`
		}
		lastErr = m.controller(waitCtx, http.MethodGet, "/proxies", nil, &reply)
		if lastErr == nil {
			missing := false
			for id := range cfg.nodes {
				if _, ok := reply.Proxies[nodeTag(id)]; !ok {
					missing = true
					break
				}
			}
			if !missing {
				return nil
			}
			lastErr = errors.New("core did not expose a requested native outbound")
		}
		if waitCtx.Err() != nil {
			return lastErr
		}
		timer := time.NewTimer(proxyVerifyRetryInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}

// verifyListener waits for a fresh core to finish binding. The first probe can
// race the kernel launch on a cold host even though the authenticated controller
// is already ready, so a single short check is not reliable. Each attempt stays
// bounded, and the outer deadline caps the total wait for one listener.
func verifyListener(ctx context.Context, listener model.Listener) error {
	waitCtx, cancel := context.WithTimeout(ctx, listenerVerifyTimeout)
	defer cancel()
	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(waitCtx, listenerVerifyAttemptTimeout)
		lastErr = verifyListenerOnce(attemptCtx, listener)
		attemptCancel()
		if lastErr == nil {
			return nil
		}
		if waitCtx.Err() != nil {
			return lastErr
		}
		timer := time.NewTimer(listenerVerifyRetryInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}

func verifyListenerOnce(ctx context.Context, listener model.Listener) error {
	if listener.Protocol == "http" || listener.Protocol == "mixed" {
		if len(listener.Users) > 0 {
			if err := verifyHTTP(ctx, listener, nil, http.StatusProxyAuthRequired); err != nil {
				return err
			}
		}
		var credential *model.Credential
		if len(listener.Users) > 0 {
			credential = &listener.Users[0]
		}
		if err := verifyHTTP(ctx, listener, credential, http.StatusBadRequest); err != nil {
			return err
		}
	}
	if listener.Protocol == "socks" || listener.Protocol == "mixed" {
		if err := verifySOCKS(ctx, listener); err != nil {
			return err
		}
	}
	if listener.UDP {
		// UDP has no unauthenticated application-level health request. Confirm that
		// the bind checked before reload was acquired. Bind errors in the bounded
		// Mihomo log are also checked before committing the transaction.
		sock, err := net.ListenPacket("udp", net.JoinHostPort(listener.Host, strconv.Itoa(listener.Port)))
		if err == nil {
			_ = sock.Close()
			return errors.New("requested UDP socket is not bound")
		}
	}
	return nil
}
func dialListener(ctx context.Context, listener model.Listener) (net.Conn, error) {
	host := listener.Host
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsUnspecified() {
		if ip.Is4() {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(listener.Port)))
	if err != nil {
		return nil, errors.New("proxy listener is not reachable")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	_ = c.SetDeadline(deadline)
	return c, nil
}
func verifyHTTP(ctx context.Context, listener model.Listener, credential *model.Credential, want int) error {
	c, err := dialListener(ctx, listener)
	if err != nil {
		return err
	}
	defer c.Close()
	// A relative target is deliberately rejected by Mihomo AFTER checking auth;
	// this proves the endpoint without sending traffic through any outbound.
	request := "GET /__uniproxy_listener_check HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n"
	if credential != nil {
		request += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(credential.Username+":"+credential.Password)) + "\r\n"
	}
	if _, err = io.WriteString(c, request+"\r\n"); err != nil {
		return errors.New("HTTP listener handshake failed")
	}
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(c, 8192)), &http.Request{Method: http.MethodGet})
	if err != nil {
		return errors.New("HTTP listener did not speak HTTP")
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		return errors.New("HTTP listener authentication does not match the requested configuration")
	}
	return nil
}
func verifySOCKS(ctx context.Context, listener model.Listener) error {
	c, err := dialListener(ctx, listener)
	if err != nil {
		return err
	}
	defer c.Close()
	method := byte(0)
	if len(listener.Users) > 0 {
		method = 2
	}
	if _, err = c.Write([]byte{5, 1, method}); err != nil {
		return errors.New("SOCKS listener handshake failed")
	}
	var reply [2]byte
	if _, err = io.ReadFull(c, reply[:]); err != nil || reply[0] != 5 || reply[1] != method {
		return errors.New("SOCKS listener authentication method is incorrect")
	}
	if method == 2 {
		user := listener.Users[0]
		auth := []byte{1, byte(len(user.Username))}
		auth = append(auth, []byte(user.Username)...)
		auth = append(auth, byte(len(user.Password)))
		auth = append(auth, []byte(user.Password)...)
		if _, err = c.Write(auth); err != nil {
			return errors.New("SOCKS listener authentication failed")
		}
		if _, err = io.ReadFull(c, reply[:]); err != nil || reply[0] != 1 || reply[1] != 0 {
			return errors.New("SOCKS listener rejected configured credentials")
		}
	}
	return nil
}
