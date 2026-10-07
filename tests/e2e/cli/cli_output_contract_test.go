package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestJSONFormatPrintsAnErrorDocumentWhenTheConfigDoesNotLoad(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.yaml"), []byte("version: \"4.0\"\nname: x\n"), 0o644))

	for _, args := range [][]string{
		{"validate", "--format", "json"},
		{"lock", "--check", "--format", "json"},
		{"roles", "show", "x", "--format", "json"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, exitCannot, res.ExitCode, strings.Join(args, " "))
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), "%v stdout: %q", args, res.Stdout)
		assert.EqualValues(t, 1, doc["schema_version"], args)
		assert.Contains(t, doc["error"], "no longer read", args)
	}
}

func TestVersionPrintsToStdoutEvenWhenQuiet(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)

	for _, args := range [][]string{{"version"}, {"-q", "version"}} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, 0, res.ExitCode, args)
		assert.Regexp(t, `^ai-rulez version \S+\n$`, res.Stdout, args)
	}
}

func TestMCPRejectsAnUnknownSubcommand(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	res := testutil.RunCLI(t, testutil.CreateTempDir(t), "mcp", "bogus")

	assert.Equal(t, exitCannot, res.ExitCode)
	assert.Contains(t, res.Stderr, "unknown command")
}
