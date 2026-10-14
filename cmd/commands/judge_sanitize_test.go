package commands

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

const judgeEscape = "\x1b[2J\x1b]0;owned\x07"

func TestJudgeStripsTerminalEscapesFromPathsAndReasons(t *testing.T) {
	skipIfNoControlCharsInNames(t)
	tests := []struct {
		name string
		args []string
	}{
		{"text", nil},
		{"json", []string{"--format", "json"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{
				"docs/a" + judgeEscape + ".md": "Run tests.\n",
				"docs/b.md":                    "Keep it short.\n",
			})
			rig := newJudgeRig(t)
			rig.reply = func(req llm.ChatRequest) (string, error) {
				if strings.Contains(req.Messages[len(req.Messages)-1].Content, "docs/b.md") {
					return judgeGoodReply, nil
				}
				return "", errors.New("provider said " + judgeEscape + " no")
			}

			// Act
			stdout, code := rig.run(t, append([]string{"--allow-llm", "--model", judgeTestModel, "--no-cache", "docs"}, tc.args...)...)

			// Assert
			require.Equal(t, 1, code, rig.stderr.String())
			assert.NotContains(t, stdout, "\x1b")
			assert.NotContains(t, stdout, "\x07")
			assert.NotContains(t, stdout, `\u001b`, "a JSON string carries no escape either")
			assert.Contains(t, stdout, "docs/b.md")
		})
	}
}

func TestJudgeStripsTerminalEscapesFromTheRubricName(t *testing.T) {
	skipIfNoControlCharsInNames(t)
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n", "r" + judgeEscape + ".md": "Score 1 when short.\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--estimate", "--model", judgeTestModel, "--rubric", "r"+judgeEscape+".md")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.NotContains(t, stdout, "\x1b")
	assert.NotContains(t, stdout, "\x07")
}

// skipIfNoControlCharsInNames skips a test that needs a file name with control
// characters, which Windows rejects.
func skipIfNoControlCharsInNames(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names cannot hold control characters")
	}
}
