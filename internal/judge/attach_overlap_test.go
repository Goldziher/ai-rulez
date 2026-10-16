package judge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleUnitAttachesOverlappingScopedRules(t *testing.T) {
	files := map[string]string{
		".claude/rules/self.md":  "---\npaths: [\"*.go\"]\n---\nUse Go.\n",
		".claude/rules/other.md": "---\npaths: [\"*.go\"]\n---\nWrap errors.\n",
	}
	u := BuildUnit(Ref{Kind: KindRule, Path: ".claude/rules/self.md", Primary: ".claude/rules/self.md", Members: []string{".claude/rules/self.md"}}, memRead(files), UnitOptions{})
	repo := Repo{Files: []string{".claude/rules/self.md", ".claude/rules/other.md"}, Read: func(p string) (string, bool) { s, ok := files[p]; return s, ok }}
	att := Attach(repo, u, AttachOptions{Rules: true})
	require.Len(t, att.Conventions, 1)
	require.Equal(t, WhyOverlap, att.Conventions[0].Why)
	require.Equal(t, "rule:.claude/rules/other.md", att.Conventions[0].ID)
}
