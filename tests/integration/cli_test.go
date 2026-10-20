package integration

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
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
	}{{"version", []string{"-version"}, true, "UniProxy "}, {"unexpected-positional", []string{"oops", "-version"}, false, "unexpected positional"}, {"public-without-token", []string{"-host", "0.0.0.0"}, false, "requires UNIPROXY_ADMIN_TOKEN"}} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(path, tc.args...)
			env := []string{}
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "UNIPROXY_") {
					env = append(env, entry)
				}
			}
			cmd.Env = env
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			e := cmd.Run()
			if (e == nil) != tc.ok || !strings.Contains(out.String(), tc.expect) {
				t.Fatalf("%v %s", e, out.String())
			}
		})
	}
}
