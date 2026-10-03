package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLocalPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr string
	}{
		{"nested table key", "defaults.effort_by_preset.claude", []string{"defaults", "effort_by_preset", "claude"}, ""},
		{"named list entry field", "mcp_servers.github.env.TOKEN", []string{"mcp_servers", "github", "env", "TOKEN"}, ""},
		{"scalar", "default", []string{"default"}, ""},
		{"$schema normalized", "$schema", []string{"schema"}, ""},
		{"named list needs an entry name", "mcp_servers", nil, "list of named entries"},
		{"unknown top-level key", "nmea", nil, "unknown config key"},
		{"empty segment", "profiles..dev", nil, "invalid key path"},
		{"bracket-quoted segment with a dot", `mcp_servers["foo.bar"].command`, []string{"mcp_servers", "foo.bar", "command"}, ""},
		{"bracket-quoted env key", `mcp_servers["a.b"].env["X.Y"]`, []string{"mcp_servers", "a.b", "env", "X.Y"}, ""},
		{"bracket escape", `mcp_servers["a\"b"].command`, []string{"mcp_servers", `a"b`, "command"}, ""},
		{"unterminated bracket", `mcp_servers["foo.command`, nil, "invalid key path"},
		{"junk after bracket", `mcp_servers["foo"]x.command`, nil, "invalid key path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := ParseLocalPath(tt.in)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func newLocalDoc(t *testing.T, doc string) *LocalDoc {
	t.Helper()
	return &LocalDoc{Format: "toml", Doc: decodeDoc(t, doc), Path: filepath.Join(t.TempDir(), "config.local.toml")}
}

func TestLocalDoc_Edits(t *testing.T) {
	tests := []struct {
		name  string
		start string
		edit  func(d *LocalDoc) error
		want  string // JSON of the resulting document
	}{
		{
			"set scalar", ``,
			func(d *LocalDoc) error { return d.SetDefault("dev") },
			`{"default":"dev"}`,
		},
		{
			"set nested value creates tables", ``,
			func(d *LocalDoc) error { return d.Set([]string{"defaults", "effort_by_preset", "claude"}, "high") },
			`{"defaults":{"effort_by_preset":{"claude":"high"}}}`,
		},
		{
			"set field of a named entry creates it", ``,
			func(d *LocalDoc) error { return d.Set([]string{"mcp_servers", "gh", "env", "TOKEN"}, "x") },
			`{"mcp_servers":[{"name":"gh","env":{"TOKEN":"x"}}]}`,
		},
		{
			"unset prunes empty tables", "[defaults.effort_by_preset]\nclaude = \"high\"\n",
			func(d *LocalDoc) error { return d.Unset([]string{"defaults", "effort_by_preset", "claude"}) },
			`{}`,
		},
		{
			"unset last field drops the named entry", "[[mcp_servers]]\nname = \"gh\"\ncommand = \"x\"\n",
			func(d *LocalDoc) error { return d.Unset([]string{"mcp_servers", "gh", "command"}) },
			`{}`,
		},
		{
			"unset whole named entry", "[[mcp_servers]]\nname = \"gh\"\ncommand = \"x\"\n[[mcp_servers]]\nname = \"b\"\ncommand = \"y\"\n",
			func(d *LocalDoc) error { return d.Unset([]string{"mcp_servers", "gh"}) },
			`{"mcp_servers":[{"name":"b","command":"y"}]}`,
		},
		{
			"upsert merges fields", "[[includes]]\nname = \"a\"\nsource = \"s\"\n",
			func(d *LocalDoc) error { return d.UpsertNamed("includes", "a", map[string]any{"ref": "main"}) },
			`{"includes":[{"name":"a","source":"s","ref":"main"}]}`,
		},
		{
			"remove writes remove marker when shared has the entry", ``,
			func(d *LocalDoc) error { return d.RemoveNamed("includes", "a", true) },
			`{"includes":[{"name":"a","remove":true}]}`,
		},
		{
			"remove drops a purely local entry", "[[includes]]\nname = \"a\"\nsource = \"s\"\n",
			func(d *LocalDoc) error { return d.RemoveNamed("includes", "a", false) },
			`{}`,
		},
		{
			"remove replaces a local override with the marker", "[[includes]]\nname = \"a\"\nsource = \"s\"\n",
			func(d *LocalDoc) error { return d.RemoveNamed("includes", "a", true) },
			`{"includes":[{"name":"a","remove":true}]}`,
		},
		{
			"profile set and remove", "[profiles]\nold = [\"x\"]\n",
			func(d *LocalDoc) error {
				if err := d.SetProfile("dev", []string{"a", "b"}); err != nil {
					return err
				}
				return d.RemoveProfile("old")
			},
			`{"profiles":{"dev":["a","b"]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			d := newLocalDoc(t, tt.start)

			// Act
			err := tt.edit(d)

			// Assert
			require.NoError(t, err)
			assert.True(t, jsonEqual(t, mustJSON(t, d.Doc), tt.want), "got %s want %s", mustJSON(t, d.Doc), tt.want)
		})
	}
}

func TestLocalDoc_RejectsBadPaths(t *testing.T) {
	d := newLocalDoc(t, "default = \"dev\"\n")

	tests := []struct {
		name string
		fn   func() error
	}{
		{"unknown key", func() error { return d.Set([]string{"nmea"}, 1) }},
		{"list without entry", func() error { return d.Set([]string{"mcp_servers"}, 1) }},
		{"set under a scalar", func() error { return d.Set([]string{"default", "x"}, 1) }},
		{"entry value must be a table", func() error { return d.Set([]string{"mcp_servers", "gh"}, "x") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.fn())
		})
	}
}

// overlayProject writes a shared config.toml and returns the project dir.
func overlayProject(t *testing.T, shared string) (base, configDir string) {
	t.Helper()
	base = t.TempDir()
	configDir = filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, configDir, "config.toml", shared)
	return base, configDir
}

func TestLocalDoc_SaveWritesOwnerOnlyAndIgnoresInGit(t *testing.T) {
	// Arrange
	base, configDir := overlayProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.SetDefault("dev"))
	require.NoError(t, d.SetProfile("dev", []string{"builtin:go"}))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(configDir, "config.local.toml"))
	require.NoError(t, err)
	if runtime.GOOS != "windows" { // Windows has no Unix permission bits
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	ignore, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(ignore), ".ai-rulez/config.local.*")
	merged, err := LoadConfig(t.Context(), base)
	require.NoError(t, err)
	assert.Equal(t, "dev", merged.Default)
}

func TestLocalDoc_SaveRollsBackWhenMergedConfigIsInvalid(t *testing.T) {
	tests := []struct {
		name     string
		existing string // existing overlay content; empty means none
	}{
		{"restores the previous overlay", "name = \"before\"\n"},
		{"removes a newly created overlay", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, configDir := overlayProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
			localPath := filepath.Join(configDir, "config.local.toml")
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(localPath, []byte(tt.existing), 0o600))
			}
			d, err := OpenLocalDoc(configDir, "config.toml")
			require.NoError(t, err)
			t.Cleanup(d.Close)
			require.NoError(t, d.Set([]string{"presets"}, []string{"not-a-real-preset"}))

			// Act
			err = d.Save(t.Context())

			// Assert
			require.Error(t, err)
			after, readErr := os.ReadFile(localPath)
			if tt.existing == "" {
				assert.True(t, os.IsNotExist(readErr), "a new overlay must be removed")
			} else {
				require.NoError(t, readErr)
				assert.Equal(t, tt.existing, string(after))
			}
			_, loadErr := LoadConfig(t.Context(), base)
			assert.NoError(t, loadErr, "the project must still load after a rejected change")
		})
	}
}

func TestLocalDoc_SaveKeepsFormatOfMainConfig(t *testing.T) {
	// Arrange
	base := t.TempDir()
	configDir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, configDir, "config.yaml", "version: \"4.0\"\nname: x\npresets: [claude]\n")
	d, err := OpenLocalDoc(configDir, "config.yaml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"description"}, "mine"))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(configDir, "config.local.yaml"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "description: mine"), string(data))
}

func TestInitLocalOverlay(t *testing.T) {
	// Arrange
	base, configDir := overlayProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n")

	// Act
	path, created, err := InitLocalOverlay(base)
	secondPath, secondCreated, secondErr := InitLocalOverlay(base)

	// Assert
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, filepath.Join(configDir, "config.local.toml"), path)
	require.NoError(t, secondErr)
	assert.False(t, secondCreated, "init is a no-op when the overlay exists")
	assert.Equal(t, path, secondPath)
	_, loadErr := LoadConfig(t.Context(), base)
	assert.NoError(t, loadErr, "the skeleton is a valid empty overlay")
}

func TestDescribeLocalOverlay_RedactsSecrets(t *testing.T) {
	// Arrange
	base, configDir := overlayProject(t, `version = "4.0"
name = "shared-name"
presets = ["claude"]

[[mcp_servers]]
name = "gh"
command = "gh"
[mcp_servers.env]
TOKEN = "shared-secret"
`)
	writeProjectFile(t, configDir, "config.local.toml", `name = "mine"
[[mcp_servers]]
name = "gh"
[mcp_servers.env]
TOKEN = "local-secret"
[mcp_servers.headers]
Authorization = "Bearer abc"
`)

	// Act
	overlay, changes, err := DescribeLocalOverlay(base)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, overlay)
	byPath := map[string]OverlayChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	assert.Equal(t, "shared-name", byPath["name"].Shared)
	assert.Equal(t, "mine", byPath["name"].Local)
	assert.False(t, byPath["name"].Redacted)
	assert.True(t, byPath["mcp_servers.gh.env.TOKEN"].Redacted)
	assert.True(t, byPath["mcp_servers.gh.headers.Authorization"].Redacted)
	assert.False(t, byPath["mcp_servers.gh.headers.Authorization"].HasShared)
}

func TestLocalDoc_DottedNamesAreAddressable(t *testing.T) {
	d := newLocalDoc(t, ``)
	path, err := ParseLocalPath(`mcp_servers["foo.bar"].command`)
	require.NoError(t, err)

	require.NoError(t, d.Set(path, "npx"))
	assert.JSONEq(t, `{"mcp_servers":[{"name":"foo.bar","command":"npx"}]}`, mustJSON(t, d.Doc))

	require.NoError(t, d.Unset(path))
	assert.JSONEq(t, `{}`, mustJSON(t, d.Doc))
}

func TestLocalDoc_ScopeWithoutNameMatchesSharedByPath(t *testing.T) {
	_, configDir := overlayProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n\n[[scopes]]\npath = \"svc/a\"\nprofile = \"p\"\n\n[profiles]\np = []\nq = []\n")
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)

	require.NoError(t, d.Set([]string{"scopes", "svc/a", "profile"}, "q"))
	require.NoError(t, d.Save(t.Context()))

	merged, err := LoadConfig(t.Context(), filepath.Dir(configDir))
	require.NoError(t, err)
	require.Len(t, merged.Scopes, 1)
	assert.Equal(t, "q", merged.Scopes[0].Profile)
}

func TestOpenLocalDoc_MissingMainConfigHintsInitBeforeLocking(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "missing", ".ai-rulez")

	_, err := OpenLocalDoc(configDir, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no main config file")
}
