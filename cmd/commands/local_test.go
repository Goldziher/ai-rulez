package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const localCmdShared = "version = \"5.0\"\nname = \"shared-name\"\npresets = [\"claude\"]\n\n" +
	"[[mcp_servers]]\nname = \"gh\"\ncommand = \"gh\"\n[mcp_servers.env]\nGH_TOKEN = \"shared-secret\"\n"

// localProject creates a project with a shared TOML config and makes it the
// working directory. It returns the config directory.
func localProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".ai-rulez")
	writeFile(t, filepath.Join(dir, "config.toml"), localCmdShared)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "domains", "backend", "rules"), 0o755))
	chdir(t, root)
	return dir
}

// captureStdout runs fn and returns what it wrote to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestParseLocalValue(t *testing.T) {
	tests := []struct {
		name  string
		path  []string
		in    string
		force bool
		want  any
	}{
		{"bool", []string{"gitignore"}, "true", false, true},
		{"int", []string{"defaults", "x"}, "8080", false, int64(8080)},
		{"quoted string", []string{"description"}, `"quoted"`, false, `"quoted"`},
		{"list", []string{"presets"}, `["a", "!b"]`, false, []any{"a", "!b"}},
		{"plain string", []string{"default"}, "plain-string", false, "plain-string"},
		{"free text", []string{"mcp_servers", "x", "command"}, "npx -y server", false, "npx -y server"},
		{"env value 123_456 stays text", []string{"mcp_servers", "x", "env", "PIN"}, "123_456", false, "123_456"},
		{"env value 0x1F stays text", []string{"mcp_servers", "x", "env", "PIN"}, "0x1F", false, "0x1F"},
		{"env value 1e5 stays text", []string{"mcp_servers", "x", "env", "PIN"}, "1e5", false, "1e5"},
		{"header value stays text", []string{"mcp_servers", "x", "headers", "X-Id"}, "42", false, "42"},
		{"known text field stays text", []string{"mcp_servers", "x", "url"}, "true", false, "true"},
		{"version field stays text", []string{"mcp_servers", "x", "self_server_version"}, "1.0", false, "1.0"},
		{"--string forces text", []string{"gitignore"}, "true", true, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLocalValue(tt.path, tt.in, tt.force))
		})
	}
}

func TestLocalSetUnsetShow(t *testing.T) {
	// Arrange
	dir := localProject(t)
	localPath := filepath.Join(dir, "config.local.toml")

	// Act: set
	require.NoError(t, localSetCmd.RunE(localSetCmd, []string{"description", "mine"}))
	require.NoError(t, localSetCmd.RunE(localSetCmd, []string{"presets", `["codex"]`}))
	require.NoError(t, localSetCmd.RunE(localSetCmd, []string{"mcp_servers.gh.env.GH_TOKEN", "local-secret"}))
	require.NoError(t, localSetCmd.RunE(localSetCmd, []string{
		"mcp_servers.web",
		`{ transport = "http", url = "https://example.com/mcp", headers = { Authorization = "Bearer abc" } }`,
	}))

	// Assert: the shared config is untouched and the overlay holds the values.
	shared, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, localCmdShared, string(shared))
	local, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Contains(t, string(local), "description = 'mine'")
	assert.Contains(t, string(local), "local-secret")

	// show never prints secret values, in text or JSON form
	for _, asJSON := range []bool{false, true} {
		localShowJSON = asJSON
		t.Cleanup(func() { localShowJSON = false })
		out := captureStdout(t, func() { require.NoError(t, localShowCmd.RunE(localShowCmd, nil)) })
		localShowJSON = false
		assert.Contains(t, out, "description")
		assert.Contains(t, out, "<redacted>")
		assert.NotContains(t, out, "local-secret")
		assert.NotContains(t, out, "shared-secret")
		assert.NotContains(t, out, "Bearer abc")
		if asJSON {
			assert.True(t, json.Valid([]byte(out)), out)
		}
	}

	// unset removes the key
	require.NoError(t, localUnsetCmd.RunE(localUnsetCmd, []string{"description"}))
	local, err = os.ReadFile(localPath)
	require.NoError(t, err)
	assert.NotContains(t, string(local), "description")

	// path prints the overlay path
	out := captureStdout(t, func() { require.NoError(t, localPathCmd.RunE(localPathCmd, nil)) })
	assert.Equal(t, filepath.Join(".ai-rulez", "config.local.toml")+"\n", out)
}

func TestLocalInit(t *testing.T) {
	// Arrange
	dir := localProject(t)

	// Act
	first := captureStdout(t, func() { require.NoError(t, localInitCmd.RunE(localInitCmd, nil)) })
	second := captureStdout(t, func() { require.NoError(t, localInitCmd.RunE(localInitCmd, nil)) })

	// Assert
	assert.Contains(t, first, "Created")
	assert.Contains(t, second, "already exists")
	assert.FileExists(t, filepath.Join(dir, "config.local.toml"))
}

func TestProfileAddLocal_WritesOnlyTheOverlay(t *testing.T) {
	// Arrange
	dir := localProject(t)
	profileLocal = true
	t.Cleanup(func() { profileLocal = false })

	// Act
	runProfileAdd(profileAddCmd, []string{"mine", "backend"})

	// Assert
	shared, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, localCmdShared, string(shared))
	local, err := os.ReadFile(filepath.Join(dir, "config.local.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(local), "mine")
}

func TestLocalCommands_HonourConfigFlag(t *testing.T) {
	// Arrange: the project lives elsewhere; the working directory is unrelated
	root := t.TempDir()
	dir := filepath.Join(root, "proj", ".ai-rulez")
	writeFile(t, filepath.Join(dir, "config.toml"), localCmdShared)
	chdir(t, t.TempDir())
	prev := cfgFile
	cfgFile = dir
	t.Cleanup(func() { cfgFile = prev })

	// Act
	path, created, err := config.InitLocalOverlayAt(localConfigDir())

	// Assert
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, filepath.Join(dir, "config.local.toml"), path)
}

func TestPrintDryRun_BlockedDriftIsAnError(t *testing.T) {
	// Arrange: the overlay renames the project, so shared CLAUDE.md would change.
	dir := localProject(t)
	writeFile(t, filepath.Join(dir, "config.toml"), "version = \"5.0\"\nname = \"shared\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(dir, "config.local.toml"), "name = \"mine\"\npresets = [\"codex\"]\n")
	cfg, err := config.LoadConfig(t.Context(), ".")
	require.NoError(t, err)

	// Act
	var runErr error
	out := captureStdout(t, func() { runErr = printDryRun(generator.NewGenerator(cfg), "") })

	// Assert
	require.Error(t, runErr)
	assert.Contains(t, out, "blocked: CLAUDE.md")
}

func TestLocalEntriesHint(t *testing.T) {
	dir := localProject(t)
	assert.Empty(t, localEntriesHint("profiles"), "no overlay, no hint")

	writeFile(t, filepath.Join(dir, "config.local.toml"),
		"[profiles]\nmine = [\"backend\"]\nother = [\"backend\"]\n\n[[includes]]\nname = \"i\"\nsource = \"./x\"\n")

	assert.Equal(t, "+ 2 local entries; see `ai-rulez local show`", localEntriesHint("profiles"))
	assert.Equal(t, "+ 1 local entries; see `ai-rulez local show`", localEntriesHint("includes"))
	assert.Empty(t, localEntriesHint("installed_skills"))
}
