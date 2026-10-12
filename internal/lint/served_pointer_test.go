package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// #363: AR990 flags a static pointer to a served skill by source path or by a
// hyphenated bare name in prose, and suggests `load_skill <name>`.
func TestStrict_ServedSkillReferencedByPathAndProse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"domain source path", "Read .ai-rulez/domains/sra/skills/dev-cycle/SKILL.md first."},
		{"domain source references path", "See .ai-rulez/domains/sra/skills/dev-cycle/references/terminal-ttyd-tmux.md."},
		{"root source path", "Read .ai-rulez/skills/dev-cycle/SKILL.md first."},
		{"ellipsis references path", "See .../skills/dev-cycle/references/x.md for details."},
		{"harness references path", "See .claude/skills/dev-cycle/references/x.md."},
		{"see skill hyphenated", "For releases, see skill dev-cycle."},
		{"load skill hyphenated", "Load skill dev-cycle before starting."},
		{"see the skill hyphenated", "Please see the skill dev-cycle."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":               baseConfig,
				".ai-rulez/skills/dev-cycle/SKILL.md": "---\nname: dev-cycle\ndescription: The development cycle. Use when following the project workflow.\ndelivery: served\n---\nBODY.\n",
				".ai-rulez/rules/use.md":              "# Use\n\n" + tt.body + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			assert.True(t, has(fs, CodeServedReferencedStatically, "rules/use.md", 0), "%q must raise AR990:\n%s", tt.body, dump(fs))
			assert.Contains(t, messages(fs, CodeServedReferencedStatically), "load_skill dev-cycle", "the message suggests the load_skill call")
		})
	}
}

// The new AR990 pointer forms must not widen AR301: prose about skills, a
// non-hyphenated word after "skill", and another tree's skills/ dir stay clean (#352).
func TestReferenceUnknown_ServedPointerFormsStayPrecise(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"unknown domain source path", "Read .ai-rulez/domains/sra/skills/ghost-skill/SKILL.md.", 1},
		{"see skill plain word", "For details see skill authoring guidance.", 0},
		{"see skill-based prose", "See skill-based design notes.", 0},
		{"see skill hyphenated word in prose", "We use skill sets to group work.", 0},
		{"a both skill", "That is a `both` skill.", 0},
		{"other tree", "Agent skills live at `src/mastra/public/skills/shared/<name>/SKILL.md`.", 0},
		{"other tree domains", "See `src/domains/sra/skills/shared/x.md`.", 0},
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
