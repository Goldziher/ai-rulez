package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestJudgeRefusesTheHomeDirectoryOutsideAProject(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"default discovery", nil},
		{"an explicit path", []string{"other/AGENTS.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			dir := judgeProject(t, map[string]string{"other/AGENTS.md": "# another project\n", "AGENTS.md": "# home\n"})
			t.Setenv("HOME", dir)
			t.Setenv("USERPROFILE", dir) // os.UserHomeDir on Windows
			rig := newJudgeRig(t)

			// Act
			stdout, code := rig.run(t, append([]string{"--allow-llm", "--model", judgeTestModel, "--no-cache"}, tc.args...)...)

			// Assert
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Contains(t, rig.stderr.String(), "it is not a project")
			assert.Zero(t, rig.built)
			assert.Empty(t, rig.fake.ChatCalls())
		})
	}
}

func TestJudgeBoundsTheWalkOfADirectoryWithoutARepository(t *testing.T) {
	// Arrange
	files := map[string]string{"AGENTS.md": "# root\n"}
	for i := range 6 {
		files[fmt.Sprintf("d%d/AGENTS.md", i)] = "# x\n"
	}
	dir := judgeProject(t, files)

	// Act
	_, err := openJudgeWorkspace(context.Background(), dir, false, lint.WalkLimits{MaxEntries: 5})

	// Assert
	require.ErrorContains(t, err, "more than 5 entries")
}
