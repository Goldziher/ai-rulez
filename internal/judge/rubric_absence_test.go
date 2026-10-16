package judge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

const verifyRubric = "---\nname: std\ndescription: standards\n---\n\n## C1 - Has a Verify heading\n\nThe skill ends with a `## Verify` heading.\n"

var verifySkill = map[string]string{
	".claude/skills/v/SKILL.md": "---\nname: v\ndescription: Verifies things. Use when asked to verify.\n---\n\nRun the checks and report.\n",
}

const verifyID = "rubric:std/C1"

func rubricFinding(quote string) Candidate {
	id := verifyID
	return Candidate{
		ID: "G1", Code: codeRubricFailure, Verdict: VerdictConfirmed, Severity: SeverityHigh, Confidence: ConfidenceHigh,
		Title: "No Verify heading", Location: Location{Path: ".claude/skills/v/SKILL.md", StartLine: 6, EndLine: 6}, Quote: quote,
		Why: "the criterion fails", RubricID: &id,
	}
}

func criteriaReply(t *testing.T, result string, findings ...Candidate) string {
	t.Helper()
	for i := range findings {
		findings[i].Chain, findings[i].StaticRefs, findings[i].AlsoAt = []Step{}, []string{}, []AlsoAt{}
	}
	return mustJSON(t, ReviewerOutput{Findings: findings, CriteriaResults: []CriterionResult{{RubricID: verifyID, Result: result, Note: "no such heading"}}, Coverage: Coverage{NotReviewed: []NotReviewed{}}})
}

func TestFailedCriterionAlwaysSurfacesAsAJRG06Finding(t *testing.T) {
	tests := []struct {
		name         string
		reply        func(t *testing.T) string
		wantFinding  bool
		wantAbsence  bool
		wantVerified bool
		wantQuote    string
	}{
		{"failure with no finding is an absence", func(t *testing.T) string { return criteriaReply(t, CriterionFail) }, true, true, true, ""},
		{"failure with a fabricated quote keeps the finding, quote unverified", func(t *testing.T) string { return criteriaReply(t, CriterionFail, rubricFinding("## Verify\nrun it")) }, true, true, false, ""},
		{"failure with a real quote is the reviewer's finding", func(t *testing.T) string {
			return criteriaReply(t, CriterionFail, rubricFinding("Run the checks and report."))
		}, true, false, true, "Run the checks and report."},
		{"a passed criterion raises nothing", func(t *testing.T) string { return criteriaReply(t, CriterionPass) }, false, false, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			u := skillUnit(t, verifySkill, ".claude/skills/v")
			rub, err := ParseRubric("std", "inline", verifyRubric, TierConfigured)
			require.NoError(t, err)
			cl := &scriptedClient{govern: func(llm.ChatRequest) (string, string) { return tc.reply(t), "stop" }}
			e := newTestEngine(cl, Options{Categories: []Category{CategoryGovernance}, Rubrics: map[Category][]Rubric{CategoryGovernance: {rub}}, ExplainDrops: true})

			// Act
			rep := e.Run(context.Background(), e.Plan(Input{Units: []Unit{u}}), RunMeta{})

			// Assert
			ur := rep.Units[0]
			if !tc.wantFinding {
				assert.Empty(t, ur.Findings)
				require.NotNil(t, ur.Scores.Governance)
				assert.Equal(t, 100, *ur.Scores.Governance)
				return
			}
			require.Len(t, ur.Findings, 1, "%+v", ur.Dropped)
			f := ur.Findings[0]
			assert.Equal(t, codeRubricFailure, f.Code)
			assert.Equal(t, verifyID, *f.RubricID)
			assert.Equal(t, tc.wantAbsence, f.Absence)
			assert.Equal(t, tc.wantVerified, f.Verification.QuoteVerified)
			assert.Equal(t, tc.wantQuote, f.Quote)
			require.NotNil(t, ur.Scores.Governance)
			assert.Less(t, *ur.Scores.Governance, 100, "a failed criterion lowers the score")
			assert.Equal(t, CriterionFail, ur.Criteria[0].Result)
		})
	}
}

func TestExplainDropsStillListsTheUngroundedRubricCandidate(t *testing.T) {
	// Arrange
	u := skillUnit(t, verifySkill, ".claude/skills/v")
	rub, err := ParseRubric("std", "inline", verifyRubric, TierConfigured)
	require.NoError(t, err)
	cl := &scriptedClient{govern: func(llm.ChatRequest) (string, string) {
		return criteriaReply(t, CriterionFail, rubricFinding("## Verify\nrun it")), "stop"
	}}
	e := newTestEngine(cl, Options{Categories: []Category{CategoryGovernance}, Rubrics: map[Category][]Rubric{CategoryGovernance: {rub}}, ExplainDrops: true})

	// Act
	rep := e.Run(context.Background(), e.Plan(Input{Units: []Unit{u}}), RunMeta{})

	// Assert
	require.Len(t, rep.Units[0].Dropped, 1)
	d := rep.Units[0].Dropped[0]
	assert.Equal(t, "verifier", d.Stage)
	assert.Equal(t, verifyID, d.RubricID)
}
