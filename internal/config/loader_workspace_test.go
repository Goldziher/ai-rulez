package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

const memConfigTOML = "version = \"5.0\"\nname = \"mem\"\npresets = [\"claude\"]\n"

// memProject is a project that exists only in memory: nothing below the virtual
// root is on the disk, so a read that bypasses the workspace fails.
func memProject() *workspace.Mem {
	ws := workspace.NewMem("/virtual/proj")
	ws.Set(".ai-rulez/config.toml", memConfigTOML, 0o644)
	ws.Set(".ai-rulez/rules/style.md", "---\npriority: high\n---\n# Style\nBe concise.\n", 0o644)
	ws.Set(".ai-rulez/context/arch.md", "# Architecture\n", 0o644)
	ws.Set(".ai-rulez/skills/review/SKILL.md", "---\nname: review\ndescription: Review code\n---\nBody\n", 0o644)
	ws.Set(".ai-rulez/skills/review/references/api.md", "# API\n", 0o644)
	ws.Set(".ai-rulez/domains/backend/rules/db.md", "# DB\n", 0o644)
	return ws
}

func TestLoadConfigReadsOnlyThroughTheWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		baseDir   string
		setup     func(*workspace.Mem)
		wantRules []string
	}{
		{name: "absolute base dir", baseDir: "/virtual/proj", wantRules: []string{"style"}},
		{name: "relative base dir resolves against the workspace root", baseDir: ".", wantRules: []string{"style"}},
		{
			name:    "local overlay and local content come from the workspace",
			baseDir: "/virtual/proj",
			setup: func(ws *workspace.Mem) {
				ws.Set(".ai-rulez/config.local.toml", "name = \"overlaid\"\n", 0o600)
				ws.Set(".ai-rulez/local/rules/mine.md", "# Mine\n", 0o644)
			},
			wantRules: []string{"style"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := memProject()
			if tt.setup != nil {
				tt.setup(ws)
			}

			// Act
			cfg, err := LoadConfig(t.Context(), tt.baseDir, WithWorkspace(ws), WithoutRemote())

			// Assert
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash("/virtual/proj"), cfg.BaseDir)
			assert.Equal(t, filepath.FromSlash("/virtual/proj/.ai-rulez"), cfg.ConfigDir)
			var rules []string
			for _, r := range cfg.Content.Rules {
				rules = append(rules, r.Name)
			}
			assert.Equal(t, tt.wantRules, rules)
			require.Len(t, cfg.Content.Skills, 1)
			require.Len(t, cfg.Content.Skills[0].Resources, 1)
			assert.Equal(t, "references/api.md", cfg.Content.Skills[0].Resources[0].RelPath)
			require.Contains(t, cfg.Content.Domains, "backend")
			assert.Same(t, ws, cfg.Workspace.(*workspace.Mem))
			if tt.setup != nil {
				assert.Equal(t, "overlaid", cfg.Name)
				require.NotNil(t, cfg.LocalContent)
				assert.Len(t, cfg.LocalContent.Rules, 1)
			}
		})
	}
}

func TestLoadConfigFromFileUsesTheWorkspace(t *testing.T) {
	// Arrange
	ws := memProject()

	// Act
	cfg, err := LoadConfigFromFile(t.Context(), ".ai-rulez/config.toml", WithWorkspace(ws), WithoutRemote())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.FromSlash("/virtual/proj"), cfg.BaseDir)
	assert.Len(t, cfg.Content.Rules, 1)
}

func TestLoadConfigInAWorkspaceFailsWithoutAConfig(t *testing.T) {
	// Arrange
	ws := workspace.NewMem("/virtual/empty")
	ws.Set("readme.md", "# nothing\n", 0o644)

	// Act
	_, err := LoadConfig(t.Context(), ".", WithWorkspace(ws), WithoutRemote())

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory not found")
}

func TestWorkspaceSymlinkPolicy(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantRules []string
		wantRefus bool
	}{
		{name: "link to a file inside the workspace is followed", target: "../../shared/common.md", wantRules: []string{"linked", "style"}},
		{name: "link leaving the workspace is refused", target: "../../../outside.md", wantRules: []string{"style"}, wantRefus: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := memProject()
			ws.Set("shared/common.md", "# Common\n", 0o644)
			ws.Symlink(".ai-rulez/rules/linked.md", tt.target)
			warned := &testutil.LogRecorder{}

			// Act
			cfg, err := LoadConfig(t.Context(), "/virtual/proj", WithWorkspace(ws), WithoutRemote(), WithHost(ambient.Host{Log: warned}))

			// Assert
			require.NoError(t, err)
			var rules []string
			for _, r := range cfg.Content.Rules {
				rules = append(rules, r.Name)
			}
			assert.Equal(t, tt.wantRules, rules)
			if tt.wantRefus {
				require.Len(t, cfg.ContentProblems, 1)
				assert.Contains(t, cfg.ContentProblems[0].Reason, "outside the repository root")
				assert.Contains(t, warned.String(), "refusing symlinked content")
			} else {
				assert.Empty(t, cfg.ContentProblems)
			}
		})
	}
}

func TestOSWorkspaceLoadMatchesPathLoad(t *testing.T) {
	// Arrange: the same project, loaded by path and through an explicit OS workspace.
	dir := t.TempDir()
	for name, content := range map[string]string{
		".ai-rulez/config.toml":          memConfigTOML,
		".ai-rulez/rules/style.md":       "# Style\n",
		".ai-rulez/context/arch.md":      "# Arch\n",
		".ai-rulez/skills/s/SKILL.md":    "---\nname: s\ndescription: d\n---\nBody\n",
		".ai-rulez/skills/s/scripts/x":   "#!/bin/sh\n",
		".ai-rulez/domains/d/rules/r.md": "# R\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o755)) //nolint:gosec // fixture
	}
	ws, err := workspace.OS(dir)
	require.NoError(t, err)

	// Act
	byPath, err := LoadConfig(t.Context(), dir, WithoutRemote())
	require.NoError(t, err)
	byWorkspace, err := LoadConfig(t.Context(), ".", WithWorkspace(ws), WithoutRemote())
	require.NoError(t, err)

	// Assert
	assert.Equal(t, byPath.BaseDir, byWorkspace.BaseDir)
	assert.Equal(t, byPath.Content, byWorkspace.Content)
}

func TestOSWorkspaceFollowsAnInsideSymlinkOnDisk(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(memConfigTOML), 0o644)) //nolint:gosec // fixture
	require.NoError(t, os.WriteFile(filepath.Join(dir, "common.md"), []byte("# Common\n"), 0o644))                 //nolint:gosec // fixture
	testutil.SymlinkOrSkip(t, "../../common.md", filepath.Join(dir, ".ai-rulez", "rules", "common.md"))
	ws, err := workspace.OS(dir)
	require.NoError(t, err)

	// Act
	cfg, err := LoadConfig(t.Context(), dir, WithWorkspace(ws), WithoutRemote())

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.Content.Rules, 1)
	assert.Equal(t, "common", cfg.Content.Rules[0].Name)
}
