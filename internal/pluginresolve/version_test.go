package pluginresolve_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
)

// advance is the policy of `ai-rulez update`: the named kind moves to the newest allowed tag.
func advance(kind string) config.LockPolicy {
	var p config.LockPolicy
	p.Advance = func(k, _ string) bool { return k == kind }
	return p
}

func TestVersionRanges_MarketplaceAndPlugin(t *testing.T) {
	tests := []struct {
		name string
		kind string
		// config returns the TOML and the repository whose tags move.
		setup func(t *testing.T) (toml string, repo *plugintest.Repo)
	}{
		{"marketplace", lockfile.KindMarketplace, func(t *testing.T) (string, *plugintest.Repo) {
			repo := plugintest.NewRepo(t, plugintest.Merge(
				marketFiles(pluginimport.IndexClaude, `{"name":"tools","source":"./plugins/tools"}`),
				plugintest.Prefix("plugins/tools", plugintest.ClaudePlugin("tools"))))
			return "[[installed_marketplaces]]\nname = \"acme\"\nsource = \"" + repo.URL() + "\"\nversion = \"^1.0\"\n\n" +
				"[[installed_plugins]]\nname = \"tools\"\nmarketplace = \"acme\"\n", repo
		}},
		{"plugin", lockfile.KindPlugin, func(t *testing.T) (string, *plugintest.Repo) {
			repo := plugintest.NewRepo(t, plugintest.ClaudePlugin("tools"))
			return "[[installed_plugins]]\nname = \"tools\"\nsource = \"" + repo.URL() + "\"\nversion = \"^1.0\"\n", repo
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			isolate(t)
			toml, repo := tt.setup(t)
			repo.Tag("v1.0.0")
			project := plugintest.Project(t, toml)
			lock := mustLock(t, project)
			entry := lock.Find(tt.kind, map[string]string{"marketplace": "acme", "plugin": "tools"}[tt.kind])
			require.NotNil(t, entry)
			assert.Equal(t, "v1.0.0", entry.Tag)
			assert.Equal(t, "^1.0", entry.Ref)
			assert.Equal(t, repo.Head(), entry.Commit)

			// A newer allowed tag and a major release appear upstream.
			repo.Commit(map[string]string{"NOTES.md": "1.1\n"}, "release 1.1")
			repo.Tag("v1.1.0")
			second := repo.Commit(map[string]string{"NOTES.md": "2.0\n"}, "release 2.0")
			repo.Tag("v2.0.0")

			// Act / Assert: a refresh keeps a pin that still satisfies the range
			res, err := writeLock(t, project, lockrun.Request{}, config.LockPolicy{})
			require.NoError(t, err)
			assert.Equal(t, "v1.0.0", res.Lock.Find(tt.kind, entry.Name).Tag)

			// Act: update moves it to the newest allowed tag, not the major
			res, err = writeLock(t, project, lockrun.Request{Kind: tt.kind}, advance(tt.kind))
			require.NoError(t, err)

			// Assert
			moved := res.Lock.Find(tt.kind, entry.Name)
			assert.Equal(t, "v1.1.0", moved.Tag)
			assert.NotEqual(t, second, moved.Commit)
			assert.NotEqual(t, entry.Commit, moved.Commit)
		})
	}
}

func TestVersionRanges_AMovedTagIsRefused(t *testing.T) {
	// Arrange
	isolate(t)
	repo := plugintest.NewRepo(t, plugintest.ClaudePlugin("tools"))
	repo.Tag("v1.0.0")
	project := plugintest.Project(t, "[[installed_plugins]]\nname = \"tools\"\nsource = \""+repo.URL()+"\"\nversion = \"^1.0\"\n")
	mustLock(t, project)
	repo.Commit(map[string]string{"commands/evil.md": "evil\n"}, "retag")
	repo.Tag("v1.0.0") // the release is moved to another commit

	// Act
	_, err := writeLock(t, project, lockrun.Request{}, config.LockPolicy{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR732")
}

func TestVersionRanges_NoMatchingTagIsReported(t *testing.T) {
	// Arrange
	isolate(t)
	repo := plugintest.NewRepo(t, plugintest.ClaudePlugin("tools"))
	repo.Tag("v0.9.0")
	project := plugintest.Project(t, "[[installed_plugins]]\nname = \"tools\"\nsource = \""+repo.URL()+"\"\nversion = \"^1.0\"\n")

	// Act
	_, err := writeLock(t, project, lockrun.Request{}, config.LockPolicy{})

	// Assert
	require.ErrorContains(t, err, `AR730 no tag satisfies "^1.0"`)
}
