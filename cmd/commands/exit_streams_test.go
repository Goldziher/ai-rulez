package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setQuiet(t *testing.T) {
	t.Helper()
	viper.Set("quiet", true)
	t.Cleanup(func() { viper.Set("quiet", false) })
}

func TestLockCheck_DriftReportIsOnStdoutAndSurvivesQuiet(t *testing.T) {
	// Arrange: the rule is edited after the lock was written
	root := lockedProject(t)
	t.Chdir(root)
	setQuiet(t)
	lockCheck = true
	t.Cleanup(func() { lockCheck = false })

	// Act
	var code int
	stdout, stderr := capture(t, func() { code = runLockFor("", nil) })

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stdout, "does not match")
	assert.NotContains(t, stderr, "does not match")
}

func TestLockCheck_UpToDateVerdictIsOnStdoutAndSurvivesQuiet(t *testing.T) {
	root := lockedProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	t.Chdir(root)
	setQuiet(t)
	lockCheck = true
	t.Cleanup(func() { lockCheck = false })

	var code int
	stdout, _ := capture(t, func() { code = runLockFor("", nil) })

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "up to date")
}

func TestGenerateCheck_DriftedFilesAreOnStdoutAndSurviveQuiet(t *testing.T) {
	root := lockedProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse tabs.\n")
	t.Chdir(root)
	setQuiet(t)

	var code int
	stdout, stderr := capture(t, func() { code = generateCheckCode(nil) })

	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stdout, "stale: ")
	assert.NotContains(t, stderr, "stale: ")
}

func TestGenerateCheck_JSONFormatEmitsOneDocumentWithTheDrift(t *testing.T) {
	root := lockedProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse tabs.\n")
	t.Chdir(root)
	generateFormat = formatJSON
	t.Cleanup(func() { generateFormat = "" })

	var code int
	stdout, _ := capture(t, func() { code = generateCheckCode(nil) })

	assert.Equal(t, exitDrift, code)
	var doc driftDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, "drift", doc.Status)
	assert.NotEmpty(t, doc.Differing)
}

func TestGenerateRunE_UsageErrorsAreExitErrorsWithCodeOne(t *testing.T) {
	generateRole, profile = "r", "p"
	t.Cleanup(func() { generateRole, profile = "", "" })

	err := runGenerate(GenerateCmd, nil)

	var exitErr *ExitError
	require.True(t, errors.As(err, &exitErr))
	assert.Equal(t, exitFailure, exitErr.Code)
	assert.Equal(t, exitFailure, exitCodeFor(err))
}

func TestReportError_RendersTextOnStderrAndJSONDocumentOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := ReportError(&stdout, &stderr, formatJSON, failWithCode(exitDrift, errors.New("boom")))

	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr.String(), "Error: boom")
	var doc errorDocument
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "boom", doc.Error)
	assert.Equal(t, exitDrift, doc.ExitCode)
}
