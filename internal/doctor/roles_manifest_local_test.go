package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDrift_LocalOnlyRoleDoesNotStaleRolesManifest(t *testing.T) {
	// Arrange: roles.json committed from the shared sources, then a machine-local
	// overlay adds a role that exists only on this machine.
	shared := baseConfig + "\n[role_manifest]\nenabled = true\n\n[[roles]]\nname = \"shared-role\"\n"
	dir := project(t, map[string]string{".ai-rulez/config.toml": shared, ".ai-rulez/rules/r.md": "# R\n\nbody\n"})
	generate(t, dir)
	overlayPath := filepath.Join(dir, ".ai-rulez", "config.local.toml")
	if err := os.WriteFile(overlayPath, []byte("[[roles]]\nname = \"local-only-role\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Act
	got := byCheck(run(t, dir), CheckDrift)

	// Assert
	for _, f := range got {
		if strings.Contains(f.Message, "roles.json") {
			t.Fatalf("roles.json reported as drifted because of a local role: %+v", got)
		}
	}
}
