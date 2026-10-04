package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadHashesProject(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return NewGenerator(cfg)
}

func TestCheckDrift(t *testing.T) {
	skill := filepath.Join(".claude", "skills", "alpha", "SKILL.md")
	tests := []struct {
		name   string
		header string
		mutate func(t *testing.T, dir string)
		want   []Drift
		// subset: a source change also moves the Source-Hash of every other file.
		subset bool
	}{
		{name: "clean tree", header: "", mutate: func(*testing.T, string) {}},
		{
			name: "hand-edited file is reported as edited",
			mutate: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "CLAUDE.md"), "tamper\n")
			},
			want: []Drift{{Path: "CLAUDE.md", Kind: DriftEdited}},
		},
		{
			name: "deleted file is reported as missing",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, skill)))
			},
			want: []Drift{{Path: filepath.ToSlash(skill), Kind: DriftMissing}},
		},
		{
			name: "changed source is reported as stale",
			mutate: func(t *testing.T, dir string) {
				writeHashesSkill(t, dir, "alpha", "A different body.")
			},
			want:   []Drift{{Path: filepath.ToSlash(skill), Kind: DriftStale}, {Path: "CLAUDE.md", Kind: DriftStale}},
			subset: true,
		},
		{
			name:   "hashes none compares the whole file",
			header: "header:\n  hashes: none\n",
			mutate: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "CLAUDE.md"), "tamper\n")
			},
			want: []Drift{{Path: "CLAUDE.md", Kind: DriftStale}},
		},
		{
			name: "removed source leaves an orphan",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "skills", "beta")))
			},
			want:   []Drift{{Path: ".claude/skills/beta/SKILL.md", Kind: DriftOrphan}, {Path: "CLAUDE.md", Kind: DriftStale}},
			subset: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := hashesProject(t, tt.header)
			generateHashesProject(t, dir)
			tt.mutate(t, dir)
			mutated := snapshotTree(t, dir)

			got, err := loadHashesProject(t, dir).CheckDrift("default")
			require.NoError(t, err)
			switch {
			case len(tt.want) == 0:
				assert.Empty(t, got)
			case tt.subset:
				assert.Subset(t, got, tt.want)
			default:
				assert.ElementsMatch(t, tt.want, got)
			}
			assert.Equal(t, mutated, snapshotTree(t, dir), "CheckDrift must not write")
		})
	}
}

func TestVerifyGenerated(t *testing.T) {
	dir := hashesProject(t, "")
	generateHashesProject(t, dir)

	drift, checked, err := loadHashesProject(t, dir).VerifyGenerated()
	require.NoError(t, err)
	assert.Empty(t, drift)
	assert.Positive(t, checked)

	appendTo(t, filepath.Join(dir, "CLAUDE.md"), "tamper\n")
	require.NoError(t, os.Remove(filepath.Join(dir, ".claude", "skills", "beta", "SKILL.md")))
	drift, _, err = loadHashesProject(t, dir).VerifyGenerated()
	require.NoError(t, err)
	assert.Equal(t, []Drift{
		{Path: ".claude/skills/beta/SKILL.md", Kind: DriftMissing},
		{Path: "CLAUDE.md", Kind: DriftEdited},
	}, drift)
}

func TestVerifyGenerated_NoManifest(t *testing.T) {
	dir := hashesProject(t, "")
	_, _, err := loadHashesProject(t, dir).VerifyGenerated()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no generated manifest")
}

func TestDryRun_ReportsUnchanged(t *testing.T) {
	dir := hashesProject(t, "")
	plan, err := loadHashesProject(t, dir).DryRun("default")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(plan, "\n"), "write-file: CLAUDE.md")

	generateHashesProject(t, dir)
	plan, err = loadHashesProject(t, dir).DryRun("default")
	require.NoError(t, err)
	joined := strings.Join(plan, "\n")
	assert.Contains(t, joined, "unchanged: CLAUDE.md")
	assert.NotContains(t, joined, "write-file:")

	appendTo(t, filepath.Join(dir, "CLAUDE.md"), "tamper\n")
	plan, err = loadHashesProject(t, dir).DryRun("default")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(plan, "\n"), "edited: CLAUDE.md")
}

func TestVerifyPlugin_NotGenerated(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"4.0\"\nname = \"demo\"\npresets = [\"claude\"]\ngitignore = false\n\n"+
			"[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\ndescription = \"d\"\nruntimes = [\"claude\"]\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	err = NewGenerator(cfg).VerifyPlugin("")
	require.ErrorIs(t, err, ErrPluginNotGenerated)
	assert.Contains(t, err.Error(), "ai-rulez generate --plugin")
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
