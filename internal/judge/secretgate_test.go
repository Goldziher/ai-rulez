package judge

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// The judge withholds a unit exactly when the security scan (AR001) reports a credential in it.
func TestSecretGateAgreesWithTheSecurityScan(t *testing.T) {
	tests := []struct {
		name, body string
		withheld   bool
		reason     []string
	}{
		{"session token lookup", `aws_session_token=credentials["SessionToken"]`, false, nil},
		{"vault read", `secret = vault.read_secret_value(name)`, false, nil},
		{"aws access key id", "ok\nkey AKIAIOSFODNN7EXAMPLE\n", true, []string{":7 (AWS access key id)"}},
		{"quoted password", `password: "Xk9fQ2pLm4Zr8TvB1nWc"`, true, []string{":6 (hard-coded credential)"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{".claude/skills/k/SKILL.md": "---\nname: k\ndescription: d. Use when d.\n---\n\n" + tc.body + "\n"}
			refs := Discover([]string{".claude/skills/k/SKILL.md"}, false)

			// Act
			u := BuildUnit(refs[0], memRead(files), UnitOptions{})

			// Assert
			if !tc.withheld {
				assert.Empty(t, u.Status)
				assert.False(t, u.Files[0].Redacted)
				assert.Contains(t, strings.Join(u.Files[0].Lines, "\n"), tc.body)
				return
			}
			assert.Equal(t, StatusWithheld, u.Status)
			assert.Contains(t, u.Reason, ".claude/skills/k/SKILL.md")
			for _, want := range tc.reason {
				assert.Contains(t, u.Reason, want)
			}
			assert.NotContains(t, u.Reason, "AKIAIOSFODNN7EXAMPLE")
			assert.NotContains(t, u.Reason, "Xk9fQ2pLm4Zr8TvB1nWc")
		})
	}
}

func TestEveryJudgeRequestMasksPromptSecrets(t *testing.T) {
	const value = "private-configured-credential-value"
	const shaped = "sk-live-credential-value-for-test"
	u := skillUnit(t, exfilSkill, ".claude/skills/sync")
	rub, err := ParseRubric("private", "inline", "## C1 - Secret\n"+value+" "+shaped, TierReference)
	require.NoError(t, err)
	cl := &scriptedClient{security: func(llm.ChatRequest) (string, string) { return reviewerReply(t, exfilFinding()), "stop" }}
	e := newTestEngine(cl, Options{
		Categories: []Category{CategorySecurity},
		Attach:     AttachOptions{Secrets: []string{value}},
		Rubrics:    map[Category][]Rubric{CategorySecurity: {rub}},
		Prompts: PromptSet{
			Override:  map[Category]string{CategorySecurity: "ROLE: security reviewer\n" + value + " " + shaped},
			RepoTraps: []Trap{{ID: "private", Text: value + " " + shaped}},
		},
	})
	e.Run(context.Background(), e.Plan(Input{Units: []Unit{u}}), RunMeta{})
	require.NotEmpty(t, cl.calls)
	require.Positive(t, cl.count("ROLE: adjudicator"))
	for _, req := range cl.calls {
		for _, msg := range req.Messages {
			assert.NotContains(t, msg.Content, value)
			assert.NotContains(t, msg.Content, shaped)
		}
	}
}
