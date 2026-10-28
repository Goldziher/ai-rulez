package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

// nestedRootsWorkspace builds a working tree with a root config and one nested
// root, makes it the working directory, and returns it.
func nestedRootsWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, "svc", ".ai-rulez", "config.toml"), validRootConfig)
	chdir(t, root)
	return root
}

// resetValidateFlags restores the validate globals a RunE call may read, so a
// test that drives the command in-process leaves no state behind.
func resetValidateFlags(t *testing.T) {
	t.Helper()
	oldRecursive, oldFormat, oldStrict := validateRecursive, validateFormat, validateStrict
	oldConfigOnly, oldWarnings, oldFailOn := validateConfigOnly, validateWarnings, validateFailOn
	oldOutput, oldExplain, oldShowPolicy := validateOutput, validateExplain, validateShowPolicy
	oldRepoRoot, oldNoLocal := validateRepoRoot, noLocal
	oldMCPUserScope := validateMCPUserScope
	t.Cleanup(func() {
		validateRecursive, validateFormat, validateStrict = oldRecursive, oldFormat, oldStrict
		validateConfigOnly, validateWarnings, validateFailOn = oldConfigOnly, oldWarnings, oldFailOn
		validateOutput, validateExplain, validateShowPolicy = oldOutput, oldExplain, oldShowPolicy
		validateRepoRoot, noLocal = oldRepoRoot, oldNoLocal
		validateMCPUserScope = oldMCPUserScope
		progress.SetQuiet(false)
	})
}

// TestValidateWarnsAboutSkippedNestedRoots pins the fix for a CI that omits
// --recursive: a non-recursive run names the nested roots it did not check, the
// recursive run (which checks them) stays silent, and a structured format keeps
// stdout parseable.
func TestValidateWarnsAboutSkippedNestedRoots(t *testing.T) {
	tests := []struct {
		name       string
		recursive  bool
		format     string
		strict     bool
		wantNotice bool
		wantJSON   bool
	}{
		{name: "non-recursive names the nested roots", wantNotice: true},
		{name: "recursive checks them and says nothing", recursive: true},
		{name: "json output stays parseable", format: "json", strict: true, wantJSON: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			nestedRootsWorkspace(t)
			resetValidateFlags(t)
			viper.Set("quiet", false)
			t.Cleanup(func() { viper.Set("quiet", nil) }) // nil clears the override, so a later test reads the environment again
			validateRecursive, validateFormat, validateStrict = tt.recursive, tt.format, tt.strict

			// Act
			var runErr error
			stdout, _ := capture(t, func() { runErr = ValidateCmd.RunE(ValidateCmd, nil) })

			// Assert
			notice := "nested root(s) not checked; run with --recursive"
			if tt.wantNotice {
				assert.Contains(t, stdout, "1 "+notice)
			} else {
				assert.NotContains(t, stdout, notice)
			}
			if tt.wantJSON {
				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc), "stdout must be one JSON document: %q (err %v)", stdout, runErr)
			}
		})
	}
}

// TestWarnNestedRootsNotChecked_Quiet pins that the notice respects --quiet,
// which is what keeps --format json parseable.
func TestWarnNestedRootsNotChecked_Quiet(t *testing.T) {
	// Arrange
	root := nestedRootsWorkspace(t)
	resetValidateFlags(t)
	cfg, err := loadProject(cmdContext(), root)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, ".ai-rulez", "config.toml"), filepath.Join(cfg.ConfigDir, cfg.ConfigFile))

	// Act
	progress.SetQuiet(true)
	stdout, _ := capture(t, func() { warnNestedRootsNotChecked(cfg) })

	// Assert
	assert.Empty(t, stdout, "--quiet must silence the nested-roots notice")
}
