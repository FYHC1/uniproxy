package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/uniproxy/uniproxy/internal/model"
)

const MaxSpeedBytes int64 = 100 << 20
const MaxSpeedDuration = 2 * time.Minute

func (m *Manager) testSnapshot(nodeID string) (BuildOptions, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.status.Running || !processAlive(m.process) || m.live == nil {
		return BuildOptions{}, ErrNotRunning
	}
	if m.status.Applying {
		return BuildOptions{}, errors.New("core configuration is being applied; retry the test")
	}
	if _, ok := m.live.nodes[nodeID]; !ok {
		return BuildOptions{}, errors.New("node is disabled or absent from the live configuration")
	}
	return m.buildOpts, nil
}
func failedCheck(result model.CheckResult, err error) (model.CheckResult, error) {
	result.OK = false
	result.Error = err.Error()
	return result, err
}

// Check uses Mihomo's actual outbound URL-test, not a TCP reachability ping.
func (m *Manager) Check(ctx context.Context, nodeID, testURL string, timeout time.Duration) (model.CheckResult, error) {
	result := model.CheckResult{NodeID: nodeID}
	if _, err := m.testSnapshot(nodeID); err != nil {
		return failedCheck(result, err)
	}
	if err := validTestURL(testURL); err != nil {
		return failedCheck(result, err)
	}
	if timeout <= 0 {
		return failedCheck(result, errors.New("test timeout must be positive"))
	}
	// v1.19.32 parses the API timeout with strconv.ParseInt(..., 16).
	if timeout > 32767*time.Millisecond {
		timeout = 32767 * time.Millisecond
	}
	if timeout < time.Millisecond {
		timeout = time.Millisecond
	}
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	query := url.Values{"url": {testURL}, "timeout": {strconv.FormatInt(timeout.Milliseconds(), 10)}, "expected": {"200-399"}}
	var reply struct {
		Delay *int64 `json:"delay"`
	}
	err := m.controller(testCtx, http.MethodGet, "/proxies/"+url.PathEscape(nodeTag(nodeID))+"/delay?"+query.Encode(), nil, &reply)
	if err != nil {
		return failedCheck(result, err)
	}
	if reply.Delay == nil || *reply.Delay <= 0 || *reply.Delay > 65535 {
		return failedCheck(result, errors.New("core returned an invalid URL-test delay"))
	}
	result.OK = true
	result.LatencyMS = reply.Delay
	return result, nil
}

// Speed downloads real bytes through a node-authenticated internal HTTP proxy.
// A new Transport prevents cross-node keep-alive reuse. There is no direct path,
// environment proxy, redirect bypass or unbounded transfer. It never serves body
// contents to the caller. Cancellation and byte/time limits apply during reads.
func (m *Manager) Speed(ctx context.Context, nodeID, testURL string, timeout time.Duration, maxBytes int64) (model.CheckResult, error) {
	result := model.CheckResult{NodeID: nodeID}
	opts, err := m.testSnapshot(nodeID)
	if err != nil {
		return failedCheck(result, err)
	}
	if err = validTestURL(testURL); err != nil {
		return failedCheck(result, err)
	}
	if timeout <= 0 || timeout > MaxSpeedDuration {
		return failedCheck(result, errors.New("speed timeout must be positive and at most two minutes"))
	}
	if maxBytes <= 0 || maxBytes > MaxSpeedBytes {
		return failedCheck(result, errors.New("speed byte limit must be between 1 and 100 MiB"))
	}
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	proxyURL := &url.URL{Scheme: "http", Host: opts.ProbeAddress, User: url.UserPassword(nodeTag(nodeID), probePassword(opts.ProbeSecret, nodeID))}
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL), DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
		DisableCompression: true, DisableKeepAlives: true, ResponseHeaderTimeout: timeout, TLSHandshakeTimeout: timeout,
		MaxResponseHeaderBytes: 64 << 10, ForceAttemptHTTP2: false,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many speed test redirects")
		}
		return validTestURL(req.URL.String())
	}}
	req, err := http.NewRequestWithContext(testCtx, http.MethodGet, testURL, nil)
	if err != nil {
		return failedCheck(result, errors.New("could not create speed test request"))
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxBytes-1))
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "UniProxy/0.1 speed-test")
	start := time.Now()
	response, err := client.Do(req)
	if err != nil {
		if testCtx.Err() != nil {
			return failedCheck(result, testCtx.Err())
		}
		return failedCheck(result, errors.New("node-proxied speed download failed"))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return failedCheck(result, fmt.Errorf("speed test endpoint returned HTTP %d", response.StatusCode))
	}
	latency := time.Since(start).Milliseconds()
	result.Bytes, err = io.Copy(io.Discard, io.LimitReader(response.Body, maxBytes))
	elapsed := time.Since(start)
	if err != nil {
		if testCtx.Err() != nil {
			return failedCheck(result, testCtx.Err())
		}
		return failedCheck(result, errors.New("node-proxied speed download was interrupted"))
	}
	if testCtx.Err() != nil {
		return failedCheck(result, testCtx.Err())
	}
	if result.Bytes == 0 {
		return failedCheck(result, errors.New("speed test endpoint returned no download bytes"))
	}
	if elapsed <= 0 {
		elapsed = time.Nanosecond
	}
	mbps := float64(result.Bytes) * 8 / elapsed.Seconds() / 1e6
	result.OK = true
	result.SpeedMbps = &mbps
	result.LatencyMS = &latency
	return result, nil
}
