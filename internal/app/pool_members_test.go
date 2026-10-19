package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
	"gopkg.in/yaml.v3"
)

func poolMemberFixture(t *testing.T) (*App, *fakeCore, string, string) {
	t.Helper()
	a, f := fixture(t, true)
	code, out := request(t, a, "POST", "/api/import", map[string]any{"mode": "text", "content": "socks5://127.0.0.1:1080?udp=true#one\nsocks5://127.0.0.1:1081?udp=true#two\nsocks5://127.0.0.1:1082?udp=true#three", "groupNames": []string{"dynamic", "excluded group"}})
	if code != 200 {
		t.Fatal(out["error"])
	}
	s := a.store.Snapshot()
	group, excludedGroup := s.Groups[0].ID, s.Groups[1].ID
	s.Subscriptions = []model.Subscription{{ID: "fixture-source", Name: "isolated source", URL: "https://source.example.invalid/sub", GroupIDs: []string{group}, Enabled: true, IntervalMinutes: 60}}
	for i := range s.Nodes {
		s.Nodes[i].ManualGroupIDs = []string{group}
		s.Nodes[i].SourceIDs = []string{"fixture-source"}
		s.Nodes[i].SourceGroups = map[string][]string{"fixture-source": {group}}
		groupsFor(&s.Nodes[i])
	}
	recount(&s)
	if err := a.store.Save(s); err != nil {
		t.Fatal(err)
	}
	code, out = request(t, a, "POST", "/api/pools", map[string]any{"name": "member target", "nodeIds": []string{s.Nodes[0].ID, s.Nodes[0].ID}, "groupIds": []string{group}, "excludeNodeIds": []string{s.Nodes[2].ID}, "excludeGroupIds": []string{excludedGroup}})
	if code != 200 {
		t.Fatal(out["error"])
	}
	pool := poolTestDecode[model.Pool](t, out["data"])
	code, out = request(t, a, "POST", "/api/pools", map[string]any{"name": "unaffected pool", "nodeIds": []string{s.Nodes[0].ID}, "groupIds": []string{group}})
	if code != 200 {
		t.Fatal(out["error"])
	}
	other := poolTestDecode[model.Pool](t, out["data"])
	base := poolTestPortRange(t, 3)
	for i, binding := range []struct{ kind, target string }{{"node", s.Nodes[0].ID}, {"pool", pool.ID}, {"pool", other.ID}} {
		code, out = request(t, a, "POST", "/api/listeners", map[string]any{"name": "fixture entrance", "kind": binding.kind, "targetId": binding.target, "port": base + i})
		if code != 200 {
			t.Fatal(out["error"])
		}
	}
	return a, f, pool.ID, s.Nodes[0].ID
}

func poolMemberIDs(s model.State, p model.Pool) []string {
	ids := []string{}
	for _, n := range model.PoolMembers(s, p) {
		ids = append(ids, n.ID)
	}
	return ids
}

func poolMemberExpected(before model.State, poolID, nodeID, updated string) model.State {
	expected := store.Clone(before)
	i := poolIndex(&expected, poolID)
	p := &expected.Pools[i]
	p.NodeIDs = without(p.NodeIDs, []string{nodeID})
	p.ExcludeNodeIDs = union(p.ExcludeNodeIDs, []string{nodeID})
	p.UpdatedAt = updated
	expected.Revision++
	return expected
}

func TestPoolMemberRemovalLocalIdempotentAndReinclude(t *testing.T) {
	a, f, poolID, nodeID := poolMemberFixture(t)
	before := a.store.Snapshot()
	if len(model.PoolMembers(before, before.Pools[0])) != 2 {
		t.Fatal("fixture should include manual and group-selected members")
	}
	originalOtherMembers := poolMemberIDs(before, before.Pools[1])
	v, ap := poolTestCoreCounts(f)
	path := "/api/pools/" + poolID + "/members/" + nodeID
	for iteration := 0; iteration < 2; iteration++ {
		prior := a.store.Snapshot()
		code, out := request(t, a, "DELETE", path, nil)
		if code != 200 {
			t.Fatalf("member DELETE failed: %v", out["error"])
		}
		after := a.store.Snapshot()
		pool := poolTestDecode[model.Pool](t, out["data"])
		if pool.ID != poolID || !reflect.DeepEqual(pool, after.Pools[0]) || pool.UpdatedAt == prior.Pools[0].UpdatedAt {
			t.Fatal("member response is not the updated Pool")
		}
		expected := poolMemberExpected(prior, poolID, nodeID, pool.UpdatedAt)
		if !reflect.DeepEqual(after, expected) {
			t.Fatal("member deletion changed unrelated nodes/groups/sources/pools/listeners/settings")
		}
		if contains(pool.NodeIDs, nodeID) || !reflect.DeepEqual(pool.ExcludeNodeIDs, []string{before.Nodes[2].ID, nodeID}) {
			t.Fatal("manual references or exclusion union are not idempotent")
		}
		members := poolMemberIDs(after, pool)
		if !reflect.DeepEqual(members, []string{before.Nodes[1].ID}) || !reflect.DeepEqual(poolMemberIDs(after, after.Pools[1]), originalOtherMembers) {
			t.Fatal("group-selected member reappeared or another pool was affected")
		}
		poolTestAssertCalls(t, f, v+iteration+1, ap+iteration+1)
		if !reflect.DeepEqual(f.validated, after) || !reflect.DeepEqual(f.applied, after) {
			t.Fatal("member removal was not validated/applied once as a whole state")
		}
		runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
		if runtime.Dirty || runtime.AppliedRevision != after.Revision {
			t.Fatal("member removal response has stale runtime")
		}
	}

	// Full Pool PUT is still the deliberate reinclude path. The existing dynamic
	// union resolver remains authoritative; no group selection is erased.
	pool := a.store.Snapshot().Pools[0]
	pool.ExcludeNodeIDs = without(pool.ExcludeNodeIDs, []string{nodeID})
	code, out := request(t, a, "PUT", "/api/pools/"+poolID, pool)
	if code != 200 {
		t.Fatal(out["error"])
	}
	after := a.store.Snapshot()
	if !contains(poolMemberIDs(after, after.Pools[0]), nodeID) || contains(after.Pools[0].NodeIDs, nodeID) {
		t.Fatal("clearing exclusion did not restore dynamic group membership")
	}
	if !reflect.DeepEqual(after.Nodes, before.Nodes) || !reflect.DeepEqual(after.Listeners, before.Listeners) || !reflect.DeepEqual(after.Pools[1], before.Pools[1]) {
		t.Fatal("reinclusion touched shared objects")
	}

	// Group-only removal must exclude even when there is no manual reference.
	code, out = request(t, a, "DELETE", path, nil)
	if code != 200 || contains(poolMemberIDs(a.store.Snapshot(), a.store.Snapshot().Pools[0]), nodeID) {
		t.Fatal("group-only member removal failed", out["error"])
	}
}

func TestPoolMemberRemovalAtomicValidationAndPendingPaths(t *testing.T) {
	for _, test := range []struct {
		name                      string
		code                      int
		validateDelta, applyDelta int
		saved                     bool
	}{
		{"core-validation", 400, 1, 0, false},
		{"store-failure", 500, 1, 0, false},
		{"apply-failure", 200, 1, 1, true},
		{"core-unavailable", 200, 1, 1, true},
		{"manual-stop", 200, 1, 0, true},
		{"missing-pool", 404, 0, 0, false},
		{"missing-node", 404, 0, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, f, poolID, nodeID := poolMemberFixture(t)
			path := "/api/pools/" + poolID + "/members/" + nodeID
			switch test.name {
			case "core-validation":
				f.validateError = errors.New("fixture removal validation failure")
			case "store-failure":
				if err := a.store.Close(); err != nil {
					t.Fatal(err)
				}
			case "apply-failure":
				f.applyError = errors.New("fixture removal apply failure")
			case "core-unavailable":
				f.status.Available, f.status.Running = false, false
			case "manual-stop":
				if code, _ := request(t, a, "POST", "/api/runtime/stop", nil); code != 200 {
					t.Fatal("could not stop fake core")
				}
			case "missing-pool":
				path = "/api/pools/missing/members/" + nodeID
			case "missing-node":
				path = "/api/pools/" + poolID + "/members/missing"
			}
			before := a.store.Snapshot()
			live := store.Clone(f.applied)
			v, ap := poolTestCoreCounts(f)
			code, out := request(t, a, "DELETE", path, nil)
			if code != test.code {
				t.Fatalf("status %d, want %d: %v", code, test.code, out["error"])
			}
			poolTestAssertCalls(t, f, v+test.validateDelta, ap+test.applyDelta)
			after := a.store.Snapshot()
			if !reflect.DeepEqual(store.Clone(f.applied), live) {
				t.Fatal("removal failure changed the old live state")
			}
			if !test.saved {
				if !reflect.DeepEqual(after, before) {
					t.Fatal("failed removal partially changed configuration")
				}
				return
			}
			pool := poolTestDecode[model.Pool](t, out["data"])
			if !reflect.DeepEqual(pool, after.Pools[0]) || !reflect.DeepEqual(after, poolMemberExpected(before, poolID, nodeID, pool.UpdatedAt)) {
				t.Fatal("pending removal affected unrelated configuration or lost exclusion")
			}
			runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
			if !runtime.Dirty || runtime.LastError == "" || runtime.AppliedRevision != before.Revision {
				t.Fatal("pending removal was incorrectly reported as live")
			}
		})
	}
}

func TestPoolMemberRemovalEmptyPoolCompilesToReject(t *testing.T) {
	a, _, poolID, nodeID := poolMemberFixture(t)
	for _, id := range []string{nodeID, a.store.Snapshot().Nodes[1].ID} {
		if code, out := request(t, a, "DELETE", "/api/pools/"+poolID+"/members/"+id, nil); code != 200 {
			t.Fatal(out["error"])
		}
	}
	s := a.store.Snapshot()
	if len(model.PoolMembers(s, s.Pools[0])) != 0 || len(s.Nodes) != 3 || !s.Nodes[0].Enabled {
		t.Fatal("empty pool removal deleted or disabled global nodes")
	}
	// Build is pure offline YAML compilation; no real core process is created.
	data, err := core.Build(s, core.BuildOptions{ControllerAddress: "127.0.0.1:1", ControllerSecret: "fixture-controller", ProbeAddress: "127.0.0.1:2", ProbeSecret: "fixture-probe"})
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Groups []struct {
			Name     string   `yaml:"name"`
			Proxies  []string `yaml:"proxies"`
			Fallback string   `yaml:"empty-fallback"`
		} `yaml:"proxy-groups"`
		Listeners []struct {
			Name  string `yaml:"name"`
			Proxy string `yaml:"proxy"`
		} `yaml:"listeners"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range document.Groups {
		if group.Name == "p-"+poolID {
			found = true
			if !reflect.DeepEqual(group.Proxies, []string{"REJECT"}) || group.Fallback != "REJECT" {
				t.Fatal("empty pool allows direct fallback")
			}
		}
	}
	if !found || !reflect.DeepEqual(document.Rules, []string{"MATCH,REJECT"}) {
		t.Fatal("empty pool reject group/global reject rule missing")
	}
	for _, expected := range []struct{ listenerID, target string }{{s.Listeners[0].ID, "n-" + nodeID}, {s.Listeners[1].ID, "p-" + poolID}, {s.Listeners[2].ID, "p-" + s.Pools[1].ID}} {
		found := false
		for _, listener := range document.Listeners {
			if listener.Name == "l-"+expected.listenerID {
				found = true
				if listener.Proxy != expected.target {
					t.Fatal("member deletion changed an unrelated listener binding")
				}
			}
		}
		if !found {
			t.Fatal("member deletion removed an existing listener")
		}
	}
}
