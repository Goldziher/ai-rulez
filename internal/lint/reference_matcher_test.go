package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A backticked term followed by the plural noun "skills" (or "rules", "agents")
// is prose — the term modifies the noun — not a pointer to a name. AR301 must
// stay quiet (#339).
func TestReferenceUnknown_PluralKindWordAfterBacktickIsProse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"skill tool line", "the Skill tool refuses `disable-model-invocation` skills"},
		{"plural rules", "Also the `ghost-thing` rules are irrelevant here."},
		{"plural agents", "The `ghost-thing` agents are configured elsewhere."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", nil))
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Zero(t, countCode(fs, CodeReferenceUnknown), "%q must not raise AR301:\n%s", tt.body, dump(fs))
		})
	}
}

// Every documented pointer form still raises AR301 for a name no namespace
// defines. The forms are the AR301 table row plus the labeled and file-path
// forms AR990 shares.
func TestReferenceUnknown_DocumentedPointerFormsFire(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"backticked name then kind", "Run the `ghost-skill` skill."},
		{"kind then backticked name", "Run skill `ghost-skill`."},
		{"slash command", "/ghost-skill"},
		{"skill call", "Run Skill(ghost-skill)."},
		{"labeled skill pointer", "Procedure/reference: skill ghost-skill"},
		{"skills directory", "Read skills/ghost-skill for the steps."},
		{"harness skill file", "Edit .claude/skills/ghost-skill/SKILL.md."},
		{"subagent type", "subagent_type: ghost-skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", nil))
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.True(t, has(fs, CodeReferenceUnknown, "rules/doc.md", 0), "%q must raise AR301:\n%s", tt.body, dump(fs))
		})
	}
}

// AR301 honors the inline ignore every other rule does, so an intentional prose
// pointer can be silenced on its line.
func TestReferenceUnknown_InlineIgnoreSilencesAR301(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, precisionTree("<!-- ai-rulez-lint-ignore: AR301 -->\nRun the `ghost-skill` skill.", "", nil))
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.Zero(t, countCode(fs, CodeReferenceUnknown), "%s", dump(fs))
}

// A backticked adjective before a singular kind noun is prose, not a pointer:
// "a `both` skill" describes a delivery kind, and "a `served` skill" is a skill
// whose delivery is served. AR301 must stay quiet (#352).
func TestReferenceUnknown_BacktickedAdjectiveBeforeKindIsProse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"article before skill", `to a ` + "`both`" + ` skill it says "read the file".`},
		{"capitalized article", "A `served` skill is delivered over MCP."},
		{"article before rule", "Add an `honor` rule for the reviewer."},
		{"article before agent", "Spawn a `both` agent for the task."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", nil))
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Zero(t, countCode(fs, CodeReferenceUnknown), "%q must not raise AR301:\n%s", tt.body, dump(fs))
		})
	}
}

// A "skills/" path segment nested in some other tree is not this project's skill
// tree, so it is not a pointer to a skill; a top-level "skills/x" and a harness
// ".claude/skills/x/SKILL.md" still are (#352).
func TestReferenceUnknown_SkillDirectoryPointerPrecision(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"top-level skills dir", "Read skills/ghost-skill for the steps.", 1},
		{"harness skills file", "Edit .claude/skills/ghost-skill/SKILL.md.", 1},
		{"nested other tree", "Agent skills live at `src/mastra/public/skills/shared/<name>/SKILL.md`.", 0},
		{"nested source tree", "See `vendor/lib/skills/shared/SKILL.md`.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", nil))
			gitAdd(t, root)

			fs := lintDir(t, root)

			if got := countCode(fs, CodeReferenceUnknown); got != tt.want {
				t.Errorf("AR301 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

// A singular "`x` rule" after a possessive or a tool name names a linter or
// business rule, not a rule item of the project (#416). A project with no rule
// items has nothing to point at, so no "`x` rule" is a reference there.
func TestReferenceUnknown_ToolRuleIsProse(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		extra map[string]string
		cfg   string
		want  int
	}{
		{"possessive tool", "The frontend ran eslint's `camelcase` rule on locale identifiers.", nil, "", 0},
		{"curly possessive", "We kept ruff\u2019s `E501` rule off, and clippy's `needless-return` rule on.", nil, "", 0},
		{"tool name before", "Enable the eslint `no-console` rule for scripts.", nil, "", 0},
		{"tool name with the", "Disable the stylelint `declaration-block-no-duplicate-properties` rule.", nil, "", 0},
		{"plain rule, project with rules", "Follow the `ghost-thing` rule.", nil, "", 1},
		{"true positive follows tool-less prose", "Please follow the `real-rule` rule.", nil, "", 1},
		{"existing rule is fine", "Please follow the `real-rule` rule.",
			map[string]string{".ai-rulez/rules/real-rule.md": "# Real\nbody\n"}, "", 0},
		{"known_names silences a rule name", "Follow the `display` rule.", nil, "\n[lint]\nknown_names = [\"display\"]\n", 0},
		{"skill after possessive still fires", "Run eslint's `ghost-skill` skill.", nil, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, tt.cfg, tt.extra))
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Equal(t, tt.want, countCode(fs, CodeReferenceUnknown), "%q:\n%s", tt.body, dump(fs))
		})
	}
}

// With no rule item anywhere, a "`x` rule" cannot point at anything, so it is
// prose; the issue's repro lives in a skill of a tree with no rules (#416).
func TestReferenceUnknown_NoRuleItemsMeansRuleWordIsProse(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		"CLAUDE.md":             "# root\n",
		".claude/skills/lint-notes/SKILL.md": "---\nname: lint-notes\ndescription: Explains why the frontend lint config changed.\n---\n# Lint notes\n\n" +
			"We disabled the `no-console` rule for scripts and kept the `display` rule for tables.\n",
	})
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.Zero(t, countCode(fs, CodeReferenceUnknown), "%s", dump(fs))
}
