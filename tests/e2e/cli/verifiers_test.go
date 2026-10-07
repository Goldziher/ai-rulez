package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/require"
)

func gitE2E(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	out, err := gitutil.CommandNoContext(dir, full...).CombinedOutput()
	require.NoError(t, err, string(out))
}

// putFile writes name under dir, creating the directory.
func putFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	testutil.WriteFile(t, dir, name, content)
}

func TestVerifiersCLI_ChangedOnlyRuleMappingAndFormats(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	cfg := filepath.Join(dir, ".ai-rulez")
	putFile(t, cfg, "config.toml", "version = \"5.0\"\nname = \"v\"\npresets = [\"claude\"]\n")
	putFile(t, filepath.Join(cfg, "rules"), "api-conventions.md", "# API\n\nEvery endpoint has a test.\n")
	putFile(t, filepath.Join(cfg, "verifiers"), "api.toml", `[[verifiers]]
id = "new-endpoints-have-tests"
rule = "api-conventions"
severity = "error"
fix = "Add tests/api/test_<name>.py."
when_changed = ["src/api/**/*.py"]
[verifiers.require.paired]
for_each = "src/api/{rel}.py"
requires_changed = "tests/api/test_{stem}.py"
[[verifiers.examples]]
name = "test changed"
files = { "src/api/a.py" = "x", "tests/api/test_a.py" = "t" }
changed = ["src/api/a.py", "tests/api/test_a.py"]
expect = "pass"
[[verifiers.examples]]
name = "test missing"
files = { "src/api/b.py" = "x" }
changed = ["src/api/b.py"]
expect = "fail"
`)
	putFile(t, dir, "README.md", "base\n")
	putFile(t, filepath.Join(dir, "src", "api"), "old.py", "x\n")
	gitE2E(t, dir, "init", "-q", "-b", "main")
	gitE2E(t, dir, "add", "-A")
	gitE2E(t, dir, "commit", "-q", "-m", "base")
	gitE2E(t, dir, "checkout", "-q", "-b", "topic")
	putFile(t, filepath.Join(dir, "src", "api"), "orders.py", "x\n")
	putFile(t, filepath.Join(dir, "src", "api"), "users.py", "x\n")
	putFile(t, filepath.Join(dir, "tests", "api"), "test_users.py", "t\n")
	gitE2E(t, dir, "add", "-A")
	gitE2E(t, dir, "commit", "-q", "-m", "topic")

	// Only the files changed since main are checked, and the failure names the rule.
	since := testutil.RunCLI(t, dir, "verifiers", "run", "--since", "main")
	require.Equal(t, 2, since.ExitCode, since.Stderr)
	since.AssertStdoutContains(t, `AR9H1 new-endpoints-have-tests (rule "api-conventions"`)
	since.AssertStdoutContains(t, "src/api/orders.py")
	since.AssertStdoutContains(t, "fix: Add tests/api/test_<name>.py.")
	require.NotContains(t, since.Stdout, "src/api/users.py  expected")
	require.NotContains(t, since.Stdout, "old.py")

	// A base that does not exist is an error, not a pass.
	missing := testutil.RunCLI(t, dir, "verifiers", "run", "--since", "origin/gone")
	require.Equal(t, 1, missing.ExitCode, missing.Stdout)
	missing.AssertStderrContains(t, "origin/gone")

	// SARIF and JUnit carry the declaring rule.
	sarif := testutil.RunCLI(t, dir, "verifiers", "run", "--since", "main", "--format", "sarif")
	require.Equal(t, 2, sarif.ExitCode)
	sarif.AssertStdoutContains(t, `"ruleId": "AR9H1/new-endpoints-have-tests"`)
	sarif.AssertStdoutContains(t, ".ai-rulez/rules/api-conventions.md")
	junit := testutil.RunCLI(t, dir, "verifiers", "run", "--since", "main", "--format", "junit")
	junit.AssertStdoutContains(t, `name="rule:api-conventions"`)

	// Self-tests and explain.
	self := testutil.RunCLIExpectSuccess(t, dir, "verifiers", "test")
	self.AssertStdoutContains(t, "2 of 2 examples passed")
	explain := testutil.RunCLIExpectSuccess(t, dir, "verifiers", "explain", "new-endpoints-have-tests")
	explain.AssertStdoutContains(t, `enforces: rule "api-conventions"`)
}
