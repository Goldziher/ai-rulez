package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// runSub runs a fresh command built by build with args inside the current
// project and returns what it printed to stdout and the error it returned.
func runSub(t *testing.T, build func() *cobra.Command, args ...string) (string, error) {
	t.Helper()
	out, _, err := runSubBoth(t, build, args...)
	return out, err
}

// runSubBoth is runSub that also returns stderr.
func runSubBoth(t *testing.T, build func() *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := build()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

// listItems decodes the document a list command prints under --format json.
func listItems(t *testing.T, doc string) []map[string]any {
	t.Helper()
	var parsed struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(doc), &parsed), doc)
	return parsed.Items
}

func configText(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	return strings.ReplaceAll(string(data), "'", `"`) // the writer quotes with a single quote; read it as the usual double quote
}

func TestMarketplaceAdd(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     []string
		wantErr  string
		wantNone bool // the config is left untouched
	}{
		{
			name: "a local directory",
			args: []string{"add", "acme", "--source", "./market"},
			want: []string{"[[installed_marketplaces]]", `name = "acme"`, `source = "./market"`},
		},
		{
			name: "a repository with path, ref and trust",
			args: []string{"add", "acme", "--source", "https://example.com/o/m.git", "--path", "plugins", "--ref", "v1.0.0", "--trust", "warn"},
			want: []string{`source = "https://example.com/o/m.git"`, `path = "plugins"`, `ref = "v1.0.0"`, `trust = "warn"`},
		},
		{
			name: "a version range with its modifiers",
			args: []string{"add", "acme", "--source", "https://example.com/o/m.git", "--version", "^1.2", "--tag-prefix", "v", "--include-prerelease", "--min-release-age", "7d"},
			want: []string{`version = "^1.2"`, `tag_prefix = "v"`, "include_prerelease = true", `min_release_age = "7d"`},
		},
		{
			name: "github shorthand",
			args: []string{"add", "acme", "--source", "owner/market"},
			want: []string{`source = "owner/market"`},
		},
		{
			name: "an https index",
			args: []string{"add", "acme", "--source", "https://example.com/marketplace.json"},
			want: []string{`source = "https://example.com/marketplace.json"`},
		},
		{
			name:     "a missing source",
			args:     []string{"add", "acme"},
			wantErr:  `required flag(s) "source" not set`,
			wantNone: true,
		},
		{
			name:     "no name",
			args:     []string{"add", "--source", "./m"},
			wantErr:  "accepts 1 arg(s), received 0",
			wantNone: true,
		},
		{
			name:     "two names",
			args:     []string{"add", "a", "b", "--source", "./m"},
			wantErr:  "accepts 1 arg(s), received 2",
			wantNone: true,
		},
		{
			name:     "an invalid trust level",
			args:     []string{"add", "acme", "--source", "https://example.com/o/m.git", "--trust", "loud"},
			wantErr:  `invalid trust level "loud"`,
			wantNone: true,
		},
		{
			name:     "credentials in the url",
			args:     []string{"add", "acme", "--source", "https://user:hunter2@example.com/o/m.git"},
			wantErr:  "embeds a credential",
			wantNone: true,
		},
		{
			name:     "plain http",
			args:     []string{"add", "acme", "--source", "http://example.com/o/m.git"},
			wantErr:  "http://",
			wantNone: true,
		},
		{
			name:     "a ref on a local directory",
			args:     []string{"add", "acme", "--source", "./market", "--ref", "main"},
			wantErr:  "need a git source",
			wantNone: true,
		},
		{
			name:     "a path that leaves the repository",
			args:     []string{"add", "acme", "--source", "https://example.com/o/m.git", "--path", "../x"},
			wantErr:  "must not contain '..'",
			wantNone: true,
		},
		{
			name:     "an option-like source",
			args:     []string{"add", "acme", "--source", "--upload-pack=x"},
			wantErr:  "source",
			wantNone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := lockProject(t, "")
			before := configText(t, root)

			// Act
			_, err := runSub(t, newMarketplaceCmd, tt.args...)

			// Assert
			after := configText(t, root)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Equal(t, before, after, "a refused add leaves the config alone")
				assert.NotContains(t, err.Error(), "hunter2")
				return
			}
			require.NoError(t, err)
			for _, w := range tt.want {
				assert.Contains(t, after, w)
			}
		})
	}
}

func TestMarketplaceAdd_DuplicateNameIsRefused(t *testing.T) {
	// Arrange
	root := lockProject(t, "")
	_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./one")
	require.NoError(t, err)
	before := configText(t, root)

	// Act
	_, err = runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./two")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a duplicate")
	assert.Equal(t, before, configText(t, root))
}

func TestMarketplaceAdd_JSONResultRedactsTheSource(t *testing.T) {
	// Arrange
	lockProject(t, "")

	// Act
	out, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "https://example.com/o/m.git", "--format", "json")

	// Assert
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
	assert.Equal(t, "created", doc["status"])
	assert.Equal(t, "installed_marketplace", doc["type"])
	assert.Equal(t, "acme", doc["name"])
	assert.Equal(t, "https://example.com/o/m.git", doc["source"])
}

func TestMarketplaceList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// Arrange
		lockProject(t, "")
		// Act
		_, errOut, err := runSubBoth(t, newMarketplaceCmd, "list")
		// Assert
		require.NoError(t, err)
		assert.Contains(t, errOut, "No installed marketplaces found")
	})
	t.Run("empty as json", func(t *testing.T) {
		// Arrange
		lockProject(t, "")
		// Act
		out, err := runSub(t, newMarketplaceCmd, "list", "--format", "json")
		// Assert
		require.NoError(t, err)
		assert.Empty(t, listItems(t, out))
	})
	t.Run("entries as text and json", func(t *testing.T) {
		// Arrange
		lockProject(t, "")
		_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "https://example.com/o/m.git", "--ref", "v2")
		require.NoError(t, err)
		_, err = runSub(t, newMarketplaceCmd, "add", "local", "--source", "./market")
		require.NoError(t, err)
		// Act
		text, textErr := runSub(t, newMarketplaceCmd, "list")
		js, jsErr := runSub(t, newMarketplaceCmd, "list", "--format", "json")
		// Assert
		require.NoError(t, textErr)
		assert.Contains(t, text, "Installed marketplaces:")
		assert.Contains(t, text, "• acme [git]")
		assert.Contains(t, text, "Source: https://example.com/o/m.git")
		assert.Contains(t, text, "Ref: v2")
		assert.Contains(t, text, "• local [local]")
		require.NoError(t, jsErr)
		rows := listItems(t, js)
		require.Len(t, rows, 2)
		assert.Equal(t, "acme", rows[0]["name"])
		assert.Equal(t, "v2", rows[0]["ref"])
		assert.Equal(t, "git", rows[0]["type"])
		assert.Equal(t, "local", rows[1]["type"])
	})
}

func TestMarketplaceRemove(t *testing.T) {
	t.Run("with --yes", func(t *testing.T) {
		// Arrange
		root := lockProject(t, "")
		_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./market")
		require.NoError(t, err)
		// Act
		out, err := runSub(t, newMarketplaceCmd, "remove", "acme", "--yes", "--format", "json")
		// Assert
		require.NoError(t, err)
		assert.NotContains(t, configText(t, root), "installed_marketplaces")
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
		assert.Equal(t, "removed", doc["status"])
		assert.Equal(t, "installed_marketplace", doc["type"])
	})
	t.Run("without a terminal and without --yes", func(t *testing.T) {
		// Arrange
		root := lockProject(t, "")
		_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./market")
		require.NoError(t, err)
		// Act
		_, err = runSub(t, newMarketplaceCmd, "remove", "acme")
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Operation canceled")
		assert.Contains(t, configText(t, root), `name = "acme"`)
	})
	t.Run("unknown name", func(t *testing.T) {
		// Arrange
		lockProject(t, "")
		// Act
		_, err := runSub(t, newMarketplaceCmd, "remove", "ghost", "--yes")
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), `installed marketplace "ghost" not found`)
	})
	t.Run("a marketplace an installed plugin names", func(t *testing.T) {
		// Arrange
		root := lockProject(t, "")
		_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./market")
		require.NoError(t, err)
		_, err = runSub(t, newPluginCmd, "install", "tools", "--from", "acme")
		require.NoError(t, err)
		// Act
		_, err = runSub(t, newMarketplaceCmd, "remove", "acme", "--yes")
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), `marketplace "acme" is used by installed plugin "tools"`)
		assert.Contains(t, configText(t, root), `name = "acme"`)
	})
}

func TestPluginInstall(t *testing.T) {
	tests := []struct {
		name    string
		market  bool // an installed marketplace called acme exists
		args    []string
		want    []string
		absent  []string
		wantErr string
	}{
		{
			name:   "from a marketplace",
			market: true,
			args:   []string{"install", "tools", "--from", "acme"},
			want:   []string{"[[installed_plugins]]", `name = "tools"`, `marketplace = "acme"`},
			absent: []string{"enable_hooks"},
		},
		{
			name:   "from a marketplace under another entry name",
			market: true,
			args:   []string{"install", "tools", "--from", "acme", "--entry", "tools-pro"},
			want:   []string{`marketplace = "acme"`, `plugin = "tools-pro"`},
		},
		{
			name: "from a repository with every selection flag",
			args: []string{"install", "tools", "--source", "https://example.com/o/p.git", "--path", "pkg", "--ref", "v1.0.0",
				"--layout", "codex", "--include", "skills,agents", "--exclude", "hooks", "--name-prefix", "acme-", "--domain", "backend",
				"--profile", "dev", "--profile", "ci", "--trust", "warn", "--enable-hooks"},
			want: []string{`source = "https://example.com/o/p.git"`, `path = "pkg"`, `ref = "v1.0.0"`, `format = "codex"`,
				`name_prefix = "acme-"`, `domain = "backend"`, `trust = "warn"`, "enable_hooks = true"},
		},
		{
			name: "mcp servers can be left out",
			args: []string{"install", "tools", "--source", "./vendor/tools", "--enable-mcp=false"},
			want: []string{"enable_mcp = false"},
		},
		{
			name: "a version range from a repository",
			args: []string{"install", "tools", "--source", "https://example.com/o/p.git", "--version", "^1", "--tag-prefix", "v", "--include-prerelease", "--min-release-age", "3d"},
			want: []string{`version = "^1"`, `tag_prefix = "v"`, "include_prerelease = true", `min_release_age = "3d"`},
		},
		{name: "neither a marketplace nor a source", args: []string{"install", "tools"}, wantErr: "tools"},
		{
			name: "both a marketplace and a source", market: true,
			args:    []string{"install", "tools", "--from", "acme", "--source", "./x"},
			wantErr: "cannot be combined with marketplace",
		},
		{name: "an unknown marketplace", args: []string{"install", "tools", "--from", "ghost"}, wantErr: "ghost"},
		{
			name:    "an unknown component kind",
			args:    []string{"install", "tools", "--source", "./x", "--include", "widgets"},
			wantErr: `unknown component kind "widgets"`,
		},
		{
			name:    "an unknown layout",
			args:    []string{"install", "tools", "--source", "./x", "--layout", "vim"},
			wantErr: "must be one of",
		},
		{
			name:    "an invalid name prefix",
			args:    []string{"install", "tools", "--source", "./x", "--name-prefix", "-bad"},
			wantErr: "name_prefix",
		},
		{
			name:    "an invalid trust level",
			args:    []string{"install", "tools", "--source", "./x", "--trust", "loud"},
			wantErr: `invalid trust level "loud"`,
		},
		{
			name:    "credentials in the source",
			args:    []string{"install", "tools", "--source", "https://u:hunter2@example.com/o/p.git"},
			wantErr: "embeds a credential",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := lockProject(t, "")
			if tt.market {
				_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./market")
				require.NoError(t, err)
			}
			before := configText(t, root)

			// Act
			_, err := runSub(t, newPluginCmd, tt.args...)

			// Assert
			after := configText(t, root)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "hunter2")
				assert.Equal(t, before, after, "a refused install leaves the config alone")
				return
			}
			require.NoError(t, err)
			for _, w := range tt.want {
				assert.Contains(t, after, w)
			}
			for _, a := range tt.absent {
				assert.NotContains(t, after, a)
			}
		})
	}
}

func TestPluginInstall_IncludeListsAreWrittenAsArrays(t *testing.T) {
	// Arrange
	root := lockProject(t, "")

	// Act
	_, err := runSub(t, newPluginCmd, "install", "tools", "--source", "./x", "--include", "skills,agents", "--profile", "dev")

	// Assert
	require.NoError(t, err)
	text := configText(t, root)
	assert.Contains(t, text, `include = ["skills", "agents"]`)
	assert.Contains(t, text, `profiles = ["dev"]`)
}

func TestPluginList(t *testing.T) {
	// Arrange
	lockProject(t, "")
	_, empty, emptyErr := runSubBoth(t, newPluginCmd, "list")
	_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", "./market")
	require.NoError(t, err)
	_, err = runSub(t, newPluginCmd, "install", "viamarket", "--from", "acme")
	require.NoError(t, err)
	_, err = runSub(t, newPluginCmd, "install", "direct", "--source", "https://example.com/o/p.git", "--include", "skills", "--enable-hooks")
	require.NoError(t, err)

	// Act
	text, textErr := runSub(t, newPluginCmd, "list")
	js, jsErr := runSub(t, newPluginCmd, "list", "--format", "json")

	// Assert
	require.NoError(t, emptyErr)
	assert.Contains(t, empty, "No installed plugins found")
	require.NoError(t, textErr)
	assert.Contains(t, text, "Installed plugins:")
	assert.Contains(t, text, "• viamarket\n    From: marketplace acme")
	assert.Contains(t, text, "• direct\n    From: https://example.com/o/p.git\n    Kinds: skills\n    Hooks: enabled")
	assert.NotContains(t, strings.SplitN(text, "• direct", 2)[0], "Kinds:", "a plugin that imports every kind lists none")
	require.NoError(t, jsErr)
	rows := listItems(t, js)
	require.Len(t, rows, 2)
	assert.Equal(t, "acme", rows[0]["marketplace"])
	assert.Equal(t, false, rows[0]["enable_hooks"])
	assert.Equal(t, true, rows[1]["enable_hooks"])
	assert.Equal(t, []any{"skills"}, rows[1]["include"])
}

func TestPluginRemove(t *testing.T) {
	t.Run("with --yes", func(t *testing.T) {
		// Arrange
		root := lockProject(t, "")
		_, err := runSub(t, newPluginCmd, "install", "tools", "--source", "./x")
		require.NoError(t, err)
		// Act
		out, err := runSub(t, newPluginCmd, "remove", "tools", "--yes", "--format", "json")
		// Assert
		require.NoError(t, err)
		assert.NotContains(t, configText(t, root), "installed_plugins")
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
		assert.Equal(t, "removed", doc["status"])
		assert.Equal(t, "installed_plugin", doc["type"])
	})
	t.Run("without a terminal and without --yes", func(t *testing.T) {
		// Arrange
		root := lockProject(t, "")
		_, err := runSub(t, newPluginCmd, "install", "tools", "--source", "./x")
		require.NoError(t, err)
		// Act
		_, err = runSub(t, newPluginCmd, "remove", "tools")
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Operation canceled")
		assert.Contains(t, configText(t, root), `name = "tools"`)
	})
	t.Run("unknown name", func(t *testing.T) {
		// Arrange
		lockProject(t, "")
		// Act
		_, err := runSub(t, newPluginCmd, "remove", "ghost", "--yes")
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), `installed plugin "ghost" not found`)
	})
}

func TestMarketplaceAndPluginUpdateRepinToTheNewUpstreamCommit(t *testing.T) {
	// Arrange: a marketplace repository listing one plugin repository, both locked
	testutil.IsolateHome(t)
	plug := plugintest.NewRepo(t, plugintest.ClaudePlugin("tools"))
	market := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("acme", `{"name":"tools","source":{"source":"url","url":"`+plug.URL()+`"}}`))
	root := lockProject(t, "")
	_, err := runSub(t, newMarketplaceCmd, "add", "acme", "--source", market.URL())
	require.NoError(t, err)
	_, err = runSub(t, newPluginCmd, "install", "tools", "--from", "acme", "--include", "skills")
	require.NoError(t, err)
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	lockPath := filepath.Join(root, ".ai-rulez", lockfile.FileName)
	pinned := func() (marketplace, plugin string) {
		lock, loadErr := lockfile.Load(filepath.Dir(lockPath))
		require.NoError(t, loadErr)
		require.NotNil(t, lock.Find(lockfile.KindMarketplace, "acme"))
		require.NotNil(t, lock.Find(lockfile.KindPlugin, "tools"))
		return lock.Find(lockfile.KindMarketplace, "acme").Commit, lock.Find(lockfile.KindPlugin, "tools").Commit
	}
	oldMarket, oldPlugin := pinned()
	require.Equal(t, market.Head(), oldMarket)
	require.Equal(t, plug.Head(), oldPlugin)

	// Act: the plugin moves upstream; update the plugin only
	newPlugin := plug.Commit(map[string]string{"skills/review-tools/references/guide.md": "# updated guide\n"}, "upstream change")
	_, pluginErr := runSub(t, newPluginCmd, "update", "tools")
	afterPluginMarket, afterPluginPlugin := pinned()

	// Act: the marketplace moves upstream; update the marketplace
	newMarket := market.Commit(map[string]string{"README.md": "# market changed\n"}, "market change")
	_, marketErr := runSub(t, newMarketplaceCmd, "update", "acme")
	afterMarketMarket, _ := pinned()

	// Assert
	require.NoError(t, pluginErr)
	assert.Equal(t, newPlugin, afterPluginPlugin, "plugin update moves the plugin pin")
	assert.Equal(t, oldMarket, afterPluginMarket, "and leaves the marketplace pin")
	require.NoError(t, marketErr)
	assert.Equal(t, newMarket, afterMarketMarket, "marketplace update moves the marketplace pin")
	// The content pins of the plugin's skills are not recomputed by a per-kind update (docs/lockfile.md): the check says so until a full lock.
	assert.Equal(t, exitDrift, checkLockAt(""), "content pins still describe the old plugin")
	require.Equal(t, 0, writeLockAt("", "", nil), "full lock")
	assert.Equal(t, 0, checkLockAt(""), "a full lock settles it")
}

func TestMarketplaceAddAndPluginInstall_ConfirmWhatWasWritten(t *testing.T) {
	tests := []struct {
		name   string
		build  func() *cobra.Command
		args   []string
		quiet  bool
		want   string
		silent bool
	}{
		{name: "marketplace add", build: newMarketplaceCmd, args: []string{"add", "acme", "--source", "owner/market"}, want: "Added marketplace acme (owner/market) to "},
		{name: "plugin install", build: newPluginCmd, args: []string{"install", "fmt", "--source", "owner/plug"}, want: "Installed plugin fmt (from owner/plug) in "},
		{name: "quiet", build: newPluginCmd, args: []string{"install", "fmt", "--source", "owner/plug"}, quiet: true, silent: true},
		{name: "json", build: newPluginCmd, args: []string{"install", "fmt", "--source", "owner/plug", "--format", "json"}, silent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			lockProject(t, "")
			viper.Set("quiet", tt.quiet)
			t.Cleanup(func() { viper.Set("quiet", nil) }) // nil clears the override, so a later test reads the environment again

			// Act
			_, stderr, err := runSubBoth(t, tt.build, tt.args...)

			// Assert
			require.NoError(t, err)
			if tt.silent {
				assert.Empty(t, stderr)
				return
			}
			assert.Contains(t, stderr, tt.want)
			assert.Contains(t, stderr, "config.toml")
		})
	}
}
