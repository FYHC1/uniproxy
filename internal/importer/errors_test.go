package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestErrorDetailCapPreservesValidURISiblings(t *testing.T) {
	for _, errors := range []int{0, maxIssues - 1, maxIssues, maxIssues + 1, 1000} {
		t.Run(fmt.Sprint(errors), func(t *testing.T) {
			content := "http://before.example:80#before\n" + strings.Repeat("unsupported://synthetic-invalid\n", errors) + "http://after.example:80#after\n"
			for _, input := range []string{content, base64.StdEncoding.EncodeToString([]byte(content))} {
				result := Parse(input)
				requireDialerResult(t, result, []string{"before", "after"}, errors)
				for i, problem := range result.Errors {
					if problem.Line != i+2 {
						t.Fatal("bounded URI diagnostics lost original line numbers")
					}
				}
				data, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var public map[string]any
				if err := json.Unmarshal(data, &public); err != nil {
					t.Fatal(err)
				}
				if public["totalErrors"] != float64(errors) || public["errorsTruncated"] != (errors > maxIssues) {
					t.Fatal("public diagnostics do not report the actual total and cap")
				}
			}
		})
	}
}

func TestErrorDetailCapPreservesDocumentSiblings(t *testing.T) {
	for _, format := range []string{"clash", "singbox"} {
		for _, errors := range []int{maxIssues, maxIssues + 1, 1000} {
			t.Run(fmt.Sprintf("%s/%d", format, errors), func(t *testing.T) {
				key, name, port := "proxies", "name", "port"
				if format == "singbox" {
					key, name, port = "outbounds", "tag", "server_port"
				}
				good := func(label string) string {
					return fmt.Sprintf("  - {%s: %s, type: http, server: good.example, %s: 80}\n", name, label, port)
				}
				content := key + ":\n" + good("before")
				for i := 0; i < errors; i++ {
					// Mix entry and normalization errors: neither kind may stop
					// the scan when the detail cap is reached.
					if i%2 == 0 {
						content += "  - not-an-object\n"
					} else {
						content += fmt.Sprintf("  - {type: http, server: bad.example, %s: 0}\n", port)
					}
				}
				content += good("after")
				result := Parse(content)
				requireDialerResult(t, result, []string{"before", "after"}, errors)
				for i, problem := range result.Errors {
					if problem.Line != i+3 {
						t.Fatal("bounded document diagnostics lost source ordering or line numbers")
					}
				}
			})
		}
	}
}

func TestErrorDetailCapPreservesForwardReferencedChain(t *testing.T) {
	const invalid = 168
	var content strings.Builder
	content.WriteString("proxies:\n")
	for i := 0; i < invalid; i++ {
		fmt.Fprintf(&content, "  - {name: invalid-%d, type: http, server: bad.example, port: 80, dialer-proxy: missing}\n", i)
	}
	content.WriteString("  - {name: exit, type: http, server: exit.example, port: 80, dialer-proxy: relay}\n")
	content.WriteString("  - {name: independent, type: http, server: good.example, port: 80}\n")
	content.WriteString("  - {name: relay, type: http, server: relay.example, port: 80}\n")
	result := Parse(content.String())
	requireDialerResult(t, result, []string{"exit", "independent", "relay"}, invalid)
	if result.Nodes[0].DialerFingerprint != Fingerprint(result.Nodes[2]) {
		t.Fatal("forward-reference relay after the error cap was not indexed")
	}
}

func TestIssueTotalsForDocumentFailures(t *testing.T) {
	for _, input := range []string{"", "proxies: [", "proxies: []\nproxy-providers: {}", strings.Repeat("x", MaxContentBytes+1)} {
		result := Parse(input)
		requireDialerResult(t, result, nil, 1)
	}
}
