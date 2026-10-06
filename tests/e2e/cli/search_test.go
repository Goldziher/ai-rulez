package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/require"
)

func TestSearchCLI(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	skill := filepath.Join(dir, ".ai-rulez", "skills", "refund-policy")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	testutil.WriteFile(t, filepath.Join(dir, ".ai-rulez"), "config.toml", "version = \"4.0\"\nname = \"search\"\npresets = [\"claude\"]\n")
	testutil.WriteFile(t, skill, "SKILL.md", "---\nname: refund-policy\ndescription: Process customer refund requests\ntriggers:\n  - customer wants money back\n---\nbody\n")
	testutil.WriteFile(t, dir, "cases.yaml", "version: 1\ncases:\n  - {id: refund, query: money back, expect: [refund-policy]}\n")

	query := testutil.RunCLIExpectSuccess(t, dir, "search", "money back")
	query.AssertStdoutContains(t, "refund-policy")

	pass := testutil.RunCLIExpectSuccess(t, dir, "search", "--eval", "cases.yaml", "--min", "top1=1", "--format", "json")
	pass.AssertStdoutContains(t, `"top1": 1`)

	testutil.WriteFile(t, dir, "miss.yaml", "version: 1\ncases:\n  - {id: miss, query: zebra, expect: [refund-policy]}\n")
	gate := testutil.RunCLI(t, dir, "search", "--eval", "miss.yaml", "--min", "top1=0.5")
	require.Equal(t, 2, gate.ExitCode, gate.Stderr)
	gate.AssertStderrContains(t, "AR9D4")

	bad := testutil.RunCLI(t, dir, "search", "--eval", "missing.yaml")
	require.Equal(t, 1, bad.ExitCode)
}
