package lint

import "testing"

func TestChecksAreScanned(t *testing.T) {
	const secret = "---\ndescription: Review\nseverity: high\n---\n\nUse key AKIAIOSFODNN7EXAMPLE when reviewing.\n"
	runRuleCases(t, []ruleCase{
		{name: "root check", files: map[string]string{".ai-rulez/checks/review.md": secret}, want: []string{"AR001:checks/review.md:6"}},
		{name: "domain check", files: map[string]string{".ai-rulez/domains/web/checks/review.md": secret}, want: []string{"AR001:checks/review.md:6"}},
		{
			name:   "a clean check raises nothing",
			files:  map[string]string{".ai-rulez/checks/review.md": "---\ndescription: Review\nseverity: high\n---\n\nCheck the diff.\n"},
			absent: []string{"AR001"},
		},
	})
}
