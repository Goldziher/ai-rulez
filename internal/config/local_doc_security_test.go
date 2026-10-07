package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const securityShared = "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n"

func TestWriteFileAtomic_Permissions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, target string)
	}{
		{"fresh file", func(*testing.T, string) {}},
		{"stale world-readable temp file beside the target", func(t *testing.T, target string) {
			require.NoError(t, os.WriteFile(target+".tmp", []byte("stale"), 0o644))
		}},
		{"existing world-readable target", func(t *testing.T, target string) {
			require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			target := filepath.Join(dir, "config.local.toml")
			tt.setup(t, target)

			// Act
			err := writeFileAtomic(target, []byte("secret = 1\n"), 0o600)

			// Assert
			require.NoError(t, err)
			info, err := os.Stat(target)
			require.NoError(t, err)
			if runtime.GOOS != "windows" { // Windows has no Unix permission bits
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
			leftovers, err := filepath.Glob(filepath.Join(dir, ".config.local.toml-*.tmp"))
			require.NoError(t, err)
			assert.Empty(t, leftovers, "the temp file must not outlive the write")
		})
	}
}

func TestWriteFileAtomic_RemovesTempFileOnFailure(t *testing.T) {
	// Arrange: a directory at the target makes the final rename fail.
	dir := t.TempDir()
	target := filepath.Join(dir, "config.local.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "child"), 0o755))

	// Act
	err := writeFileAtomic(target, []byte("x"), 0o600)

	// Assert
	require.Error(t, err)
	leftovers, globErr := filepath.Glob(filepath.Join(dir, ".config.local.toml-*.tmp"))
	require.NoError(t, globErr)
	assert.Empty(t, leftovers)
}

func TestLocalDoc_SaveRefusesSymlinkedOverlay(t *testing.T) {
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	real := filepath.Join(t.TempDir(), "elsewhere.toml")
	require.NoError(t, os.WriteFile(real, []byte("name = \"kept\"\n"), 0o600))
	testutil.SymlinkOrSkip(t, real, filepath.Join(configDir, "config.local.toml"))
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"description"}, "mine"))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symbolic link")
	info, lerr := os.Lstat(filepath.Join(configDir, "config.local.toml"))
	require.NoError(t, lerr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link must not be replaced")
	data, rerr := os.ReadFile(real)
	require.NoError(t, rerr)
	assert.Equal(t, "name = \"kept\"\n", string(data))
}

func TestLocalDoc_SaveRewritesWorldReadableOverlayAsOwnerOnly(t *testing.T) {
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	path := filepath.Join(configDir, "config.local.toml")
	require.NoError(t, os.WriteFile(path, []byte("name = \"before\"\n"), 0o644))
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"description"}, "mine"))

	// Act
	require.NoError(t, d.Save(t.Context()))

	// Assert
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" { // Windows has no Unix permission bits
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestLocalDoc_SaveWritesNothingWhenGitignoreCannotBeUpdated(t *testing.T) {
	// Arrange: a directory named .gitignore makes the ignore update fail.
	base, configDir := overlayProject(t, securityShared)
	require.NoError(t, os.Mkdir(filepath.Join(base, ".gitignore"), 0o755))
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"description"}, "mine"))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(configDir, "config.local.toml"), "no overlay may exist unignored")
}

func TestLocalDoc_LockSerializesEditors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the overlay lock is a no-op on Windows")
	}
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	first, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	acquired := make(chan *LocalDoc, 1)

	// Act: a second editor must wait for the first to close.
	go func() {
		second, openErr := OpenLocalDoc(configDir, "config.toml")
		if openErr == nil {
			acquired <- second
		}
	}()

	// Assert
	select {
	case <-acquired:
		t.Fatal("second editor acquired the lock while the first still held it")
	case <-time.After(200 * time.Millisecond):
	}
	first.Close()
	select {
	case second := <-acquired:
		second.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("second editor never acquired the lock after the first closed")
	}
}

func TestLocalDoc_RestoreFailureIsReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced on Windows")
	}
	// Arrange: the overlay's directory is not writable, so restoring fails.
	dir := t.TempDir()
	d := &LocalDoc{Path: filepath.Join(dir, "config.local.toml")}
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:errcheck,gosec // test cleanup

	// Act
	err := d.restore([]byte("name = \"before\"\n"), true)

	// Assert
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	require.Error(t, err)
}

func TestLocalDoc_SaveRejectsIncompleteNewServer(t *testing.T) {
	tests := []struct {
		name    string
		set     []string
		value   any
		wantErr string
	}{
		{"new server with only env", []string{"mcp_servers", "fresh", "env", "K"}, "v", "mcp_servers.fresh is new and incomplete"},
		{"new server with a command is accepted", []string{"mcp_servers", "fresh", "command"}, "npx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			_, configDir := overlayProject(t, securityShared)
			d, err := OpenLocalDoc(configDir, "config.toml")
			require.NoError(t, err)
			t.Cleanup(d.Close)
			require.NoError(t, d.Set(tt.set, tt.value))

			// Act
			err = d.Save(t.Context())

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.wantErr), err.Error())
			assert.NoFileExists(t, filepath.Join(configDir, "config.local.toml"))
		})
	}
}

func TestLocalDoc_SaveRejectsSchemaInvalidOverlay(t *testing.T) {
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"header", "style"}, "weird"))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(configDir, "config.local.toml"))
}

func TestLocalDoc_UpsertReplacesRemoveMarker(t *testing.T) {
	// Arrange
	d := newLocalDoc(t, "[[includes]]\nname = \"a\"\nremove = true\n")

	// Act
	require.NoError(t, d.UpsertNamed("includes", "a", map[string]any{"source": "s"}))

	// Assert
	assert.True(t, jsonEqual(t, mustJSON(t, d.Doc), `{"includes":[{"name":"a","source":"s"}]}`), mustJSON(t, d.Doc))
}

func TestShowAllowed(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"name", true}, {"description", true}, {"default", true}, {"presets", true}, {"gitignore", true},
		{"compact", true}, {"builtins", true}, {"profiles.dev", true}, {"defaults.effort_by_preset.claude", true},
		{"rules.mode", true}, {"header.style", true}, {"mcp_servers.gh.transport", true},
		{"mcp_servers.gh.enabled", true}, {"includes.inc.remove", true}, {"mcp.self_server", true},
		{"mcp.self_server_version", true},
		{"mcp_servers.gh.url", false}, {"mcp_servers.gh.args", false}, {"mcp_servers.gh.env.TOKEN", false},
		{"mcp_servers.gh.auth", false}, {"mcp_servers.gh.dsn", false}, {"includes.inc.source", false},
		{"mcp.self_server_command", false}, {"schema", false}, {"foo.[0].token", false},
		{"plugins.p.source", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, showAllowed(strings.Split(tt.path, ".")))
		})
	}
}

func TestDescribeLocalOverlay_DefaultDeny(t *testing.T) {
	// Arrange
	base, configDir := overlayProject(t, securityShared)
	writeProjectFile(t, configDir, "config.local.toml", `name = "mine"
[[mcp_servers]]
name = "gh"
auth = "a"
args = ["--token", "t"]
[[foo]]
token = "u"
`)

	// Act
	_, changes, err := DescribeLocalOverlay(base)

	// Assert
	require.NoError(t, err)
	got := map[string]bool{}
	for _, c := range changes {
		got[c.Path] = c.Redacted
	}
	assert.Equal(t, map[string]bool{
		"name": false, "mcp_servers.gh.name": true, "mcp_servers.gh.auth": true,
		"mcp_servers.gh.args": true, "foo.[0].token": true,
	}, got)
}

func TestLoadConfig_OverlayTypeErrorsNeverEchoValues(t *testing.T) {
	tests := []struct {
		name    string
		mainFn  string
		main    string
		localFn string
		local   string
	}{
		{
			name: "yaml overlay on yaml main", mainFn: "config.yaml",
			main:    "version: \"5.0\"\nname: x\npresets: [claude]\n",
			localFn: "config.local.yaml",
			local:   "mcp_servers:\n  - name: x\n    command: c\n    args: \"SECRETY2\"\n",
		},
		{
			name: "toml overlay on toml main", mainFn: "config.toml",
			main:    securityShared,
			localFn: "config.local.toml",
			local:   "[[mcp_servers]]\nname = \"x\"\ncommand = \"c\"\nargs = \"SECRETY2\"\n",
		},
		{
			name: "json overlay on json main", mainFn: "config.json",
			main:    "{\"version\":\"4.0\",\"name\":\"x\",\"presets\":[\"claude\"]}",
			localFn: "config.local.json",
			local:   "{\"mcp_servers\":[{\"name\":\"x\",\"command\":\"c\",\"args\":\"SECRETY2\"}]}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, configDir := overlayProject(t, tt.main)
			require.NoError(t, os.Remove(filepath.Join(configDir, "config.toml")))
			writeProjectFile(t, configDir, tt.mainFn, tt.main)
			writeProjectFile(t, configDir, tt.localFn, tt.local)

			// Act
			_, err := LoadConfig(t.Context(), base)

			// Assert
			require.Error(t, err)
			rendered := fmt.Sprintf("%+v", err)
			if o, ok := oops.AsOops(err); ok {
				rendered += fmt.Sprint(o.Context())
			}
			assert.NotContains(t, rendered, "SECRET")
		})
	}
}

func TestShowTypeMismatchIsRedacted(t *testing.T) {
	tests := []struct {
		name     string
		local    string
		redacted bool
		path     string
	}{
		{"bool key holding a string", "gitignore = \"SECRETTYPE1\"\n", true, "gitignore"},
		{"bool key holding a bool", "gitignore = true\n", false, "gitignore"},
		{"string key holding a list", "default = [\"SECRETY6\"]\n", true, "default"},
		{"string key holding a string", "name = \"mine\"\n", false, "name"},
		{"list key holding a string", "presets = \"SECRETTYPE2\"\n", true, "presets"},
		{"list key holding a list", "presets = [\"codex\"]\n", false, "presets"},
		{"list key holding a mixed list", "presets = [\"codex\", 5]\n", true, "presets"},
		{"profile holding a string", "[profiles]\ndev = \"SECRETTYPE3\"\n", true, "profiles.dev"},
		{"header bool holding a string", "[header]\ntimestamp = \"SECRETTYPE4\"\n", true, "header.timestamp"},
		{"header style string", "[header]\nstyle = \"compact\"\n", false, "header.style"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, configDir := overlayProject(t, securityShared)
			writeProjectFile(t, configDir, "config.local.toml", tt.local)

			// Act
			_, changes, err := DescribeLocalOverlay(base)

			// Assert
			require.NoError(t, err)
			found := false
			for _, c := range changes {
				if c.Path == tt.path {
					found = true
					assert.Equal(t, tt.redacted, c.Redacted)
				}
			}
			assert.True(t, found, "path %s not listed", tt.path)
		})
	}
}

func TestValidate_OverlayDefaultNeverEchoed(t *testing.T) {
	tests := []struct {
		name  string
		local string
	}{
		{"no profiles defined", "default = \"SECRETY6\"\n"},
		{"profile missing", "default = \"SECRETY6\"\n[profiles]\ndev = [\"x\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, configDir := overlayProject(t, securityShared)
			writeProjectFile(t, configDir, "config.local.toml", tt.local)
			cfg, err := LoadConfig(t.Context(), base)
			require.NoError(t, err)

			// Act
			err = cfg.Validate()

			// Assert
			require.Error(t, err)
			rendered := fmt.Sprintf("%+v", err)
			if o, ok := oops.AsOops(err); ok {
				rendered += fmt.Sprint(o.Context())
			}
			assert.NotContains(t, rendered, "SECRET")
		})
	}
}
