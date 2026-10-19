package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/safehttp"
	"github.com/uniproxy/uniproxy/internal/store"
)

const diagnosticCanary = "synthetic-private-diagnostic-canary"
const diagnosticNodeCanary = "synthetic-node-password-canary"
const diagnosticSourceURL = "https://subscription.example.invalid/" + diagnosticCanary + "?token=" + diagnosticCanary
const diagnosticOtherURL = "https://other.example.invalid/fixture-subscription"
const diagnosticSourceNodes = "http://fixture-user:" + diagnosticNodeCanary + "@only.example.invalid:8080#only\nhttp://shared.example.invalid:8081#shared\nhttp://manual.example.invalid:8082#manual"

type diagnosticRoundTripper func(*http.Request) (*http.Response, error)

func (fn diagnosticRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func diagnosticResponse(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}
}

func diagnosticAssertSafe(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{diagnosticCanary, diagnosticNodeCanary} {
		if strings.Contains(string(raw), canary) {
			t.Fatal("private synthetic URL/content/credential reached a diagnostic or public response")
		}
	}
}

// Build real source attribution in temporary SQLite, but never start a core or
// fetch a source. The target has one orphan, one shared node, and one manual node.
func diagnosticSeedSource(t *testing.T, a *App, raw string) string {
	t.Helper()
	const target = "diagnostic-source"
	_, err := a.mutate(context.Background(), "prepare isolated diagnostic fixture", func(s *model.State) (any, error) {
		s.Groups = append(s.Groups, model.Group{ID: "diagnostic-group", Name: "diagnostic group"}, model.Group{ID: "diagnostic-other-group", Name: "other group"})
		s.Subscriptions = append(s.Subscriptions,
			model.Subscription{ID: target, Name: "diagnostic source", URL: raw, GroupIDs: []string{"diagnostic-group"}, Enabled: true, IntervalMinutes: 60, LastFetchedAt: "2000-01-01T00:00:00Z", UpdatedAt: model.Now()},
			model.Subscription{ID: "diagnostic-other-source", Name: "other source", URL: diagnosticOtherURL, GroupIDs: []string{"diagnostic-other-group"}, Enabled: true, IntervalMinutes: 60, UpdatedAt: model.Now()},
		)
		out := emptyImport()
		parsed := importer.Parse(diagnosticSourceNodes)
		if parsed.TotalErrors != 0 || len(parsed.Nodes) != 3 {
			return nil, errors.New("invalid synthetic subscription fixture")
		}
		if err := addParsed(s, parsed.Nodes, []string{"diagnostic-group"}, target, &out); err != nil {
			return nil, err
		}
		if err := addParsed(s, parsed.Nodes[1:2], []string{"diagnostic-other-group"}, "diagnostic-other-source", &out); err != nil {
			return nil, err
		}
		if err := addParsed(s, parsed.Nodes[2:3], []string{"diagnostic-other-group"}, "", &out); err != nil {
			return nil, err
		}
		s.Pools = append(s.Pools, model.Pool{ID: "diagnostic-pool", Name: "diagnostic pool", Enabled: true, Strategy: "round-robin", GroupMode: "any", NodeIDs: []string{s.Nodes[0].ID, s.Nodes[1].ID, s.Nodes[2].ID}, GroupIDs: []string{"diagnostic-group"}, TestURL: s.Settings.TestURL, IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50})
		// fakeCore never opens these configured ports.
		s.Listeners = append(s.Listeners,
			model.Listener{ID: "diagnostic-orphan-listener", Name: "orphan entrance", Kind: "node", TargetID: s.Nodes[0].ID, Host: "127.0.0.1", Port: 24701, Protocol: "mixed", Enabled: true},
			model.Listener{ID: "diagnostic-shared-listener", Name: "shared entrance", Kind: "node", TargetID: s.Nodes[1].ID, Host: "127.0.0.1", Port: 24702, Protocol: "mixed", Enabled: true},
			model.Listener{ID: "diagnostic-pool-listener", Name: "pool entrance", Kind: "pool", TargetID: "diagnostic-pool", Host: "127.0.0.1", Port: 24703, Protocol: "mixed", Enabled: true},
		)
		recount(s)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func diagnosticAssertFailedRefresh(t *testing.T, a *App, f *fakeCore, sourceID, expected string, before model.State, validates, applies int, out map[string]any) {
	t.Helper()
	if out["error"] != expected {
		t.Fatalf("classified refresh reason lost; got %v, want %s", out["error"], expected)
	}
	after := a.store.Snapshot()
	matched := false
	for i, sub := range after.Subscriptions {
		if sub.ID == sourceID {
			matched = true
			if sub.LastError != expected || sub.LastFetchedAt == "" || sub.LastFetchedAt == before.Subscriptions[i].LastFetchedAt {
				t.Fatal("failed refresh did not store safe current diagnostic metadata")
			}
			diagnosticAssertSafe(t, sub.LastError)
			after.Subscriptions[i].LastError = before.Subscriptions[i].LastError
			after.Subscriptions[i].LastFetchedAt = before.Subscriptions[i].LastFetchedAt
		}
	}
	if !matched || !reflect.DeepEqual(after, before) {
		t.Fatal("failed refresh changed configuration, source attribution, nodes, pools, or listeners")
	}
	poolTestAssertCalls(t, f, validates, applies)
	live := store.Clone(f.applied)
	desired := store.Clone(before)
	// Failure metadata is persisted without changing configuration revision and
	// therefore need not appear in the last live core snapshot on a retry.
	for i := range live.Subscriptions {
		live.Subscriptions[i].LastError, live.Subscriptions[i].LastFetchedAt = "", ""
	}
	for i := range desired.Subscriptions {
		desired.Subscriptions[i].LastError, desired.Subscriptions[i].LastFetchedAt = "", ""
	}
	if !reflect.DeepEqual(live, desired) {
		t.Fatal("failed refresh replaced the fake core's old live configuration")
	}
	activities := a.store.Activities()
	if len(activities) == 0 || activities[0].Level != "warning" || !strings.Contains(activities[0].Message, expected) {
		t.Fatal("safe classified refresh warning was not recorded")
	}
	diagnosticAssertSafe(t, activities)
	diagnosticAssertSafe(t, out)
	code, public := request(t, a, "GET", "/api/bootstrap", nil)
	if code != 200 {
		t.Fatal("bootstrap failed after subscription diagnostic")
	}
	diagnosticAssertSafe(t, public)
}

func diagnosticExerciseDownloadFailure(t *testing.T, a *App, f *fakeCore, raw, expected string) {
	t.Helper()
	before := a.store.Snapshot()
	v, ap := poolTestCoreCounts(f)
	activities := a.store.Activities()
	code, preview := request(t, a, "POST", "/api/import", map[string]any{"mode": "subscriptions", "content": raw, "preview": true})
	if code != 200 {
		t.Fatalf("download diagnostic preview returned %d", code)
	}
	parsed := poolTestDecode[model.ParseResult](t, preview)
	if len(parsed.Nodes) != 0 || parsed.TotalErrors != 1 || len(parsed.Errors) != 1 || parsed.Errors[0].Line != 1 || parsed.Errors[0].Message != expected {
		t.Fatal("preview lost source line or classified download reason")
	}
	diagnosticAssertSafe(t, preview)
	code, commit := request(t, a, "POST", "/api/import", map[string]any{"mode": "subscriptions", "content": raw})
	if code != 400 || !strings.Contains(commit["error"].(string), expected) {
		t.Fatal("failed source commit did not return its classified reason")
	}
	diagnosticAssertSafe(t, commit)
	if !reflect.DeepEqual(a.store.Snapshot(), before) || !reflect.DeepEqual(a.store.Activities(), activities) {
		t.Fatal("failed preview/import persisted source, nodes, or diagnostics")
	}
	poolTestAssertCalls(t, f, v, ap)

	sourceID := diagnosticSeedSource(t, a, raw)
	before = a.store.Snapshot()
	v, ap = poolTestCoreCounts(f)
	code, out := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{"force": true})
	if code != 400 {
		t.Fatalf("failed forced refresh returned %d", code)
	}
	diagnosticAssertFailedRefresh(t, a, f, sourceID, expected, before, v, ap, out)
}

func TestSubscriptionDiagnosticsTypedErrorsStaySafeAcrossAPIs(t *testing.T) {
	for _, kind := range []safehttp.ErrorKind{
		safehttp.KindURL, safehttp.KindDNS, safehttp.KindPrivateAddress, safehttp.KindFakeIPFallback,
		safehttp.KindConnect, safehttp.KindConnectTimeout, safehttp.KindTLS, safehttp.KindRedirect,
		safehttp.KindHTTPStatus, safehttp.KindBodyRead, safehttp.KindBodyTooLarge, safehttp.KindHTML,
		safehttp.KindTimeout, safehttp.KindCanceled, safehttp.KindDownload,
	} {
		t.Run(string(kind), func(t *testing.T) {
			a, f := fixture(t, true)
			typed := &safehttp.FetchError{Kind: kind, StatusCode: http.StatusForbidden}
			a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "Get " + diagnosticCanary, URL: "https://fixture-user:" + diagnosticCanary + "@nested.example.invalid/" + diagnosticCanary, Err: typed}
			})}
			diagnosticExerciseDownloadFailure(t, a, f, diagnosticSourceURL, typed.Error())
		})
	}
}

func TestSubscriptionDiagnosticsUnknownTransportErrorDoesNotLeak(t *testing.T) {
	a, f := fixture(t, true)
	a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: diagnosticSourceURL, Err: errors.New("opaque transport failure: " + diagnosticCanary)}
	})}
	diagnosticExerciseDownloadFailure(t, a, f, diagnosticSourceURL, (&safehttp.FetchError{Kind: safehttp.KindDownload}).Error())
}

func TestSubscriptionDiagnosticsLocalHTTPFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		kind   safehttp.ErrorKind
		status int
	}{
		{"http-401", safehttp.KindHTTPStatus, 401},
		{"http-403", safehttp.KindHTTPStatus, 403},
		{"http-429", safehttp.KindHTTPStatus, 429},
		{"http-503", safehttp.KindHTTPStatus, 503},
		{"html-content-type", safehttp.KindHTML, 200},
		{"html-sniffed", safehttp.KindHTML, 200},
		{"too-large-header", safehttp.KindBodyTooLarge, 200},
		{"too-large-stream", safehttp.KindBodyTooLarge, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f := fixture(t, true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				switch test.name {
				case "html-content-type":
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = io.WriteString(w, "<!doctype html><html><body>"+diagnosticCanary+"</body></html>")
				case "html-sniffed":
					_, _ = io.WriteString(w, "<!DOCTYPE html><html><body>"+diagnosticCanary+"</body></html>")
				case "too-large-header":
					w.Header().Set("Content-Length", strconv.Itoa(safehttp.MaxBody+1))
					w.WriteHeader(http.StatusOK)
				case "too-large-stream":
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					_, _ = io.WriteString(w, strings.Repeat("x", safehttp.MaxBody+1)+diagnosticCanary)
				default:
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, diagnosticCanary)
				}
			}))
			defer server.Close()
			a.client = safehttp.NewClient(true) // explicit test-only access to our loopback server
			diagnosticExerciseDownloadFailure(t, a, f, server.URL+"/"+diagnosticCanary+"?token="+diagnosticCanary, (&safehttp.FetchError{Kind: test.kind, StatusCode: test.status}).Error())
		})
	}
}

type diagnosticFailedBody struct{}

func (diagnosticFailedBody) Read([]byte) (int, error) {
	return 0, errors.New("body read: " + diagnosticCanary)
}
func (diagnosticFailedBody) Close() error { return nil }

func TestSubscriptionDiagnosticsBodyReadAndTLSRemainSanitized(t *testing.T) {
	t.Run("body-read", func(t *testing.T) {
		a, f := fixture(t, true)
		a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: diagnosticFailedBody{}, ContentLength: -1, Request: r}, nil
		})}
		diagnosticExerciseDownloadFailure(t, a, f, diagnosticSourceURL, (&safehttp.FetchError{Kind: safehttp.KindBodyRead}).Error())
	})
	t.Run("untrusted-local-tls", func(t *testing.T) {
		a, f := fixture(t, true)
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, diagnosticSourceNodes)
		}))
		server.Config.ErrorLog = log.New(io.Discard, "", 0)
		server.StartTLS()
		defer server.Close()
		a.client = safehttp.NewClient(true) // allow loopback, never bypass certificate verification
		diagnosticExerciseDownloadFailure(t, a, f, server.URL+"/"+diagnosticCanary, (&safehttp.FetchError{Kind: safehttp.KindTLS}).Error())
	})
}

func TestSubscriptionDiagnosticsMixedSourcesImportOnlyCompleteSource(t *testing.T) {
	a, f := fixture(t, true)
	diagnosticSeedSource(t, a, diagnosticSourceURL)
	const goodURL = "https://good.example.invalid/fixture?token=" + diagnosticCanary
	const partialURL = "https://partial.example.invalid/fixture?token=" + diagnosticCanary
	const failedURL = "https://failed.example.invalid/fixture?token=" + diagnosticCanary
	const emptyURL = "https://empty.example.invalid/fixture?token=" + diagnosticCanary
	goodBody := "http://good-new.example.invalid:8080#good-new"
	partialBody := "http://partial-new.example.invalid:8080#partial-new\nunsupported://" + diagnosticCanary + "@invalid.example.invalid:1\nunsupported://" + diagnosticCanary + "@invalid.example.invalid:2"
	partial := importer.Parse(partialBody)
	if partial.TotalErrors != 2 || len(partial.Nodes) != 1 {
		t.Fatal("invalid partial source fixture")
	}
	calls := map[string]int{}
	a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.String()]++
		switch r.URL.String() {
		case goodURL:
			return diagnosticResponse(r, goodBody), nil
		case partialURL:
			return diagnosticResponse(r, partialBody), nil
		case failedURL:
			return nil, &url.Error{Op: "Get", URL: failedURL, Err: &safehttp.FetchError{Kind: safehttp.KindDNS}}
		case emptyURL:
			return diagnosticResponse(r, "proxies: []"), nil
		default:
			return nil, errors.New("unexpected synthetic URL")
		}
	})}
	before := a.store.Snapshot()
	activities := a.store.Activities()
	v, ap := poolTestCoreCounts(f)
	body := map[string]any{"mode": "subscriptions", "content": strings.Join([]string{goodURL, partialURL, failedURL, emptyURL, goodURL}, "\n"), "groupIds": []string{"diagnostic-group"}, "preview": true}
	code, preview := request(t, a, "POST", "/api/import", body)
	if code != 200 {
		t.Fatal("mixed-source preview failed")
	}
	result := poolTestDecode[model.ParseResult](t, preview)
	if len(result.Nodes) != 1 || result.Nodes[0].Server != "good-new.example.invalid" || result.Nodes[0].Config != nil || result.TotalErrors != 3 || len(result.Errors) != 3 {
		t.Fatal("mixed-source preview included partial nodes or lost failed-source counts")
	}
	for i, issue := range result.Errors {
		if issue.Line != i+2 {
			t.Fatal("subscription issue lost input source line")
		}
	}
	if !strings.Contains(result.Errors[0].Message, "共 2 项错误") || !strings.Contains(result.Errors[0].Message, "配置第 2 项") || !strings.Contains(result.Errors[0].Message, partial.Errors[0].Message) {
		t.Fatal("partial source preview lost actual count/first parser location/reason")
	}
	if result.Errors[1].Message != (&safehttp.FetchError{Kind: safehttp.KindDNS}).Error() || !strings.Contains(result.Errors[2].Message, "没有有效节点") {
		t.Fatal("mixed sources did not retain independent download/empty reasons")
	}
	if !reflect.DeepEqual(a.store.Snapshot(), before) || !reflect.DeepEqual(a.store.Activities(), activities) {
		t.Fatal("mixed-source preview persisted data")
	}
	poolTestAssertCalls(t, f, v, ap)
	diagnosticAssertSafe(t, preview)
	body["preview"] = false
	code, out := request(t, a, "POST", "/api/import", body)
	if code != 200 {
		t.Fatal("complete source was lost because another source failed")
	}
	imported := poolTestDecode[model.ImportResult](t, out["data"])
	after := a.store.Snapshot()
	if imported.Added != 1 || imported.TotalErrors != 3 || len(imported.Errors) != 3 || len(imported.SubscriptionIDs) != 1 || !reflect.DeepEqual(imported.Errors, result.Errors) {
		t.Fatal("mixed source commit disagrees with preview or imported partial entries")
	}
	if after.Revision != before.Revision+1 || len(after.Nodes) != len(before.Nodes)+1 || len(after.Subscriptions) != len(before.Subscriptions)+1 || after.Subscriptions[len(before.Subscriptions)].URL != goodURL {
		t.Fatal("mixed source mutation did not commit only the complete source once")
	}
	if !reflect.DeepEqual(after.Nodes[:len(before.Nodes)], before.Nodes) || !reflect.DeepEqual(after.Subscriptions[:len(before.Subscriptions)], before.Subscriptions) || !reflect.DeepEqual(after.Pools, before.Pools) || !reflect.DeepEqual(after.Listeners, before.Listeners) {
		t.Fatal("mixed import altered an existing source, node, pool, or listener")
	}
	for _, raw := range []string{goodURL, partialURL, failedURL, emptyURL} {
		if calls[raw] != 2 {
			t.Fatal("duplicate subscription URL was fetched more than once per request")
		}
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
	diagnosticAssertSafe(t, out)
	diagnosticAssertSafe(t, a.store.Activities())
	_, public := request(t, a, "GET", "/api/bootstrap", nil)
	diagnosticAssertSafe(t, public)
}

func TestSubscriptionDiagnosticsPartialRefreshNeverPrunesEvenForced(t *testing.T) {
	a, f := fixture(t, true)
	sourceID := diagnosticSeedSource(t, a, diagnosticSourceURL)
	partialBody := "http://partial-new.example.invalid:8080#partial-new\n" + strings.Repeat("unsupported://"+diagnosticCanary+"@invalid.example.invalid:1\n", 150)
	parsed := importer.Parse(partialBody)
	if parsed.TotalErrors != 150 || len(parsed.Errors) != 128 || !parsed.ErrorsTruncated || len(parsed.Nodes) != 1 {
		t.Fatal("invalid capped partial source fixture")
	}
	a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) { return diagnosticResponse(r, partialBody), nil })}
	for _, force := range []bool{false, true} {
		before := a.store.Snapshot()
		v, ap := poolTestCoreCounts(f)
		code, out := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{"force": force})
		if code != 400 {
			t.Fatal("incomplete source was force-accepted")
		}
		expected := fmt.Sprintf("订阅已下载，但配置解析不完整，已保留原节点：共 150 项错误；配置第 %d 项：%s", parsed.Errors[0].Line, parsed.Errors[0].Message)
		diagnosticAssertFailedRefresh(t, a, f, sourceID, expected, before, v, ap, out)
	}
}

func TestSubscriptionDiagnosticsEmptyHTTP200KeepsGuardAndForceSemantics(t *testing.T) {
	a, f := fixture(t, true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, " \n\t")
	}))
	defer server.Close()
	a.client = safehttp.NewClient(true)
	sourceID := diagnosticSeedSource(t, a, server.URL+"/"+diagnosticCanary+"?token="+diagnosticCanary)
	before := a.store.Snapshot()
	v, ap := poolTestCoreCounts(f)
	code, rejected := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{})
	if code != 409 || rejected["code"] != "subscription_shrink" {
		t.Fatal("empty HTTP 200 bypassed the deletion confirmation guard")
	}
	diagnosticAssertFailedRefresh(t, a, f, sourceID, "订阅为空或超过一半节点将被删除；已保留原节点，请确认后强制刷新", before, v, ap, rejected)
	code, out := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{"force": true})
	if code != 200 {
		t.Fatal("explicit empty-source confirmation no longer works")
	}
	after := a.store.Snapshot()
	result := poolTestDecode[model.ImportResult](t, out["data"])
	if after.Revision != before.Revision+1 || result.Removed != 1 || len(after.Nodes) != 2 || nodeIndex(&after, before.Nodes[0].ID) >= 0 {
		t.Fatal("forced empty source did not remove only its orphan")
	}
	for _, n := range after.Nodes {
		if contains(n.SourceIDs, sourceID) || len(n.GroupIDs) != 1 || n.GroupIDs[0] != "diagnostic-other-group" {
			t.Fatal("forced empty source damaged other group/source attribution")
		}
		if n.ID == before.Nodes[1].ID && (n.Manual || !reflect.DeepEqual(n.SourceIDs, []string{"diagnostic-other-source"})) {
			t.Fatal("shared node lost its other source")
		}
		if n.ID == before.Nodes[2].ID && (!n.Manual || len(n.SourceIDs) != 0) {
			t.Fatal("manual node was not retained")
		}
	}
	if after.Listeners[0].Enabled || after.Listeners[0].TargetID != before.Nodes[0].ID || !reflect.DeepEqual(after.Listeners[1:], before.Listeners[1:]) || contains(after.Pools[0].NodeIDs, before.Nodes[0].ID) {
		t.Fatal("orphan cleanup rebound/deleted listeners or left stale pool references")
	}
	if after.Subscriptions[0].LastError != "" || after.Subscriptions[0].NodeCount != 0 || after.Subscriptions[1].NodeCount != 1 {
		t.Fatal("successful confirmed refresh did not clear diagnostic/recount sources")
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
	if !reflect.DeepEqual(store.Clone(f.applied), after) {
		t.Fatal("confirmed refresh was not applied as one snapshot")
	}
	diagnosticAssertSafe(t, out)
	diagnosticAssertSafe(t, a.store.Activities())
}

func TestSubscriptionDiagnosticsSuccessfulRetryClearsOldError(t *testing.T) {
	a, f := fixture(t, true)
	sourceID := diagnosticSeedSource(t, a, diagnosticSourceURL)
	attempt := 0
	a.client = &http.Client{Timeout: time.Second, Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
		attempt++
		if attempt == 1 {
			return nil, &url.Error{Op: "Get", URL: diagnosticSourceURL, Err: &safehttp.FetchError{Kind: safehttp.KindFakeIPFallback}}
		}
		return diagnosticResponse(r, diagnosticSourceNodes), nil
	})}
	before := a.store.Snapshot()
	v, ap := poolTestCoreCounts(f)
	code, failed := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{})
	if code != 400 {
		t.Fatal("synthetic fallback failure was not surfaced")
	}
	diagnosticAssertFailedRefresh(t, a, f, sourceID, (&safehttp.FetchError{Kind: safehttp.KindFakeIPFallback}).Error(), before, v, ap, failed)
	code, out := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{})
	if code != 200 {
		t.Fatal("retry blocked by stale refresh marker")
	}
	after := a.store.Snapshot()
	if after.Subscriptions[0].LastError != "" || after.Revision != before.Revision+1 || len(after.Nodes) != len(before.Nodes) || !reflect.DeepEqual(after.Listeners, before.Listeners) || !reflect.DeepEqual(after.Pools, before.Pools) {
		t.Fatal("successful retry retained an error or damaged stable identities")
	}
	for i := range before.Nodes {
		if after.Nodes[i].ID != before.Nodes[i].ID || after.Nodes[i].Fingerprint != before.Nodes[i].Fingerprint {
			t.Fatal("successful retry duplicated/replaced source nodes")
		}
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
	diagnosticAssertSafe(t, out)
	_, public := request(t, a, "GET", "/api/bootstrap", nil)
	diagnosticAssertSafe(t, public)
}
