package airulez_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

func TestGenerateReportsAHandWrittenFileAsRefused(t *testing.T) {
	// Arrange: a CLAUDE.md that ai-rulez did not write.
	dir := t.TempDir()
	writeSources(t, dir)
	handWritten := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(handWritten, []byte("# Mine\n\nWritten by hand.\n"), 0o644))
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
	require.NoError(t, err)

	// Act
	_, err = project.Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Write})

	// Assert: a refusal a caller can tell apart from any other apply failure, and the file is kept.
	var apiErr *airulez.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, airulez.CodeRefused, apiErr.Code)
	assert.True(t, errors.Is(err, airulez.ErrRefused))
	assert.Contains(t, err.Error(), "refusing to overwrite 1 existing file(s)")
	got, readErr := os.ReadFile(handWritten)
	require.NoError(t, readErr)
	assert.Equal(t, "# Mine\n\nWritten by hand.\n", string(got))
}
