package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

// runWithBuffers runs the RunE of cmd with separate stdout and stderr buffers.
func runWithBuffers(t *testing.T, cmd *cobra.Command) (stdout, stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	t.Cleanup(func() { cmd.SetOut(nil); cmd.SetErr(nil) })
	err = cmd.RunE(cmd, nil)
	return out.String(), errBuf.String(), err
}

func setQuiet(t *testing.T) {
	t.Helper()
	viper.Set("quiet", true)
	t.Cleanup(func() { viper.Set("quiet", false) })
}

func listFixture(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	resetContentFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"streams\"\npresets = [\"claude\"]\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "alpha.md"), "---\npriority: high\n---\n# Alpha\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez", "domains", "team", "rules"), 0o755))
	chdir(t, root)
}

// A list is the command's result: it is on stdout, stderr stays empty, and -q
// does not hide it.
func TestListResultsGoToStdoutAndSurviveQuiet(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{"list rules", listRulesCmd, "alpha"},
		{"domain list", domainListCmd, "team"},
	}
	for _, quiet := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(tt.name+map[bool]string{false: "", true: " -q"}[quiet], func(t *testing.T) {
				listFixture(t)
				if quiet {
					setQuiet(t)
				}

				stdout, stderr, err := runWithBuffers(t, tt.cmd)

				require.NoError(t, err)
				assert.Contains(t, stdout, tt.want)
				assert.Empty(t, stderr)
			})
		}
	}
}

// "Nothing found" is a diagnostic: stderr, and -q removes it. JSON mode still
// prints its document on stdout.
func TestEmptyListNoticeIsADiagnostic(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		json *bool
	}{
		{"profile list", profileListCmd, &profileJSON},
		{"include list", includeListCmd, &includeJSON},
		{"skill list", skillListCmd, &skillJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listFixture(t)

			stdout, stderr, err := runWithBuffers(t, tt.cmd)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "found")

			setQuiet(t)
			stdout, stderr, err = runWithBuffers(t, tt.cmd)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)

			*tt.json = true
			t.Cleanup(func() { *tt.json = false })
			stdout, _, err = runWithBuffers(t, tt.cmd)
			require.NoError(t, err)
			var doc struct {
				SchemaVersion int `json:"schema_version"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
			assert.Equal(t, 1, doc.SchemaVersion)
		})
	}
}

func generatedProject(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic"), dir)
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).Generate("default"))
	chdir(t, dir)
	prev := cleanDryRun
	t.Cleanup(func() { cleanDryRun, cleanFormat = prev, "" })
	cleanDryRun = true
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path) //nolint:errcheck // path is below src
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}))
}

// The clean --dry-run plan is the result: stdout, surviving -q, nothing removed.
func TestCleanDryRunPlanGoesToStdoutAndSurvivesQuiet(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "quiet"}[quiet], func(t *testing.T) {
			generatedProject(t)
			if quiet {
				setQuiet(t)
			}

			stdout, stderr, err := runWithBuffers(t, CleanCmd)

			require.NoError(t, err)
			assert.Contains(t, stdout, "remove file:")
			if quiet {
				assert.Empty(t, stderr)
			} else {
				assert.Contains(t, stderr, "Dry run")
			}
			assert.DirExists(t, ".claude")
		})
	}
}

func TestCleanFormatJSONPlan(t *testing.T) {
	generatedProject(t)
	cleanFormat = formatJSON

	stdout, _, err := runWithBuffers(t, CleanCmd)

	require.NoError(t, err)
	var doc struct {
		SchemaVersion int      `json:"schema_version"`
		Status        string   `json:"status"`
		DryRun        bool     `json:"dry_run"`
		Files         []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, "planned", doc.Status)
	assert.True(t, doc.DryRun)
	assert.NotEmpty(t, doc.Files)
}

// Without --yes and without a terminal, clean refuses with the standard hint and
// removes nothing.
func TestCleanNeedsYesWithoutTerminal(t *testing.T) {
	generatedProject(t)
	cleanDryRun = false
	prevCheck := stdinInteractive
	stdinInteractive = func() bool { return false }
	t.Cleanup(func() { stdinInteractive = prevCheck })

	_, _, err := runWithBuffers(t, CleanCmd)

	require.ErrorIs(t, err, ErrNeedsYes)
	assert.Equal(t, exitFailure, exitCodeFor(err))
	assert.DirExists(t, ".claude")
}
