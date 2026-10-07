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

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

func resetConvertFlags(t *testing.T, source string) {
	t.Helper()
	convertFrom, convertSource, convertInto, convertDomain = []string{"auto"}, source, ".ai-rulez", ""
	convertDryRun, convertWrite, convertForce = false, false, false
	convertReport, convertFormat, convertFailOn = "", "text", nil
	convertBestEffort, convertSplitHeadings, convertList = false, false, false
	convertEnableHooks, convertEnablePerms = false, false
	convertMerge, convertKeepNames, convertLock, convertDelivery, convertFetch = false, false, false, "", false
	t.Cleanup(func() { resetConvertFlags2() })
}

func resetConvertFlags2() {
	convertFrom, convertSource, convertInto, convertDomain = []string{"auto"}, ".", ".ai-rulez", ""
	convertDryRun, convertWrite, convertForce, convertReport, convertFormat, convertFailOn = false, false, false, "", "text", nil
	convertEnableHooks, convertEnablePerms = false, false
	convertMerge, convertKeepNames, convertLock, convertDelivery, convertFetch = false, false, false, "", false
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
	existing := "version = \"5.0\"\nname = \"mine\"\npresets = [\"codex\"]\n"
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

func TestRunConvert_EnableFlagsDecideWhetherHooksAreLive(t *testing.T) {
	const settings = `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]},"permissions":{"allow":["Bash(ls)"]}}`
	tests := []struct {
		name         string
		hooks, perms bool
		wantHooks    int
		wantAllow    int
	}{
		{name: "nothing enabled by default"},
		{name: "hooks only", hooks: true, wantHooks: 1},
		{name: "permissions only", perms: true, wantAllow: 1},
		{name: "both", hooks: true, perms: true, wantHooks: 1, wantAllow: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := convertProject(t)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(settings), 0o644))
			resetConvertFlags(t, dir)
			convertWrite, convertEnableHooks, convertEnablePerms = true, tt.hooks, tt.perms

			// Act
			code := runConvert(context.Background(), &bytes.Buffer{}, false)

			// Assert
			require.Equal(t, 0, code)
			data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
			require.NoError(t, err)
			cfg, err := config.DecodeTOMLConfig(data, "config.toml")
			require.NoError(t, err)
			assert.Len(t, cfg.Hooks, tt.wantHooks)
			allow := 0
			if cfg.Permissions != nil {
				allow = len(cfg.Permissions.Allow)
			}
			assert.Equal(t, tt.wantAllow, allow)
		})
	}
}

func TestRunConvert_WithoutFetchRemoteSourcesStayOffline(t *testing.T) {
	// Arrange: a rulesync source on an unroutable host; any fetch would hang or fail.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".rulesync", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rulesync.jsonc"), []byte(`{"sources":[{"source":"acme/skills","skills":["x"]}]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".rulesync", "rules", "r.md"), []byte("Rule.\n"), 0o644))
	resetConvertFlags(t, dir)
	convertDryRun, convertFormat = true, "json"
	var out bytes.Buffer

	// Act
	code := runConvert(context.Background(), &out, false)

	// Assert
	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "convert uses the network only with --fetch")
}

func TestCheckConvertFlags(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantErr string
	}{
		{name: "merge and force", setup: func() { convertWrite, convertMerge, convertForce = true, true, true }, wantErr: "--merge and --force"},
		{name: "lock needs write", setup: func() { convertDryRun, convertLock = true, true }, wantErr: "--lock pins what was written"},
		{name: "merge alone", setup: func() { convertWrite, convertMerge = true, true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConvertFlags(t, t.TempDir())
			tt.setup()

			err := checkConvertFlags(false)

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRunConvert_MergeKeepsExistingAndDeliverySetsTheDefault(t *testing.T) {
	// Arrange
	dir := convertProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "skills", "lint"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: Lint\n---\nRun lint.\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "context"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "context", "claude.md"), []byte("Mine.\n"), 0o644))
	resetConvertFlags(t, dir)
	convertWrite, convertMerge, convertDelivery = true, true, "served"

	// Act
	code := runConvert(context.Background(), &bytes.Buffer{}, false)

	// Assert
	require.Equal(t, 0, code)
	kept, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "context", "claude.md"))
	require.NoError(t, err)
	assert.Equal(t, "Mine.\n", string(kept))
	_, err = os.Stat(filepath.Join(dir, ".ai-rulez", "context", "claude-imported.md"))
	assert.NoError(t, err)
	cfg, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "delivery = 'served'")
}

func TestRunConvert_LockPinsTheConvertedTree(t *testing.T) {
	// Arrange
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	dir := convertProject(t)
	resetConvertFlags(t, dir)
	convertWrite, convertLock = true, true

	// Act
	code := runConvert(context.Background(), &bytes.Buffer{}, false)

	// Assert
	require.Equal(t, 0, code)
	_, err := os.Stat(filepath.Join(dir, ".ai-rulez", "ai-rulez.lock"))
	assert.NoError(t, err, "--lock writes the lock next to the converted config")
}

func TestRunConvert_LockToleratesAnUnsetMCPVariable(t *testing.T) {
	// Arrange: an imported MCP server whose token placeholder is not set in this environment.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("AI_RULEZ_TEST_UNSET_TOKEN", "")
	require.NoError(t, os.Unsetenv("AI_RULEZ_TEST_UNSET_TOKEN"))
	dir := convertProject(t)
	mcp := `{"mcpServers":{"api":{"type":"http","url":"https://mcp.example.com/mcp","headers":{"Authorization":"Bearer ${AI_RULEZ_TEST_UNSET_TOKEN}"}}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcp), 0o644))
	resetConvertFlags(t, dir)
	convertWrite, convertLock = true, true

	// Act
	var out bytes.Buffer
	code := runConvert(context.Background(), &out, false)

	// Assert
	require.Equal(t, 0, code, out.String())
	_, err := os.Stat(filepath.Join(dir, ".ai-rulez", "ai-rulez.lock"))
	assert.NoError(t, err, "the lock is written although the variable is unset")
}

func TestPrintConvertReport_ReportsAFailureToWriteTheFile(t *testing.T) {
	resetConvertFlags(t, t.TempDir())
	convertReport = t.TempDir() // a directory: the file cannot be written
	var out bytes.Buffer

	err := printConvertReport(&out, &importer.Report{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write report file")
}

func TestConvertAllowFindingsCodeFormat(t *testing.T) {
	for code, ok := range map[string]bool{"AR001": true, "ar001": true, "AR9F5": true, "x": false, "AR": false, "../x": false} {
		if got := allowCodeRe.MatchString(code); got != ok {
			t.Errorf("%q matched = %v, want %v", code, got, ok)
		}
	}
}

// Nothing to import is not a failure: a script that converts a fleet of
// repositories must not stop at the ones that have no tool files.
func TestRunConvert_NothingToConvertExitsZero(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
	}{
		{"dry run", func() { convertDryRun = true }},
		{"write", func() { convertWrite = true }},
		{"json", func() { convertDryRun, convertFormat = true, "json" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hi\n"), 0o644))
			resetConvertFlags(t, dir)
			tt.setup()
			var out bytes.Buffer

			// Act
			code := runConvert(context.Background(), &out, false)

			// Assert
			assert.Equal(t, 0, code)
			_, err := os.Stat(filepath.Join(dir, ".ai-rulez"))
			assert.True(t, os.IsNotExist(err), "nothing is written")
			if convertFormat == "text" {
				assert.Contains(t, out.String(), "Nothing to convert")
			}
		})
	}
}
