package version

import (
	"strings"
	"testing"
)

func TestStringIncludesOnlyRealBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	defer func() { Version, Commit, Date = oldVersion, oldCommit, oldDate }()

	// A development build must not pretend to have a commit or date.
	Version, Commit, Date = "0.2.0", "dev", "unknown"
	if got := String(); got != "0.2.0" {
		t.Fatalf("development build string = %q", got)
	}
	// A release build reports all three, with a shortened commit.
	Version, Commit, Date = "0.2.0", "abcdef1234567890", "2026-10-20T08:00:00Z"
	got := String()
	for _, want := range []string{"0.2.0", "commit abcdef123456", "built 2026-10-20T08:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("release string %q missing %q", got, want)
		}
	}
	if UserAgent() != "UniProxy/0.2.0" {
		t.Fatalf("user agent = %q", UserAgent())
	}
}
