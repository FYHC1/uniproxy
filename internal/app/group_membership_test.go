package app

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
	"github.com/uniproxy/uniproxy/internal/store"
)

const membershipTargetGroup = "membership-target"
const membershipAddedGroup = "membership-added"
const membershipSourceA = "membership-source-a"
const membershipSourceB = "membership-source-b"
const membershipSourceAURL = "https://source-a.example.invalid/fixture-subscription"
const membershipSourceBURL = "https://source-b.example.invalid/fixture-subscription"
const membershipSourceAContent = "http://source-only.example.invalid:8080#source-only\nhttp://shared.example.invalid:8081#shared\nhttp://manual.example.invalid:8082#manual"
const membershipSourceBContent = "http://shared.example.invalid:8081#shared\nhttp://untouched.example.invalid:8083#untouched"

type membershipFixtureIDs struct {
	sourceOnly string
	shared     string
	manual     string
	untouched  string
}

func groupMembershipFixture(t *testing.T) (*App, *fakeCore, membershipFixtureIDs) {
	t.Helper()
	a, f := fixture(t, true)
	ids := membershipFixtureIDs{}
	_, err := a.mutate(context.Background(), "prepare isolated group membership fixture", func(s *model.State) (any, error) {
		s.Groups = []model.Group{
			{ID: membershipTargetGroup, Name: "managed group"},
			{ID: membershipAddedGroup, Name: "additional group"},
			{ID: "membership-other-a", Name: "source A other group"},
			{ID: "membership-other-b", Name: "source B other group"},
			{ID: "membership-manual-other", Name: "manual other group"},
		}
		s.Subscriptions = []model.Subscription{
			{ID: membershipSourceA, Name: "source A", URL: membershipSourceAURL, GroupIDs: []string{membershipTargetGroup, "membership-other-a"}, Enabled: true, IntervalMinutes: 60, UpdatedAt: model.Now()},
			{ID: membershipSourceB, Name: "source B", URL: membershipSourceBURL, GroupIDs: []string{membershipTargetGroup, "membership-other-b"}, Enabled: true, IntervalMinutes: 60, UpdatedAt: model.Now()},
		}
		out := emptyImport()
		for i, content := range []string{membershipSourceAContent, membershipSourceBContent} {
			pr := importer.Parse(content)
			if len(pr.Errors) != 0 {
				return nil, errors.New("invalid synthetic group subscription")
			}
			if err := addParsed(s, pr.Nodes, s.Subscriptions[i].GroupIDs, s.Subscriptions[i].ID, &out); err != nil {
				return nil, err
			}
		}
		pr := importer.Parse("http://manual.example.invalid:8082#manual")
		if err := addParsed(s, pr.Nodes, []string{membershipTargetGroup, "membership-manual-other"}, "", &out); err != nil {
			return nil, err
		}
		for i := range s.Nodes {
			n := &s.Nodes[i]
			switch n.Name {
			case "source-only":
				ids.sourceOnly = n.ID
			case "shared":
				ids.shared = n.ID
				// Manual group attribution on a source-owned node must not imply
				// manual retention of the global node when its sources disappear.
				n.ManualGroupIDs = []string{"membership-manual-other", membershipTargetGroup}
				groupsFor(n)
			case "manual":
				ids.manual = n.ID
			case "untouched":
				ids.untouched = n.ID
			}
		}
		s.Pools = []model.Pool{
			{ID: "membership-dynamic-pool", Name: "dynamic group pool", Enabled: true, Strategy: "round-robin", GroupMode: "any", GroupIDs: []string{membershipTargetGroup}, TestURL: s.Settings.TestURL, IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50},
			{ID: "membership-static-pool", Name: "static node pool", Enabled: true, Strategy: "round-robin", GroupMode: "all", NodeIDs: []string{ids.sourceOnly, ids.shared, ids.manual}, GroupIDs: []string{"membership-other-a", "membership-other-b"}, ExcludeGroupIDs: []string{membershipAddedGroup}, TestURL: s.Settings.TestURL, IntervalSeconds: 300, TimeoutMS: 8000, ToleranceMS: 50},
		}
		// Port values describe fake-core fixtures; no sockets are opened here.
		s.Listeners = []model.Listener{
			{ID: "membership-node-listener", Name: "source-owned node entrance", Kind: "node", TargetID: ids.sourceOnly, Host: "127.0.0.1", Port: 24801, Protocol: "mixed", Enabled: true},
			{ID: "membership-dynamic-listener", Name: "dynamic pool entrance", Kind: "pool", TargetID: "membership-dynamic-pool", Host: "127.0.0.1", Port: 24802, Protocol: "mixed", Enabled: true},
			{ID: "membership-static-listener", Name: "static pool entrance", Kind: "pool", TargetID: "membership-static-pool", Host: "127.0.0.1", Port: 24803, Protocol: "mixed", Enabled: true},
		}
		recount(s)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ids.sourceOnly == "" || ids.shared == "" || ids.manual == "" || ids.untouched == "" {
		t.Fatal("incomplete membership fixture")
	}
	return a, f, ids
}

func groupMembershipExpected(before model.State, ids, groups []string, action string, after model.State) model.State {
	expected := store.Clone(before)
	expected.Revision++
	for i := range expected.Nodes {
		n := &expected.Nodes[i]
		if !contains(ids, n.ID) {
			continue
		}
		if action == "add_groups" {
			n.ManualGroupIDs = union(n.ManualGroupIDs, groups)
		} else {
			n.ManualGroupIDs = without(n.ManualGroupIDs, groups)
			for source, refs := range n.SourceGroups {
				n.SourceGroups[source] = without(refs, groups)
			}
		}
		groupsFor(n)
		n.UpdatedAt = after.Nodes[nodeIndex(&after, n.ID)].UpdatedAt
	}
	return expected
}

func TestGroupMembershipBatchAddPreservesAttributionWithoutManualPromotion(t *testing.T) {
	a, f, ids := groupMembershipFixture(t)
	before := a.store.Snapshot()
	v, ap := poolTestCoreCounts(f)
	selected := []string{ids.sourceOnly, ids.shared, ids.manual, ids.shared}
	code, out := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": selected, "action": "add_groups", "groupIds": []string{membershipAddedGroup, membershipAddedGroup}})
	if code != 200 || out["data"].(map[string]any)["affected"] != float64(3) {
		t.Fatal("batch group add failed or counted duplicate node IDs")
	}
	after := a.store.Snapshot()
	if !reflect.DeepEqual(after, groupMembershipExpected(before, selected, []string{membershipAddedGroup}, "add_groups", after)) {
		t.Fatal("batch group add changed source ownership, unrelated nodes/pools/listeners, or advanced extra revisions")
	}
	for _, id := range []string{ids.sourceOnly, ids.shared} {
		n := after.Nodes[nodeIndex(&after, id)]
		old := before.Nodes[nodeIndex(&before, id)]
		if n.Manual || !contains(n.ManualGroupIDs, membershipAddedGroup) || !reflect.DeepEqual(n.SourceIDs, old.SourceIDs) || !reflect.DeepEqual(n.SourceGroups, old.SourceGroups) {
			t.Fatal("adding a group promoted source retention or damaged source attribution")
		}
	}
	if !after.Nodes[nodeIndex(&after, ids.manual)].Manual || !reflect.DeepEqual(after.Nodes[nodeIndex(&after, ids.untouched)], before.Nodes[nodeIndex(&before, ids.untouched)]) {
		t.Fatal("manual ownership/unselected node changed")
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
	if !reflect.DeepEqual(store.Clone(f.validated), after) || !reflect.DeepEqual(store.Clone(f.applied), after) {
		t.Fatal("batch add was not validated/applied once as a complete snapshot")
	}
	runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
	if runtime.Dirty || runtime.AppliedRevision != after.Revision {
		t.Fatal("group add runtime does not identify the applied revision")
	}
}

func TestGroupMembershipBatchRemoveOnlyGroupAndIsIdempotent(t *testing.T) {
	a, f, ids := groupMembershipFixture(t)
	selected := []string{ids.sourceOnly, ids.shared, ids.manual}
	baseline := a.store.Snapshot()
	v, ap := poolTestCoreCounts(f)
	for iteration := 0; iteration < 2; iteration++ {
		before := a.store.Snapshot()
		code, out := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": append(selected, ids.shared), "action": "remove_groups", "groupIds": []string{membershipTargetGroup}})
		if code != 200 || out["data"].(map[string]any)["affected"] != float64(3) {
			t.Fatal("group removal failed or counted duplicate IDs")
		}
		after := a.store.Snapshot()
		if !reflect.DeepEqual(after, groupMembershipExpected(before, selected, []string{membershipTargetGroup}, "remove_groups", after)) {
			t.Fatal("group removal changed global node retention, source configuration, pools/listeners, or unrelated memberships")
		}
		for _, id := range selected {
			n := after.Nodes[nodeIndex(&after, id)]
			old := baseline.Nodes[nodeIndex(&baseline, id)]
			if contains(n.GroupIDs, membershipTargetGroup) || contains(n.ManualGroupIDs, membershipTargetGroup) || !reflect.DeepEqual(n.SourceIDs, old.SourceIDs) || n.Manual != old.Manual || n.Enabled != old.Enabled {
				t.Fatal("removed group persisted or group removal changed node/source retention")
			}
			for _, refs := range n.SourceGroups {
				if contains(refs, membershipTargetGroup) {
					t.Fatal("group removal left inherited source references")
				}
			}
		}
		if !reflect.DeepEqual(poolMemberIDs(after, after.Pools[0]), []string{ids.untouched}) || !reflect.DeepEqual(poolMemberIDs(after, after.Pools[1]), []string{ids.sourceOnly, ids.shared, ids.manual}) {
			t.Fatal("group removal changed resolver semantics instead of only membership")
		}
		if !reflect.DeepEqual(after.Subscriptions, baseline.Subscriptions) || !reflect.DeepEqual(after.Listeners, baseline.Listeners) || !reflect.DeepEqual(after.Pools, baseline.Pools) {
			t.Fatal("group removal deleted/rebound shared objects")
		}
		poolTestAssertCalls(t, f, v+iteration+1, ap+iteration+1)
		if !reflect.DeepEqual(store.Clone(f.applied), after) {
			t.Fatal("group removal fake core did not receive the saved snapshot")
		}
	}
}

func TestGroupMembershipRemovedSourceGroupReturnsOnPatchOrReconcile(t *testing.T) {
	for _, restoration := range []string{"subscription-patch", "subscription-refresh"} {
		t.Run(restoration, func(t *testing.T) {
			a, f, ids := groupMembershipFixture(t)
			selected := []string{ids.sourceOnly, ids.shared, ids.manual}
			code, out := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": selected, "action": "remove_groups", "groupIds": []string{membershipTargetGroup}})
			if code != 200 {
				t.Fatal("fixture group removal failed")
			}
			before := a.store.Snapshot()
			v, ap := poolTestCoreCounts(f)
			if restoration == "subscription-patch" {
				code, out = request(t, a, "PATCH", "/api/subscriptions/"+membershipSourceA, map[string]any{"groupIds": before.Subscriptions[0].GroupIDs})
			} else {
				a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != membershipSourceAURL {
						return nil, errors.New("unexpected synthetic source request")
					}
					return diagnosticResponse(r, membershipSourceAContent), nil
				})}
				code, out = request(t, a, "POST", "/api/subscriptions/"+membershipSourceA+"/refresh", map[string]any{})
			}
			if code != 200 {
				t.Fatalf("source membership restoration failed: %v", out["error"])
			}
			after := a.store.Snapshot()
			if after.Revision != before.Revision+1 || !reflect.DeepEqual(after.Listeners, before.Listeners) || !reflect.DeepEqual(after.Pools, before.Pools) || !reflect.DeepEqual(after.Groups, before.Groups) || !reflect.DeepEqual(after.Subscriptions[1], before.Subscriptions[1]) {
				t.Fatal("source patch/refresh changed unrelated configuration")
			}
			for _, id := range selected {
				n := after.Nodes[nodeIndex(&after, id)]
				old := before.Nodes[nodeIndex(&before, id)]
				if !contains(n.GroupIDs, membershipTargetGroup) || contains(n.ManualGroupIDs, membershipTargetGroup) || !reflect.DeepEqual(n.SourceGroups[membershipSourceA], before.Subscriptions[0].GroupIDs) || !reflect.DeepEqual(n.SourceIDs, old.SourceIDs) || n.Manual != old.Manual {
					t.Fatal("source-derived group did not return with correct attribution")
				}
				if !reflect.DeepEqual(n.SourceGroups[membershipSourceB], old.SourceGroups[membershipSourceB]) {
					t.Fatal("refreshing source A restored unrelated source B attribution")
				}
			}
			if !reflect.DeepEqual(after.Nodes[nodeIndex(&after, ids.untouched)], before.Nodes[nodeIndex(&before, ids.untouched)]) || len(model.PoolMembers(after, after.Pools[0])) != 4 {
				t.Fatal("restoration touched an unrelated node or failed dynamic pool inclusion")
			}
			poolTestAssertCalls(t, f, v+1, ap+1)
		})
	}
}

func TestGroupMembershipManualGroupDoesNotRetainSourceOnlyOrphan(t *testing.T) {
	a, f, ids := groupMembershipFixture(t)
	code, _ := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{ids.sourceOnly}, "action": "add_groups", "groupIds": []string{membershipAddedGroup}})
	if code != 200 {
		t.Fatal("fixture group add failed")
	}
	before := a.store.Snapshot()
	if before.Nodes[nodeIndex(&before, ids.sourceOnly)].Manual {
		t.Fatal("group add changed global manual retention")
	}
	v, ap := poolTestCoreCounts(f)
	code, out := request(t, a, "DELETE", "/api/subscriptions/"+membershipSourceA, nil)
	if code != 200 {
		t.Fatal("source deletion failed")
	}
	after := a.store.Snapshot()
	if len(after.Nodes) != 3 || nodeIndex(&after, ids.sourceOnly) >= 0 || nodeIndex(&after, ids.shared) < 0 || nodeIndex(&after, ids.manual) < 0 || nodeIndex(&after, ids.untouched) < 0 || out["data"].(map[string]any)["removed"] != float64(1) {
		t.Fatal("group-only manual attribution retained an orphan or deleted shared/manual nodes")
	}
	if after.Listeners[0].Enabled || after.Listeners[0].TargetID != ids.sourceOnly || !reflect.DeepEqual(after.Listeners[1:], before.Listeners[1:]) {
		t.Fatal("source orphan cleanup rebound/deleted listeners")
	}
	poolTestAssertCalls(t, f, v+1, ap+1)
}

func TestGroupMembershipBatchValidationAndPendingAreAtomic(t *testing.T) {
	for _, action := range []string{"add_groups", "remove_groups"} {
		for _, mode := range []string{"core-validation", "store-failure", "apply-failure", "unavailable", "manual-stop"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				a, f, ids := groupMembershipFixture(t)
				group := membershipAddedGroup
				if action == "remove_groups" {
					group = membershipTargetGroup
				}
				switch mode {
				case "core-validation":
					f.validateError = errors.New("synthetic group validation failure")
				case "store-failure":
					if err := a.store.Close(); err != nil {
						t.Fatal(err)
					}
				case "apply-failure":
					f.applyError = errors.New("synthetic group apply failure")
				case "unavailable":
					f.status.Available, f.status.Running = false, false
				case "manual-stop":
					if code, _ := request(t, a, "POST", "/api/runtime/stop", nil); code != 200 {
						t.Fatal("fake core stop failed")
					}
				}
				before := a.store.Snapshot()
				live := store.Clone(f.applied)
				v, ap := poolTestCoreCounts(f)
				selected := []string{ids.sourceOnly, ids.shared, ids.manual}
				code, out := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": selected, "action": action, "groupIds": []string{group}})
				after := a.store.Snapshot()
				if !reflect.DeepEqual(store.Clone(f.applied), live) {
					t.Fatal("unsuccessful group apply changed the old live configuration")
				}
				if mode == "core-validation" || mode == "store-failure" {
					want := 400
					if mode == "store-failure" {
						want = 500
					}
					if code != want || !reflect.DeepEqual(after, before) {
						t.Fatal("failed group validation/store partially persisted a batch")
					}
					poolTestAssertCalls(t, f, v+1, ap)
					return
				}
				if code != 200 || !reflect.DeepEqual(after, groupMembershipExpected(before, selected, []string{group}, action, after)) {
					t.Fatal("pending group batch did not save exactly the intended membership change")
				}
				applyCount := ap + 1
				if mode == "manual-stop" {
					applyCount = ap
				}
				poolTestAssertCalls(t, f, v+1, applyCount)
				runtime := poolTestDecode[model.RuntimeStatus](t, out["runtime"])
				if !runtime.Dirty || runtime.LastError == "" || runtime.AppliedRevision != before.Revision || (mode != "apply-failure" && runtime.Running) {
					t.Fatal("pending group mutation was presented as live")
				}
			})
		}
	}
}

func TestGroupMembershipInvalidBatchDoesNotPartiallyModifyNodes(t *testing.T) {
	for _, action := range []string{"add_groups", "remove_groups"} {
		for _, invalid := range []string{"empty-selection", "mixed-missing-node", "wrong-group-type"} {
			t.Run(action+"/"+invalid, func(t *testing.T) {
				a, f, ids := groupMembershipFixture(t)
				before := a.store.Snapshot()
				v, ap := poolTestCoreCounts(f)
				body := map[string]any{"ids": []string{ids.shared}, "action": action, "groupIds": []string{membershipTargetGroup}}
				want := 400
				switch invalid {
				case "empty-selection":
					body["ids"] = []string{}
				case "mixed-missing-node":
					body["ids"] = []string{ids.shared, "deleted-node"}
					want = 404
				case "wrong-group-type":
					body["groupIds"] = "not an array"
				}
				code, _ := request(t, a, "POST", "/api/nodes/batch", body)
				if code != want || !reflect.DeepEqual(a.store.Snapshot(), before) {
					t.Fatal("invalid group batch partially mutated existing nodes")
				}
				poolTestAssertCalls(t, f, v, ap)
			})
		}
	}
	t.Run("add-missing-group", func(t *testing.T) {
		a, f, ids := groupMembershipFixture(t)
		before := a.store.Snapshot()
		v, ap := poolTestCoreCounts(f)
		code, _ := request(t, a, "POST", "/api/nodes/batch", map[string]any{"ids": []string{ids.shared, ids.manual}, "action": "add_groups", "groupIds": []string{membershipAddedGroup, "missing-group"}})
		if code != 400 || !reflect.DeepEqual(a.store.Snapshot(), before) {
			t.Fatal("missing group allowed partial valid-group addition")
		}
		poolTestAssertCalls(t, f, v, ap)
	})
}
