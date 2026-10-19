package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCore struct {
	mu            sync.Mutex
	status        model.RuntimeStatus
	validateError error
	applyError    error
	validateCalls int
	applyCalls    int
	validated     model.State
	applyAttempt  model.State
	applied       model.State
	probeStarted  chan struct{}
	probeRelease  chan struct{}
}

func (f *fakeCore) Inspect(context.Context) model.RuntimeStatus { return f.Status() }
func (f *fakeCore) Status() model.RuntimeStatus                 { f.mu.Lock(); defer f.mu.Unlock(); return f.status }
func (f *fakeCore) Validate(_ context.Context, s model.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateCalls++
	f.validated = s
	if !f.status.Available {
		return errors.New("未安装内核")
	}
	return f.validateError
}
func (f *fakeCore) Apply(_ context.Context, s model.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	f.applyAttempt = s
	if !f.status.Available {
		return errors.New("未安装内核")
	}
	if f.applyError != nil {
		return f.applyError
	}
	f.applied = s
	f.status.Running = true
	f.status.AppliedRevision = s.Revision
	return nil
}
func (f *fakeCore) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.Running = false
	return nil
}
func (f *fakeCore) Check(ctx context.Context, id, url string, timeout time.Duration) (model.CheckResult, error) {
	if f.probeStarted != nil {
		close(f.probeStarted)
		select {
		case <-f.probeRelease:
		case <-ctx.Done():
			return model.CheckResult{}, ctx.Err()
		}
	}
	v := int64(12)
	return model.CheckResult{NodeID: id, OK: true, LatencyMS: &v}, nil
}
func (f *fakeCore) Speed(ctx context.Context, id, url string, timeout time.Duration, max int64) (model.CheckResult, error) {
	v := 12.3
	return model.CheckResult{NodeID: id, OK: true, SpeedMbps: &v, Bytes: max}, nil
}
func fixture(t *testing.T, available bool) (*App, *fakeCore) {
	t.Helper()
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	f := &fakeCore{status: model.RuntimeStatus{Available: available}}
	a := New(s, f, Options{AdminPort: 9090})
	t.Cleanup(func() { a.Close(); s.Close() })
	return a, f
}
func request(t *testing.T, a *App, method, path string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://127.0.0.1:9090"+path, bytes.NewReader(b))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	var out map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatalf("non-json status%d %s", w.Code, w.Body.String())
	}
	return w.Code, out
}
func importOne(t *testing.T, a *App, groups ...string) string {
	t.Helper()
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "http://user:secret@example.com:8080#example", "groupNames": groups, "groupIds": []string{}})
	if code != 200 {
		t.Fatalf("import %d %#v", code, out)
	}
	s := a.store.Snapshot()
	return s.Nodes[0].ID
}
func TestImportBatchGroupsAndSecretRedaction(t *testing.T) {
	a, f := fixture(t, true)
	id := importOne(t, a, "浏览器", "自动化")
	s := a.store.Snapshot()
	if len(s.Nodes) != 1 || len(s.Nodes[0].GroupIDs) != 2 {
		t.Fatal("multi groups not persisted")
	}
	code, out := request(t, a, "GET", "/api/bootstrap", nil)
	b, _ := json.Marshal(out)
	if code != 200 || bytes.Contains(b, []byte("secret")) || bytes.Contains(b, []byte("config")) {
		t.Fatalf("unsafe bootstrap %s", b)
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{id}, "action": "disable"})
	if a.store.Snapshot().Nodes[0].Enabled || f.applied.Nodes[0].Enabled {
		t.Fatal("disable not auto applied")
	}
	importOne(t, a, "浏览器")
	if len(a.store.Snapshot().Nodes) != 1 {
		t.Fatal("duplicate node created")
	}
}
func TestMissingCoreSavesHonestPendingState(t *testing.T) {
	a, _ := fixture(t, false)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "socks5://127.0.0.1:1080", "groupIds": []string{}})
	if code != 200 {
		t.Fatal(out)
	}
	r := out["runtime"].(map[string]any)
	if r["dirty"] != true || r["running"] != false || r["lastError"] == "" {
		t.Fatal(r)
	}
	code, _ = request(t, a, "POST", "/api/tests", map[string]any{"kind": "latency", "nodeIds": []string{a.store.Snapshot().Nodes[0].ID}})
	if code != 409 {
		t.Fatal(code)
	}
}
func TestPreviewDoesNotPersist(t *testing.T) {
	a, _ := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "http://u:secret@example.com:8080", "preview": true})
	if code != 200 || len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal(out)
	}
	b, _ := json.Marshal(out)
	if bytes.Contains(b, []byte("secret")) {
		t.Fatal(string(b))
	}
}
func TestInvalidCoreMutationIsAtomic(t *testing.T) {
	a, f := fixture(t, true)
	importOne(t, a)
	before := a.store.Snapshot()
	f.validateError = errors.New("bad config")
	code, _ := request(t, a, "POST", "/api/groups", map[string]any{"name": "not committed"})
	if code != 400 || a.store.Snapshot().Revision != before.Revision || len(a.store.Snapshot().Groups) != len(before.Groups) {
		t.Fatal("invalid mutation committed")
	}
}
func TestApplyFailureReportsPending(t *testing.T) {
	a, f := fixture(t, true)
	importOne(t, a)
	f.applyError = errors.New("port occupied")
	code, out := request(t, a, "POST", "/api/groups", map[string]any{"name": "pending"})
	if code != 200 {
		t.Fatal(code)
	}
	r := out["runtime"].(map[string]any)
	if r["dirty"] != true || !strings.Contains(r["lastError"].(string), "port occupied") {
		t.Fatal(r)
	}
}
func TestListenerValidationAndPasswordPreservation(t *testing.T) {
	a, _ := fixture(t, true)
	id := importOne(t, a)
	body := map[string]any{"name": "my proxy", "kind": "node", "targetId": id, "protocol": "mixed", "host": "0.0.0.0", "port": 18080, "udp": false, "enabled": true, "users": []any{}}
	code, _ := request(t, a, "POST", "/api/listeners", body)
	if code != 400 {
		t.Fatal("public unauthenticated allowed")
	}
	body["users"] = []map[string]string{{"username": "alice", "password": "do-not-show"}}
	code, out := request(t, a, "POST", "/api/listeners", body)
	if code != 200 {
		t.Fatal(out)
	}
	b, _ := json.Marshal(out)
	if bytes.Contains(b, []byte("do-not-show")) {
		t.Fatal("password leaked")
	}
	lid := out["data"].(map[string]any)["id"].(string)
	body["users"] = []map[string]string{{"username": "alice"}}
	code, out = request(t, a, "PUT", "/api/listeners/"+lid, body)
	if code != 200 {
		t.Fatal(out)
	}
	if a.store.Snapshot().Listeners[0].Users[0].Password != "do-not-show" {
		t.Fatal("password lost")
	}
	body["port"] = 9090
	code, _ = request(t, a, "PUT", "/api/listeners/"+lid, body)
	if code != 400 {
		t.Fatal("admin port collision allowed")
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{id}, "action": "delete"})
	if a.store.Snapshot().Listeners[0].Enabled {
		t.Fatal("orphan listener still enabled")
	}
}
func TestAPIHostOriginAndToken(t *testing.T) {
	a, _ := fixture(t, true)
	for _, test := range []struct{ host, origin string }{{"evil.example", ""}, {"127.0.0.1:9090", "https://evil.example"}} {
		r := httptest.NewRequest("GET", "http://"+test.host+"/api/bootstrap", nil)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("allowed %v %d", test, w.Code)
		}
	}
	a.opts.Token = "a-long-auth-token-over-twenty-four-chars"
	code, _ := request(t, a, "GET", "/api/bootstrap", nil)
	if code != 401 {
		t.Fatal(code)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:9090/api/bootstrap", nil)
	r.Header.Set("Authorization", "Bearer "+a.opts.Token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestSourceReconciliationProtectsOtherSources(t *testing.T) {
	s := model.DefaultState()
	s.Groups = []model.Group{{ID: "g1", Name: "one"}, {ID: "g2", Name: "two"}}
	s.Subscriptions = []model.Subscription{{ID: "s1", Name: "one", GroupIDs: []string{"g1"}}, {ID: "s2", Name: "two", GroupIDs: []string{"g2"}}}
	pr := importer.Parse("http://a:secret@example.com:80")
	if len(pr.Nodes) != 1 {
		t.Fatal(pr)
	}
	out := emptyImport()
	addParsed(&s, pr.Nodes, []string{"g1"}, "s1", &out)
	addParsed(&s, pr.Nodes, []string{"g2"}, "s2", &out)
	if len(s.Nodes) != 1 || len(s.Nodes[0].GroupIDs) != 2 {
		t.Fatal("source dedup failed")
	}
	if _, e := reconcile(&s, "s1", model.ParseResult{}, false); e == nil {
		t.Fatal("empty refresh unprotected")
	}
	if _, e := reconcile(&s, "s1", model.ParseResult{}, true); e != nil {
		t.Fatal(e)
	}
	if len(s.Nodes) != 1 || contains(s.Nodes[0].GroupIDs, "g1") || !contains(s.Nodes[0].SourceIDs, "s2") {
		t.Fatal("shared node damaged")
	}
	s.Nodes[0].Manual = true
	s.Nodes[0].ManualGroupIDs = []string{"g1"}
	if _, e := reconcile(&s, "s2", model.ParseResult{}, true); e != nil {
		t.Fatal(e)
	}
	if len(s.Nodes) != 1 || len(s.Nodes[0].SourceIDs) != 0 || !contains(s.Nodes[0].GroupIDs, "g1") {
		t.Fatal("manual node deleted")
	}
}
func TestJobResultsAreRealCoreResults(t *testing.T) {
	a, f := fixture(t, true)
	id := importOne(t, a)
	f.status.Running = true
	j, e := a.startJob([]string{id}, "latency")
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.After(2 * time.Second)
	for {
		got, _ := a.getJob(j.ID)
		if got.Status == "completed" {
			if got.Completed != 1 || !got.Results[0].OK || *got.Results[0].LatencyMS != 12 {
				t.Fatal(got)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("job timeout")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if a.store.Snapshot().Nodes[0].Status != "healthy" {
		t.Fatal("health not persisted")
	}
}
func TestPoolGroupSelectionAndUDP(t *testing.T) {
	s := model.DefaultState()
	s.Nodes = []model.Node{{ID: "a", Enabled: true, GroupIDs: []string{"g1", "g2"}, ParsedNode: model.ParsedNode{UDP: true}}, {ID: "b", Enabled: true, GroupIDs: []string{"g1"}}, {ID: "c", Enabled: false, GroupIDs: []string{"g1"}}}
	p := model.Pool{Enabled: true, GroupIDs: []string{"g1", "g2"}, GroupMode: "all"}
	ns := model.PoolMembers(s, p)
	if len(ns) != 1 || ns[0].ID != "a" {
		t.Fatal(ns)
	}
	p.GroupMode = "any"
	if len(model.PoolMembers(s, p)) != 2 {
		t.Fatal("union wrong")
	}
	p.UDP = true
	if len(model.PoolMembers(s, p)) != 1 {
		t.Fatal("UDP filtering wrong")
	}
	p.ExcludeNodeIDs = []string{"a"}
	if len(model.PoolMembers(s, p)) != 0 {
		t.Fatal("exclude precedence wrong")
	}
}

var _ http.Handler = (*http.ServeMux)(nil)

func TestUnsafeBackupRejectedWithoutPanic(t *testing.T) {
	a, _ := fixture(t, true)
	s := model.DefaultState()
	s.Nodes = []model.Node{{ID: "n", ParsedNode: model.ParsedNode{Name: "broken", Server: "example.com", Port: 80, Protocol: "http"}}}
	code, _ := request(t, a, "POST", "/api/import/backup", map[string]any{"state": s})
	if code != 400 {
		t.Fatal(code)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("broken backup committed")
	}
}
func TestFailedSubscriptionDoesNotPrune(t *testing.T) {
	a, _ := fixture(t, true)
	a.client = &http.Client{Timeout: time.Second}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	s := a.store.Snapshot()
	s.Subscriptions = []model.Subscription{{ID: "sub", Name: "protected", URL: server.URL, Enabled: true, IntervalMinutes: 60}}
	pr := importer.Parse("http://example.com:8080")
	out := emptyImport()
	addParsed(&s, pr.Nodes, nil, "sub", &out)
	if e := a.store.Save(s); e != nil {
		t.Fatal(e)
	}
	if _, e := a.refresh(context.Background(), "sub", false); e == nil {
		t.Fatal("failure not returned")
	}
	if len(a.store.Snapshot().Nodes) != 1 {
		t.Fatal("nodes deleted on failed fetch")
	}
}
func TestSchemaChecks(t *testing.T) {
	a, _ := fixture(t, true)
	for _, body := range []any{map[string]any{"name": "x", "unexpected": true}, map[string]any{"name": "x", "color": "url(javascript:bad)"}} {
		code, _ := request(t, a, "POST", "/api/groups", body)
		if code != 400 {
			t.Fatal(code)
		}
	}
}

func TestEditingEffectiveGroupsPreservesAttribution(t *testing.T) {
	n := model.Node{GroupIDs: []string{"g1", "g2"}, ManualGroupIDs: []string{}, SourceGroups: map[string][]string{"s1": {"g1"}, "s2": {"g2"}}}
	setNodeGroups(&n, []string{"g1", "g2", "g3"})
	if len(n.ManualGroupIDs) != 1 || n.ManualGroupIDs[0] != "g3" {
		t.Fatal(n.ManualGroupIDs)
	}
	delete(n.SourceGroups, "s1")
	groupsFor(&n)
	if contains(n.GroupIDs, "g1") {
		t.Fatal("inherited group became pinned")
	}
}
func TestDirtyRuntimeRejectsProbeJobs(t *testing.T) {
	a, f := fixture(t, true)
	id := importOne(t, a)
	f.applyError = errors.New("test bind failure")
	_, _ = request(t, a, "POST", "/api/groups", map[string]any{"name": "pending"})
	if _, e := a.startJob([]string{id}, "latency"); e == nil {
		t.Fatal("probed mismatched runtime")
	}
	if a.store.Snapshot().Nodes[0].Status != "unknown" {
		t.Fatal("incorrect health persisted")
	}
}

func TestManualStopNotUndoneByMutation(t *testing.T) {
	a, f := fixture(t, true)
	importOne(t, a)
	code, _ := request(t, a, "POST", "/api/runtime/stop", nil)
	if code != 200 {
		t.Fatal(code)
	}
	_, _ = request(t, a, "POST", "/api/groups", map[string]any{"name": "stored while stopped"})
	if f.Status().Running {
		t.Fatal("mutation undid intentional stop")
	}
	_, _ = request(t, a, "POST", "/api/runtime/apply", nil)
	if !f.Status().Running {
		t.Fatal("explicit apply did not restart")
	}
}
func TestBatchListenerRejectsHostnameBeforeAllocation(t *testing.T) {
	a, _ := fixture(t, true)
	id := importOne(t, a)
	code, _ := request(t, a, "POST", "/api/listeners/batch", map[string]any{"nodeIds": []string{id}, "protocol": "mixed", "host": "unresolvable.invalid"})
	if code != 400 {
		t.Fatal(code)
	}
	if len(a.store.Snapshot().Listeners) != 0 {
		t.Fatal("invalid allocation committed")
	}
}

func TestRefreshCannotOverwriteRestoredSourceGeneration(t *testing.T) {
	a, _ := fixture(t, true)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte("http://new.example.com:8080#stale"))
	}))
	defer server.Close()
	a.client = &http.Client{Timeout: 3 * time.Second}
	s := a.store.Snapshot()
	s.Subscriptions = []model.Subscription{{ID: "sub-generation", Name: "source", URL: server.URL, IntervalMinutes: 60, UpdatedAt: "old", Enabled: true}}
	parsed := importer.Parse("http://old.example.com:8080#original")
	out := emptyImport()
	addParsed(&s, parsed.Nodes, nil, "sub-generation", &out)
	if e := a.store.Save(s); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := a.refresh(context.Background(), "sub-generation", true); done <- e }()
	<-started
	restored := a.store.Snapshot()
	restored.Subscriptions[0].URL = "https://restored.example.com/sub"
	code, res := request(t, a, "POST", "/api/import/backup", map[string]any{"state": restored})
	if code != 200 {
		close(release)
		t.Fatal(res)
	}
	close(release)
	if e := <-done; e == nil {
		t.Fatal("stale response committed")
	}
	current := a.store.Snapshot()
	if len(current.Nodes) != 1 || current.Nodes[0].Server != "old.example.com" || current.Subscriptions[0].LastError != "" {
		t.Fatal("stale response damaged restored state")
	}
}
func TestBackupDecoderAcceptsExportableLargeState(t *testing.T) {
	state := model.DefaultState()
	state.Nodes = []model.Node{{ParsedNode: model.ParsedNode{Name: strings.Repeat("x", 13*1024*1024)}}}
	raw, _ := json.Marshal(map[string]any{"state": state})
	req := httptest.NewRequest("POST", "http://127.0.0.1:9090/api/import/backup", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	var body struct {
		State model.State `json:"state"`
	}
	if e := decode(httptest.NewRecorder(), req, &body); e != nil {
		t.Fatal("backup body >12MiB rejected before validation", e)
	}
	if len(body.State.Nodes[0].Name) != 13*1024*1024 {
		t.Fatal("large body truncated")
	}
}

func TestConfigurationChangeMidProbeDiscardsOldResult(t *testing.T) {
	a, f := fixture(t, true)
	id := importOne(t, a)
	f.probeStarted = make(chan struct{})
	f.probeRelease = make(chan struct{})
	j, e := a.startJob([]string{id}, "latency")
	if e != nil {
		t.Fatal(e)
	}
	<-f.probeStarted
	code, out := request(t, a, "PATCH", "/api/nodes/"+id, map[string]any{"enabled": false})
	if code != 200 {
		close(f.probeRelease)
		t.Fatal(out)
	}
	close(f.probeRelease)
	deadline := time.After(2 * time.Second)
	for {
		current, _ := a.getJob(j.ID)
		if current.Status == "completed" {
			if current.Results[0].OK {
				t.Fatal("stale probe success published")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("job timeout")
		case <-time.After(time.Millisecond):
		}
	}
	if a.store.Snapshot().Nodes[0].Status != "unknown" {
		t.Fatal("stale health overwrote desired node")
	}
}

func TestConfirmedEmptySubscriptionDeletesOrphans(t *testing.T) {
	a, _ := fixture(t, true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	a.client = &http.Client{Timeout: time.Second}
	s := a.store.Snapshot()
	s.Subscriptions = []model.Subscription{{ID: "empty-sub", Name: "empty", URL: server.URL, Enabled: true, IntervalMinutes: 60}}
	parsed := importer.Parse("http://example.com:8080")
	out := emptyImport()
	addParsed(&s, parsed.Nodes, nil, "empty-sub", &out)
	if e := a.store.Save(s); e != nil {
		t.Fatal(e)
	}
	if _, e := a.refresh(context.Background(), "empty-sub", false); e == nil {
		t.Fatal("empty source not protected")
	}
	if len(a.store.Snapshot().Nodes) != 1 {
		t.Fatal("unconfirmed source deleted nodes")
	}
	if _, e := a.refresh(context.Background(), "empty-sub", true); e != nil {
		t.Fatal(e)
	}
	if len(a.store.Snapshot().Nodes) != 0 {
		t.Fatal("confirmed deletion failed")
	}
}
func TestBackupRoundTripMaintainsCredentialsAndGroups(t *testing.T) {
	a, _ := fixture(t, true)
	id := importOne(t, a, "one", "two")
	code, out := request(t, a, "GET", "/api/export/backup", nil)
	if code != 200 {
		t.Fatal(out)
	}
	_, _ = request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{id}, "action": "delete"})
	code, res := request(t, a, "POST", "/api/import/backup", out)
	if code != 200 {
		t.Fatal(res)
	}
	s := a.store.Snapshot()
	if len(s.Nodes) != 1 || s.Nodes[0].Config["password"] != "secret" || len(s.Nodes[0].GroupIDs) != 2 {
		t.Fatal("backup round trip changed state")
	}
}
