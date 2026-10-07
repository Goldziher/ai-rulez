package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const unreachableIncludeConfig = `version = "5.0"
name = "x"
presets = ["claude"]

[[includes]]
name = "gone"
source = "https://example.invalid/none/repo.git"
path = "modules/core"
`

func isolatedEnv(t *testing.T) map[string]string {
	t.Helper()
	home := t.TempDir()
	return map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": home + "/c", "XDG_CACHE_HOME": home + "/k",
		"AI_RULEZ_HOME": home + "/a", "GIT_TERMINAL_PROMPT": "0",
	}
}

func TestCLI_UnresolvedIncludeIsReportedOnce(t *testing.T) {
	// Arrange
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", unreachableIncludeConfig)
	writeIn(t, dir+"/.ai-rulez/rules", "r.md", "---\npriority: high\n---\n# r\n\nBe nice.\n")

	// Act
	res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "validate")

	// Assert
	require.NotEqual(t, 0, res.ExitCode)
	out := res.Stdout + res.Stderr
	assert.Equal(t, 1, strings.Count(out, "failed to fetch include 'gone'"), out)
}

func TestCLI_ListReportsUnresolvedIncludes(t *testing.T) {
	// Arrange
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", unreachableIncludeConfig)
	writeIn(t, dir+"/.ai-rulez/rules", "r.md", "---\npriority: high\n---\n# r\n\nBe nice.\n")

	// Act
	res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "list", "rules")

	// Assert
	require.Equal(t, 0, res.ExitCode, res.Stderr)
	out := res.Stdout + res.Stderr
	assert.Contains(t, out, "gone", "the listing says an include was not resolved")
	assert.Equal(t, 1, strings.Count(out, "Failed to process include"), out)
	assert.Contains(t, out, "r", "local content is still listed")
}

func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	testutil.WriteFile(t, dir, name, content)
}
