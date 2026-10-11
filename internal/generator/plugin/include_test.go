package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// writeIncludeTree writes files (content-root-relative slash paths) under a new
// temp directory and returns it.
func writeIncludeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o755))
	}
	return root
}

// includeManifest is a minimal agent-plugins manifest whose content root is root.
func includeManifest(root string, patterns []string) *Manifest {
	return &Manifest{
		Name:      "acme",
		Version:   "1.0.0",
		Config:    &config.Config{ConfigDir: root},
		SourceDir: root,
		Runtimes:  []string{config.PluginRuntimeAgentPlugins},
		Include:   patterns,
	}
}

// relativePaths maps include outputs to their bundle-relative slash paths.
func relativePaths(t *testing.T, baseDir string, outputs []config.OutputFile) []string {
	t.Helper()
	var out []string
	for _, o := range outputs {
		rel, err := filepath.Rel(baseDir, o.Path)
		require.NoError(t, err)
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func TestIncludeOutputs(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		patterns []string
		want     []string
	}{
		{
			name:     "double star copies a nested launcher",
			files:    map[string]string{"scripts/mcp-launch.sh": "#!/bin/sh\n"},
			patterns: []string{"scripts/**"},
			want:     []string{"scripts/mcp-launch.sh"},
		},
		{
			name: "double star recurses",
			files: map[string]string{
				"scripts/a.sh":     "a",
				"scripts/sub/b.sh": "b",
			},
			patterns: []string{"scripts/**"},
			want:     []string{"scripts/a.sh", "scripts/sub/b.sh"},
		},
		{
			name:     "single star stays in one directory",
			files:    map[string]string{"assets/logo.png": "png", "assets/icons/deep.png": "png"},
			patterns: []string{"assets/*.png"},
			want:     []string{"assets/logo.png"},
		},
		{
			name:     "non-matching files are left out",
			files:    map[string]string{"scripts/run.sh": "x", "notes.md": "x"},
			patterns: []string{"scripts/**"},
			want:     []string{"scripts/run.sh"},
		},
		{
			name:     "reserved package paths are never clobbered",
			files:    map[string]string{"skills/deploy/SKILL.md": "x", "plugin.json": "{}"},
			patterns: []string{"**"},
			want:     nil,
		},
		{
			name:     "no patterns yields nothing",
			files:    map[string]string{"scripts/run.sh": "x"},
			patterns: nil,
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeIncludeTree(t, tc.files)
			baseDir := t.TempDir()
			m := includeManifest(root, tc.patterns)

			outputs, err := includeOutputs(m, baseDir)
			require.NoError(t, err)

			assert.Equal(t, tc.want, relativePaths(t, baseDir, outputs))
		})
	}
}

func TestIncludeOutputs_PreservesExecutableBit(t *testing.T) {
	root := writeIncludeTree(t, map[string]string{"scripts/mcp-launch.sh": "#!/bin/sh\n"})
	outputs, err := includeOutputs(includeManifest(root, []string{"scripts/**"}), t.TempDir())
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.NotZero(t, outputs[0].Mode&0o111, "the launcher keeps its executable bit")
}

// TestIncludeOutputs_ResolvesAgainstContentRoot guards the content_root-relative
// rule: with [plugin] content_root set, the glob resolves there, not at the
// config directory.
func TestIncludeOutputs_ResolvesAgainstContentRoot(t *testing.T) {
	root := writeIncludeTree(t, map[string]string{"plugin/scripts/mcp-launch.sh": "x"})
	baseDir := t.TempDir()
	m := &Manifest{
		Name: "acme", Version: "1.0.0",
		Config: &config.Config{
			ConfigDir: filepath.Join(root, "unused"),
			Plugin:    &config.PluginAuthoring{ContentRoot: "plugin"},
		},
		SourceDir: root,
		Include:   []string{"scripts/**"},
	}

	outputs, err := includeOutputs(m, baseDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"scripts/mcp-launch.sh"}, relativePaths(t, baseDir, outputs))
}

func TestMatchInclude(t *testing.T) {
	tests := []struct {
		pattern string
		rel     string
		want    bool
	}{
		{"scripts/**", "scripts/a.sh", true},
		{"scripts/**", "scripts/a/b.sh", true},
		{"scripts/**", "other/a.sh", false},
		{"scripts/*.sh", "scripts/a.sh", true},
		{"scripts/*.sh", "scripts/sub/a.sh", false},
		{"assets/*.png", "assets/logo.png", true},
		{"**/*.sh", "a/b/c.sh", true},
		{"**/*.sh", "a/b/c.txt", false},
		{"scripts/", "scripts/a.sh", true},
		{"*.md", "notes.md", true},
		{"*.md", "docs/notes.md", false},
		{"scripts/a?.sh", "scripts/ab.sh", true},
		{"scripts/a?.sh", "scripts/abc.sh", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+" "+tc.rel, func(t *testing.T) {
			assert.Equal(t, tc.want, matchInclude(tc.pattern, tc.rel))
		})
	}
}

// TestRenderAgentPlugins_IncludeSilencesMCPContainment is the AR9O2 half of the
// fix: a `./`-relative command stops warning once the include ships the file,
// and still warns when it does not.
func TestRenderAgentPlugins_IncludeSilencesMCPContainment(t *testing.T) {
	root := writeIncludeTree(t, map[string]string{"scripts/mcp-launch.sh": "#!/bin/sh\n"})
	m := includeManifest(root, []string{"scripts/**"})
	m.MCP = []config.PluginMCPLaunch{{Name: "s", Command: "./scripts/mcp-launch.sh", Transport: config.TransportStdio}}

	findings, err := AgentPluginFindings(m, t.TempDir())
	require.NoError(t, err)
	assert.NotContains(t, codesOf(findings), agentplugins.CodeCommandNotBundled)

	// Without the include the command has no file to resolve to.
	m.Include = nil
	findings, err = AgentPluginFindings(m, t.TempDir())
	require.NoError(t, err)
	assert.Contains(t, codesOf(findings), agentplugins.CodeCommandNotBundled)
}

func codesOf(findings []agentplugins.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Code)
	}
	return out
}

// TestGenerate_IncludeIsRecordedAndVerified covers the sidecar record and the
// verify drift: a changed or missing included file is drift.
func TestGenerate_IncludeIsRecordedAndVerified(t *testing.T) {
	root := writeIncludeTree(t, map[string]string{"scripts/mcp-launch.sh": "#!/bin/sh\n"})
	m := includeManifest(root, []string{"scripts/**"})
	m.MCP = []config.PluginMCPLaunch{{Name: "s", Command: "./scripts/mcp-launch.sh", Transport: config.TransportStdio}}

	outDir := t.TempDir()
	outputs, err := Generate(m, outDir)
	require.NoError(t, err)

	byPath := map[string]config.OutputFile{}
	for _, o := range outputs {
		rel, err := filepath.Rel(outDir, o.Path)
		require.NoError(t, err)
		byPath[filepath.ToSlash(rel)] = o
		require.NoError(t, os.MkdirAll(filepath.Dir(o.Path), 0o755))
		body := o.RawContent
		if body == nil {
			body = []byte(o.Content)
		}
		require.NoError(t, os.WriteFile(o.Path, body, 0o644))
	}

	launcher, ok := byPath["scripts/mcp-launch.sh"]
	require.True(t, ok, "the included launcher lands in the package")
	assert.Equal(t, "#!/bin/sh\n", string(launcher.RawContent), "an included file is copied verbatim")

	sidecar, ok := byPath[ProvenanceFileName]
	require.True(t, ok)
	includes, err := ProvenanceIncludes(sidecar.RawContent)
	require.NoError(t, err)
	assert.Equal(t, []string{"scripts/mcp-launch.sh"}, includes)

	hashes, _, err := ProvenanceOutputs(sidecar.RawContent)
	require.NoError(t, err)
	assert.Contains(t, hashes, "scripts/mcp-launch.sh", "the included file is recorded with a content hash")

	require.NoError(t, VerifyProvenance(outDir))

	// A changed included file is drift.
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "scripts", "mcp-launch.sh"), []byte("changed\n"), 0o755))
	assert.Error(t, VerifyProvenance(outDir))

	// A missing included file is drift.
	require.NoError(t, os.Remove(filepath.Join(outDir, "scripts", "mcp-launch.sh")))
	assert.Error(t, VerifyProvenance(outDir))
}

// TestGenerate_IncludeAbsentKeepsOutputIdentical guards the omitempty sidecar:
// without include the bundle is what it was before the feature.
func TestGenerate_IncludeAbsentKeepsOutputIdentical(t *testing.T) {
	skill := writeSkill(t, "deploy", "---\nname: deploy\ndescription: Deploy.\n---\n", nil)
	build := func(include []string) map[string]string {
		m := &Manifest{
			Name: "acme", Version: "1.0.0",
			Runtimes: []string{config.PluginRuntimeAgentPlugins},
			Skills:   []config.ContentFile{skill},
			Include:  include,
		}
		outDir := t.TempDir()
		outputs, err := Generate(m, outDir)
		require.NoError(t, err)
		out := map[string]string{}
		for _, o := range outputs {
			rel, err := filepath.Rel(outDir, o.Path)
			require.NoError(t, err)
			body := o.RawContent
			if body == nil {
				body = []byte(o.Content)
			}
			out[filepath.ToSlash(rel)] = string(body)
		}
		return out
	}

	absent := build(nil)
	empty := build([]string{})
	assert.Equal(t, absent, empty)

	sidecar := absent[ProvenanceFileName]
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(sidecar), &doc))
	assert.NotContains(t, doc, "includes", "an include-free bundle carries no includes list")
}
