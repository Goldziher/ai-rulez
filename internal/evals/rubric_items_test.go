package evals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFile_RubricItems(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"valid", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n  - {text: names the cluster, weight: 3}\n  - {text: no new dependency}\nrubric_min_score: 0.8\n", ""},
		{"both rubric forms", "id: a\nprompt: p\nexpect_trigger: true\nrubric: good\nrubric_items:\n  - {text: x}\n", "mutually exclusive"},
		{"empty text", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n  - {text: '  '}\n", "rubric_items[0].text is empty"},
		{"zero weight", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n  - {text: x, weight: 0}\n", "weight must be a finite number > 0"},
		{"negative weight", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n  - {text: x, weight: -2}\n", "weight must be a finite number > 0"},
		{"min score alone is still an error", "id: a\nprompt: p\nexpect_trigger: true\nrubric_min_score: 0.5\n", "needs a rubric or rubric_items"},
		{"unknown item field", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n  - {text: x, nope: 1}\n", "field nope not found"},
		{"too many items", "id: a\nprompt: p\nexpect_trigger: true\nrubric_items:\n" + strings.Repeat("  - {text: x}\n", maxRubricItems+1), "the limit is 50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cases, problems := ParseFile("c.eval.yaml", []byte(tt.body))

			// Assert
			if tt.want == "" {
				require.Empty(t, problems)
				require.Len(t, cases, 1)
				assert.True(t, cases[0].HasRubric())
				return
			}
			require.NotEmpty(t, problems)
			var all []string
			for _, p := range problems {
				all = append(all, p.Message)
			}
			assert.Contains(t, strings.Join(all, "\n"), tt.want)
		})
	}
}

func TestCase_RubricText(t *testing.T) {
	three, half := 3.0, 0.5
	tests := []struct {
		name string
		c    Case
		want string
	}{
		{name: "free text is verbatim", c: Case{Rubric: "names the cluster"}, want: "names the cluster"},
		{name: "no rubric", c: Case{}, want: ""},
		{name: "a checklist states the weights and the rule",
			c: Case{RubricItems: []RubricItem{{Text: "Registers GET /health", Weight: &three}, {Text: " No new dependency "}, {Text: "Short", Weight: &half}}},
			want: "Score the answer against this weighted checklist (total weight 4.5). The score is the sum of the weights of the items the answer satisfies divided by the total weight, a number from 0 to 1.\n" +
				"1. (weight 3) Registers GET /health\n2. (weight 1) No new dependency\n3. (weight 0.5) Short"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.RubricText())
			assert.Equal(t, tt.want != "", tt.c.HasRubric())
		})
	}
}

func TestGradeOutcome_RubricItemsUseTheRubricScore(t *testing.T) {
	c := &Case{ID: "a", ExpectTrigger: bp(true), RubricItems: []RubricItem{{Text: "x"}}}
	score := func(v float64) *Result { return &Result{RubricScore: &v} }

	assert.True(t, GradeOutcome(c, score(0.9), GradeOptions{}).Passed)
	low := GradeOutcome(c, score(0.4), GradeOptions{})
	assert.False(t, low.Passed)
	assert.Contains(t, strings.Join(low.Failures, "\n"), "rubric score 0.40 is below 0.70")
	assert.False(t, GradeOutcome(c, &Result{}, GradeOptions{}).Passed, "an ungraded checklist does not pass")
	assert.True(t, GradeOutcome(c, &Result{}, GradeOptions{}).Graded)
}

func TestBuildClaudePlugin_RubricItemsBecomeTheLLMGraderBody(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	skill := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: s\n---\n"), 0o600))
	req := &Request{Skill: SkillRef{ID: "s", Dir: skill}, Cases: []Case{
		{ID: "a", Prompt: "p", ExpectTrigger: bp(true), RubricItems: []RubricItem{{Text: "names the cluster"}}},
	}}

	// Act
	_, err := BuildClaudePlugin(dir, req)

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "evals", "a", "graders", "rubric.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "type: llm")
	assert.Contains(t, string(data), "1. (weight 1) names the cluster")
}

func TestEstimateRun_CountsAJudgeCallForRubricItems(t *testing.T) {
	counter := mustCounter(t)
	plain := &Request{Cases: []Case{{ID: "a", Prompt: "p", ExpectTrigger: bp(true)}}}
	graded := &Request{Cases: []Case{{ID: "a", Prompt: "p", ExpectTrigger: bp(true), RubricItems: []RubricItem{{Text: "x"}}}}}

	without := EstimateRun(plain, 0, 1, Price{InPerMTok: 1, OutPerMTok: 1}, counter)
	with := EstimateRun(graded, 0, 1, Price{InPerMTok: 1, OutPerMTok: 1}, counter)

	assert.Greater(t, with.InputTokens, without.InputTokens)
	assert.Greater(t, with.OutputTokens, without.OutputTokens)
}
