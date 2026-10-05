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

const deliveryConfig = `version = "4.0"
name = "delivery"
gitignore = false
presets = ["claude", "cursor", "rovodev"]

[skills]
delivery = "static"

[domains.billing]
delivery = "served"
`

func deliveryProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"config.toml":                                    deliveryConfig,
		"skills/core/SKILL.md":                           "---\ndescription: Core conventions\n---\nCORE\n",
		"skills/heavy/SKILL.md":                          "---\ndescription: Heavy frontmatter served skill\ndelivery: served\ntriggers: [big migration, schema change]\n---\nHEAVY\n",
		"domains/billing/skills/refunds/SKILL.md":        "---\ndescription: Process refunds\n---\nREFUNDS\n",
		"domains/billing/skills/invoices/SKILL.md":       "---\ndescription: Invoices, kept static too\ndelivery: both\n---\nINVOICES\n",
		"domains/billing/skills/refunds/references/r.md": "R\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestGenerate_ServedSkillsLeaveStaticTrees(t *testing.T) {
	root := deliveryProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))

	for _, dir := range []string{".claude/skills", ".cursor/skills"} {
		base := filepath.Join(root, filepath.FromSlash(dir))
		if !exists(t, base) {
			continue
		}
		assert.True(t, exists(t, filepath.Join(base, "core", "SKILL.md")), "%s: static skill is written", dir)
		assert.True(t, exists(t, filepath.Join(base, "invoices", "SKILL.md")), "%s: delivery=both is written", dir)
		assert.False(t, exists(t, filepath.Join(base, "heavy")), "%s: served skill is excluded", dir)
		assert.False(t, exists(t, filepath.Join(base, "refunds")), "%s: skill of a served domain is excluded", dir)
		assert.True(t, exists(t, filepath.Join(base, "dynamic-skills", "SKILL.md")), "%s: the stub is written", dir)
	}
	assert.True(t, exists(t, filepath.Join(root, ".claude", "skills", "dynamic-skills", "SKILL.md")))

	stub, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "dynamic-skills", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(stub), "find_skill")
	assert.Contains(t, string(stub), "load_skill")

	core, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "core", "SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(core), "delivery", "delivery steers ai-rulez and is not written into frontmatter")
}

func TestGenerate_StubAppearsOncePerTree(t *testing.T) {
	root := deliveryProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	outputs, _, err := NewGenerator(cfg).collectOutputs("")
	require.NoError(t, err)
	var stubs []string
	for _, out := range outputs {
		if filepath.Base(filepath.Dir(out.Path)) == config.DynamicSkillsName && filepath.Base(out.Path) == "SKILL.md" {
			stubs = append(stubs, out.Path)
		}
	}
	// One stub per MCP-capable harness tree (claude, cursor), none for cline, and never twice in a tree.
	seen := map[string]bool{}
	for _, p := range stubs {
		assert.False(t, seen[p], "duplicate stub output %s", p)
		seen[p] = true
	}
	assert.NotEmpty(t, stubs)
}

func TestGenerate_NoMCPHarnessKeepsServedSkillsStatic(t *testing.T) {
	root := deliveryProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	outputs, _, err := NewGenerator(cfg).collectOutputs("")
	require.NoError(t, err)
	var rovoHeavy, rovoStub, rovoCore bool
	for _, out := range outputs {
		rel, relErr := filepath.Rel(root, out.Path)
		require.NoError(t, relErr)
		if !strings.HasPrefix(rel, ".rovodev") || filepath.Base(out.Path) != "SKILL.md" {
			continue
		}
		switch filepath.Base(filepath.Dir(out.Path)) {
		case "heavy":
			rovoHeavy = true
		case config.DynamicSkillsName:
			rovoStub = true
		case "core":
			rovoCore = true
		}
	}
	require.True(t, rovoCore, "rovodev writes skills")
	assert.True(t, rovoHeavy, "a harness without MCP keeps the served skill as a static file instead of dropping it")
	assert.False(t, rovoStub, "and gets no stub that points at tools it cannot call")
}

func TestServedSkills_IncludeServedItemsAndCarryDelivery(t *testing.T) {
	root := deliveryProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	_, served, err := NewGenerator(cfg).ServedSkills("", "claude")
	require.NoError(t, err)
	byID := map[string]ServedSkill{}
	for _, s := range served {
		byID[s.ID] = s
	}
	require.Contains(t, byID, "heavy", "serving renders served skills that the static tree leaves out")
	require.Contains(t, byID, "refunds")
	assert.NotContains(t, byID, config.DynamicSkillsName, "the stub is not itself a served skill")
	assert.Equal(t, config.DeliveryServed, byID["heavy"].Delivery)
	assert.Equal(t, config.DeliveryServed, byID["refunds"].Delivery)
	assert.Equal(t, config.DeliveryBoth, byID["invoices"].Delivery)
	assert.Equal(t, config.DeliveryStatic, byID["core"].Delivery)
	assert.Equal(t, []string{"big migration", "schema change"}, byID["heavy"].Triggers)
}
