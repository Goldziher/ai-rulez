package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const itemsAgentsMD = "# Demo\n\nIntro.\n\n## Setup\n\nRun setup.\n\n## Style\n\nUse tabs.\n"

func agentsMDItemsProject(t *testing.T, split bool) (dir, agentsMD string) {
	t.Helper()
	dir = t.TempDir()
	cfg := agentsMCPConfig
	if split {
		cfg += "agents_md_split = true\n"
	}
	writeAgentsMCPFile(t, dir, ".agents/ai-rulez.toml", cfg)
	writeAgentsMCPFile(t, dir, "AGENTS.md", itemsAgentsMD)
	return dir, filepath.Join(dir, "AGENTS.md")
}

func resultField(t *testing.T, text, key string) string {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out), text)
	s, _ := out[key].(string)
	return s
}

func TestReadContextOfAnAgentsMDItemReturnsOnlyThatItem(t *testing.T) {
	tests := []struct {
		name  string
		split bool
		item  string
		want  string
	}{
		{"split preamble", true, "agents", "# Demo\n\nIntro.\n\n"},
		{"split middle section", true, "agents-setup", "## Setup\n\nRun setup.\n\n"},
		{"split last section", true, "agents-style", "## Style\n\nUse tabs.\n"},
		{"whole file", false, "agents", itemsAgentsMD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, agentsMD := agentsMDItemsProject(t, tt.split)

			// Act
			res, err := ReadContextHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir, "name": tt.item}))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			assert.Equal(t, tt.want, resultField(t, textOf(t, res), "content"))
			assert.Equal(t, filepath.ToSlash(agentsMD), resultField(t, textOf(t, res), "path"))
		})
	}
}

func TestUpdateContextOfASplitAgentsMDItemRewritesOnlyThatSection(t *testing.T) {
	// Arrange
	dir, agentsMD := agentsMDItemsProject(t, true)
	args := map[string]any{"working_directory": dir, "name": "agents-setup", "content": "## Setup\n\nRun it twice.\n\n"}

	// Act
	res, err := UpdateContextHandler(context.Background(), newRequestWithArgs(args))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	got, readErr := os.ReadFile(agentsMD)
	require.NoError(t, readErr)
	assert.Equal(t, "# Demo\n\nIntro.\n\n## Setup\n\nRun it twice.\n\n## Style\n\nUse tabs.\n", string(got))
	assert.Equal(t, filepath.ToSlash(agentsMD), filepath.ToSlash(resultField(t, textOf(t, res), "path")))
	entries, dirErr := os.ReadDir(dir)
	require.NoError(t, dirErr)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".AGENTS.md.", "no temporary file is left behind")
	}
}

func TestUpdateContextOfAnAgentsMDItemRefusesWhatItCannotDoSafely(t *testing.T) {
	tests := []struct {
		name    string
		split   bool
		item    string
		content string
		want    string
	}{
		{"unsplit file is the author's own", false, "agents", "# New\n", "one item"},
		{"new content would add a section", true, "agents-setup", "## Setup\n\nx\n\n## Extra\n\ny\n", "section"},
		{"new content would drop the heading", true, "agents-setup", "no heading\n", "section"},
		{"empty content", true, "agents-setup", "", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, agentsMD := agentsMDItemsProject(t, tt.split)
			args := map[string]any{"working_directory": dir, "name": tt.item, "content": tt.content}

			// Act
			res, err := UpdateContextHandler(context.Background(), newRequestWithArgs(args))

			// Assert
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Contains(t, textOf(t, res), tt.want)
			assert.NotContains(t, textOf(t, res), "file not found")
			got, readErr := os.ReadFile(agentsMD)
			require.NoError(t, readErr)
			assert.Equal(t, itemsAgentsMD, string(got), "AGENTS.md is untouched")
		})
	}
}

func TestDeleteContextOfASplitAgentsMDItemCutsOutOnlyThatSection(t *testing.T) {
	// Arrange
	dir, agentsMD := agentsMDItemsProject(t, true)

	// Act
	res, err := DeleteContextHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir, "name": "agents-style"}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	got, readErr := os.ReadFile(agentsMD)
	require.NoError(t, readErr)
	assert.Equal(t, "# Demo\n\nIntro.\n\n## Setup\n\nRun setup.\n", string(got))
}

func TestDeleteContextOfTheUnsplitAgentsMDItemIsRefused(t *testing.T) {
	// Arrange
	dir, agentsMD := agentsMDItemsProject(t, false)

	// Act
	res, err := DeleteContextHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir, "name": "agents"}))

	// Assert
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, textOf(t, res), "one item")
	got, readErr := os.ReadFile(agentsMD)
	require.NoError(t, readErr)
	assert.Equal(t, itemsAgentsMD, string(got))
}
