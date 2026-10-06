package lint

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func activationCaseFixture(config, frontmatter, cases string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":                             baseConfig + config,
		".ai-rulez/skills/deploy-staging/SKILL.md":          "---\nname: deploy-staging\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster.\n" + frontmatter + "---\nbody\n",
		".ai-rulez/skills/deploy-staging/evals/a.eval.yaml": cases,
	}
}

const positiveCase = "cases:\n  - id: ships\n    prompt: Ship the billing service to staging\n    expect_trigger: true\n    near_miss: [Explain staging]\n  - id: quiet\n    prompt: bake bread\n    expect_trigger: false\n"

func TestActivationPolicyConflict(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		cases       string
		want        int
	}{
		{name: "model invocation disabled and a positive case", frontmatter: "disable-model-invocation: true\n", cases: positiveCase, want: 1},
		{name: "implicit invocation disallowed", frontmatter: "allow_implicit_invocation: false\n", cases: positiveCase, want: 1},
		{name: "the underscore spelling counts too", frontmatter: "disable_model_invocation: true\n", cases: positiveCase, want: 1},
		{name: "a string true counts", frontmatter: "disable-model-invocation: \"true\"\n", cases: positiveCase, want: 1},
		{name: "a skill the model may invoke", frontmatter: "", cases: positiveCase, want: 0},
		{name: "disabled false is not a conflict", frontmatter: "disable-model-invocation: false\n", cases: positiveCase, want: 0},
		{name: "implicit allowed", frontmatter: "allow_implicit_invocation: true\n", cases: positiveCase, want: 0},
		{name: "only negative cases hold trivially", frontmatter: "disable-model-invocation: true\n", cases: "prompt: bake bread\nexpect_trigger: false\nid: q\n", want: 0},
		{name: "one finding per positive case", frontmatter: "disable-model-invocation: true\n",
			cases: positiveCase + "  - id: second\n    prompt: roll out billing\n    expect_trigger: true\n", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, activationCaseFixture("", tt.frontmatter, tt.cases))
			gitAdd(t, root)

			// Act
			findings := lintDir(t, root)

			// Assert
			if got := countCode(findings, CodeActivationPolicyConflict); got != tt.want {
				t.Errorf("AR9A3 count = %d, want %d: %v", got, tt.want, findings)
			}
			for _, f := range findings {
				if f.Code == CodeActivationPolicyConflict && f.Severity != SeverityWarning {
					t.Errorf("AR9A3 severity = %s, want warning", f.Severity)
				}
			}
		})
	}
}

func TestActivationPolicyConflict_PointsAtTheCase(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, activationCaseFixture("", "disable-model-invocation: true\n", positiveCase))
	gitAdd(t, root)

	findings := lintDir(t, root)

	if !has(findings, CodeActivationPolicyConflict, "skills/deploy-staging/evals/a.eval.yaml", 2) {
		t.Errorf("expected AR9A3 on the case at line 2, got %v", findings)
	}
}

func TestActivationPromptNamesSkill(t *testing.T) {
	named := func(prompt string) string {
		return "cases:\n  - id: named\n    prompt: " + prompt + "\n    expect_trigger: true\n"
	}
	tests := []struct {
		name   string
		config string
		cases  string
		want   int
	}{
		{name: "off by default", cases: named("Use deploy-staging to ship billing")},
		{name: "enabled and the prompt names the skill", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: named("Use deploy-staging to ship billing"), want: 1},
		{name: "case does not matter", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: named("run DEPLOY-STAGING now"), want: 1},
		{name: "a slash command names it", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: named("/deploy-staging billing"), want: 1},
		{name: "a different word is fine", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: named("Deploy billing to the staging environment")},
		{name: "a longer name is a different word", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: named("use deploy-staging-eu for this")},
		{name: "negative prompts may name it", config: "[lint.severity]\nAR9A4 = \"warning\"\n", cases: "prompt: tell me about deploy-staging\nexpect_trigger: false\nid: n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, activationCaseFixture(tt.config, "", tt.cases))
			gitAdd(t, root)

			findings := lintDir(t, root)

			if got := countCode(findings, CodeActivationPromptNames); got != tt.want {
				t.Errorf("AR9A4 count = %d, want %d: %v", got, tt.want, findings)
			}
		})
	}
}

func TestValidateSettings_EstimateAssumptionsMustBeNonNegative(t *testing.T) {
	tests := []struct {
		name string
		est  config.LintEvalsEstimate
		want bool
	}{
		{name: "defaults", est: config.LintEvalsEstimate{}},
		{name: "measured values", est: config.LintEvalsEstimate{OverheadTokens: 25000, AssumedOutputTokens: 400, ActivationOutputTokens: 60, ToolLoopFactor: 1.5}},
		{name: "negative overhead", est: config.LintEvalsEstimate{OverheadTokens: -5}, want: true},
		{name: "negative factor", est: config.LintEvalsEstimate{ToolLoopFactor: -0.1}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &config.LintConfig{Evals: &config.LintEvals{Estimate: &tt.est}}

			got := ValidateSettings(lc)

			if (len(got) > 0) != tt.want {
				t.Errorf("ValidateSettings = %v, want problems: %v", got, tt.want)
			}
		})
	}
}
