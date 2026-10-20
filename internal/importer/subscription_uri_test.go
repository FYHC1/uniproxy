package importer

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestURIQueryDiagnosticsAreDeterministic(t *testing.T) {
	for _, query := range []string{"sni=a&sni=a&peer=%FF", "%FF=a&sni=a&sni=a", "sni=a&sni=a&peer=%00", "%00=a&sni=a&sni=a"} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 1000; i++ {
				result := Parse("trojan://synthetic@example.invalid:443?" + query)
				if result.TotalErrors != 1 || len(result.Nodes) != 0 || result.Errors[0].Message != "invalid URI query encoding" {
					t.Fatal("mixed invalid query fields yielded an unstable error")
				}
			}
		})
	}
}

func TestEquivalentSNIAliasesAreNotConflicts(t *testing.T) {
	base := "trojan://synthetic-password@example.invalid:443?allowInsecure=1&sni=front.example.invalid"
	want := parseOne(t, base)
	for _, suffix := range []string{"&peer=front.example.invalid", "&serverName=front.example.invalid&servername=front.example.invalid"} {
		got := parseOne(t, base+suffix)
		assertSameTLSIdentity(t, want, got)
	}
	for _, suffix := range []string{"&peer=other.example.invalid", "&peer=", "&sni=front.example.invalid", "&serverName=other.example.invalid"} {
		requireInvalid(t, base+suffix)
	}
	// This exception is SNI-only, not a precedence policy for every query option.
	requireInvalid(t, "vless://"+testUUID+"@example.invalid:443?security=tls&allowInsecure=1&insecure=1")
	requireInvalid(t, "vless://"+testUUID+"@example.invalid:443?type=ws&network=ws")
	// Equal aliases cannot turn invalid SNI into a valid hostname.
	for _, sni := range []string{"contact@example.invalid", "*.example.invalid", "bad/host", "front.example.invalid:443", " "} {
		requireInvalid(t, "trojan://p@example.invalid:443?sni="+url.QueryEscape(sni)+"&peer="+url.QueryEscape(sni))
	}
}

func TestSubscriptionStatusCannotStealYAMLBlockScalars(t *testing.T) {
	for _, indicator := range []string{"|", ">", "|-", ">+", "|2-"} {
		body := "STATUS=note: " + indicator + "\n  trojan://synthetic@example.invalid:443\n"
		for _, input := range []string{body, base64.StdEncoding.EncodeToString([]byte(body))} {
			result := Parse(input)
			if len(result.Nodes) != 0 || result.TotalErrors == 0 {
				t.Fatal("YAML scalar contents were reclassified as standalone proxies")
			}
		}
		body += "proxies:\n - {name: real, type: http, server: actual.example.invalid, port: 8080}\n"
		for _, input := range []string{body, base64.StdEncoding.EncodeToString([]byte(body))} {
			node := parseOne(t, input)
			if node.Name != "real" || node.Protocol != "http" {
				t.Fatal("STATUS metadata diverted a genuine Clash document")
			}
		}
	}
}

func TestSubscriptionStatusHeaderKeepsNodeAndErrorLimits(t *testing.T) {
	content := "STATUS=synthetic status\n" + strings.Repeat("http://example.invalid:8080\n", MaxNodes)
	result := Parse(content)
	if result.TotalErrors != 0 || len(result.Nodes) != MaxNodes {
		t.Fatal("status metadata incorrectly consumed a node slot")
	}
	result = Parse(content + "http://example.invalid:8081\n")
	if len(result.Nodes) != MaxNodes || result.TotalErrors != 1 || result.Errors[0].Line != MaxNodes+2 {
		t.Fatal("status metadata bypassed node limit or shifted source lines")
	}
	result = Parse("STATUS=synthetic status\n" + strings.Repeat("unsupported://synthetic\n", 150) + "http://example.invalid:8080")
	if result.TotalErrors != 150 || len(result.Errors) != 128 || !result.ErrorsTruncated || len(result.Nodes) != 1 {
		t.Fatal("status metadata changed error cap or dropped a later valid node")
	}
}

func TestSubscriptionStatusHeaderIsMetadataOnly(t *testing.T) {
	uri := "trojan://synthetic-password@example.invalid:443?sni=front.example.invalid&peer=front.example.invalid#fixture"
	baseline := parseOne(t, uri)
	plain := "\ufeff# comment\r\nSTATUS=synthetic-account-status\r\n" + uri + "\r\n"
	for i, input := range []string{plain, base64.StdEncoding.EncodeToString([]byte(plain)), base64.RawURLEncoding.EncodeToString([]byte(plain))} {
		t.Run(fmt.Sprint(i), func(t *testing.T) { assertSameTLSIdentity(t, baseline, parseOne(t, input)) })
	}
	for _, status := range []string{"quota: 1", "vmess", "trojan", "status:"} {
		assertSameTLSIdentity(t, baseline, parseOne(t, "STATUS="+status+"\n"+uri))
	}
	// Never turn a non-proxy or malformed source into a successful empty refresh.
	for _, input := range []string{"STATUS=metadata only", "STATUS=" + strings.Repeat("x", 1025) + "\n" + uri, "STATUS=bad\x00status\n" + uri, "STATUS=one\nSTATUS=two\n" + uri, uri + "\nSTATUS=late", "UNKNOWN=not-metadata\n" + uri} {
		result := Parse(input)
		if result.TotalErrors == 0 {
			t.Fatal("unrecognized/misplaced metadata was silently skipped")
		}
		for _, issue := range result.Errors {
			if strings.Contains(issue.Message, "metadata") || strings.Contains(issue.Message, "synthetic-account-status") {
				t.Fatal("status value leaked in diagnostics")
			}
		}
	}
	result := Parse("STATUS=synthetic-account-status\nunsupported://secret\n" + uri)
	if result.TotalErrors != 1 || len(result.Nodes) != 1 || result.Errors[0].Line != 2 {
		t.Fatal("status header changed source line/error accounting")
	}
}
