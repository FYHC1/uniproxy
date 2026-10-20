package integration

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentManifestsParse(t *testing.T) {
	for _, name := range []string{"compose.yaml", "compose.host.yaml", ".github/workflows/ci.yml", ".github/workflows/release.yml", "pnpm-workspace.yaml"} {
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

func TestPackagingUsesOnlyIntegratedCoreInstaller(t *testing.T) {
	for _, name := range []string{"Dockerfile", "scripts/build-binaries.mjs", "scripts/build.sh", "scripts/build.ps1", ".github/workflows/release.yml", "package.json"} {
		b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "cmd/core-install") || strings.Contains(string(b), "bin/core-install") || strings.Contains(string(b), "dist/core-install-") || strings.Contains(string(b), "core:install") || strings.Contains(string(b), "out/core-install") {
			t.Errorf("%s still depends on the standalone installer", name)
		}
	}
}
