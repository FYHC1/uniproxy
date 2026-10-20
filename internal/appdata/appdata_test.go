package appdata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDeps describes a platform without touching the real filesystem.
type fakeDeps struct {
	goos       string
	env        map[string]string
	home       string
	exe        string
	cwd        string
	existing   map[string]bool
	unwritable map[string]bool
}

func (f *fakeDeps) deps(created *[]string) Deps {
	has := func(dir string) bool { return f.existing[dir] }
	mkdir := func(dir string, _ os.FileMode) error {
		if f.unwritable[dir] {
			return errors.New("permission denied")
		}
		*created = append(*created, dir)
		return nil
	}
	return Deps{
		GOOS:        f.goos,
		Getenv:      func(k string) string { return f.env[k] },
		UserHomeDir: func() (string, error) { return f.home, nil },
		Executable:  func() (string, error) { return f.exe, nil },
		WorkingDir:  func() (string, error) { return f.cwd, nil },
		HasDatabase: has,
		MkdirAll:    mkdir,
		// Identity: filepath.Abs would resolve a Windows fixture against the
		// Linux CI working directory, which is not what precedence tests assert.
		Abs: func(p string) (string, error) { return p, nil },
	}
}

func TestWindowsUsesProgramData(t *testing.T) {
	f := &fakeDeps{goos: "windows", env: map[string]string{"ProgramData": `C:\ProgramData`}, existing: map[string]bool{}, unwritable: map[string]bool{}}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != `C:\ProgramData\UniProxy` || got.Source != SourceStandard {
		t.Fatalf("windows default = %+v", got)
	}
	if len(created) != 1 {
		t.Fatalf("standard directory not created: %v", created)
	}
}

func TestWindowsFallsBackToLocalAppDataWhenProgramDataUnwritable(t *testing.T) {
	standard := `C:\ProgramData\UniProxy`
	f := &fakeDeps{
		goos:       "windows",
		env:        map[string]string{"ProgramData": `C:\ProgramData`, "LOCALAPPDATA": `C:\Users\u\AppData\Local`},
		existing:   map[string]bool{},
		unwritable: map[string]bool{standard: true},
	}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != `C:\Users\u\AppData\Local\UniProxy` || got.Source != SourceFallback || got.Note == "" {
		t.Fatalf("fallback = %+v", got)
	}
}

func TestLinuxUsesFHSAndXDGFallback(t *testing.T) {
	f := &fakeDeps{goos: "linux", env: map[string]string{}, home: "/home/u", existing: map[string]bool{}, unwritable: map[string]bool{"/var/lib/uniproxy": true}}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != "/home/u/.local/share/uniproxy" || got.Source != SourceFallback {
		t.Fatalf("linux fallback = %+v", got)
	}
	// XDG_DATA_HOME takes precedence over the home directory.
	f.env["XDG_DATA_HOME"] = "/data/state"
	f.unwritable["/data/state/uniproxy"] = false
	got, err = Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != "/data/state/uniproxy" {
		t.Fatalf("xdg fallback = %+v", got)
	}
}

func TestExistingStandardDirectoryIsReusedWithoutLegacyAdoption(t *testing.T) {
	f := &fakeDeps{
		goos:       "windows",
		env:        map[string]string{"ProgramData": `C:\ProgramData`},
		cwd:        `C:\app`,
		existing:   map[string]bool{`C:\ProgramData\UniProxy`: true, `C:\app\data`: true},
		unwritable: map[string]bool{},
	}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceStandard || got.Dir != `C:\ProgramData\UniProxy` {
		t.Fatalf("standard should win over legacy: %+v", got)
	}
}

// The critical upgrade case: an existing installation keeps its data instead of
// silently starting with an empty database in the new default location.
func TestLegacyWorkingDirectoryIsAdopted(t *testing.T) {
	f := &fakeDeps{
		goos:       "windows",
		env:        map[string]string{"ProgramData": `C:\ProgramData`},
		cwd:        `C:\app\bin`,
		existing:   map[string]bool{`C:\app\bin\data`: true},
		unwritable: map[string]bool{},
	}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != `C:\app\bin\data` || got.Source != SourceLegacy || got.Note == "" {
		t.Fatalf("legacy adoption = %+v", got)
	}
	if len(created) != 0 {
		t.Fatalf("legacy adoption must not create a new store: %v", created)
	}
}

func TestLegacyExecutableDirectoryIsAdoptedWhenCWDHasNoData(t *testing.T) {
	f := &fakeDeps{
		goos:       "linux",
		env:        map[string]string{},
		home:       "/home/u",
		exe:        "/opt/uniproxy/bin/uniproxy",
		cwd:        "/tmp",
		existing:   map[string]bool{"/opt/uniproxy/bin/data": true},
		unwritable: map[string]bool{},
	}
	var created []string
	got, err := Resolve("", f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	// The fake host is Windows, so filepath.Dir keeps the POSIX fixture intact;
	// the candidate is joined with the target separator, giving a mixed path on
	// this host. Assert on the suffix and the decision, not on host separators.
	if got.Source != SourceLegacy || !strings.HasSuffix(got.Dir, "data") || !strings.Contains(got.Dir, "uniproxy") {
		t.Fatalf("exe-relative adoption = %+v", got)
	}
}

func TestExplicitDirectoryWinsAndIsCreated(t *testing.T) {
	f := &fakeDeps{goos: "windows", env: map[string]string{"ProgramData": `C:\ProgramData`}, existing: map[string]bool{}, unwritable: map[string]bool{}}
	var created []string
	got, err := Resolve(`D:\state\uniproxy`, f.deps(&created))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceConfigured || got.Dir != `D:\state\uniproxy` {
		t.Fatalf("explicit = %+v", got)
	}
	if len(created) != 1 || created[0] != `D:\state\uniproxy` {
		t.Fatalf("explicit directory not created: %v", created)
	}
}

func TestExplicitDirectoryFailureIsReported(t *testing.T) {
	f := &fakeDeps{goos: "windows", env: map[string]string{}, existing: map[string]bool{}, unwritable: map[string]bool{`D:\nope`: true}}
	var created []string
	if _, err := Resolve(`D:\nope`, f.deps(&created)); err == nil {
		t.Fatal("unwritable explicit directory must fail")
	}
}

func TestZeroLengthDatabaseIsNotAdopted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "uniproxy.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if hasDatabase(dir) {
		t.Fatal("a zero-length database is not existing data")
	}
	if err := os.WriteFile(filepath.Join(dir, "uniproxy.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasDatabase(dir) {
		t.Fatal("non-empty database must be detected")
	}
}

func TestNoPlatformDirectoryIsAnError(t *testing.T) {
	f := &fakeDeps{goos: "windows", env: map[string]string{}, existing: map[string]bool{}, unwritable: map[string]bool{}}
	var created []string
	if _, err := Resolve("", f.deps(&created)); err == nil {
		t.Fatal("missing ProgramData/SystemDrive must be reported, not guessed")
	}
}
