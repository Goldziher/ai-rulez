package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadHashesProject(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return NewGenerator(cfg)
}

// v4Defaults pins the pre-v5 defaults these drift tests were written against:
// CLAUDE.md carries the instructions itself instead of importing AGENTS.md.
const v4Defaults = "agents_md = false\n"

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
			name:   "hashes content reports a hand edit as edited",
			header: v4Defaults + hdr("content"),
			mutate: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "CLAUDE.md"), "tamper\n")
			},
			want: []Drift{{Path: "CLAUDE.md", Kind: DriftEdited}},
		},
		{
			name:   "hashes none compares the whole file",
			header: v4Defaults + hdr("none"),
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
			header := tt.header
			if header == "" {
				header = v4Defaults + hdr("full")
			}
			dir := hashesProject(t, header)
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
	dir := hashesProject(t, v4Defaults+hdr("full"))
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
	assert.Contains(t, strings.Join(plan, "\n"), "write-file: CLAUDE.md", "generate repairs a hand edit")
}

func TestVerifyPlugin_NotGenerated(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\ngitignore = false\n\n"+
			"[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\ndescription = \"d\"\nruntimes = [\"claude\"]\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	err = NewGenerator(cfg).VerifyPlugin("")
	require.ErrorIs(t, err, ErrPluginNotGenerated)
	require.ErrorIs(t, err, ErrPluginDrift)
	assert.Contains(t, err.Error(), "ai-rulez generate --plugin")
}

func TestVerifyPlugin_TamperedBundleIsDrift(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\ngitignore = false\n\n"+
			"[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\ndescription = \"d\"\nruntimes = [\"claude\"]\n"), 0o644))
	load := func() *Generator {
		cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
		require.NoError(t, err)
		return NewGenerator(cfg)
	}
	require.NoError(t, load().GeneratePlugin(""))
	require.NoError(t, load().VerifyPlugin(""))

	manifest := filepath.Join(dir, ".claude-plugin", "plugin.json")
	appendTo(t, manifest, "\n")
	err := load().VerifyPlugin("")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginDrift, "a hand-edited bundle file is drift, not a failed run")
	assert.NotErrorIs(t, err, ErrPluginNotGenerated)
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
