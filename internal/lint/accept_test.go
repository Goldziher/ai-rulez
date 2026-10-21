package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestAcceptEntriesAreValidated(t *testing.T) {
	ok := config.LintAccept{Rule: "AR401", Path: "a.md", Match: "x", Reason: "because"}
	with := func(f func(*config.LintAccept)) *config.LintConfig {
		e := ok
		f(&e)
		return &config.LintConfig{Accept: []config.LintAccept{e}}
	}
	tests := []struct {
		name string
		lc   *config.LintConfig
		want string // substring of the single problem, "" for none
	}{
		{"valid code", with(func(*config.LintAccept) {}), ""},
		{"valid name", with(func(e *config.LintAccept) { e.Rule = "path-missing" }), ""},
		{"line only", with(func(e *config.LintAccept) { e.Match, e.Line = "", 3 }), ""},
		{"no reason", with(func(e *config.LintAccept) { e.Reason = "" }), "reason"},
		{"blank reason", with(func(e *config.LintAccept) { e.Reason = "  \t" }), "reason"},
		{"no rule", with(func(e *config.LintAccept) { e.Rule = "" }), "rule"},
		{"unknown rule", with(func(e *config.LintAccept) { e.Rule = "AR999" }), "unknown rule"},
		{"wildcard rule", with(func(e *config.LintAccept) { e.Rule = "AR4*" }), "one code"},
		{"bad code format", with(func(e *config.LintAccept) { e.Rule = "ar-4" }), "unknown rule"},
		{"no path", with(func(e *config.LintAccept) { e.Path = "" }), "path"},
		{"malformed glob", with(func(e *config.LintAccept) { e.Path = "a/[b" }), "malformed"},
		{"no selector", with(func(e *config.LintAccept) { e.Match = "" }), "match"},
		{"negative line", with(func(e *config.LintAccept) { e.Line = -1 }), "line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSettings(tt.lc)

			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1, "%v", got)
			assert.Contains(t, got[0], "lint.accept[0]")
			assert.Contains(t, got[0], tt.want)
		})
	}
}

func TestAcceptMarksOnlyMatchingFindings(t *testing.T) {
	const body = "See `src/gone.py`.\nSee `src/other.py`."
	accept := func(extra string) string {
		return "\n[[lint.accept]]\nrule = \"AR401\"\npath = \"rules/doc.md\"\nreason = \"documented\"\n" + extra
	}
	tests := []struct {
		name         string
		cfg          string
		wantAccepted []string // substrings of the accepted AR401 messages
		wantOpen     int      // AR401 findings still reported as not accepted
		wantStale    int
	}{
		{"none", "", nil, 2, 0},
		{"match one", accept("match = \"gone.py\"\n"), []string{"gone.py"}, 1, 0},
		{"rule by name", "\n[[lint.accept]]\nrule = \"path-missing\"\npath = \"rules/doc.md\"\nmatch = \"other.py\"\nreason = \"r\"\n", []string{"other.py"}, 1, 0},
		{"match both", accept("match = \"src/\"\n"), []string{"gone.py", "other.py"}, 0, 0},
		{"line", accept("line = 2\n"), []string{"gone.py"}, 1, 0},
		{"line elsewhere", accept("line = 9\n"), nil, 2, 1},
		{"match and line must both hold", accept("match = \"gone.py\"\nline = 9\n"), nil, 2, 1},
		{"glob path", "\n[[lint.accept]]\nrule = \"AR401\"\npath = \"rules/**\"\nmatch = \"gone.py\"\nreason = \"r\"\n", []string{"gone.py"}, 1, 0},
		{"repo relative path", "\n[[lint.accept]]\nrule = \"AR401\"\npath = \".ai-rulez/rules/doc.md\"\nmatch = \"gone.py\"\nreason = \"r\"\n", []string{"gone.py"}, 1, 0},
		{"unmatched text is stale", accept("match = \"nothing here\"\n"), nil, 2, 1},
		{"other path is stale", "\n[[lint.accept]]\nrule = \"AR401\"\npath = \"rules/gone.md\"\nmatch = \"x\"\nreason = \"r\"\n", nil, 2, 1},
		{"other rule is stale", "\n[[lint.accept]]\nrule = \"AR402\"\npath = \"rules/doc.md\"\nmatch = \"gone.py\"\nreason = \"r\"\n", nil, 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree("", tt.cfg, map[string]string{
				".ai-rulez/rules/doc.md": "# Doc\n" + body + "\n",
			}))
			gitAdd(t, root)

			fs := lintDir(t, root)

			var accepted []string
			open, stale := 0, 0
			for i := range fs {
				switch {
				case fs[i].Code == CodeAcceptUnused:
					stale++
					assert.Contains(t, fs[i].File, "config.toml")
				case fs[i].Code == CodePathMissing && fs[i].IsAccepted():
					accepted = append(accepted, fs[i].Message)
					assert.NotEmpty(t, fs[i].Meta.AcceptReason)
				case fs[i].Code == CodePathMissing:
					open++
				}
			}
			assert.Len(t, accepted, len(tt.wantAccepted), dump(fs))
			for _, want := range tt.wantAccepted {
				assert.True(t, anyContains(accepted, want), "%q not accepted:\n%s", want, dump(fs))
			}
			assert.Equal(t, tt.wantOpen, open, dump(fs))
			assert.Equal(t, tt.wantStale, stale, dump(fs))
		})
	}
}

func TestAcceptedFindingsDoNotFailTheRun(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, precisionTree("See `src/gone.py`.", "\n[[lint.accept]]\nrule = \"AR401\"\npath = \"rules/doc.md\"\nmatch = \"gone.py\"\nreason = \"r\"\n", nil))
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.False(t, Failed(fs, "warning"), dump(fs))
}

func TestAcceptIsOnlyJudgedForLintedFiles(t *testing.T) {
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Use when running the " + name + " tasks.\nallowed-tools: Bash(*)\n---\n# " + name + "\n"
	}
	cfg := "\n[[lint.accept]]\nrule = \"AR007\"\npath = \".claude/skills/foo/SKILL.md\"\nmatch = \"no such tool\"\nreason = \"r\"\n"
	tests := []struct {
		name      string
		spec      TargetSpec
		wantStale bool
	}{
		{"detected run lints the file", TargetSpec{Detect: true}, true},
		{"run on another file says nothing", TargetSpec{Paths: []string{".claude/skills/bar/SKILL.md"}}, false},
		{"run on the file judges it", TargetSpec{Paths: []string{".claude/skills/foo/SKILL.md"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree("", cfg, map[string]string{
				".claude/skills/foo/SKILL.md": skill("foo"),
				".claude/skills/bar/SKILL.md": skill("bar"),
			}))
			gitAdd(t, root)

			fs, _ := lintWithTargets(t, root, tt.spec)

			stale := false
			for i := range fs {
				stale = stale || fs[i].Code == CodeAcceptUnused
			}
			assert.Equal(t, tt.wantStale, stale, dump(fs))
		})
	}
}

func anyContains(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
