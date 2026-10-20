package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuiltCLIVersionAndPublicBindGuard(t *testing.T) {
	path := os.Getenv("UNIPROXY_TEST_APP_PATH")
	if path == "" {
		t.Skip("set UNIPROXY_TEST_APP_PATH")
	}
	for _, tc := range []struct {
		name   string
		args   []string
		ok     bool
		expect string
	}{{"version", []string{"-version"}, true, "UniProxy "}, {"syntax", []string{"-port", "invalid"}, false, "invalid command-line"}, {"unexpected-positional", []string{"oops", "-version"}, false, "unexpected positional"}, {"public-without-token", []string{"-host", "0.0.0.0"}, false, "requires UNIPROXY_ADMIN_TOKEN"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Defense in depth if validation ever regresses: never let this CLI
			// test open production data or discover/start a user-installed core.
			args := []string{"-data", filepath.Join(root, "data"), "-core", filepath.Join(root, "absent-core")}
			args = append(args, tc.args...)
			cmd := exec.CommandContext(ctx, path, args...)
			cmd.Dir = root
			cmd.WaitDelay = time.Second
			env := []string{}
			for _, entry := range os.Environ() {
				upper := strings.ToUpper(entry)
				if !strings.HasPrefix(upper, "UNIPROXY_") && !strings.HasPrefix(upper, "LOCALAPPDATA=") && !strings.HasPrefix(upper, "XDG_CACHE_HOME=") {
					env = append(env, entry)
				}
			}
			// Failed CLI invocations write diagnostics outside the data directory.
			cmd.Env = append(env, "LOCALAPPDATA="+root, "XDG_CACHE_HOME="+root)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			if ctx.Err() != nil {
				t.Fatal("CLI validation hung instead of returning")
			}
			if tc.name == "syntax" && cmd.ProcessState.ExitCode() != 2 {
				t.Fatal("invalid CLI syntax no longer exits with status 2")
			}
			if (err == nil) != tc.ok || !strings.Contains(out.String(), tc.expect) {
				t.Fatalf("%v %s", err, out.String())
			}
		})
	}
}
