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
// defines. The forms are the AR301 table row plus the labelled and file-path
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
		{"labelled skill pointer", "Procedure/reference: skill ghost-skill"},
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
