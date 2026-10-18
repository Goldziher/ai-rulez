package cli

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDuplicateSkillWarningHonoursAllowOverrides: a skill defined at the root and
// in a domain is collapsed with a warning on generate and an AR703 on validate, unless
// lint.allow_overrides lists the name.
func TestDuplicateSkillWarningHonoursAllowOverrides(t *testing.T) {
	tests := []struct {
		name     string
		lint     string
		wantWarn bool
	}{
		{"no allow list warns", "", true},
		{"bare name silences", "\n[lint]\nallow_overrides = [\"dup\"]\n", false},
		{"name scoped to the domain silences", "\n[lint]\nallow_overrides = [\"backend/dup\"]\n", false},
		{"another name still warns", "\n[lint]\nallow_overrides = [\"other\"]\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(testutil.CleanupTestBinary)
			// Arrange
			dir := testutil.CreateTempDir(t)
			cfg := filepath.Join(dir, ".ai-rulez")
			putFile(t, cfg, "config.toml", "version = \"5.0\"\nname = \"dup\"\npresets = [\"claude\"]\n"+tt.lint)
			putFile(t, filepath.Join(cfg, "skills", "dup"), "SKILL.md", "---\nname: dup\ndescription: Use when testing, root copy.\n---\nroot\n")
			putFile(t, filepath.Join(cfg, "domains", "backend", "skills", "dup"), "SKILL.md", "---\nname: dup\ndescription: Use when testing, domain copy.\n---\ndomain\n")

			for _, c := range []struct {
				args    []string
				warning string
			}{{[]string{"generate", "--dry-run"}, "Duplicate skill collapsed"}, {[]string{"validate"}, "AR703"}} {
				// Act
				res := testutil.RunCLI(t, dir, c.args...)

				// Assert
				require.Equal(t, 0, res.ExitCode, c.args[0]+": "+res.Stdout+res.Stderr)
				if tt.wantWarn {
					assert.Contains(t, res.Stdout+res.Stderr, c.warning, c.args[0])
				} else {
					assert.NotContains(t, res.Stdout+res.Stderr, c.warning, c.args[0])
				}
			}
		})
	}
}
