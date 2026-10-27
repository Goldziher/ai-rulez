package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

func TestGenerate_LocalManifestIgnoredOnFirstRun(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["vibe", "claude"]`)

	// Act
	generateRepo(t, root)
	first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	generateRepo(t, root)
	second, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)

	// Assert
	assert.Contains(t, string(first), ".ai-rulez/.generated-manifest.local.json")
	assert.Equal(t, string(first), string(second), "run 1 and run 2 write the same .gitignore")
}

func TestGenerate_LocalManifestIgnoredWithoutManagedIgnores(t *testing.T) {
	tests := []struct {
		name    string
		presets string
	}{
		{name: "digest-only manifest", presets: `["claude"]`},
		{name: "several presets", presets: `["claude", "cursor", "codex"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newSecretMCPRepo(t, tt.presets)
			cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
			data, err := os.ReadFile(cfgPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(string(data), "gitignore = true\n", "", 1)), 0o644))

			// Act
			generateRepo(t, root)
			first, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
			require.NoError(t, err)
			generateRepo(t, root)
			second, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
			require.NoError(t, err)

			// Assert: with gitignore off the manifest is excluded per clone, and no
			// tracked .gitignore is written for it.
			assert.Contains(t, string(first), "/.ai-rulez/.generated-manifest.local.json")
			assert.NotContains(t, string(first), ".ai-rulez/local/", "the manifest does not imply the local content tree")
			assert.NoFileExists(t, filepath.Join(root, ".gitignore"))
			assert.Equal(t, string(first), string(second), "run 1 and run 2 write the same exclude block")
		})
	}
}

func withoutManagedIgnores(t *testing.T, root string) {
	t.Helper()
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(string(data), "gitignore = true\n", "", 1)), 0o644))
}

// A manifest that is already on disk in a repository is ignored by a run that
// writes nothing new: with gitignore off the exclude block it lacked is rebuilt
// with the entry, and no .gitignore is written.
func TestGenerate_LocalManifestOnDiskInARepoGetsIgnored(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["claude"]`)
	withoutManagedIgnores(t, root)
	generateRepo(t, root)
	excludePath := filepath.Join(root, ".git", "info", "exclude")
	require.NoError(t, os.WriteFile(excludePath, nil, 0o644))

	// Act
	g := generateRepo(t, root)

	// Assert
	assert.True(t, g.manifestExcluded())
	got, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "/.ai-rulez/.generated-manifest.local.json")
	assert.NoFileExists(t, filepath.Join(root, ".gitignore"))
}

// Outside a git repository there is nothing to stage, so a manifest that only
// sits on disk creates no .gitignore.
func TestLocalManifestIgnorable_OutsideARepo(t *testing.T) {
	tests := []struct {
		name    string
		onDisk  bool
		pending bool
		inRepo  bool
		want    bool
	}{
		{name: "on disk in a repository, held by .git/info/exclude", onDisk: true, inRepo: true, want: false},
		{name: "on disk outside a repository", onDisk: true, want: false},
		{name: "absent in a repository", inRepo: true, want: false},
		{name: "written this run, anywhere", pending: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newSecretMCPRepo(t, `["claude"]`)
			if !tt.inRepo {
				require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
			}
			withoutManagedIgnores(t, root)
			cfg, err := config.LoadConfig(t.Context(), root)
			require.NoError(t, err)
			g := NewGenerator(cfg)
			if tt.onDisk {
				require.NoError(t, os.WriteFile(g.localManifestPath(), []byte("{}\n"), 0o600))
			}
			g.localManifestPending = tt.pending

			// Act
			got := g.localManifestIgnorable()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

// The early pass saw no manifest and the run created it: the closing pass adds
// the entry even though the user's own .gitignore already held a block.
func TestGenerate_LocalManifestCreatedByTheFirstRunIsIgnoredAfterTheEarlyPass(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["claude"]`)
	withoutManagedIgnores(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0o644))
	require.NoFileExists(t, filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json"))

	// Act
	generateRepo(t, root)

	// Assert
	got, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(got), "the user's file is not touched with gitignore off")
	exclude, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(exclude), "/.ai-rulez/.generated-manifest.local.json")
	assert.FileExists(t, filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json"))
}

func TestGenerate_NoLocalIgnoreBlockIgnoresOwnershipOnlyMetadata(t *testing.T) {
	root := t.TempDir()
	writeDomainsFile(t, filepath.Join(root, ".ai-rulez/config.toml"), `version="5.0"
name="ownership"
presets=["codex"]
gitignore=true
[defaults.effort_by_preset]
codex="xhigh"
`)
	writeDomainsFile(t, filepath.Join(root, ".codex/config.toml"), "model_reasoning_effort = \"xhigh\"\n")
	gitCmd(t, root, "init", "-q")
	generate := func() *Generator {
		cfg, err := config.LoadConfig(t.Context(), root, config.WithoutLocal())
		require.NoError(t, err)
		g := NewGenerator(cfg)
		require.NoError(t, g.Generate(""))
		return g
	}
	generate()
	baseline, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	localPath := filepath.Join(root, ".ai-rulez", generatedLocalManifestName)
	require.NoError(t, writeManifestFileDirs(localPath, nil, map[string][]jsonmerge.Claim{
		".codex/config.toml": {{Path: []string{"model_reasoning_effort"}, Sum: jsonmerge.Digest("xhigh")}},
	}, nil, nil, nil))
	g := generate()
	require.True(t, g.localManifestPending, "preserved claims really exercise the pending manifest path")
	got, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, string(baseline), string(got), "ownership metadata must not imply local source inputs")
	require.Contains(t, string(got), ".ai-rulez/.generated-manifest.local.json")
	require.FileExists(t, localPath)
	require.Len(t, g.readManifest(localPath).Merged[".codex/config.toml"], 1, "machine ownership survives")
	require.Equal(t, "model_reasoning_effort = \"xhigh\"\n", readFileString(t, filepath.Join(root, ".codex/config.toml")))
}
