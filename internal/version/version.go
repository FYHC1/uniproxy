// Package version is the single source of build identity.
//
// Release builds override these with -ldflags "-X ...Version=... -X ...Commit=...
// -X ...Date=..."; a plain `go build` keeps the source version but omits build
// metadata. Unreleased artifacts should explicitly stamp a development version.
package version

import "strings"

var (
	// Version is the released semantic version.
	Version = "0.2.3"
	// Commit is the short git commit the binary was built from.
	Commit = "dev"
	// Date is the build date in RFC3339 form.
	Date = "unknown"
)

// String renders a single line suitable for `-version` and logs.
func String() string {
	parts := []string{Version}
	if Commit != "" && Commit != "dev" {
		parts = append(parts, "commit "+shortCommit(Commit))
	}
	if Date != "" && Date != "unknown" {
		parts = append(parts, "built "+Date)
	}
	return strings.Join(parts, " · ")
}

// UserAgent identifies management-plane HTTP requests without leaking build
// internals or secrets.
func UserAgent() string { return "UniProxy/" + Version }

func shortCommit(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
