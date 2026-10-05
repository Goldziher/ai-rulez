package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetConvertFlags(t *testing.T, source string) {
	t.Helper()
	convertFrom, convertSource, convertInto, convertDomain = []string{"auto"}, source, ".ai-rulez", ""
	convertDryRun, convertWrite, convertForce = false, false, false
	convertReport, convertFormat, convertFailOn = "", "text", nil
	convertBestEffort, convertSplitHeadings, convertList = false, false, false
	t.Cleanup(func() { resetConvertFlags2() })
}

func resetConvertFlags2() {
	convertFrom, convertSource, convertInto, convertDomain = []string{"auto"}, ".", ".ai-rulez", ""
	convertDryRun, convertWrite, convertForce, convertReport, convertFormat, convertFailOn = false, false, false, "", "text", nil
}

func convertProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Project\n\nUse Go.\n"), 0o644))
	return dir
}

func TestRunConvert(t *testing.T) {
	tests := []struct {
		name        string
		setup       func()
		interactive bool
		wantCode    int
		wantWritten bool
	}{
		{name: "script without write or dry-run is refused", setup: func() {}, wantCode: exitConvertCannotRun},
		{name: "terminal defaults to a dry run", setup: func() {}, interactive: true, wantCode: 0},
		{name: "explicit dry-run", setup: func() { convertDryRun = true }, wantCode: 0},
		{name: "write", setup: func() { convertWrite = true }, wantCode: 0, wantWritten: true},
		{name: "write and dry-run conflict", setup: func() { convertWrite, convertDryRun = true, true }, wantCode: exitConvertCannotRun},
		{name: "fail-on without a match", setup: func() { convertDryRun, convertFailOn = true, []string{"approximated"} }, wantCode: 0},
		{name: "unknown format", setup: func() { convertFormat = "yaml" }, wantCode: exitConvertCannotRun},
		{name: "unknown fail-on", setup: func() { convertDryRun, convertFailOn = true, []string{"bogus"} }, wantCode: exitConvertCannotRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := convertProject(t)
			resetConvertFlags(t, dir)
			tt.setup()
			var out bytes.Buffer

			// Act
			code := runConvert(context.Background(), &out, tt.interactive)

			// Assert
			assert.Equal(t, tt.wantCode, code)
			_, err := os.Stat(filepath.Join(dir, ".ai-rulez", "config.toml"))
			assert.Equal(t, tt.wantWritten, err == nil)
		})
	}
}

func TestRunConvert_JSONReportAndFailOn(t *testing.T) {
	dir := convertProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "GEMINI.md"), []byte("@AGENTS.md\n"), 0o644))
	resetConvertFlags(t, dir)
	convertDryRun, convertFormat = true, "json"
	reportFile := filepath.Join(t.TempDir(), "report.json")
	convertReport = reportFile
	var out bytes.Buffer

	code := runConvert(context.Background(), &out, false)

	require.Equal(t, 0, code)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.EqualValues(t, 1, decoded["schema_version"])
	onDisk, err := os.ReadFile(reportFile)
	require.NoError(t, err)
	assert.JSONEq(t, out.String(), string(onDisk))

	convertFailOn = []string{"dropped"}
	assert.Equal(t, exitConvertBlocked, runConvert(context.Background(), &bytes.Buffer{}, false), "the GEMINI.md pointer is a dropped finding")
}

func TestRunConvert_ListAndConflict(t *testing.T) {
	dir := convertProject(t)
	resetConvertFlags(t, dir)
	convertList = true
	var out bytes.Buffer
	require.Equal(t, 0, runConvert(context.Background(), &out, false))
	assert.Contains(t, out.String(), "native")
	assert.Contains(t, out.String(), "CLAUDE.md")

	convertList, convertWrite = false, true
	require.Equal(t, 0, runConvert(context.Background(), &bytes.Buffer{}, false))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "context", "claude.md"), []byte("edited\n"), 0o644))
	assert.Equal(t, exitConvertCannotRun, runConvert(context.Background(), &bytes.Buffer{}, false), "existing differing file needs --force")
	convertForce = true
	assert.Equal(t, 0, runConvert(context.Background(), &bytes.Buffer{}, false))
}

func TestRunConvert_AutoRunsEveryDetectedImporter(t *testing.T) {
	// Arrange
	dir := convertProject(t)
	lock := `{"version":1,"skills":{"alpha":{"source":"acme/skills","sourceType":"github","ref":"main","skillPath":"skills/alpha/SKILL.md"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills-lock.json"), []byte(lock), 0o644))
	resetConvertFlags(t, dir)
	convertWrite = true

	// Act
	code := runConvert(context.Background(), &bytes.Buffer{}, false)

	// Assert
	require.Equal(t, 0, code)
	cfg, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "[[installed_skills]]")
	_, err = os.Stat(filepath.Join(dir, ".ai-rulez", "context", "claude.md"))
	assert.NoError(t, err)
}

func TestRunConvert_ForceKeepsExistingConfig(t *testing.T) {
	// Arrange
	dir := convertProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	existing := "version = \"4.0\"\nname = \"mine\"\npresets = [\"codex\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(existing), 0o644))
	resetConvertFlags(t, dir)
	convertWrite, convertForce = true, true

	// Act
	code := runConvert(context.Background(), &bytes.Buffer{}, false)

	// Assert
	require.Equal(t, 0, code)
	cfg, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "name = 'mine'")
	assert.Contains(t, string(cfg), "'codex'")
}
