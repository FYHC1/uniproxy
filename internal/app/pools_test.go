package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

// Reserve a contiguous test-only TCP/UDP range simultaneously, then release it.
// No production listeners, databases, or core processes are involved.
//
// Two constraints shape this helper. Ephemeral allocation (":0") is not a usable
// starting point because on Windows the dynamic range overlaps Hyper-V/WinNAT
// reserved ranges, so a freshly assigned port often has no free neighbours. A
// fixed low range is stable per process but collides across packages, because
// "go test ./..." runs package test binaries in parallel. Randomized candidate
// bases inside a wide unprivileged band avoid both problems; the bind attempt
// itself remains the authority on availability.
func poolTestPortRange(t *testing.T, size int) int {
	t.Helper()
	if size < 1 {
		t.Fatalf("port range size must be positive")
	}
	// This band is disjoint from the core package's fixture band, so parallel
	// package test binaries cannot be handed the same port. It stays below the
	// Windows dynamic range (49152+) and above privileged ports.
	const low, high = 20000, 34000
	if size > high-low {
		t.Fatalf("requested port range of %d exceeds the test band", size)
	}
	spread := high - low - size
	for attempt := 0; attempt < 200; attempt++ {
		base := low + testPortOffset(spread, attempt)
		tcp := []net.Listener{}
		udp := []net.PacketConn{}
		ok := true
		for i := 0; i < size; i++ {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(base+i))
			l, err := net.Listen("tcp", addr)
			if err != nil {
				ok = false
				break
			}
			tcp = append(tcp, l)
			packet, err := net.ListenPacket("udp", addr)
			if err != nil {
				ok = false
				break
			}
			udp = append(udp, packet)
		}
		for _, l := range tcp {
			_ = l.Close()
		}
		for _, packet := range udp {
			_ = packet.Close()
		}
		if ok {
			return base
		}
	}
	t.Fatalf("could not obtain %d isolated TCP/UDP test ports", size)
	return 0
}

// testPortOffset spreads concurrently running package test binaries over the
// candidate band. crypto/rand keeps it unpredictable without touching global
// math/rand state, and the caller still verifies every port by binding it.
func testPortOffset(spread, attempt int) int {
	if spread <= 0 {
		return 0
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil {
		offset := int(binary.BigEndian.Uint32(b[:]) % uint32(spread+1))
		return (offset + attempt) % (spread + 1)
	}
	return (attempt * 7919) % (spread + 1)
}

func poolTestSetRange(t *testing.T, a *App, start, end int) {
	t.Helper()
	s := a.store.Snapshot()
	s.Settings.PortRangeStart, s.Settings.PortRangeEnd = start, end
	if err := a.store.Save(s); err != nil {
		t.Fatal(err)
	}
}

func poolTestCoreCounts(f *fakeCore) (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.validateCalls, f.applyCalls
}

func poolTestAssertCalls(t *testing.T, f *fakeCore, validates, applies int) {
	t.Helper()
	v, a := poolTestCoreCounts(f)
	if v != validates || a != applies {
		t.Fatalf("core calls validate=%d apply=%d; want %d/%d", v, a, validates, applies)
	}
}

func poolTestDecode[T any](t *testing.T, value any) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

type poolTestCreated struct {
	Pool     model.Pool     `json:"pool"`
	Listener model.Listener `json:"listener"`
}

func poolTestBody() map[string]any {
	return map[string]any{"pool": map[string]any{"name": "fixture pool"}, "listener": map[string]any{}}
}

func TestPoolWithListenerAtomicDefaultsAndRedaction(t *testing.T) {
	a, f := fixture(t, true)
	base := poolTestPortRange(t, 2)
	poolTestSetRange(t, a, base, base+1)
	id := importOne(t, a, "fixture group")
	for i, explicitZero := range []bool{false, true} {
		body := poolTestBody()
		pool := body["pool"].(map[string]any)
		pool["name"] = strings.Repeat("池", 160)
		pool["nodeIds"] = []string{id}
		listener := body["listener"].(map[string]any)
		listener["users"] = []model.Credential{{Username: "fixture-user", Password: "test-only-pool-password", HasPassword: true}}
		if explicitZero {
			listener["port"] = 0
		}
		before := a.store.Snapshot()
		v, ap := poolTestCoreCounts(f)
		code, out := request(t, a, "POST", "/api/pools/with-listener", body)
		if code != 200 {
			t.Fatalf("create returned %d: %v", code, out["error"])
		}
		after := a.store.Snapshot()
		result := poolTestDecode[poolTestCreated](t, out["data"])
		p, l := result.Pool, result.Listener
		if after.Revision != before.Revision+1 || len(after.Pools) != len(before.Pools)+1 || len(after.Listeners) != len(before.Listeners)+1 {
			t.Fatal("pool and listener were not saved as one revision")
		}
		poolTestAssertCalls(t, f, v+1, ap+1)
		if !reflect.DeepEqual(f.validated, after) || !reflect.DeepEqual(f.applyAttempt, after) || !reflect.DeepEqual(f.applied, after) {
			t.Fatal("core did not receive exactly the complete saved candidate")
		}
		if p.ID == "" || l.ID == "" || p.ID == l.ID || l.Kind != "pool" || l.TargetID != p.ID || !idPattern.MatchString(p.ID) || !idPattern.MatchString(l.ID) {
			t.Fatal("server-generated identity/binding mismatch")
		}
		if p.CreatedAt == "" || p.CreatedAt != p.UpdatedAt || l.CreatedAt != p.CreatedAt || l.UpdatedAt != p.UpdatedAt {
			t.Fatal("server timestamps were not generated together")
		}
		if p.Strategy != "url-test" || p.GroupMode != "any" || !p.Enabled || p.IntervalSeconds != 300 || p.TimeoutMS != 8000 || p.ToleranceMS != 50 {
			t.Fatal("pool defaults changed")
		}
		if l.Protocol != "mixed" || l.Host != "127.0.0.1" || l.UDP || !l.Enabled || l.Name != p.Name || l.Port != base+i {
			t.Fatal("safe listener defaults or automatic port selection changed")
		}
		if p.GroupIDs == nil || p.ExcludeNodeIDs == nil || p.ExcludeGroupIDs == nil || l.Users == nil {
			t.Fatal("public collections must be arrays")
		}
		if !reflect.DeepEqual(p, after.Pools[i]) || !reflect.DeepEqual(l, publicListener(after.Listeners[i])) {
			t.Fatal("response does not describe the saved pair")
		}
		if len(l.Users) != 1 || l.Users[0].Password != "" || !l.Users[0].HasPassword || after.Listeners[i].Users[0].Password != "test-only-pool-password" || after.Listeners[i].Users[0].HasPassword {
			t.Fatal("credentials were leaked, lost, or trusted from public metadata")
		}
		encoded, _ := json.Marshal(out)
		if bytes.Contains(encoded, []byte("test-only-pool-password")) || bytes.Contains(encoded, []byte(`"password"`)) {
			t.Fatal("create response exposed a password")
		}
		runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
		if runtime.Dirty || !runtime.Running || runtime.AppliedRevision != after.Revision || runtime.LastError != "" {
			t.Fatal("successful atomic apply was not reflected in runtime")
		}
	}
	_, public := request(t, a, "GET", "/api/bootstrap", nil)
	encoded, _ := json.Marshal(public)
	if bytes.Contains(encoded, []byte("test-only-pool-password")) {
		t.Fatal("bootstrap exposed combined listener credentials")
	}
}

func TestPoolWithListenerAtomicFailureAndPendingPaths(t *testing.T) {
	for _, test := range []struct {
		name          string
		code          int
		validateDelta int
		applyDelta    int
		saved         bool
	}{
		{"state-validation", 400, 0, 0, false},
		{"core-validation", 400, 1, 0, false},
		{"store-failure", 500, 1, 0, false},
		{"apply-failure", 200, 1, 1, true},
		{"core-unavailable", 200, 1, 1, true},
		{"manual-stop", 200, 1, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f := fixture(t, true)
			port := poolTestPortRange(t, 1)
			poolTestSetRange(t, a, port, port)
			importOne(t, a)
			body := poolTestBody()
			switch test.name {
			case "state-validation":
				body["pool"].(map[string]any)["strategy"] = "invalid"
			case "core-validation":
				f.validateError = errors.New("fixture validation failure")
			case "store-failure":
				if err := a.store.Close(); err != nil {
					t.Fatal(err)
				}
			case "apply-failure":
				f.applyError = errors.New("fixture apply failure")
			case "core-unavailable":
				f.status.Available, f.status.Running = false, false
			case "manual-stop":
				if code, _ := request(t, a, "POST", "/api/runtime/stop", nil); code != 200 {
					t.Fatal("could not stop fake core")
				}
			}
			before := a.store.Snapshot()
			live := store.Clone(f.applied)
			v, ap := poolTestCoreCounts(f)
			code, out := request(t, a, "POST", "/api/pools/with-listener", body)
			if code != test.code {
				t.Fatalf("status %d, want %d: %v", code, test.code, out["error"])
			}
			poolTestAssertCalls(t, f, v+test.validateDelta, ap+test.applyDelta)
			after := a.store.Snapshot()
			if !reflect.DeepEqual(store.Clone(f.applied), live) {
				t.Fatal("unsuccessful apply modified the old live state")
			}
			if !test.saved {
				if !reflect.DeepEqual(before, after) || out["data"] != nil {
					t.Fatal("failed validation/store persisted a partial pair")
				}
				return
			}
			if after.Revision != before.Revision+1 || len(after.Pools) != len(before.Pools)+1 || len(after.Listeners) != len(before.Listeners)+1 {
				t.Fatal("pending mutation failed to save the entire pair once")
			}
			result := poolTestDecode[poolTestCreated](t, out["data"])
			if result.Pool.ID != after.Pools[0].ID || result.Listener.TargetID != result.Pool.ID || result.Listener.ID != after.Listeners[0].ID {
				t.Fatal("pending response lost the saved pair")
			}
			runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
			if !runtime.Dirty || runtime.LastError == "" || runtime.AppliedRevision != before.Revision || runtime.AppliedRevision == after.Revision {
				t.Fatal("pending revision was reported as applied")
			}
			if test.name != "apply-failure" && runtime.Running {
				t.Fatal("unavailable/stopped core was reported as running")
			}
			if test.name == "apply-failure" && (!runtime.Running || !strings.Contains(runtime.LastError, "fixture apply failure")) {
				t.Fatal("apply failure did not retain old running revision and error")
			}
			if !reflect.DeepEqual(f.validated, after) || (test.applyDelta == 1 && !reflect.DeepEqual(f.applyAttempt, after)) {
				t.Fatal("pending core attempt did not include both objects")
			}
			if test.name == "manual-stop" {
				if code, _ := request(t, a, "POST", "/api/runtime/apply", nil); code != 200 || !reflect.DeepEqual(f.applied, after) || a.Runtime().Dirty {
					t.Fatal("explicit apply did not resume the whole pending pair")
				}
				poolTestAssertCalls(t, f, v+1, ap+1)
			}
		})
	}
}

func TestPoolWithListenerRejectsClientIdentityAndMalformedBodies(t *testing.T) {
	for _, field := range []string{"id", "createdAt", "updatedAt", "ID", "Kind"} {
		t.Run("pool-"+field, func(t *testing.T) {
			a, f := fixture(t, true)
			body := poolTestBody()
			body["pool"].(map[string]any)[field] = "client-override"
			before := a.store.Snapshot()
			if code, _ := request(t, a, "POST", "/api/pools/with-listener", body); code != 400 {
				t.Fatalf("pool identity injection accepted: %d", code)
			}
			if !reflect.DeepEqual(a.store.Snapshot(), before) {
				t.Fatal("identity rejection mutated configuration")
			}
			poolTestAssertCalls(t, f, 0, 0)
		})
	}
	for _, field := range []string{"id", "kind", "targetId", "createdAt", "updatedAt", "ID", "TargetID"} {
		t.Run("listener-"+field, func(t *testing.T) {
			a, f := fixture(t, true)
			body := poolTestBody()
			body["listener"].(map[string]any)[field] = "client-override"
			before := a.store.Snapshot()
			if code, _ := request(t, a, "POST", "/api/pools/with-listener", body); code != 400 {
				t.Fatalf("listener binding injection accepted: %d", code)
			}
			if !reflect.DeepEqual(a.store.Snapshot(), before) {
				t.Fatal("identity rejection mutated configuration")
			}
			poolTestAssertCalls(t, f, 0, 0)
		})
	}
	for _, body := range []any{
		map[string]any{},
		map[string]any{"pool": map[string]any{"name": "missing listener"}},
		map[string]any{"listener": map[string]any{}},
		map[string]any{"pool": nil, "listener": map[string]any{}},
		map[string]any{"pool": map[string]any{"name": "null listener"}, "listener": nil},
		map[string]any{"pool": []any{}, "listener": map[string]any{}},
		map[string]any{"pool": map[string]any{"name": "wrong listener"}, "listener": "invalid"},
		map[string]any{"pool": map[string]any{"name": "extra field"}, "listener": map[string]any{}, "id": "override"},
		map[string]any{"pool": map[string]any{"name": "wrong types", "nodeIds": "not an array"}, "listener": map[string]any{}},
		map[string]any{"pool": map[string]any{"name": "wrong types"}, "listener": map[string]any{"enabled": "true"}},
	} {
		a, f := fixture(t, true)
		before := a.store.Snapshot()
		if code, _ := request(t, a, "POST", "/api/pools/with-listener", body); code != 400 {
			t.Fatalf("invalid envelope accepted: %d", code)
		}
		if !reflect.DeepEqual(a.store.Snapshot(), before) {
			t.Fatal("invalid envelope mutated configuration")
		}
		poolTestAssertCalls(t, f, 0, 0)
	}
}

func TestPoolWithListenerConcurrentAllocationsSerialize(t *testing.T) {
	a, f := fixture(t, true)
	const count = 4
	base := poolTestPortRange(t, count)
	poolTestSetRange(t, a, base, base+count-1)
	before := a.store.Snapshot()
	handler := a.Handler()
	start := make(chan struct{})
	results := make(chan int, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			body := poolTestBody()
			body["pool"].(map[string]any)["name"] = fmt.Sprintf("concurrent %d", index)
			encoded, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9090/api/pools/with-listener", bytes.NewReader(encoded))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			results <- res.Code
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	for code := range results {
		if code != 200 {
			t.Fatalf("concurrent create failed: %d", code)
		}
	}
	after := a.store.Snapshot()
	if len(after.Pools) != count || len(after.Listeners) != count || after.Revision != before.Revision+count {
		t.Fatal("concurrent creations lost a pair or advanced extra revisions")
	}
	ports := map[int]bool{}
	pools := map[string]bool{}
	for _, pool := range after.Pools {
		pools[pool.ID] = true
	}
	for _, listener := range after.Listeners {
		if ports[listener.Port] || listener.Port < base || listener.Port >= base+count || !pools[listener.TargetID] || listener.Kind != "pool" {
			t.Fatal("concurrent allocation reused a port or lost atomic binding")
		}
		ports[listener.Port] = true
	}
	poolTestAssertCalls(t, f, count, count)
	if !reflect.DeepEqual(f.applied, after) {
		t.Fatal("last apply did not include all concurrent pairs")
	}
}

func TestPoolWithListenerCancelledRequestDoesNotSave(t *testing.T) {
	a, f := fixture(t, true)
	base := poolTestPortRange(t, 1)
	poolTestSetRange(t, a, base, base)
	for _, manual := range []bool{false, true} {
		body := poolTestBody()
		if manual {
			body["listener"].(map[string]any)["port"] = base
		}
		encoded, _ := json.Marshal(body)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9090/api/pools/with-listener", bytes.NewReader(encoded)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		before := a.store.Snapshot()
		a.Handler().ServeHTTP(res, req)
		if res.Code != 400 || !reflect.DeepEqual(a.store.Snapshot(), before) {
			t.Fatal("cancelled create saved a pair")
		}
		poolTestAssertCalls(t, f, 0, 0)
	}
}

func TestPoolMutationRoutesEnforceAdminAndOrigin(t *testing.T) {
	a, f, poolID, nodeID := poolMemberFixture(t)
	a.opts.Token = "fixture-administrator-token-for-tests"
	for _, route := range []struct{ method, path string }{{"POST", "/api/pools/with-listener"}, {"DELETE", "/api/pools/" + poolID + "/members/" + nodeID}} {
		for _, test := range []struct {
			name, auth, origin, site string
			status                   int
		}{
			{"no-token", "", "", "", 401},
			{"wrong-token", "Bearer wrong-fixture-token", "", "", 401},
			{"foreign-origin", "Bearer " + a.opts.Token, "https://untrusted.example.invalid", "", 403},
			{"cross-site", "Bearer " + a.opts.Token, "", "cross-site", 403},
		} {
			t.Run(route.method+"-"+test.name, func(t *testing.T) {
				encoded, _ := json.Marshal(poolTestBody())
				req := httptest.NewRequest(route.method, "http://127.0.0.1:9090"+route.path, bytes.NewReader(encoded))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", test.auth)
				req.Header.Set("Origin", test.origin)
				req.Header.Set("Sec-Fetch-Site", test.site)
				res := httptest.NewRecorder()
				before := a.store.Snapshot()
				v, ap := poolTestCoreCounts(f)
				a.Handler().ServeHTTP(res, req)
				if res.Code != test.status || !reflect.DeepEqual(a.store.Snapshot(), before) {
					t.Fatalf("unauthorized pool operation accepted: %d", res.Code)
				}
				poolTestAssertCalls(t, f, v, ap)
			})
		}
	}
}

func TestPoolWithListenerLegacyPoolRoutesRemainPoolOnly(t *testing.T) {
	a, f := fixture(t, true)
	code, out := request(t, a, "POST", "/api/pools", map[string]any{"name": "legacy pool"})
	if code != 200 {
		t.Fatal(out["error"])
	}
	pool := poolTestDecode[model.Pool](t, out["data"])
	created := pool.CreatedAt
	pool.Name, pool.Strategy = "edited legacy pool", "round-robin"
	pool.ID, pool.CreatedAt = "ignored-client-id", "ignored-client-time"
	code, out = request(t, a, "PUT", "/api/pools/"+a.store.Snapshot().Pools[0].ID, pool)
	if code != 200 {
		t.Fatal(out["error"])
	}
	s := a.store.Snapshot()
	if len(s.Pools) != 1 || len(s.Listeners) != 0 || s.Pools[0].Name != pool.Name || s.Pools[0].CreatedAt != created || s.Pools[0].ID == pool.ID || s.Revision != 2 {
		t.Fatal("legacy pool-only behavior changed")
	}
	poolTestAssertCalls(t, f, 2, 2)
}
