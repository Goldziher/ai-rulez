package providers_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batchContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "always", Path: "/p/.ai-rulez/rules/always.md", Content: "ALWAYS_RULE"},
			{
				Name: "tsx", Path: "/p/.ai-rulez/rules/tsx.md", Content: "TSX_RULE",
				Metadata: &config.Metadata{Globs: []string{"**/*.tsx", "src/**/*.ts"}},
			},
			{
				Name: "auto-rule", Path: "/p/.ai-rulez/rules/auto.md", Content: "AUTO_RULE",
				Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "when X"}},
			},
			{
				Name: "manual-rule", Path: "/p/.ai-rulez/rules/manual.md", Content: "MANUAL_RULE",
				Metadata: &config.Metadata{Activation: "manual"},
			},
		},
		Context: []config.ContentFile{{Name: "layout", Path: "/p/.ai-rulez/context/layout.md", Content: "LAYOUT_CTX"}},
		Skills: []config.ContentFile{{
			Name: "demo", Path: "/p/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Demo skill"}},
		}},
		Agents: []config.ContentFile{{
			Name: "scout", Path: "/p/.ai-rulez/agents/scout.md", Content: "Scout.",
			Metadata: &config.Metadata{Tools: []string{"read"}, Extra: map[string]string{"description": "Recon"}},
		}},
		Commands: []config.ContentFile{{
			Name: "ship", Path: "/p/.ai-rulez/commands/ship.md", Content: "Ship it.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship"}},
		}},
	}
}

func batchConfig() *config.Config {
	return &config.Config{
		Name: "demo", BaseDir: "/p", ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez",
		MCPServers: map[string]*config.MCPServer{"fs": {Name: "fs", Command: "npx", Args: []string{"x"}}},
	}
}

func batchPaths(outputs []config.OutputFile) []string {
	var paths []string
	for _, o := range outputs {
		if !o.IsDir {
			rel, _ := filepath.Rel("/p", o.Path)
			paths = append(paths, filepath.ToSlash(rel))
		}
	}
	return paths
}

// TestHarnessBatch_Outputs pins the exact files each preset writes and the
// frontmatter keys of one representative file per kind.
func TestHarnessBatch_Outputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset    string
		wantFiles []string
		notFiles  []string
		// frontmatter checks: file -> key -> value ("" asserts the key is absent)
		frontmatter map[string]map[string]string
	}{
		{
			preset: "kiro",
			wantFiles: []string{
				"AGENTS.md", ".kiro/steering/always.md", ".kiro/steering/tsx.md", ".kiro/steering/auto-rule.md",
				".kiro/steering/manual-rule.md", ".kiro/skills/demo/SKILL.md", ".kiro/agents/scout.md",
				".kiro/prompts/ship.md", ".kiro/settings/mcp.json",
			},
			frontmatter: map[string]map[string]string{
				".kiro/steering/always.md":      {"inclusion": ""},
				".kiro/steering/tsx.md":         {"inclusion": "fileMatch"},
				".kiro/steering/auto-rule.md":   {"inclusion": "auto", "name": "auto-rule", "description": "when X"},
				".kiro/steering/manual-rule.md": {"inclusion": "manual"},
				".kiro/skills/demo/SKILL.md":    {"name": "demo", "description": "Demo skill"},
				".kiro/agents/scout.md":         {"name": "scout", "description": "Recon"},
				".kiro/prompts/ship.md":         {"name": ""},
			},
		},
		{
			preset: "trae",
			wantFiles: []string{
				".trae/rules/always.md", ".trae/rules/tsx.md", ".trae/rules/auto-rule.md", ".trae/rules/manual-rule.md",
				".trae/rules/context-layout.md", ".trae/skills/demo/SKILL.md", ".trae/mcp.json",
			},
			notFiles: []string{"AGENTS.md"},
			frontmatter: map[string]map[string]string{
				".trae/rules/always.md":      {"alwaysApply": "true"},
				".trae/rules/tsx.md":         {"alwaysApply": "false", "globs": "**/*.tsx,src/**/*.ts"},
				".trae/rules/auto-rule.md":   {"alwaysApply": "false", "description": "when X"},
				".trae/rules/manual-rule.md": {"alwaysApply": "false", "globs": "", "description": ""},
			},
		},
		{
			preset: "aiassistant",
			wantFiles: []string{
				".aiassistant/rules/always.md", ".aiassistant/rules/tsx.md", ".aiassistant/rules/auto-rule.md",
				".aiassistant/rules/manual-rule.md", ".aiassistant/rules/context-layout.md",
				".agents/skills/demo/SKILL.md", ".ai/mcp/mcp.json",
			},
			notFiles: []string{"AGENTS.md"},
			frontmatter: map[string]map[string]string{
				".aiassistant/rules/always.md":      {"apply": "always"},
				".aiassistant/rules/tsx.md":         {"apply": "by file patterns", "patterns": "**/*.tsx, src/**/*.ts"},
				".aiassistant/rules/auto-rule.md":   {"apply": "by model decision", "instructions": "when X"},
				".aiassistant/rules/manual-rule.md": {"apply": "manually"},
			},
		},
		{
			preset: "augment",
			wantFiles: []string{
				"AGENTS.md", ".augment/rules/always.md", ".augment/rules/auto-rule.md", ".augment/rules/manual-rule.md",
				".augment/skills/demo/SKILL.md", ".augment/agents/scout.md", ".augment/commands/ship.md",
				".augment/settings.json",
			},
			frontmatter: map[string]map[string]string{
				".augment/rules/always.md":      {"type": "always_apply"},
				".augment/rules/auto-rule.md":   {"type": "agent_requested", "description": "when X"},
				".augment/rules/manual-rule.md": {"type": "manual"},
				".augment/commands/ship.md":     {"description": "Ship", "name": ""},
				".augment/agents/scout.md":      {"name": "scout", "description": "Recon"},
			},
		},
		{
			preset: "zoocode",
			wantFiles: []string{
				"AGENTS.md", ".roo/skills/demo/SKILL.md",
				".roo/commands/ship.md", ".roo/mcp.json",
			},
			notFiles: []string{".roomodes", ".roo/agents/scout.md", ".roo/rules/always.md"},
			frontmatter: map[string]map[string]string{
				".roo/commands/ship.md":     {"description": "Ship", "name": ""},
				".roo/skills/demo/SKILL.md": {"name": "demo", "description": "Demo skill"},
			},
		},
		{
			preset: "takt",
			wantFiles: []string{
				".takt/facets/policies/always.md", ".takt/facets/policies/tsx.md", ".takt/facets/policies/context-layout.md",
				".takt/facets/knowledge/demo.md", ".takt/facets/personas/scout.md", ".takt/facets/instructions/ship.md",
			},
			notFiles: []string{"AGENTS.md", ".takt/mcp.json", ".takt/config.yaml"},
			frontmatter: map[string]map[string]string{
				".takt/facets/knowledge/demo.md":    {"name": ""},
				".takt/facets/personas/scout.md":    {"name": ""},
				".takt/facets/instructions/ship.md": {"name": ""},
			},
		},
		{
			preset: "cortex",
			wantFiles: []string{
				"AGENTS.md", ".cortex/skills/demo/SKILL.md", ".cortex/agents/scout.md",
			},
			notFiles: []string{".cortex/mcp.json", ".cortex/rules/always.md"},
			frontmatter: map[string]map[string]string{
				".cortex/skills/demo/SKILL.md": {"name": "demo", "description": "Demo skill"},
				".cortex/agents/scout.md":      {"name": "scout", "description": "Recon"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)

			// Act
			outputs, err := gen.Generate(batchContent(), "/p", batchConfig())

			// Assert
			require.NoError(t, err)
			paths := batchPaths(outputs)
			for _, want := range tt.wantFiles {
				assert.Contains(t, paths, want)
			}
			for _, not := range tt.notFiles {
				assert.NotContains(t, paths, not)
			}
			for file, keys := range tt.frontmatter {
				out := requireFile(t, outputs, file)
				for key, want := range keys {
					assert.Equal(t, want, frontmatterValue(out.Content, key), "%s: %s", file, key)
				}
			}
		})
	}
}

// TestHarnessBatch_RootlessRulesEveryModeIsFile checks always_files: a preset
// with no root file writes always-on rules as files in inline mode too.
func TestHarnessBatch_RootlessRulesEveryModeIsFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ preset, dir string }{
		{"trae", ".trae/rules"}, {"aiassistant", ".aiassistant/rules"}, {"takt", ".takt/facets/policies"},
	} {
		for _, mode := range []string{"inline", "split"} {
			t.Run(tc.preset+"/"+mode, func(t *testing.T) {
				t.Parallel()

				// Arrange
				gen, err := providers.LoadBuiltin(tc.preset)
				require.NoError(t, err)
				cfg := batchConfig()
				cfg.Rules = &config.RulesConfig{ModeByPreset: map[string]string{tc.preset: mode}}

				// Act
				outputs, err := gen.Generate(batchContent(), "/p", cfg)

				// Assert
				require.NoError(t, err)
				assert.Contains(t, batchPaths(outputs), tc.dir+"/always.md")
				assert.Contains(t, batchPaths(outputs), tc.dir+"/context-layout.md")
			})
		}
	}
}

// TestHarnessBatch_MCPDocuments pins the MCP document location and shape.
func TestHarnessBatch_MCPDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct{ preset, path string }{
		{"kiro", ".kiro/settings/mcp.json"},
		{"trae", ".trae/mcp.json"},
		{"aiassistant", ".ai/mcp/mcp.json"},
		{"augment", ".augment/settings.json"},
		{"zoocode", ".roo/mcp.json"},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, "/p", batchConfig())

			// Assert
			require.NoError(t, err)
			var doc map[string]map[string]map[string]any
			require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, tt.path).Content), &doc))
			assert.Equal(t, "npx", doc["mcpServers"]["fs"]["command"])
			assert.Equal(t, []any{"x"}, doc["mcpServers"]["fs"]["args"])
		})
	}
}

// TestHarnessBatch_GlobalPaths pins the user-scope layout declared per preset.
func TestHarnessBatch_GlobalPaths(t *testing.T) {
	t.Parallel()

	home := "/home/u"
	j := func(p string) string { return filepath.Join(home, filepath.FromSlash(p)) }
	tests := []struct {
		preset string
		want   providers.GlobalPaths
	}{
		{"kiro", providers.GlobalPaths{
			RootFile: j(".kiro/steering/AGENTS.md"), RulesDir: j(".kiro/steering"),
			SkillsDir: j(".kiro/skills"), AgentsDir: j(".kiro/agents"), CommandsDir: j(".kiro/prompts"),
			Sidecars: map[string]string{".kiro/settings/mcp.json": j(".kiro/settings/mcp.json")},
		}},
		{"trae", providers.GlobalPaths{SkillsDir: j(".trae/skills"), Sidecars: map[string]string{}}},
		{"zoocode", providers.GlobalPaths{
			RootFile: j(".roo/rules/AGENTS.md"), RulesDir: j(".roo/rules"),
			SkillsDir: j(".roo/skills"), CommandsDir: j(".roo/commands"), Sidecars: map[string]string{},
		}},
		{"augment", providers.GlobalPaths{
			RulesDir: j(".augment/rules"), SkillsDir: j(".augment/skills"),
			AgentsDir: j(".augment/agents"), CommandsDir: j(".augment/commands"),
			Sidecars: map[string]string{".augment/settings.json": j(".augment/settings.json")},
		}},
		{"takt", providers.GlobalPaths{
			RulesDir: j(".takt/facets/policies"), SkillsDir: j(".takt/facets/knowledge"),
			AgentsDir: j(".takt/facets/personas"), CommandsDir: j(".takt/facets/instructions"),
			Sidecars: map[string]string{},
		}},
		{"cortex", providers.GlobalPaths{
			SkillsDir: j(".snowflake/cortex/skills"), AgentsDir: j(".snowflake/cortex/agents"),
			Sidecars: map[string]string{},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)

			// Act
			got := gen.Spec.GlobalPaths(home, func(string) string { return "" })

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

// TestHarnessBatch_AlwaysFilesValidation covers the always_files extension: it
// needs split, and then needs no rules_inline root section.
func TestHarnessBatch_AlwaysFilesValidation(t *testing.T) {
	t.Parallel()

	const rules = "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nalways_files = true\n"
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"without split", "name = \"t\"\n" + rules, "require split = true"},
		{"split without a root", "name = \"t\"\n" + rules + "split = true\ndialect = \"junie\"\n", ""},
		{
			"split with a root lacking rules_inline",
			"name = \"t\"\n[root]\nfile = \"AGENTS.md\"\nsections = [\"title\"]\n" + rules + "split = true\ndialect = \"junie\"\n", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			_, err := providers.LoadProviderSpec([]byte(tt.raw), "t.toml", providers.FormatTOML)

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
