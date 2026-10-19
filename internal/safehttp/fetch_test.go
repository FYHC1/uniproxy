package safehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestBlockedAddresses(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::ffff:127.0.0.1", "fc00::1", "fec0::1", "feff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "198.18.0.1", "64:ff9b::7f00:1"} {
		if PublicIP(netip.MustParseAddr(s)) {
			t.Errorf("allowed %s", s)
		}
	}
	if !PublicIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("blocked public address")
	}
}
func TestFetchRestriction(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello")) }))
	defer s.Close()
	if _, e := Fetch(context.Background(), NewClient(false), s.URL); e == nil {
		t.Fatal("private fetch permitted")
	}
	if b, e := Fetch(context.Background(), NewClient(true), s.URL); e != nil || b != "hello" {
		t.Fatalf("%q %v", b, e)
	}
}
func TestURLValidationAndRedaction(t *testing.T) {
	for _, s := range []string{"file:///etc/passwd", "http://user:secret@example.com", "ftp://example.com", "https://x/#secret"} {
		if _, e := ValidateURL(s); e == nil {
			t.Fatal(s)
		}
	}
	r := RedactURL("https://example.com/private-token?key=secret")
	if strings.Contains(r, "token") || strings.Contains(r, "secret") {
		t.Fatal(r)
	}
}

func TestRedirectDoesNotLeakSubscriptionToken(t *testing.T) {
	referrer := "unset"
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		referrer = r.Header.Get("Referer")
		w.Write([]byte("body"))
	}))
	defer dest.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dest.URL, http.StatusFound) }))
	defer source.Close()
	if _, e := Fetch(context.Background(), NewClient(true), source.URL+"/PRIVATE?token=SECRET"); e != nil {
		t.Fatal(e)
	}
	if referrer != "" {
		t.Fatal("subscription credentials leaked via redirect referrer")
	}
}
