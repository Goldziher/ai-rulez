package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deliveryProject(t *testing.T, presets, extraConfig string, files map[string]string) *config.Config {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write("config.toml", "version = \"4.0\"\nname = \"p\"\ngitignore = false\npresets = "+presets+"\n"+extraConfig)
	for rel, content := range files {
		write(rel, content)
	}
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

func codesOf(fs []lint.DeliveryFinding) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.Code] += f.Message + "\n"
	}
	return out
}

var servedSkillFiles = map[string]string{
	"skills/heavy/SKILL.md": "---\ndescription: Heavy served skill\ndelivery: served\n---\nHEAVY\n",
	"skills/core/SKILL.md":  "---\ndescription: Core skill\n---\nCORE\n",
}

func TestDeliveryFindings_NothingServedIsSilent(t *testing.T) {
	cfg := deliveryProject(t, `["claude"]`, "", map[string]string{"skills/core/SKILL.md": servedSkillFiles["skills/core/SKILL.md"]})
	assert.Empty(t, deliveryFindings(cfg))
}

func TestDeliveryFindings_FallbackAndNoServer(t *testing.T) {
	cfg := deliveryProject(t, `["claude", "rovodev"]`, "", servedSkillFiles)
	got := codesOf(deliveryFindings(cfg))
	assert.Contains(t, got[lint.CodeDeliveryStaticFallback], `"rovodev"`)
	assert.Contains(t, got[lint.CodeDeliveryStaticFallback], "heavy")
	assert.Contains(t, got, lint.CodeServedNoServer, "claude can call MCP but nothing runs --serve-skills")
	assert.NotContains(t, got, lint.CodeDeliveryStubMissing, "claude renders the stub")

	withServer := deliveryProject(t, `["claude", "rovodev"]`,
		"\n[[mcp_servers]]\nname = \"skills\"\ncommand = \"ai-rulez\"\nargs = [\"mcp\", \"--serve-skills\"]\n", servedSkillFiles)
	assert.NotContains(t, codesOf(deliveryFindings(withServer)), lint.CodeServedNoServer)
}

func TestDeliveryFindings_StubMissingWhenAnMCPHarnessRendersNoSkills(t *testing.T) {
	cfg := deliveryProject(t, `["claude", "codebuff"]`, "", servedSkillFiles)
	got := codesOf(deliveryFindings(cfg))
	require.Contains(t, got, lint.CodeDeliveryStubMissing)
	assert.Contains(t, got[lint.CodeDeliveryStubMissing], `"codebuff"`)
	assert.NotContains(t, got[lint.CodeDeliveryStubMissing], `"claude"`)
}

func TestDeliveryFindings_LockEnforcementReportsUnpinnedSkills(t *testing.T) {
	cfg := deliveryProject(t, `["claude"]`, "\n[lock]\nenforce = true\n", servedSkillFiles)
	got := codesOf(deliveryFindings(cfg))
	require.Contains(t, got, lint.CodeServedLockMismatch)
	assert.Contains(t, got[lint.CodeServedLockMismatch], "served heavy: not pinned")
	assert.True(t, strings.Contains(got[lint.CodeServedLockMismatch], "ai-rulez lock"))
}
