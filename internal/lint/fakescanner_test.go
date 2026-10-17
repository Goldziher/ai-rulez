package lint

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/require"
)

// The scanner tests run the test binary itself as the scanner (see
// testutil.RunFakeScanner), so they start a real process on every platform.

// fakeScannerExe returns the test binary, linked as a file called name when one
// is given (so a test can make the "interpreter" a known one such as node).
func fakeScannerExe(t *testing.T, name string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	if name == "" {
		return exe
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.Link(exe, dst); err != nil {
		data, rerr := os.ReadFile(exe)
		require.NoError(t, rerr)
		require.NoError(t, os.WriteFile(dst, data, 0o755)) //nolint:gosec // a copy of the test binary
	}
	return dst
}

// tomlArray renders argv as a TOML array of strings.
func tomlArray(args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = strconv.Quote(a)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// fakeScannerArgv is the argv that runs the fake scanner with args, through exe.
func fakeScannerArgv(exe string, args ...string) []string {
	return slices.Concat([]string{filepath.ToSlash(exe), testutil.FakeScannerArg}, args)
}

// fakeScannerCommand is the TOML `command = [...]` line that runs the fake scanner with args.
func fakeScannerCommand(t *testing.T, args ...string) string {
	t.Helper()
	return "command = " + tomlArray(fakeScannerArgv(fakeScannerExe(t, ""), args...)...) + "\n"
}

// isolationNone is a [lint.scanner_policy] that runs scanners unconfined, so a
// fake scanner may write its --log file outside the scratch directory.
const isolationNone = "\n[lint.scanner_policy]\nisolation = \"none\"\n"

// fakeSkill is a minimal valid skill file.
func fakeSkill(name string) string {
	return "---\nname: " + name + "\ndescription: Use when running " + name + " tasks in this repository.\n---\n# " + name + "\n\nSteps.\n"
}

// fakeProject writes a project whose [[lint.external]] entry "fake" has the keys
// in body (command included); skills are created as .ai-rulez/skills/<name>.
// policy is TOML placed before the entry.
func fakeProject(t *testing.T, policy, body string, skills ...string) *scannerProject {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/rules/r.md":  "# Rule\n\nbody\n",
		".ai-rulez/config.toml": baseConfig + policy + "\n[[lint.external]]\nname = \"fake\"\negress = false\n" + body,
	}
	for _, s := range skills {
		files[".ai-rulez/skills/"+s+"/SKILL.md"] = fakeSkill(s)
	}
	writeFiles(t, root, files)
	gitAdd(t, root)
	return &scannerProject{t: t, root: root}
}

func fakeCache(t *testing.T) *ScanCache {
	t.Helper()
	cache := NewScanCache(filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "secret", "scan.key"))
	require.NotNil(t, cache)
	return cache
}

// logCount counts the "start" lines of a --log file.
func logCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return strings.Count(string(data), "start ")
}
