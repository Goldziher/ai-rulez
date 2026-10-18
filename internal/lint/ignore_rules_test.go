package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestIgnoreRulesScopeFindingsToPaths(t *testing.T) {
	const missing = "See `src/gone.py`."
	tests := []struct {
		name      string
		cfg       string
		wantDoc   int // AR401 findings in rules/doc.md
		wantOther int // AR401 findings in rules/other.md
	}{
		{"none", "", 1, 1},
		{"code and path", "[[lint.ignore_rules]]\nrules = [\"AR401\"]\npaths = [\"rules/doc.md\"]\n", 0, 1},
		{"name", "[[lint.ignore_rules]]\nrules = [\"path-missing\"]\npaths = [\"rules/doc.md\"]\n", 0, 1},
		{"prefix glob", "[[lint.ignore_rules]]\nrules = [\"AR4*\"]\npaths = [\"rules/**\"]\n", 0, 0},
		{"other rule family", "[[lint.ignore_rules]]\nrules = [\"AR9*\"]\npaths = [\"rules/**\"]\n", 1, 1},
		{"other path", "[[lint.ignore_rules]]\nrules = [\"AR401\"]\npaths = [\"skills/**\"]\n", 1, 1},
		{"repo relative path", "[[lint.ignore_rules]]\nrules = [\"AR401\"]\npaths = [\".ai-rulez/rules/other.md\"]\n", 1, 0},
		{"two entries", "[[lint.ignore_rules]]\nrules = [\"AR401\"]\npaths = [\"rules/doc.md\"]\n[[lint.ignore_rules]]\nrules = [\"AR401\"]\npaths = [\"rules/other.md\"]\n", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := ""
			if tt.cfg != "" {
				cfg = "\n" + tt.cfg
			}
			writeFiles(t, root, precisionTree(missing, cfg, map[string]string{
				".ai-rulez/rules/other.md": "# Other\n" + missing + "\n",
			}))
			gitAdd(t, root)

			fs := lintDir(t, root)

			doc, other := 0, 0
			for _, f := range fs {
				if f.Code != CodePathMissing {
					continue
				}
				if strings.HasSuffix(f.File, "rules/doc.md") {
					doc++
				}
				if strings.HasSuffix(f.File, "rules/other.md") {
					other++
				}
			}
			assert.Equal(t, tt.wantDoc, doc, "doc.md:\n%s", dump(fs))
			assert.Equal(t, tt.wantOther, other, "other.md:\n%s", dump(fs))
		})
	}
}

func TestValidateSettingsIgnoreRules(t *testing.T) {
	rule := func(rules []string, paths []string) *config.LintConfig {
		return &config.LintConfig{IgnoreRules: []config.LintIgnoreRule{{Rules: rules, Paths: paths}}}
	}
	tests := []struct {
		name string
		lc   *config.LintConfig
		want string // substring of the single problem, "" for none
	}{
		{"valid code", rule([]string{"AR901"}, []string{"playground/**"}), ""},
		{"valid name", rule([]string{"path-missing"}, []string{"a/**"}), ""},
		{"valid prefix glob", rule([]string{"AR9*"}, []string{"a/**"}), ""},
		{"unknown code", rule([]string{"AR999"}, []string{"a/**"}), "unknown rule"},
		{"glob matching nothing", rule([]string{"ZZ*"}, []string{"a/**"}), "matches no rule"},
		{"no rules", rule(nil, []string{"a/**"}), "rules"},
		{"no paths", rule([]string{"AR901"}, nil), "paths"},
		{"blank path", rule([]string{"AR901"}, []string{" "}), "paths"},
		{"malformed path glob", rule([]string{"AR901"}, []string{"a/[b"}), "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSettings(tt.lc)

			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1, "%v", got)
			assert.Contains(t, got[0], "lint.ignore_rules[0]")
			assert.Contains(t, got[0], tt.want)
		})
	}
}
