package commands

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/require"
)

// The end-to-end scanner tests run the test binary as the external scanner (see
// testutil.RunFakeScanner), so they start a real process on every platform.

// scannerEntry is a [[lint.external]] entry that runs the fake scanner with args.
func scannerEntry(t *testing.T, keys string, args ...string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	quoted := make([]string, 0, len(args)+2)
	for _, a := range slices.Concat([]string{filepath.ToSlash(exe), testutil.FakeScannerArg}, args) {
		quoted = append(quoted, strconv.Quote(a))
	}
	return "\n[[lint.external]]\nname = \"fake\"\negress = false\ncommand = [" + strings.Join(quoted, ", ") + "]\n" + keys
}

// scannerProjectDir writes a project with one clean rule and the given extra
// configuration, plus a skill per name, and makes it the working directory.
func scannerProjectDir(t *testing.T, extraConfig string, skills ...string) string {
	t.Helper()
	testutil.ScrubCIEnv(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig+extraConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse tabs.\n")
	for _, s := range skills {
		writeFile(t, filepath.Join(root, ".ai-rulez", "skills", s, "SKILL.md"),
			"---\nname: "+s+"\ndescription: Use when running "+s+" tasks in this repository.\n---\n# "+s+"\n\nSteps.\n")
	}
	chdir(t, root)
	return root
}

// runExternalStrict runs `validate --strict --external`-style linting of root
// with the given --fail-on and returns the exit code it earns and the report.
func runExternalStrict(t *testing.T, root, failOn string) (int, *lint.Report) {
	t.Helper()
	testutil.IsolateHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	oldExt, oldStrict, oldFail := validateExtern, validateStrict, validateFailOn
	t.Cleanup(func() { validateExtern, validateStrict, validateFailOn = oldExt, oldStrict, oldFail })
	validateExtern, validateStrict, validateFailOn = true, true, failOn
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	var code int
	var report *lint.Report
	capture(t, func() {
		report, err = strictLint(t.Context(), cfg)
		if err == nil {
			code = reportStrict([]*lint.Report{report}, []*config.Config{cfg})
		}
	})
	require.NoError(t, err)
	return code, report
}

func TestStrictExternalExitCodeFollowsTheScannerFailOn(t *testing.T) {
	policy := func(failOn string) string { return "\n[lint.scanner_policy]\nfail_on = \"" + failOn + "\"\n" }
	tests := []struct {
		name     string
		policy   string
		keys     string
		level    string
		failOn   string
		wantCode int
	}{
		{"--strict fails on a scanner warning by default", "", "", "warning", "warning", 2},
		{"policy fail_on = error keeps --strict from failing on a scanner warning", policy("error"), "", "warning", "warning", 0},
		{"a scanner error still fails under that policy", policy("error"), "", "error", "warning", 2},
		{"entry fail_on = error does the same for one scanner", "", "fail_on = \"error\"\n", "warning", "warning", 0},
		{"policy fail_on = warning fails the default error gate", policy("warning"), "", "warning", "error", 2},
		{"the default gate ignores a scanner warning", "", "", "warning", "error", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := scannerProjectDir(t, tt.policy+scannerEntry(t, tt.keys, "--level", tt.level, "--msg", "scanner says no"))

			// Act
			code, report := runExternalStrict(t, root, tt.failOn)

			// Assert
			require.Equal(t, tt.wantCode, code, "findings: %+v", report.Findings)
		})
	}
}
