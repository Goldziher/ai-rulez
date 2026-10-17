package lockfile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSave_MarketplaceAndPluginEntriesRoundTrip(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	f := &File{Version: Version}
	f.Set(KindPlugin, Entry{Name: "zeta", Source: "marketplace:acme", Commit: "c2", Digest: "sha256:2", Origin: "https://example.com/x.git#sub@c2"})
	f.Set(KindPlugin, Entry{Name: "alpha", Source: "marketplace:acme", Commit: "c1", Digest: "sha256:1"})
	f.Set(KindMarketplace, Entry{Name: "acme", Source: "acme/plugins", Ref: "^1.2", Tag: "v1.2.3", Commit: "c0", Digest: "sha256:0"})

	// Act
	require.NoError(t, Save(dir, f))
	loaded, err := Load(dir)
	require.NoError(t, err)
	first, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	require.NoError(t, Save(dir, loaded))
	second, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	// Assert
	assert.Equal(t, string(first), string(second), "saving again is byte-identical")
	require.Len(t, loaded.Plugin, 2)
	assert.Equal(t, "alpha", loaded.Plugin[0].Name, "entries are sorted by name")
	assert.Equal(t, "https://example.com/x.git#sub@c2", loaded.Find(KindPlugin, "zeta").Origin)
	assert.Equal(t, "v1.2.3", loaded.Find(KindMarketplace, "acme").Tag)
	assert.Nil(t, loaded.Find(KindMarketplace, "alpha"), "kinds do not share names")
	assert.Contains(t, string(first), "[[marketplace]]")
	assert.Contains(t, string(first), "[[plugin]]")
}

func TestSet_ReplacesWithinItsKind(t *testing.T) {
	// Arrange
	f := &File{}
	f.Set(KindPlugin, Entry{Name: "p", Commit: "old"})
	f.Set(KindMarketplace, Entry{Name: "p", Commit: "mp"})

	// Act
	f.Set(KindPlugin, Entry{Name: "p", Commit: "new"})

	// Assert
	assert.Len(t, f.Plugin, 1)
	assert.Equal(t, "new", f.Find(KindPlugin, "p").Commit)
	assert.Equal(t, "mp", f.Find(KindMarketplace, "p").Commit)
}

func TestCovers_MarketplaceKeyedPlugin(t *testing.T) {
	e := &Entry{Name: "p", Source: "marketplace:acme", Commit: "c", Digest: "d"}
	tests := []struct {
		name string
		want Want
		ok   bool
	}{
		{"same marketplace", Want{Kind: KindPlugin, Name: "p", Source: "marketplace:acme"}, true},
		{"another marketplace", Want{Kind: KindPlugin, Name: "p", Source: "marketplace:other"}, false},
		{"moved to a repository", Want{Kind: KindPlugin, Name: "p", Source: "acme/tools"}, false},
		{"a ref appeared", Want{Kind: KindPlugin, Name: "p", Source: "marketplace:acme", Ref: "main"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.ok, e.Covers(tt.want)) })
	}
}
