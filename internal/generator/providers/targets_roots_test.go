package providers_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func targeted(name string, targets ...string) config.ContentFile {
	return config.ContentFile{
		Name: name, Content: "BODY_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
		Path: "/p/.ai-rulez/rules/" + name + ".md", Metadata: &config.Metadata{Targets: targets},
	}
}

func targetedContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			targeted("free"),
			targeted("to-codex", "codex"),
			targeted("to-gemini", "gemini"),
			targeted("to-claude", "claude"),
			targeted("to-cursor", "cursor"),
			targeted("skill-only", ".claude/skills/*/SKILL.md"),
			targeted("windows", `.\GEMINI.md`),
		},
		Context: []config.ContentFile{targeted("ctx-free"), targeted("ctx-cursor", "cursor"), targeted("ctx-codex", "codex")},
	}
}

func assertBodies(t *testing.T, doc string, want, notWant []string) {
	t.Helper()
	for _, b := range want {
		assert.Contains(t, doc, b)
	}
	for _, b := range notWant {
		assert.NotContains(t, doc, b)
	}
}

// A rule naming any preset that writes AGENTS.md lands in it for all of them.
func TestAgentsMD_TargetsIdenticalAcrossSharingPresets(t *testing.T) {
	gens := presetGenerators(t)
	var first, firstName string
	for _, name := range []string{"codex", "opencode", "xum", "amp"} {
		baseDir := t.TempDir()
		outputs, err := gens[name].Generate(targetedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}})
		require.NoError(t, err)
		doc := rootFile(t, outputs, "AGENTS.md")
		assertBodies(t, doc,
			[]string{"BODY_FREE", "BODY_TO_CODEX", "BODY_CTX_FREE", "BODY_CTX_CODEX"},
			[]string{"BODY_TO_GEMINI", "BODY_TO_CLAUDE", "BODY_TO_CURSOR", "BODY_SKILL_ONLY", "BODY_CTX_CURSOR"})
		if first == "" {
			first, firstName = doc, name
			continue
		}
		assert.Equal(t, first, doc, "AGENTS.md from %s differs from %s", name, firstName)
	}
}

// GEMINI.md is written by gemini and antigravity; both must keep the same
// items whichever writes last. Windows-style and ./ targets are accepted.
func TestGeminiMD_TargetsIdenticalAcrossGeminiAndAntigravity(t *testing.T) {
	gens := presetGenerators(t)
	var docs []string
	for _, name := range []string{"gemini", "antigravity"} {
		baseDir := t.TempDir()
		outputs, err := gens[name].Generate(targetedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}})
		require.NoError(t, err)
		doc := rootFile(t, outputs, "GEMINI.md")
		assertBodies(t, doc,
			[]string{"BODY_FREE", "BODY_TO_GEMINI", "BODY_WINDOWS", "BODY_CTX_FREE"},
			[]string{"BODY_TO_CODEX", "BODY_TO_CLAUDE", "BODY_TO_CURSOR", "BODY_SKILL_ONLY", "BODY_CTX_CURSOR"})
		docs = append(docs, extractSections(doc))
	}
	assert.Equal(t, docs[0], docs[1], "rule sections must not depend on which preset writes GEMINI.md")
}

// extractSections returns the document from the first rules heading on, to
// skip preset-specific headers.
func extractSections(doc string) string {
	if i := strings.Index(doc, "## Rules"); i >= 0 {
		return doc[i:]
	}
	return doc
}

func TestGeminiMD_TargetsOrderIndependent(t *testing.T) {
	gens := presetGenerators(t)
	render := func(first, second string) string {
		var last string
		for _, name := range []string{first, second} {
			baseDir := t.TempDir()
			outputs, err := gens[name].Generate(targetedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}})
			require.NoError(t, err)
			last = extractSections(rootFile(t, outputs, "GEMINI.md"))
		}
		return last
	}

	assert.Equal(t, render("gemini", "antigravity"), render("antigravity", "gemini"))
}

func TestHermes_TargetsFilterRoot(t *testing.T) {
	baseDir := t.TempDir()

	outputs, err := presetGenerators(t)["hermes"].Generate(targetedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir})

	require.NoError(t, err)
	assertBodies(t, rootFile(t, outputs, ".hermes.md"),
		[]string{"BODY_FREE", "BODY_CTX_FREE"},
		[]string{"BODY_TO_CODEX", "BODY_TO_CURSOR", "BODY_SKILL_ONLY", "BODY_CTX_CURSOR"})
}

func TestCopilot_TargetsFilterRootAndFiles(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantFiles []string
		wantRoot  []string
		notRoot   []string
	}{
		{
			name: "inline", mode: "inline",
			wantFiles: []string{".github/instructions/dir-only.instructions.md"},
			wantRoot:  []string{"BODY_FREE", "BODY_TO_COPILOT", "BODY_ROOT_ONLY"},
			notRoot:   []string{"BODY_TO_CURSOR", "BODY_SKILL_ONLY", "BODY_DIR_ONLY", "BODY_CTX_CURSOR"},
		},
		{
			name: "split", mode: "split",
			wantFiles: []string{
				".github/instructions/free.instructions.md", ".github/instructions/to-copilot.instructions.md",
				".github/instructions/dir-only.instructions.md", ".github/instructions/github-dir.instructions.md",
			},
			notRoot: []string{"BODY_TO_CURSOR", "BODY_SKILL_ONLY", "BODY_CTX_CURSOR"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			content := &config.ContentTree{Rules: []config.ContentFile{
				targeted("free"), targeted("to-copilot", "copilot"), targeted("to-cursor", "cursor"),
				targeted("skill-only", ".claude/skills/*/SKILL.md"),
				targeted("dir-only", ".github/instructions/"),
				targeted("github-dir", `.\.github\`),
				targeted("root-only", ".github/copilot-instructions.md"),
			}, Context: []config.ContentFile{targeted("ctx-cursor", "cursor")}}
			cfg := &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: tt.mode}}

			// Act
			outputs, err := presetGenerators(t)["copilot"].Generate(content, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			for _, f := range tt.wantFiles {
				_, ok := findOutput(outputs, f)
				assert.True(t, ok, "missing %s", f)
			}
			_, ok := findOutput(outputs, "to-cursor.instructions.md")
			assert.False(t, ok)
			assertBodies(t, rootFile(t, outputs, "copilot-instructions.md"), tt.wantRoot, tt.notRoot)
		})
	}
}

func TestAntigravity_DirTargetedRuleBecomesFileInInlineMode(t *testing.T) {
	baseDir := t.TempDir()
	content := &config.ContentTree{Rules: []config.ContentFile{
		targeted("free"), targeted("dir-only", ".agents/rules/"), targeted("to-cursor", "cursor"),
	}}

	outputs, err := presetGenerators(t)["antigravity"].Generate(content, baseDir, &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}})

	require.NoError(t, err)
	_, ok := findOutput(outputs, ".agents/rules/dir-only.md")
	assert.True(t, ok, "a rule targeted only at the rules folder is written as a file")
	_, ok = findOutput(outputs, ".agents/rules/to-cursor.md")
	assert.False(t, ok)
	assertBodies(t, rootFile(t, outputs, "GEMINI.md"), []string{"BODY_FREE"}, []string{"BODY_DIR_ONLY", "BODY_TO_CURSOR"})
}

func TestJunie_TargetsFilterRootAndFiles(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantFiles []string
		wantRoot  []string
		notRoot   []string
	}{
		{
			name: "inline", mode: "inline",
			wantRoot: []string{"BODY_FREE", "BODY_TO_JUNIE"},
			notRoot:  []string{"BODY_TO_CURSOR", "BODY_RULES_ONLY", "BODY_JUNIE_DIR"},
		},
		{
			name: "split", mode: "split",
			wantFiles: []string{".junie/rules/free.md", ".junie/rules/to-junie.md", ".junie/rules/rules-only.md", ".junie/rules/junie-dir.md"},
			notRoot:   []string{"BODY_TO_CURSOR"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			gen, err := providers.LoadBuiltin("junie")
			require.NoError(t, err)
			baseDir := t.TempDir()
			content := &config.ContentTree{Rules: []config.ContentFile{
				targeted("free"), targeted("to-junie", "JUNIE"), targeted("to-cursor", "cursor"),
				targeted("rules-only", ".junie/rules/"), targeted("junie-dir", ".junie/"),
			}}
			cfg := &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: tt.mode}}

			// Act
			outputs, err := gen.Generate(content, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			for _, f := range tt.wantFiles {
				_, ok := findOutput(outputs, f)
				assert.True(t, ok, "missing %s", f)
			}
			assertBodies(t, rootFile(t, outputs, "AGENTS.md"), tt.wantRoot, tt.notRoot)
		})
	}
}

func TestClaude_TargetsRootExcludesSkillOnlyAndRoutesDirOnly(t *testing.T) {
	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	baseDir := t.TempDir()

	outputs, err := gen.Generate(targetedContent(), baseDir, &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}})

	require.NoError(t, err)
	assertBodies(t, rootFile(t, outputs, "CLAUDE.md"),
		[]string{"BODY_FREE", "BODY_TO_CLAUDE", "BODY_CTX_FREE"},
		[]string{"BODY_SKILL_ONLY", "BODY_TO_CODEX", "BODY_TO_GEMINI", "BODY_TO_CURSOR", "BODY_CTX_CURSOR"})
}

func TestRenderLocalRoot_TargetsFilter(t *testing.T) {
	cfg := &config.Config{Name: "demo", Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
	tests := []struct {
		file    string
		want    []string
		notWant []string
	}{
		{"CLAUDE.local.md", []string{"BODY_FREE", "BODY_TO_CLAUDE"}, []string{"BODY_TO_CODEX", "BODY_TO_CURSOR"}},
		{"AGENTS.local.md", []string{"BODY_FREE", "BODY_TO_CODEX"}, []string{"BODY_TO_CLAUDE", "BODY_TO_CURSOR"}},
		{"GEMINI.local.md", []string{"BODY_FREE", "BODY_TO_GEMINI"}, []string{"BODY_TO_CODEX", "BODY_TO_CURSOR"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			doc := presetsRenderLocalRoot(targetedContent(), cfg, tt.file)

			assertBodies(t, doc, tt.want, tt.notWant)
		})
	}
}

func presetsRenderLocalRoot(c *config.ContentTree, cfg *config.Config, file string) string {
	return presets.RenderLocalRootRules(c, presets.AllInlineRules(c), cfg, file)
}
