package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/uniproxy/uniproxy/internal/importer"
	"github.com/uniproxy/uniproxy/internal/model"
)

const compatUUID = "11111111-1111-4111-8111-111111111111"
const compatSourceURL = "https://subscription.example.invalid/compat?token=" + diagnosticCanary

func compatTLSBody(t *testing.T, defaults bool, invalid bool) string {
	t.Helper()
	proxies := []map[string]any{}
	// Reproduce the reported shape: four plaintext and three TLS VMess entries.
	for i := 0; i < 7; i++ {
		obj := map[string]any{"name": "fixture", "type": "vmess", "server": "fixture.example.invalid", "port": 20000 + i, "uuid": compatUUID, "tls": i >= 4}
		// Explicit names in a Clash document must be unique.
		obj["name"] = []string{"plain-one", "plain-two", "plain-three", "plain-four", "tls-one", "tls-two", "tls-three"}[i]
		if defaults {
			obj["skip-cert-verify"] = false
		}
		if invalid && i == 0 {
			obj["skip-cert-verify"] = true
		}
		proxies = append(proxies, obj)
	}
	raw, err := json.Marshal(map[string]any{"proxies": proxies})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSubscriptionTLSDefaultPreviewCommitRefreshAndSafety(t *testing.T) {
	a, f := fixture(t, true)
	body := compatTLSBody(t, true, false)
	calls := 0
	a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) { calls++; return diagnosticResponse(r, body), nil })}
	before := a.store.Snapshot()
	activities := a.store.Activities()
	v, ap := poolTestCoreCounts(f)
	requestBody := map[string]any{"mode": "subscriptions", "content": compatSourceURL, "name": "TLS defaults source", "groupNames": []string{"TLS defaults group"}, "preview": true}
	code, out := request(t, a, "POST", "/api/import", requestBody)
	parsed := poolTestDecode[model.ParseResult](t, out)
	if code != 200 || parsed.TotalErrors != 0 || len(parsed.Nodes) != 7 {
		t.Fatal("complete TLS defaults source not previewed")
	}
	for _, n := range parsed.Nodes {
		if n.Config != nil {
			t.Fatal("preview leaked normalized config")
		}
	}
	if !reflect.DeepEqual(a.store.Snapshot(), before) || !reflect.DeepEqual(a.store.Activities(), activities) {
		t.Fatal("preview changed state")
	}
	poolTestAssertCalls(t, f, v, ap)
	diagnosticAssertSafe(t, out)
	requestBody["preview"] = false
	code, out = request(t, a, "POST", "/api/import", requestBody)
	imported := poolTestDecode[model.ImportResult](t, out["data"])
	if code != 200 || imported.Added != 7 || len(imported.SubscriptionIDs) != 1 || imported.TotalErrors != 0 {
		t.Fatal("complete TLS defaults subscription was not committed")
	}
	saved := a.store.Snapshot()
	sourceID := imported.SubscriptionIDs[0]
	ids := map[string]string{}
	for _, n := range saved.Nodes {
		ids[n.Fingerprint] = n.ID
		if len(n.SourceIDs) != 1 || n.SourceIDs[0] != sourceID || len(n.GroupIDs) != 1 {
			t.Fatal("source/group attribution lost")
		}
		if n.Config["tls"] == false {
			if _, ok := n.Config["skip-cert-verify"]; ok {
				t.Fatal("inert TLS flag survived persistence")
			}
		} else if n.Config["skip-cert-verify"] != false {
			t.Fatal("TLS certificate verification changed")
		}
	}
	// Removing template defaults must not churn the seven identities on refresh.
	body = compatTLSBody(t, false, false)
	code, out = request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{})
	refreshed := poolTestDecode[model.ImportResult](t, out["data"])
	if code != 200 || refreshed.Added != 0 || refreshed.Removed != 0 {
		t.Fatal("template cleanup changed subscription membership")
	}
	for _, n := range a.store.Snapshot().Nodes {
		if ids[n.Fingerprint] != n.ID {
			t.Fatal("refresh changed canonical node identity")
		}
	}
	diagnosticAssertSafe(t, out)
	// A real contradiction invalidates the entire source, even when forced.
	body = compatTLSBody(t, true, true)
	for _, force := range []bool{false, true} {
		before = a.store.Snapshot()
		v, ap = poolTestCoreCounts(f)
		code, out = request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{"force": force})
		if code != 400 {
			t.Fatal("unsafe partial TLS source was accepted")
		}
		diagnosticAssertFailedRefresh(t, a, f, sourceID, subscriptionParseError(importer.Parse(body), true), before, v, ap, out)
	}
	// A fresh partial import may not create a group or commit its valid siblings.
	requestBody["content"] = "https://other.example.invalid/compat?token=" + diagnosticCanary
	requestBody["groupNames"] = []string{"must-not-create"}
	requestBody["preview"] = true
	before = a.store.Snapshot()
	activities = a.store.Activities()
	v, ap = poolTestCoreCounts(f)
	code, out = request(t, a, "POST", "/api/import", requestBody)
	parsed = poolTestDecode[model.ParseResult](t, out)
	if code != 200 || len(parsed.Nodes) != 0 || parsed.TotalErrors != 1 {
		t.Fatal("partial source siblings leaked into preview")
	}
	requestBody["preview"] = false
	code, out = request(t, a, "POST", "/api/import", requestBody)
	if code != 400 || !reflect.DeepEqual(a.store.Snapshot(), before) || !reflect.DeepEqual(a.store.Activities(), activities) {
		t.Fatal("invalid source mutated state")
	}
	poolTestAssertCalls(t, f, v, ap)
	diagnosticAssertSafe(t, out)
	if calls != 7 {
		t.Fatal("unexpected number of bounded source fetches")
	}
}

func TestSubscriptionClientProviderAndDirectiveFailuresNeverPrune(t *testing.T) {
	for _, suffix := range []string{"\n[Remote Proxy]\nhttps://provider.example.invalid/private", "\n#!include https://provider.example.invalid/private", "\nother = trojan, other.example.invalid, 443, password=synthetic,underlying-proxy=missing", "\n[server_remote]\nhttps://provider.example.invalid/private", "\n[Proxy Group]\nexternal = select, policy-path=https://provider.example.invalid/nodes", "\n[Proxy Group]\n#!include https://provider.example.invalid/groups"} {
		a, f := fixture(t, true)
		sourceID := diagnosticSeedSource(t, a, diagnosticSourceURL)
		body := "[Proxy]\nfixture = trojan, fixture.example.invalid, 443, password=" + diagnosticNodeCanary + ",sni=front.example.invalid,skip-cert-verify=false" + suffix
		parsed := importer.Parse(body)
		if parsed.TotalErrors == 0 || len(parsed.Nodes) != 1 {
			t.Fatal("invalid client partial-source fixture")
		}
		a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) { return diagnosticResponse(r, body), nil })}
		for _, force := range []bool{false, true} {
			before := a.store.Snapshot()
			v, ap := poolTestCoreCounts(f)
			code, out := request(t, a, "POST", "/api/subscriptions/"+sourceID+"/refresh", map[string]any{"force": force})
			if code != 400 {
				t.Fatal("provider/directive partial source was force accepted")
			}
			diagnosticAssertFailedRefresh(t, a, f, sourceID, subscriptionParseError(parsed, true), before, v, ap, out)
		}
	}
}

func TestSubscriptionStaticClientFormatsPreviewAndCommit(t *testing.T) {
	bodies := []string{
		"[server_local]\ntrojan=fixture.example.invalid:443,password=" + diagnosticNodeCanary + ",over-tls=true,tls-host=front.example.invalid,tls-verification=true,fast-open=false,tag=fixture\n[filter_local]\nfinal,direct\n",
		"[Proxy]\nfixture = trojan, fixture.example.invalid, 443, password=" + diagnosticNodeCanary + ",sni=front.example.invalid,skip-cert-verify=false,tfo=false\n[Rule]\nFINAL,DIRECT\n",
		"[Proxy]\nfixture = trojan, fixture.example.invalid, 443, \"" + diagnosticNodeCanary + "\",tls-name=front.example.invalid,skip-cert-verify=false\n",
		"STATUS=synthetic status\ntrojan://" + diagnosticNodeCanary + "@fixture.example.invalid:443?sni=front.example.invalid&peer=front.example.invalid#fixture\n",
	}
	bodies = append(bodies, base64.StdEncoding.EncodeToString([]byte(bodies[0])))
	for i, body := range bodies {
		t.Run([]string{"qx", "surge", "loon", "status-uri", "base64-qx"}[i], func(t *testing.T) {
			a, f := fixture(t, true)
			a.client = &http.Client{Transport: diagnosticRoundTripper(func(r *http.Request) (*http.Response, error) { return diagnosticResponse(r, body), nil })}
			before := a.store.Snapshot()
			requestBody := map[string]any{"mode": "subscriptions", "content": compatSourceURL, "preview": true}
			code, out := request(t, a, "POST", "/api/import", requestBody)
			parsed := poolTestDecode[model.ParseResult](t, out)
			if code != 200 || parsed.TotalErrors != 0 || len(parsed.Nodes) != 1 || parsed.Nodes[0].Config != nil {
				t.Fatal("static client-format source not previewed safely")
			}
			if !reflect.DeepEqual(before, a.store.Snapshot()) {
				t.Fatal("static profile preview saved state")
			}
			diagnosticAssertSafe(t, out)
			v, ap := poolTestCoreCounts(f)
			requestBody["preview"] = false
			code, out = request(t, a, "POST", "/api/import", requestBody)
			result := poolTestDecode[model.ImportResult](t, out["data"])
			if code != 200 || result.Added != 1 || result.TotalErrors != 0 || len(result.SubscriptionIDs) != 1 {
				t.Fatal("static profile not committed")
			}
			saved := a.store.Snapshot()
			if len(saved.Nodes) != 1 || saved.Nodes[0].Config["password"] != diagnosticNodeCanary || saved.Nodes[0].Config["skip-cert-verify"] != false {
				t.Fatal("static profile credentials or verification changed")
			}
			if len(saved.Listeners) != 0 || len(saved.Pools) != 0 {
				t.Fatal("global profile rules/listeners were adopted")
			}
			poolTestAssertCalls(t, f, v+1, ap+1)
			diagnosticAssertSafe(t, out)
			diagnosticAssertSafe(t, a.store.Activities())
			if strings.Contains(saved.Subscriptions[0].Name, diagnosticCanary) {
				t.Fatal("default source name exposed token")
			}
		})
	}
}
