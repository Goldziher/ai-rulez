package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func roleOutputProject(t *testing.T, mode string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("config.toml", `version = "5.0"
name = "role-outputs"
presets = ["claude"]
gitignore = false

[[roles]]
name = "dev"
pin = true
[roles.skill_mode]
deploy = "`+mode+`"
`)
	write("rules/style.md", "# Style\nUse tabs.\n")
	write("skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy. Use when releasing.\n---\nDeploy.\n")
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	return cfg
}

func TestLockRoleOutputs(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		same bool
	}{
		{name: "same skill_mode renders the same bytes", a: "off", b: "off", same: true},
		{name: "a different skill_mode changes the rendering", a: "off", b: "name-only", same: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfgA, cfgB := roleOutputProject(t, tc.a), roleOutputProject(t, tc.b)

			// Act
			outA, errA := LockRoleOutputs(cfgA, "dev")
			outB, errB := LockRoleOutputs(cfgB, "dev")

			// Assert
			require.NoError(t, errA)
			require.NoError(t, errB)
			require.NotEmpty(t, outA)
			digest := func(outs []string) string { return strings.Join(outs, "\x00") }
			var da, db []string
			for _, o := range outA {
				da = append(da, o.Path+"="+string(o.Data))
			}
			for _, o := range outB {
				db = append(db, o.Path+"="+string(o.Data))
			}
			if tc.same {
				assert.Equal(t, digest(da), digest(db))
			} else {
				assert.NotEqual(t, digest(da), digest(db))
			}
		})
	}
}

func TestLockRoleOutputsWritesNothing(t *testing.T) {
	// Arrange
	cfg := roleOutputProject(t, "off")

	// Act
	_, err := LockRoleOutputs(cfg, "dev")

	// Assert
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(cfg.BaseDir, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(filepath.Join(cfg.BaseDir, ".claude"))
	assert.True(t, os.IsNotExist(statErr))
}
