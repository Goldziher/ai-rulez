package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/require"
)

// fakeScanner reads the staged skill directory ($1), writes SARIF to the out file ($2)
// and reports the line of BADTOKEN in SKILL.md. It uses printf, not a here-document: macOS's
// /bin/sh writes here-documents to /tmp, which the scanner sandbox (isolation = "auto") denies.
const fakeScanner = `#!/bin/sh
[ "$1" = "--version" ] && echo "fake-scan 1.2.3" && exit 0
n=$(grep -n BADTOKEN "$1/SKILL.md" | head -1 | cut -d: -f1)
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"FAKE-1","level":"error","message":{"text":"bad token"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"SKILL.md"},"region":{"startLine":%s}}}]}]}]}' "$n" > "$2"
`

func scannerProject(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake scanner is a POSIX shell script")
	}
	dir := testutil.CreateTempDir(t)
	cfg := filepath.Join(dir, ".ai-rulez")
	putFile(t, cfg, "config.toml", `version = "4.0"
name = "s"
presets = ["claude"]

[[lint.external]]
name = "fake"
command = ["./bin/fake-scan.sh", "{skill_dirs}", "{out}"]
egress = false
inputs = ["skills"]
`)
	putFile(t, filepath.Join(cfg, "skills", "deploy"), "SKILL.md", "---\nname: deploy\ndescription: Use when deploying.\n---\n# Deploy\nrun BADTOKEN now\n")
	putFile(t, filepath.Join(dir, "bin"), "fake-scan.sh", fakeScanner)
	require.NoError(t, os.Chmod(filepath.Join(dir, "bin", "fake-scan.sh"), 0o755))
	gitE2E(t, dir, "init", "-q", "-b", "main")
	gitE2E(t, dir, "add", "-A")
	return dir
}

func TestScannersCLI_ListAndDoctor(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := scannerProject(t)

	list := testutil.RunCLI(t, dir, "scanners", "list")
	require.Equal(t, 0, list.ExitCode, list.Stderr)
	list.AssertStdoutContains(t, "fake")
	list.AssertStdoutContains(t, "skills")
	list.AssertStdoutContains(t, "found")

	doctor := testutil.RunCLI(t, dir, "scanners", "doctor", "fake")
	require.Equal(t, 0, doctor.ExitCode, doctor.Stdout+doctor.Stderr)
	doctor.AssertStdoutContains(t, "not probed (pass --external)")
	probed := testutil.RunCLI(t, dir, "scanners", "doctor", "fake", "--external")
	require.Equal(t, 0, probed.ExitCode, probed.Stdout+probed.Stderr)
	probed.AssertStdoutContains(t, "version   fake-scan 1.2.3")
	doctor.AssertStdoutContains(t, "false: scrubbed environment")

	unknown := testutil.RunCLI(t, dir, "scanners", "doctor", "ghost")
	require.Equal(t, 1, unknown.ExitCode)
	unknown.AssertStderrContains(t, "unknown scanner ghost")
}

func TestScannersCLI_ScanBaselineLifecycle(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := scannerProject(t)

	// The scanner's finding is reported at the real line of the file (frontmatter included).
	scan := testutil.RunCLI(t, dir, "scan", "--external")
	require.Equal(t, 2, scan.ExitCode, scan.Stdout+scan.Stderr)
	scan.AssertStdoutContains(t, ".ai-rulez/skills/deploy/SKILL.md:6")
	scan.AssertStdoutContains(t, "[fake] FAKE-1: bad token")

	// Accepting needs a reason.
	noReason := testutil.RunCLI(t, dir, "scan", "--external", "--write-baseline")
	require.Equal(t, 1, noReason.ExitCode)
	noReason.AssertStderrContains(t, "--write-baseline needs --reason")

	// With one, the finding is accepted and the next run is clean.
	write := testutil.RunCLI(t, dir, "scan", "--external", "--write-baseline", "--reason", "fixture token")
	require.Equal(t, 0, write.ExitCode, write.Stdout+write.Stderr)
	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "scanner-baseline.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"scanner": "fake"`)
	require.Contains(t, string(data), `"reason": "fixture token"`)
	clean := testutil.RunCLI(t, dir, "scan", "--external")
	require.Equal(t, 0, clean.ExitCode, clean.Stdout+clean.Stderr)

	// Editing the flagged line makes it new again; so does expiry.
	putFile(t, filepath.Join(dir, ".ai-rulez", "skills", "deploy"), "SKILL.md", "---\nname: deploy\ndescription: Use when deploying.\n---\n# Deploy\nrun BADTOKEN differently\n")
	edited := testutil.RunCLI(t, dir, "scan", "--external")
	require.Equal(t, 2, edited.ExitCode)
	putFile(t, filepath.Join(dir, ".ai-rulez", "skills", "deploy"), "SKILL.md", "---\nname: deploy\ndescription: Use when deploying.\n---\n# Deploy\nrun BADTOKEN now\n")
	expired := testutil.RunCLIWithEnv(t, dir, map[string]string{"AI_RULEZ_TODAY": "2999-01-01"}, "scan", "--external")
	require.Equal(t, 0, expired.ExitCode, expired.Stdout) // no expiry date was set: the entry stays valid

	// The flags mean nothing without --external.
	bad := testutil.RunCLI(t, dir, "scan", "--write-baseline", "--reason", "x")
	require.Equal(t, 1, bad.ExitCode)
	bad.AssertStderrContains(t, "require --external")
}
