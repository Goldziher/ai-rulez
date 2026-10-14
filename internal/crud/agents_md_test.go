package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

const sectionedAgentsMD = "# Demo\n\nIntro.\n\n## Setup\n\nRun setup.\n\n```sh\n## not a heading\n```\n\n## Style\n\nUse tabs.\n"

func agentsMDProject(t *testing.T, p struct{ name, configPath, configBody, stateDir string }, split bool) (string, *crud.OperatorImpl) {
	t.Helper()
	body := p.configBody
	if split {
		body += "agents_md_split = true\n"
	}
	root := agentsProject(t, p.configPath, body)
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(sectionedAgentsMD), 0o644))
	op, err := crud.NewOperator(root)
	require.NoError(t, err)
	return root, op
}

func contextNames(t *testing.T, op *crud.OperatorImpl) []string {
	t.Helper()
	files, err := op.ListFiles(context.Background(), "", crud.ContentTypeContext)
	require.NoError(t, err)
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func TestListShowsTheAGENTSMDItems(t *testing.T) {
	for _, p := range agentsProjects {
		for _, split := range []bool{false, true} {
			name := p.name + "/whole file"
			want := []string{"agents"}
			if split {
				name = p.name + "/split"
				want = []string{"agents", "agents-setup", "agents-style"}
			}
			t.Run(name, func(t *testing.T) {
				// Arrange
				root, op := agentsMDProject(t, p, split)
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".agents", "context"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, ".agents", "context", "arch.md"), []byte("# Arch\n"), 0o644))

				// Act
				files, err := op.ListFiles(context.Background(), "", crud.ContentTypeContext)

				// Assert
				require.NoError(t, err)
				got := make([]string, 0, len(files))
				for _, f := range files {
					got = append(got, f.Name)
				}
				assert.Equal(t, append(want, "arch"), got)
				assert.Equal(t, filepath.Join(root, "AGENTS.md"), files[0].Path)
			})
		}
	}
}

func TestRemoveDeletesOneAGENTSMDSection(t *testing.T) {
	for _, p := range agentsProjects {
		t.Run(p.name, func(t *testing.T) {
			// Arrange
			root, op := agentsMDProject(t, p, true)

			// Act
			require.NoError(t, op.RequireContent(context.Background(), "", crud.ContentTypeContext, "agents-setup"))
			err := op.RemoveFile(context.Background(), "", crud.ContentTypeContext, "agents-setup")

			// Assert
			require.NoError(t, err)
			data, readErr := os.ReadFile(filepath.Join(root, "AGENTS.md"))
			require.NoError(t, readErr)
			assert.Equal(t, "# Demo\n\nIntro.\n\n## Style\n\nUse tabs.\n", string(data))
			assert.Equal(t, []string{"agents", "agents-style"}, contextNames(t, op))
		})
	}
}

func TestRemoveKeepsTheFencedHeadingWithItsSection(t *testing.T) {
	// Arrange
	root, op := agentsMDProject(t, agentsProjects[0], true)

	// Act
	require.NoError(t, op.RemoveFile(context.Background(), "", crud.ContentTypeContext, "agents-style"))

	// Assert
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Demo\n\nIntro.\n\n## Setup\n\nRun setup.\n\n```sh\n## not a heading\n```\n", string(data))
}

func TestRemoveRefusesTheWholeAGENTSMD(t *testing.T) {
	// Arrange: without agents_md_split the item named agents is the whole file
	root, op := agentsMDProject(t, agentsProjects[0], false)

	// Act
	err := op.RemoveFile(context.Background(), "", crud.ContentTypeContext, "agents")

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "AGENTS.md")
	data, readErr := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, readErr)
	assert.Equal(t, sectionedAgentsMD, string(data))
}

func TestContentPathNamesAGENTSMDForASplitItem(t *testing.T) {
	// Arrange
	root, op := agentsMDProject(t, agentsProjects[0], true)

	// Act
	path := op.ContentPath("", crud.ContentTypeContext, "agents-setup")

	// Assert
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), path, "removing the item edits AGENTS.md, no file of its own exists")
	assert.True(t, op.IsAgentsMDItem("", crud.ContentTypeContext, "agents-setup"))
	assert.False(t, op.IsAgentsMDItem("", crud.ContentTypeContext, "arch"))
	assert.False(t, op.IsAgentsMDItem("", crud.ContentTypeRules, "agents-setup"))
}

func TestAGENTSMDItemsAreNotListedForTheLocalTree(t *testing.T) {
	// Arrange
	_, op := agentsMDProject(t, agentsProjects[0], true)

	// Act
	files, err := op.Local().ListFiles(context.Background(), "", crud.ContentTypeContext)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, files)
}
