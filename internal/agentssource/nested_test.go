package agentssource_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func contextByName(cfg *config.Config, name string) *config.ContentFile {
	for i := range cfg.Content.Context {
		if cfg.Content.Context[i].Name == name {
			return &cfg.Content.Context[i]
		}
	}
	return nil
}

func nestedProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", agentsConfig)
	write(t, root, "AGENTS.md", "# Root\n\nROOT_TEXT\n")
	for rel, content := range files {
		write(t, root, rel, content)
	}
	return root
}

func TestNestedAGENTSMDIsScopedContext(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{
		"svc/AGENTS.md":         "# Service\n\nSERVICE_TEXT\n",
		"svc/api/AGENTS.md":     "---\ndescription: The API\n---\n# API\n\nAPI_TEXT\n",
		"docs/other/readme.txt": "no instructions here\n",
	})

	// Act
	cfg := load(t, root)

	// Assert
	assert.ElementsMatch(t, []string{"agents", "agents-svc", "agents-svc-api"}, names(cfg.Content.Context))
	svc := contextByName(cfg, "agents-svc")
	require.NotNil(t, svc)
	assert.Equal(t, filepath.Join(cfg.BaseDir, "svc", "AGENTS.md"), svc.Path)
	assert.Contains(t, svc.Content, "SERVICE_TEXT")
	require.NotNil(t, svc.Metadata)
	assert.Equal(t, []string{"svc/**"}, svc.Metadata.PathScope())
	api := contextByName(cfg, "agents-svc-api")
	require.NotNil(t, api)
	assert.Equal(t, []string{"svc/api/**"}, api.Metadata.PathScope())
	assert.NotContains(t, api.Content, "description:", "frontmatter is parsed, not left in the text")
	assert.Empty(t, contextByName(cfg, "agents").Metadata.PathScope(), "the root file is not scoped")
}

func TestNestedAGENTSMDIsLeftOutWhen(t *testing.T) {
	generated := "<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\n\n# Generated\n"
	tests := []struct {
		name  string
		files map[string]string
	}{
		{"it carries the generated banner", map[string]string{"svc/AGENTS.md": generated}},
		{"it is empty", map[string]string{"svc/AGENTS.md": "\n"}},
		{"it only points at another file", map[string]string{"svc/AGENTS.md": "@../AGENTS.md\n"}},
		{"the directory is a dependency tree", map[string]string{"node_modules/pkg/AGENTS.md": "# Pkg\n", "vendor/x/AGENTS.md": "# X\n"}},
		{"the directory is hidden", map[string]string{".cache/AGENTS.md": "# Cache\n", ".git/AGENTS.md": "# Git\n"}},
		{"the directory is another project", map[string]string{
			"svc/.agents/ai-rulez.toml": agentsConfig, "svc/AGENTS.md": "# Own\n", "svc/api/AGENTS.md": "# Below\n",
		}},
		{"the directory is an ai-rulez project", map[string]string{"svc/.ai-rulez/config.toml": agentsConfig, "svc/AGENTS.md": "# Own\n"}},
		{"it is deeper than the walk goes", map[string]string{"a/b/c/d/e/f/g/h/i/AGENTS.md": "# Deep\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := nestedProject(t, tt.files)

			// Act
			cfg := load(t, root)

			// Assert
			assert.Equal(t, []string{"agents"}, names(cfg.Content.Context))
		})
	}
}

func TestNestedAGENTSMDWithinTheDepthBoundIsRead(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{"a/b/c/d/e/f/g/AGENTS.md": "# Seven\n\nSEVEN_TEXT\n"})

	// Act
	cfg := load(t, root)

	// Assert
	assert.Contains(t, names(cfg.Content.Context), "agents-a-b-c-d-e-f-g")
}

func TestNestedAGENTSMDNeverFollowsASymlink(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{
		"real/AGENTS.md":   "# Real\n\nREAL_TEXT\n",
		"other/notes.md":   "# Notes\n",
		"shared/AGENTS.md": "# Target\n\nTARGET_TEXT\n",
	})
	testutil.SymlinkOrSkip(t, "../real/AGENTS.md", filepath.Join(root, "other", "AGENTS.md"))
	testutil.SymlinkOrSkip(t, "shared", filepath.Join(root, "linked"))

	// Act
	cfg := load(t, root)

	// Assert
	assert.ElementsMatch(t, []string{"agents", "agents-real", "agents-shared"}, names(cfg.Content.Context))
}

func TestNestedAGENTSMDHonoursGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Arrange
	root := nestedProject(t, map[string]string{
		".gitignore":          "scratch/\n/svc/local/AGENTS.md\n",
		"scratch/AGENTS.md":   "# Scratch\n",
		"svc/AGENTS.md":       "# Service\n",
		"svc/local/AGENTS.md": "# Local\n",
	})
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())

	// Act
	cfg := load(t, root)

	// Assert
	assert.ElementsMatch(t, []string{"agents", "agents-svc"}, names(cfg.Content.Context))
}

func TestNestedAGENTSMDNamesNeverCollide(t *testing.T) {
	// Arrange: svc/api-v1 and svc/api/v1 slug to the same name; a context file takes agents-svc too
	root := nestedProject(t, map[string]string{
		"svc/api-v1/AGENTS.md":          "# One\n",
		"svc/api/v1/AGENTS.md":          "# Two\n",
		"svc/AGENTS.md":                 "# Svc\n",
		".agents/context/agents-svc.md": "# Mine\n",
	})

	// Act
	cfg := load(t, root)

	// Assert
	got := names(cfg.Content.Context)
	assert.Len(t, got, 5)
	seen := map[string]bool{}
	for _, n := range got {
		assert.False(t, seen[n], "%s appears twice", n)
		seen[n] = true
	}
}

func TestNestedAGENTSMDIsRenderedScopedAndNeverWritten(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", presetConfig(""))
	write(t, root, "AGENTS.md", "# Root\n\nROOT_TEXT\n")
	write(t, root, "svc/AGENTS.md", "# Service\n\nSERVICE_TEXT\n")
	before := snapshot(t, root)

	// Act
	generate(t, root)
	generate(t, root)
	after := snapshot(t, root)

	// Assert
	assert.Equal(t, before["svc/AGENTS.md"], after["svc/AGENTS.md"], "the nested file is source")
	assert.Equal(t, before["AGENTS.md"], after["AGENTS.md"])
	assert.Contains(t, after[".claude/rules/context-agents-svc.md"], "SERVICE_TEXT", "Claude gets it as a rule scoped to svc")
	assert.Contains(t, after[".claude/rules/context-agents-svc.md"], "svc/**")
	assert.NotContains(t, after["CLAUDE.md"], "SERVICE_TEXT", "a scoped file is not in the always-on text")
	for rel, content := range after {
		if strings.HasPrefix(rel, ".cursor/") || strings.HasPrefix(rel, ".codex/") {
			assert.NotContains(t, content, "SERVICE_TEXT", "%s: Cursor and Codex read the nested AGENTS.md themselves", rel)
		}
	}
	drift, err := generator.NewGenerator(load(t, root)).CheckDrift("")
	require.NoError(t, err)
	assert.Empty(t, drift, "generate --check is clean")
}

func TestGeneratedNestedAGENTSMDIsNeverTheSource(t *testing.T) {
	// Arrange: an earlier source = "ai-rulez" run with a scope wrote svc/AGENTS.md
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.toml", presetConfig("agents_md = true\n\n[[scopes]]\npath = \"svc\"\n"))
	write(t, root, ".ai-rulez/context/overview.md", "# Overview\n\nORIGINAL_CONTEXT\n")
	generate(t, root)

	// Act
	write(t, root, ".ai-rulez/config.toml", presetConfig("source = \"agents\"\n"))
	cfg := load(t, root)

	// Assert
	for _, n := range names(cfg.Content.Context) {
		assert.NotContains(t, n, "svc", "a generated nested AGENTS.md is output")
	}
}

func TestNestedWalkWritesNothing(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{"svc/AGENTS.md": "# Service\n"})
	before := snapshot(t, root)

	// Act
	load(t, root)

	// Assert
	assert.Equal(t, before, snapshot(t, root))
	_, err := os.Stat(filepath.Join(root, "svc", "CLAUDE.md"))
	assert.True(t, os.IsNotExist(err))
}
