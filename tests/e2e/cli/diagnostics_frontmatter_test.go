package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestCLI_ValidateReportsMalformedFrontmatterOnce(t *testing.T) {
	// Arrange
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	writeIn(t, dir+"/.ai-rulez/agents", "bad.md", "---\nname: a: b: c\n---\nAgent\n")

	// Act
	validate := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "validate")
	list := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "list", "agents")

	// Assert
	require.NotEqual(t, 0, validate.ExitCode)
	vout := validate.Stdout + validate.Stderr
	assert.Equal(t, 0, strings.Count(vout, "Ignoring malformed YAML frontmatter"), vout)
	assert.Contains(t, vout, "bad.md", "the error names the file")
	require.Equal(t, 0, list.ExitCode, list.Stderr)
	assert.Equal(t, 1, strings.Count(list.Stdout+list.Stderr, "Ignoring malformed YAML frontmatter"), "list has no error, so it keeps the warning")
}
