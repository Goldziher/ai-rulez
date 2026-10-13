package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
)

func TestGenerate_HumanLinesAroundTheManagedBlockSurvive(t *testing.T) {
	tests := []struct {
		name  string
		human string // written to .gitignore before the first generate
		// tail is what must follow "# END ai-rulez\n" byte for byte after generate.
		tail string
		// absent are block lines that a human negation must have removed.
		absent []string
	}{
		{
			name:   "negations directly under the block, file and directory alike",
			human:  gitignore.BeginMarker + "\nold\n" + gitignore.EndMarker + "\n!CLAUDE.md\n!.claude/skills/\n",
			tail:   "!CLAUDE.md\n!.claude/skills/\n",
			absent: []string{"CLAUDE.md\n", ".claude/skills/\n"},
		},
		{
			name:   "a blank line under the block is the user's and stays",
			human:  gitignore.BeginMarker + "\nold\n" + gitignore.EndMarker + "\n\n!CLAUDE.md\n",
			tail:   "\n!CLAUDE.md\n",
			absent: []string{"CLAUDE.md\n"},
		},
		{
			name:   "negations above the block",
			human:  "!.claude/skills/\n",
			absent: []string{".claude/skills/\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newPlainRepo(t, true)
			skill := filepath.Join(root, ".ai-rulez", "skills", "s", "SKILL.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
			require.NoError(t, os.WriteFile(skill, []byte("---\nname: s\ndescription: d\n---\n\nbody\n"), 0o644))
			path := filepath.Join(root, ".gitignore")
			require.NoError(t, os.WriteFile(path, []byte(tt.human), 0o644))

			// Act
			generateRepo(t, root)
			first, err := os.ReadFile(path)
			require.NoError(t, err)
			generateRepo(t, root)
			second, err := os.ReadFile(path)
			require.NoError(t, err)

			// Assert
			content := string(first)
			assert.Equal(t, content, string(second), "a second generate changes nothing")
			if tt.tail != "" {
				assert.True(t, strings.HasSuffix(content, gitignore.EndMarker+"\n"+tt.tail),
					"the lines under the block are untouched, no blank line injected:\n%s", content)
			}
			block := content[strings.Index(content, gitignore.BeginMarker):strings.Index(content, gitignore.EndMarker)]
			for _, line := range tt.absent {
				assert.NotContains(t, block, "\n"+line, "the block must not re-ignore what a negation re-includes")
			}
			assert.Contains(t, block, ".ai-rulez/.generated-manifest.local.json", "everything else stays managed")
		})
	}
}
