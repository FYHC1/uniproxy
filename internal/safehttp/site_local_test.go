package safehttp

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
)

func TestSiteLocalIPv6NeverDialsByDefault(t *testing.T) {
	for _, tt := range []struct {
		name    string
		literal bool
		doh     bool
		mixed   bool
	}{
		{name: "literal", literal: true},
		{name: "system single"},
		{name: "system mixed", mixed: true},
		{name: "DoH single", doh: true},
		{name: "DoH mixed", doh: true, mixed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := isolatedDependencies(t) // any numeric target dial fails this test
			raw := "http://source.example/PRIVATE_CANARY?token=SECRET_CANARY"
			if tt.literal {
				raw = "http://[fec0::1]/PRIVATE_CANARY?token=SECRET_CANARY"
			} else {
				deps.lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
					if tt.doh {
						return addresses("198.18.0.230"), nil
					}
					ips := addresses("fec0::1")
					if tt.mixed {
						ips = append(ips, addresses("8.8.4.4")...)
					}
					return ips, nil
				}
			}
			providers := 0
			if tt.doh {
				deps.dohTransport = func(dohProvider) http.RoundTripper {
					providers++
					return roundTripFunc(func(req *http.Request) (*http.Response, error) {
						kind, _ := strconv.Atoi(req.URL.Query().Get("type"))
						values := []string(nil)
						if kind == 28 {
							values = []string{"fec0::1"}
							if tt.mixed {
								values = append(values, "2606:4700:4700::1111")
							}
						}
						return response(dohJSON(dohMessage("source.example", kind, values...))), nil
					})
				}
			}
			_, err := Fetch(context.Background(), newClient(false, deps), raw)
			if tt.doh {
				assertKind(t, err, KindFakeIPFallback)
				assertKind(t, errors.Unwrap(err), KindPrivateAddress)
				if providers != 1 {
					t.Fatal("site-local DoH result tried another provider")
				}
			} else {
				assertKind(t, err, KindPrivateAddress)
			}
			assertSecretFree(t, err, "CANARY", "source.example", "fec0")
		})
	}
}
