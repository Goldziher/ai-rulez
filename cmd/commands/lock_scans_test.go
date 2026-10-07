package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// scanLockProject is a lock project with one staged fake scanner that reports a
// warning, with the user cache and config directories isolated.
func scanLockProject(t *testing.T) (root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	script := filepath.Join(t.TempDir(), "fake-scan")
	body := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "fake-scan 2.0.1"; exit 0; fi
echo '{"version":"2.1.0","runs":[{"results":[{"ruleId":"W1","level":"warning","message":{"text":"mild"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/style.md"},"region":{"startLine":1}}}]}]}]}'
`
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755)) //nolint:gosec // test script
	root = lockProject(t, "\n[lint.scanner_policy]\nisolation = \"none\"\n\n[[lint.external]]\nname = \"fake\"\ncommand = [\""+filepath.ToSlash(script)+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\", \"skills\"]\n")
	t.Cleanup(func() { validateExtern, validateStrict, validateDryRun = false, false, false })
	return root
}

// scanOnce runs `scan --external` for the project in the working directory so
// the scanner result cache holds the current result.
func scanOnce(t *testing.T, root string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	validateExtern = true
	report, err := strictLint(t.Context(), cfg)
	require.NoError(t, err)
	require.NotEmpty(t, report.Findings)
}

func TestLockRecordsTheCachedScannerResult(t *testing.T) {
	root := scanLockProject(t)
	configDir := filepath.Join(root, ".ai-rulez")

	// Before any scan there is nothing to record, and lock starts no scanner.
	require.Equal(t, 0, writeLockAt("", "", nil))
	lock, err := lockfile.Load(configDir)
	require.NoError(t, err)
	assert.Empty(t, lock.Scan)

	// After a scan the lock carries the result.
	scanOnce(t, root)
	require.Equal(t, 0, writeLockAt("", "", nil))
	lock, err = lockfile.Load(configDir)
	require.NoError(t, err)
	require.Len(t, lock.Scan, 1)
	got := lock.Scan[0]
	assert.Equal(t, "fake", got.Scanner)
	assert.Equal(t, "fake-scan 2.0.1", got.Version)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, got.Tree)
	assert.Equal(t, 1, got.Findings)
	assert.Equal(t, "warning", got.MaxSeverity)
	assert.Equal(t, lockfile.ScanPass, got.Result, "a warning does not fail the default threshold")
	data, err := os.ReadFile(lockfile.Path(configDir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "[[scan]]")

	// Deterministic: locking again changes nothing.
	require.Equal(t, 0, writeLockAt("", "", nil))
	again, err := os.ReadFile(lockfile.Path(configDir))
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))

	// Changed content has no cached result, so its record is dropped.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"), []byte("# Style\nUse spaces.\n"), 0o600))
	require.Equal(t, 0, writeLockAt("", "", nil))
	lock, err = lockfile.Load(configDir)
	require.NoError(t, err)
	assert.Empty(t, lock.Scan)
}

func TestLockRecordsFailWhenAFindingReachesFailOn(t *testing.T) {
	root := scanLockProject(t)
	configDir := filepath.Join(root, ".ai-rulez")
	cfgPath := filepath.Join(configDir, "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(string(body), "isolation = \"none\"", "isolation = \"none\"\nfail_on = \"warning\"", 1)), 0o600))

	scanOnce(t, root)
	require.Equal(t, 0, writeLockAt("", "", nil))

	lock, err := lockfile.Load(configDir)
	require.NoError(t, err)
	require.Len(t, lock.Scan, 1)
	assert.Equal(t, lockfile.ScanFail, lock.Scan[0].Result)
}

func TestLimitedLockRefreshKeepsTheScanRecords(t *testing.T) {
	root := scanLockProject(t)
	configDir := filepath.Join(root, ".ai-rulez")
	scanOnce(t, root)
	require.Equal(t, 0, writeLockAt("", "", nil))

	// A refresh of one named source is not a full re-pin: the records stay.
	current, err := lockfile.Load(configDir)
	require.NoError(t, err)
	next := &lockfile.File{Version: lockfile.Version}
	pinScans(&config.Config{}, current, next, false)

	assert.Equal(t, current.Scan, next.Scan)
}

func TestPinScansIgnoresAProjectWithoutScanners(t *testing.T) {
	next := &lockfile.File{Version: lockfile.Version}
	pinScans(&config.Config{Lint: &config.LintConfig{}}, nil, next, true)
	assert.Empty(t, next.Scan)
}
