package improve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestShow_ReportsTheRunAndItsDiff(t *testing.T) {
	// Arrange
	_, configDir, plan, report := acceptedRun(t)

	// Act
	res, err := Show(configDir, plan.RunID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ShowSchema, res.Schema)
	assert.True(t, res.Signed)
	assert.Equal(t, report.RunID, res.Report.RunID)
	assert.Contains(t, res.Diff, "+GOOD advice.")
	text := res.Text()
	assert.Contains(t, text, "Run "+plan.RunID+" for deploy: accepted")
	assert.Contains(t, text, "95% CI")
	assert.Contains(t, text, "improve pr "+plan.RunID)
	assert.NotContains(t, text, "not signed")
}

func TestShow_FlagsAnUnsignedRunAndRefusesBadIDs(t *testing.T) {
	// Arrange
	_, configDir, plan, _ := acceptedRun(t)
	require.NoError(t, os.Remove(filepath.Join(plan.RunDir(), ReportMACFile)))

	// Act
	res, err := Show(configDir, plan.RunID)
	_, badID := Show(configDir, "../../etc")
	_, missing := Show(configDir, "imp-00000000")

	// Assert
	require.NoError(t, err)
	assert.False(t, res.Signed)
	assert.Contains(t, res.Text(), "not signed")
	require.Error(t, badID)
	require.Error(t, missing)
	assert.Contains(t, missing.Error(), "no saved run")
}

func TestSanitizeMultiline(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"keeps lines and tabs", "a\n\tb\r\nc", "a\n\tb\nc"},
		{"terminal escape", "x\x1b[2Jy", "x�[2Jy"},
		{"bidi override", "a\u202eb", "a�b"},
		{"bell and nul", "a\x07\x00b", "a��b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeMultiline(tt.in))
		})
	}
}

func TestApplyAndShow_NeverPrintTerminalEscapesFromTheCandidate(t *testing.T) {
	// Arrange: the optimizer plants an escape sequence in the skill text.
	root, configDir := project(t)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) { appendSkill(t, dir, "\nGOOD \x1b[2J advice.\n") })
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds = 1
	plan := mustPrepare(t, &o)
	report, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, report.Status)
	var out strings.Builder

	// Act
	shown, serr := Show(configDir, plan.RunID)
	_, aerr := Apply(context.Background(), &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Out: &out, Confirm: func(string) bool { return false }})

	// Assert
	require.NoError(t, serr)
	require.Error(t, aerr, "not confirmed")
	assert.NotContains(t, shown.Text(), "\x1b")
	assert.NotContains(t, out.String(), "\x1b")
	assert.Contains(t, out.String(), "GOOD")
}

func makeRuns(t *testing.T, configDir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		dir := filepath.Join(configDir, LocalDir, id)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "workspace"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), []byte("{}"), 0o600))
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		name     string
		opts     CleanOptions
		dry      bool
		want     []string
		survives []string
		wantErr  string
	}{
		{name: "one run", opts: CleanOptions{RunID: "imp-aaaaaaaa"}, want: []string{"imp-aaaaaaaa"}, survives: []string{"imp-bbbbbbbb", "imp-cccccccc-2"}},
		{name: "all", opts: CleanOptions{All: true}, want: []string{"imp-aaaaaaaa", "imp-bbbbbbbb", "imp-cccccccc-2"}},
		{name: "dry run removes nothing", opts: CleanOptions{All: true, DryRun: true}, want: []string{"imp-aaaaaaaa", "imp-bbbbbbbb", "imp-cccccccc-2"}, survives: []string{"imp-aaaaaaaa", "imp-bbbbbbbb", "imp-cccccccc-2"}},
		{name: "neither id nor all", opts: CleanOptions{}, wantErr: "run id or pass --all"},
		{name: "both id and all", opts: CleanOptions{RunID: "imp-aaaaaaaa", All: true}, wantErr: "run id or pass --all"},
		{name: "traversal id", opts: CleanOptions{RunID: "../.."}, wantErr: "not a run id"},
		{name: "unknown run", opts: CleanOptions{RunID: "imp-dddddddd"}, wantErr: "no saved run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			_, configDir := project(t)
			makeRuns(t, configDir, "imp-aaaaaaaa", "imp-bbbbbbbb", "imp-cccccccc-2")
			require.NoError(t, os.MkdirAll(filepath.Join(configDir, LocalDir, "not-a-run"), 0o750))
			tt.opts.ConfigDir = configDir

			// Act
			res, err := Clean(context.Background(), &tt.opts)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Removed)
			for _, id := range []string{"imp-aaaaaaaa", "imp-bbbbbbbb", "imp-cccccccc-2"} {
				_, statErr := os.Stat(filepath.Join(configDir, LocalDir, id))
				want := false
				for _, s := range tt.survives {
					want = want || s == id
				}
				assert.Equal(t, want, statErr == nil, id)
			}
			assert.DirExists(t, filepath.Join(configDir, LocalDir, "not-a-run"), "a directory that is not a run is never touched")
		})
	}
}

func TestClean_NothingToCleanAndSymlinks(t *testing.T) {
	t.Run("no improve directory", func(t *testing.T) {
		_, configDir := project(t)
		res, err := Clean(context.Background(), &CleanOptions{ConfigDir: configDir, All: true})
		require.NoError(t, err)
		assert.Empty(t, res.Removed)
	})
	t.Run("a symlinked run directory is unlinked, not followed", func(t *testing.T) {
		_, configDir := project(t)
		outside := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("keep"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(configDir, LocalDir), 0o750))
		testutil.SymlinkOrSkip(t, outside, filepath.Join(configDir, LocalDir, "imp-eeeeeeee"))

		res, err := Clean(context.Background(), &CleanOptions{ConfigDir: configDir, RunID: "imp-eeeeeeee"})

		require.NoError(t, err)
		assert.Equal(t, []string{"imp-eeeeeeee"}, res.Removed)
		assert.FileExists(t, filepath.Join(outside, "keep.txt"))
	})
	t.Run("a symlinked improve directory is refused", func(t *testing.T) {
		_, configDir := project(t)
		outside := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(outside, "imp-ffffffff"), 0o750))
		require.NoError(t, os.MkdirAll(filepath.Join(configDir, "local"), 0o750))
		testutil.SymlinkOrSkip(t, outside, filepath.Join(configDir, LocalDir))

		_, err := Clean(context.Background(), &CleanOptions{ConfigDir: configDir, All: true})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a plain directory")
		assert.DirExists(t, filepath.Join(outside, "imp-ffffffff"))
	})
}
