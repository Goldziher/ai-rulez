package providers_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopedContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "plain", Content: "Always on."},
			{Name: "go-only", Content: "Go rule.", Metadata: &config.Metadata{Globs: []string{"**/*.go", "go.mod"}}},
			{Name: "sql", Content: "SQL rule.", Metadata: &config.Metadata{
				Activation: "auto", Extra: map[string]string{"description": "when editing SQL"},
			}},
		},
	}
}

type generator interface {
	Generate(*config.ContentTree, string, *config.Config) ([]config.OutputFile, error)
}

func presetGenerators(t *testing.T) map[string]generator {
	t.Helper()
	gens := map[string]generator{
		"gemini":      &presets.GeminiPresetGenerator{},
		"codex":       &presets.CodexPresetGenerator{},
		"opencode":    &presets.OpencodePresetGenerator{},
		"xum":         &presets.XumPresetGenerator{},
		"copilot":     &presets.CopilotPresetGenerator{},
		"antigravity": &presets.AntigravityPresetGenerator{},
	}
	for _, name := range []string{"amp", "hermes"} {
		gen, err := providers.LoadBuiltin(name)
		require.NoError(t, err)
		gens[name] = gen
	}
	return gens
}

func rootFile(t *testing.T, outputs []config.OutputFile, name string) string {
	t.Helper()
	for _, o := range outputs {
		if strings.HasSuffix(strings.ReplaceAll(o.Path, "\\", "/"), "/"+name) {
			return o.Content
		}
	}
	t.Fatalf("no %s output", name)
	return ""
}

func findOutput(outputs []config.OutputFile, suffix string) (string, bool) {
	for _, o := range outputs {
		if strings.HasSuffix(strings.ReplaceAll(o.Path, "\\", "/"), suffix) {
			return o.Content, true
		}
	}
	return "", false
}

func TestInlinePresets_AppliesTo(t *testing.T) {
	tests := []struct {
		preset string
		file   string
	}{
		{"gemini", "GEMINI.md"},
		{"codex", "AGENTS.md"},
		{"opencode", "AGENTS.md"},
		{"xum", "AGENTS.md"},
		{"amp", "AGENTS.md"},
		{"hermes", ".hermes.md"},
		{"copilot", "copilot-instructions.md"},
		{"antigravity", "GEMINI.md"},
	}
	gens := presetGenerators(t)
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			baseDir := t.TempDir()
			outputs, err := gens[tt.preset].Generate(scopedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir})
			require.NoError(t, err)

			doc := rootFile(t, outputs, tt.file)
			if tt.preset == "copilot" {
				// Path-scoped rules move to .github/instructions in inline mode.
				assert.NotContains(t, doc, "### go-only")
				file, ok := findOutput(outputs, ".github/instructions/go-only.instructions.md")
				require.True(t, ok, "scoped rule file missing")
				assert.Contains(t, file, "applyTo: '**/*.go,go.mod'")
			} else {
				assert.Contains(t, doc, "### go-only\n\n_Applies to: `**/*.go`, `go.mod`_\n\n")
			}
			assert.Contains(t, doc, "### sql\n\n_When relevant: when editing SQL_\n\n")
			assert.Contains(t, doc, "### plain\n\nAlways on.\n\n")
		})
	}
}

func TestAgentsMD_IdenticalAcrossSharingPresets(t *testing.T) {
	gens := presetGenerators(t)
	var first, firstName string
	for _, name := range []string{"codex", "opencode", "xum", "amp"} {
		baseDir := t.TempDir()
		outputs, err := gens[name].Generate(scopedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir})
		require.NoError(t, err)
		doc := rootFile(t, outputs, "AGENTS.md")
		if first == "" {
			first, firstName = doc, name
			continue
		}
		assert.Equal(t, first, doc, "AGENTS.md from %s differs from %s", name, firstName)
	}
}

func TestRenderLocalRoot_AppliesTo(t *testing.T) {
	doc := presets.RenderLocalRoot(scopedContent(), &config.Config{Name: "demo"}, "AGENTS.local.md")
	assert.Contains(t, doc, "### go-only\n\n_Applies to: `**/*.go`, `go.mod`_\n\n")
	assert.Contains(t, doc, "### sql\n\n_When relevant: when editing SQL_\n\n")
}

// A legacy `trigger: glob` rule is path-scoped, so the claude provider moves
// it to .claude/rules and keeps it out of CLAUDE.md, like a `paths` rule.
func TestClaude_LegacyTriggerGlobSkippedFromInline(t *testing.T) {
	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	baseDir := t.TempDir()
	content := &config.ContentTree{Rules: []config.ContentFile{
		{Name: "plain", Content: "Always on."},
		{Name: "legacy", Content: "Legacy glob.", Metadata: &config.Metadata{
			Extra: map[string]string{"trigger": "glob", "glob": "src/**/*.ts"},
		}},
	}}
	outputs, err := gen.Generate(content, baseDir, &config.Config{Name: "demo", BaseDir: baseDir})
	require.NoError(t, err)
	doc := rootFile(t, outputs, "CLAUDE.md")
	assert.Contains(t, doc, "### plain")
	assert.NotContains(t, doc, "### legacy")
}
