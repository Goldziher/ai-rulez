package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivationFromRuleFile(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    config.ActivationMode
	}{
		{"claude no frontmatter", ".claude/rules/a.md", "# body\n", config.ActivationAlways},
		{"claude paths", ".claude/rules/a.md", "---\npaths:\n  - src/**\n---\nbody\n", config.ActivationGlob},
		{"claude empty paths", ".claude/rules/a.md", "---\npaths: []\n---\nbody\n", config.ActivationAlways},
		{"cline paths", "/r/.clinerules/a.md", "---\npaths:\n  - \"**/*.go\"\n---\nbody\n", config.ActivationGlob},
		{"cline always", "/r/.clinerules/a.md", "body\n", config.ActivationAlways},
		{"cursor always", ".cursor/rules/a.mdc", "---\nalwaysApply: true\ndescription: d\n---\nb\n", config.ActivationAlways},
		{"cursor glob", ".cursor/rules/a.mdc", "---\nglobs: \"src/**\"\nalwaysApply: false\n---\nb\n", config.ActivationGlob},
		{"cursor auto", ".cursor/rules/a.mdc", "---\ndescription: use when testing\n---\nb\n", config.ActivationAuto},
		{"cursor manual", ".cursor/rules/a.mdc", "---\nalwaysApply: false\n---\nb\n", config.ActivationManual},
		{"windsurf always", ".windsurf/rules/a.md", "---\ntrigger: always_on\n---\nb\n", config.ActivationAlways},
		{"windsurf glob", ".windsurf/rules/a.md", "---\ntrigger: glob\nglobs: src/**\n---\nb\n", config.ActivationGlob},
		{"windsurf auto", ".windsurf/rules/a.md", "---\ntrigger: model_decision\ndescription: d\n---\nb\n", config.ActivationAuto},
		{"windsurf manual", ".windsurf/rules/a.md", "---\ntrigger: manual\n---\nb\n", config.ActivationManual},
		{"antigravity manual", "/r/.agents/rules/a.md", "---\ntrigger: manual\n---\nb\n", config.ActivationManual},
		{"antigravity glob", "/r/.agents/rules/a.md", "---\ntrigger: glob\nglobs: x\n---\nb\n", config.ActivationGlob},
		{"copilot always", ".github/instructions/a.instructions.md", "---\napplyTo: \"**\"\n---\nb\n", config.ActivationAlways},
		{"copilot glob", ".github/instructions/a.instructions.md", "---\napplyTo: \"src/**\"\n---\nb\n", config.ActivationGlob},
		{"copilot auto", ".github/instructions/a.instructions.md", "---\ndescription: d\n---\nb\n", config.ActivationAuto},
		{"copilot manual", ".github/instructions/a.instructions.md", "---\n---\nb\n", config.ActivationManual},
		{"continue always", ".continue/rules/a.md", "---\nname: a\nalwaysApply: true\n---\nb\n", config.ActivationAlways},
		{"continue glob", ".continue/rules/a.md", "---\nname: a\nglobs:\n  - src/**\nalwaysApply: false\n---\nb\n", config.ActivationGlob},
		{"continue auto", ".continue/rules/a.md", "---\nname: a\ndescription: d\n---\nb\n", config.ActivationAuto},
		{"continue manual", ".continue/rules/a.md", "---\nname: a\nalwaysApply: false\n---\nb\n", config.ActivationManual},
		{"junie always", ".junie/rules/a.md", "---\npaths: [x]\n---\nb\n", config.ActivationAlways},
		{"unknown path", "docs/a.md", "---\ntrigger: manual\n---\nb\n", config.ActivationAlways},
		{"malformed frontmatter", ".cursor/rules/a.mdc", "---\n: : [\n---\nb\n", config.ActivationAlways},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, activationFromRuleFile(tt.path, tt.content))
		})
	}
}

func TestInferOutputKind_RuleFiles(t *testing.T) {
	for _, path := range []string{
		".github/instructions/a.instructions.md", ".agents/rules/a.md", ".junie/rules/a.md",
		".clinerules/a.md", ".continue/rules/a.md", ".claude/rules/a.md",
		".cursor/rules/a.mdc", ".windsurf/rules/a.md",
	} {
		assert.Equal(t, config.OutputKindRuleFile, config.InferOutputKind("/repo/"+path, "/repo"), path)
	}
}

// ruleFileTokenReport builds a claude report for a project with one always-on
// rule and one path-scoped rule.
func ruleFileTokenReport(t *testing.T) *TokenReport {
	t.Helper()
	dir := t.TempDir()
	rules := filepath.Join(dir, ".ai-rulez", "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.yaml"),
		[]byte("version: \"3.0\"\nname: t\npresets:\n  - claude\ngitignore: false\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "always-rule.md"),
		[]byte("---\npriority: high\n---\n\nAlways on rule body.\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "go-rule.md"),
		[]byte("---\nactivation: glob\npaths:\n  - \"**/*.go\"\n---\n\nGo only rule body with several more words in it.\n"), 0o600))
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	report, err := NewGenerator(cfg).TokenReport(TokenReportOptions{Counter: tokens.CL100KBase()})
	require.NoError(t, err)
	return report
}

func TestTokenReport_ScopedRuleFilesConditional(t *testing.T) {
	// Arrange / Act
	report := ruleFileTokenReport(t)
	runtime := findRuntime(t, report, "claude")

	// Assert
	scoped := findEntry(t, runtime, "path-scoped rule files")
	assert.Equal(t, BucketConditional, scoped.Bucket)
	assert.Positive(t, scoped.Tokens)
	assert.Equal(t, 1, scoped.Artifacts)
	assert.GreaterOrEqual(t, runtime.Conditional, scoped.Tokens)

	// The always-on rule stays inlined in the root file, which is always loaded.
	assert.Positive(t, runtime.Always)
	for _, entry := range runtime.Entries {
		if entry.Label != "path-scoped rule files" {
			assert.NotEqual(t, BucketConditional, entry.Bucket, entry.Label)
		}
	}
}
