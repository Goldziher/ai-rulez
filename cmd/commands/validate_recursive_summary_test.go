package commands

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

// nestedBrokenLinkWorkspace builds a two-root tree whose nested root "b" holds a
// rule with an unresolved link (AR201), makes it the working directory, resets
// the strict tree loader, and turns progress output back on so a test can read
// the summary. twoRoots already silences progress.
func nestedBrokenLinkWorkspace(t *testing.T) string {
	t.Helper()
	root := twoRoots(t, validRootConfig)
	writeFile(t, filepath.Join(root, "b", ".ai-rulez", "rules", "bad.md"), brokenLinkRule)
	strictTreeCache = &lint.Loader{}
	t.Cleanup(func() { strictTreeCache = &lint.Loader{} })
	progress.SetQuiet(false)
	return root
}

// TestRunRecursiveValidate_FindingsSummaryReflectsExit is the regression test
// for #338: a recursive run with a nested AR201 exited 2 while still printing
// "All N config(s) are valid". The summary must name how many configs were
// checked and that findings were reported instead of claiming the tree is valid.
func TestRunRecursiveValidate_FindingsSummaryReflectsExit(t *testing.T) {
	// Arrange
	nestedBrokenLinkWorkspace(t)
	t.Cleanup(func() { validateStrict = false })
	validateStrict = true

	// Act
	var code int
	stdout, stderr := capture(t, func() { code = runRecursiveValidate() })

	// Assert
	require.Equal(t, exitStrictFindings, code)
	combined := stdout + stderr
	assert.NotContains(t, combined, "config(s) are valid")
	assert.Contains(t, combined, "2 config(s) checked; findings were reported")
}

// TestRecursiveValidateAndGenerateCountAgree pins that the same clean tree
// yields the same root count in `validate --recursive` and `generate
// --recursive`, so the two commands cannot disagree on how many configs exist.
func TestRecursiveValidateAndGenerateCountAgree(t *testing.T) {
	// Arrange
	twoRoots(t, validRootConfig)
	t.Cleanup(func() { validateStrict, pluginMode, dryRun = false, false, false })
	validateStrict, pluginMode, dryRun = true, false, true
	progress.SetQuiet(false)

	// Act
	var validateCode int
	vOut, vErr := capture(t, func() { validateCode = runRecursiveValidate() })
	var generateCode int
	gOut, gErr := capture(t, func() { generateCode = runRecursiveGenerate() })

	// Assert
	require.Equal(t, 0, validateCode)
	require.Equal(t, 0, generateCode)
	assert.Equal(t,
		configCount(t, vOut+vErr, `All (\d+) config\(s\) are valid`),
		configCount(t, gOut+gErr, `from (\d+) config\(s\)`),
		"validate and generate must count the same roots")
}

// TestRunRecursiveValidate_JSONStaysParseableWithFindings pins that the error
// summary goes to stderr, leaving stdout one parseable JSON document.
func TestRunRecursiveValidate_JSONStaysParseableWithFindings(t *testing.T) {
	// Arrange
	nestedBrokenLinkWorkspace(t)
	resetValidateFlags(t)
	t.Setenv(repoRootEnv, "")
	viper.Set("quiet", false)
	t.Cleanup(func() { viper.Set("quiet", nil) }) // nil clears the override, so a later test reads the environment again
	validateRecursive, validateFormat, validateStrict = true, "json", true

	// Act
	var runErr error
	stdout, _ := capture(t, func() { runErr = ValidateCmd.RunE(ValidateCmd, nil) })

	// Assert
	require.Error(t, runErr)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), "stdout must be one JSON document: %q (err %v)", stdout, runErr)
}

// configCount extracts the integer captured by re from text, failing the test
// when the pattern does not match.
func configCount(t *testing.T, text, re string) int {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(text)
	require.Len(t, m, 2, "no count matched %q in:\n%s", re, text)
	n, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	return n
}
