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

// hashesProject writes a project with one rule and two skills, generating
// CLAUDE.md and .claude/skills/<name>/SKILL.md. headerBlock is appended to the
// TOML config, so a test picks the [header] hashes mode.
func hashesProject(t *testing.T, headerBlock string) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"hashes\"\npresets = [\"claude\"]\n\n"+headerBlock), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"),
		[]byte("---\npriority: high\n---\n# Style\n\nUse tabs.\n"), 0o644))
	for _, name := range []string{"alpha", "beta"} {
		writeHashesSkill(t, dir, name, "Body of "+name+".")
	}
	return dir
}

// hdr returns a TOML [header] block with the given hashes mode.
func hdr(mode string) string {
	return "[header]\nhashes = \"" + mode + "\"\n"
}

func writeHashesSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".ai-rulez", "skills", name)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: Use when testing "+name+".\n---\n# "+name+"\n\n"+body+"\n"), 0o644))
}

func generateHashesProject(t *testing.T, dir string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	require.NoError(t, NewGenerator(cfg).Generate("default"))
}

func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range []string{"CLAUDE.md", ".claude"} {
		base := filepath.Join(dir, root)
		require.NoError(t, filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			data, readErr := os.ReadFile(path)
			rel, _ := filepath.Rel(dir, path)
			out[rel] = string(data)
			return readErr
		}))
	}
	return out
}

func changedPaths(before, after map[string]string) []string {
	var changed []string
	for path, content := range after {
		if before[path] != content {
			changed = append(changed, path)
		}
	}
	return changed
}

func TestHeaderHashes_HeaderLinesPerMode(t *testing.T) {
	tests := []struct {
		name         string
		header       string
		wantContent  bool
		wantSource   bool
		wantFileHash bool
	}{
		{name: "default is content", header: "", wantContent: true},
		{name: "explicit full", header: hdr("full"), wantContent: true, wantSource: true},
		{name: "content drops Source-Hash", header: hdr("content"), wantContent: true},
		{name: "none drops both", header: hdr("none")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := hashesProject(t, tt.header)
			generateHashesProject(t, dir)
			for path, content := range snapshotTree(t, dir) {
				if !strings.HasSuffix(path, "SKILL.md") && path != "CLAUDE.md" {
					continue
				}
				assert.Equal(t, tt.wantContent, strings.Contains(content, "Content-Hash: blake3:"), path)
				assert.Equal(t, tt.wantSource, strings.Contains(content, "Source-Hash: blake3:"), path)
			}
		})
	}
}

func TestHeaderHashes_EditingOneSkillChangesOnlyItsOutput(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		wantChanged []string
		wantAll     bool
	}{
		{
			name:        "content",
			header:      hdr("content"),
			wantChanged: []string{filepath.Join(".claude", "skills", "alpha", "SKILL.md")},
		},
		{
			name:        "none",
			header:      hdr("none"),
			wantChanged: []string{filepath.Join(".claude", "skills", "alpha", "SKILL.md")},
		},
		{name: "full keeps whole-tree Source-Hash churn", header: hdr("full"), wantAll: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := hashesProject(t, tt.header)
			generateHashesProject(t, dir)
			before := snapshotTree(t, dir)

			writeHashesSkill(t, dir, "alpha", "Edited body of alpha.")
			generateHashesProject(t, dir)
			changed := changedPaths(before, snapshotTree(t, dir))

			if tt.wantAll {
				assert.Contains(t, changed, filepath.Join(".claude", "skills", "beta", "SKILL.md"))
				assert.Contains(t, changed, "CLAUDE.md")
				return
			}
			assert.ElementsMatch(t, tt.wantChanged, changed)
		})
	}
}

func TestHeaderHashes_Idempotent(t *testing.T) {
	for _, header := range []string{"", hdr("content"), hdr("none"), hdr("full"),
		hdr("content") + "timestamp = true\n", hdr("none") + "timestamp = true\n"} {
		t.Run(strings.ReplaceAll(header, "\n", " "), func(t *testing.T) {
			dir := hashesProject(t, header)
			generateHashesProject(t, dir)
			first := snapshotTree(t, dir)
			generateHashesProject(t, dir)
			assert.Empty(t, changedPaths(first, snapshotTree(t, dir)))
		})
	}
}

func TestHeaderHashes_ReRendersWhenHeaderStyleChanges(t *testing.T) {
	dir := hashesProject(t, hdr("content"))
	generateHashesProject(t, dir)
	before := snapshotTree(t, dir)

	// The body (and so the Content-Hash) is unchanged; only the banner differs,
	// which no Source-Hash is around to flag.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(
		"version = \"5.0\"\nname = \"hashes\"\npresets = [\"claude\"]\n\n[header]\nhashes = \"content\"\nstyle = \"detailed\"\n"), 0o644))
	generateHashesProject(t, dir)
	assert.Contains(t, changedPaths(before, snapshotTree(t, dir)), "CLAUDE.md")
}

func TestHeaderHashes_InvalidModeFailsValidation(t *testing.T) {
	dir := hashesProject(t, hdr("sometimes"))
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	assert.ErrorContains(t, cfg.Validate(), "header.hashes")
}

func TestHeaderHashes_CleanRemovesOutputsInEveryMode(t *testing.T) {
	for _, mode := range []string{"full", "content", "none"} {
		t.Run(mode, func(t *testing.T) {
			dir := hashesProject(t, hdr(mode))
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			gen := NewGenerator(cfg)
			require.NoError(t, gen.Generate("default"))
			require.FileExists(t, filepath.Join(dir, "CLAUDE.md"))

			_, err = gen.Clean("default", CleanOptions{})
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
			assert.NoDirExists(t, filepath.Join(dir, ".claude", "skills"))
			assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "skills", "alpha", "SKILL.md"))
		})
	}
}

// Plugin provenance (`verify --plugin`) keeps its own per-bundle scheme and is
// independent of [header] hashes, so verify must pass after generate in every mode.
func TestHeaderHashes_VerifyPluginPassesInEveryMode(t *testing.T) {
	for _, mode := range []string{"full", "content", "none"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "plugin", "basemind"), dir)
			cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
			raw, err := os.ReadFile(cfgPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfgPath, append(raw, []byte("\n[header]\nhashes = \""+mode+"\"\n")...), 0o644))

			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			gen := NewGenerator(cfg)
			require.NoError(t, gen.GeneratePlugin(""))
			require.NoError(t, gen.VerifyPlugin(""))
		})
	}
}

// With timestamp = true the Generated: stamp is ignored when deciding whether to
// rewrite, but only in the header: a body line that happens to start with
// "Generated: " is real content and its edits must reach disk.
func TestHeaderHashes_TimestampIgnoresOnlyTheHeaderStamp(t *testing.T) {
	for _, header := range []string{hdr("content") + "timestamp = true\n", hdr("none") + "timestamp = true\n"} {
		t.Run(strings.ReplaceAll(header, "\n", " "), func(t *testing.T) {
			dir := hashesProject(t, header)
			writeHashesSkill(t, dir, "alpha", "Generated: see docs/a.md")
			generateHashesProject(t, dir)

			writeHashesSkill(t, dir, "alpha", "Generated: see docs/b.md")
			generateHashesProject(t, dir)

			data, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "Generated: see docs/b.md")
		})
	}
}
