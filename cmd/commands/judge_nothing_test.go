package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudgeNamingOnlySkippedFilesListsThemAndExitsOne(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		reason string
		bytes  int
	}{
		{"a file over 2 MiB", "docs/big.md", "2 MiB", 3 << 20},
		{"a generated file", "docs/generated.md", "generated", len(judgeGeneratedBody)},
		{"a binary file", "docs/binary.md", "UTF-8", len("head\x00tail\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			skippedProject(t)
			rig := newJudgeRig(t)

			// Act
			stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json", tc.path)

			// Assert
			assert.Equal(t, 1, code, "nothing was judged")
			assert.Contains(t, rig.stderr.String(), "nothing judged")
			assert.NotContains(t, rig.stderr.String(), "nothing to judge")
			assert.Zero(t, rig.built)
			assert.Empty(t, rig.fake.ChatCalls())
			doc := parseJudgeDoc(t, stdout)
			require.Len(t, doc.Files, 1)
			assert.Equal(t, tc.path, doc.Files[0].Path)
			assert.Equal(t, "skipped", doc.Files[0].Status)
			assert.Contains(t, doc.Files[0].Reason, tc.reason)
			assert.Equal(t, tc.bytes, doc.Files[0].Bytes)
			assert.Equal(t, 1, doc.Summary.Skipped)
		})
	}
}

func TestJudgeEstimateOfOnlySkippedFilesListsThemAndExitsZero(t *testing.T) {
	// Arrange
	skippedProject(t)
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--estimate", "--model", judgeTestModel, "docs/big.md")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Contains(t, stdout, "skip  docs/big.md")
}

func TestJudgeDefaultDiscoveryOutsideAProjectNeedsExplicitPaths(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		args    []string
		refused bool
	}{
		{"unrelated nested projects, no paths", map[string]string{"a/AGENTS.md": "# a\n", "b/.agents/rules/r.md": "Rule.\n"}, nil, true},
		{"nested projects, a path is named", map[string]string{"a/AGENTS.md": "# a\n", "b/AGENTS.md": "# b\n"}, []string{"a/AGENTS.md"}, false},
		{"AGENTS.md at the root", map[string]string{"AGENTS.md": "# root\n", "a/AGENTS.md": "# a\n"}, nil, false},
		{"an .agents directory at the root", map[string]string{".agents/rules/r.md": "Rule.\n"}, nil, false},
		{"an ai-rulez config at the root", map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\n", ".ai-rulez/rules/r.md": "Rule.\n"}, nil, false},
		{"a user-level ai-rulez config at the root", map[string]string{".config/ai-rulez/config.toml": "version = \"5.0\"\n", ".config/ai-rulez/rules/r.md": "Rule.\n"}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, tc.files)
			rig := newJudgeRig(t)

			// Act
			_, code := rig.run(t, append([]string{"--estimate", "--model", judgeTestModel}, tc.args...)...)

			// Assert
			if tc.refused {
				assert.Equal(t, 1, code)
				assert.Contains(t, rig.stderr.String(), "name the files or directories to judge")
				return
			}
			assert.Equal(t, 0, code, rig.stderr.String())
		})
	}
}
