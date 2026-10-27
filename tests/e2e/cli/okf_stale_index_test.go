package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// TestStaleOKFIndexAfterSkillEdit: editing a skill description leaves the
// migrated bundle's index.md stale; validate and generate --check report it
// (as migrate okf --check does), and generate refreshes it (issue #501).
func TestStaleOKFIndexAfterSkillEdit(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	cfg := filepath.Join(dir, ".ai-rulez")
	putFile(t, cfg, "config.toml", "version = \"5.0\"\nname = \"okf\"\npresets = [\"claude\"]\n")
	skill := filepath.Join(cfg, "skills", "demo")
	putFile(t, skill, "SKILL.md", "---\nname: demo\ndescription: Old description of the demo skill.\n---\n\nBody.\n")
	testutil.RunCLIExpectSuccess(t, dir, "migrate", "okf")
	testutil.RunCLIExpectSuccess(t, dir, "lock")
	testutil.RunCLIExpectSuccess(t, dir, "generate", "--no-local", "--yes")

	// The backup migrate okf leaves is not content: it is no duplicate skill.
	res := testutil.RunCLI(t, dir, "validate", "--strict", "--no-local")
	require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	assert.NotContains(t, res.Stdout+res.Stderr, "AR703")

	data, err := os.ReadFile(filepath.Join(skill, "SKILL.md"))
	require.NoError(t, err)
	edited := strings.ReplaceAll(string(data), "Old description of the demo skill.", "New description after an edit.")
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(edited), 0o644))
	testutil.RunCLIExpectSuccess(t, dir, "lock")

	// Act and assert: every check that gates CI agrees on the stale index.
	for _, args := range [][]string{
		{"migrate", "okf", "--check"},
		{"validate", "--strict", "--no-local"},
		{"generate", "--check", "--no-local"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, 2, res.ExitCode, strings.Join(args, " ")+": "+res.Stdout+res.Stderr)
		assert.Contains(t, res.Stdout+res.Stderr, "skills/demo/index.md", strings.Join(args, " "))
	}
	res = testutil.RunCLI(t, dir, "validate", "--strict", "--no-local")
	assert.Contains(t, res.Stdout+res.Stderr, "AR9B0")

	// generate refreshes the index, and the checks pass again.
	testutil.RunCLIExpectSuccess(t, dir, "generate", "--no-local", "--yes")
	assert.Contains(t, testutil.ReadFile(t, filepath.Join(skill, "index.md")), "New description after an edit.")
	for _, args := range [][]string{
		{"migrate", "okf", "--check"},
		{"validate", "--strict", "--no-local"},
		{"generate", "--check", "--no-local"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, 0, res.ExitCode, strings.Join(args, " ")+": "+res.Stdout+res.Stderr)
	}
}
