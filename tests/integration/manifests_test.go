package integration

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentManifestsParse(t *testing.T) {
	for _, name := range []string{"compose.yaml", "compose.host.yaml", ".github/workflows/ci.yml", "pnpm-workspace.yaml"} {
		t.Run(name, func(t *testing.T) {
			b, e := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
			if e != nil {
				t.Fatal(e)
			}
			var doc map[string]any
			if e = yaml.Unmarshal(b, &doc); e != nil {
				t.Fatal(e)
			}
			if len(doc) == 0 {
				t.Fatal("empty manifest")
			}
		})
	}
}
