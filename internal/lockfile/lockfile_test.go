package lockfile

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLoad_RoundTripIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	f := &File{}
	f.Set(KindInclude, Entry{Name: "zeta", Source: "https://example.com/z", Commit: "c1", Digest: "sha256:1"})
	f.Set(KindInclude, Entry{Name: "alpha", Source: "https://example.com/a", Ref: "v1", Commit: "c2", Digest: "sha256:2"})
	f.Set(KindSkill, Entry{Name: "s", Source: "https://example.com/s", Path: "skills/s", Commit: "c3", Digest: "sha256:3"})
	require.NoError(t, Save(dir, f))
	first, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	got, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, Save(dir, got))
	second, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
	assert.Equal(t, "alpha", got.Include[0].Name, "entries are sorted by name")
	assert.Equal(t, "c3", got.Find(KindSkill, "s").Commit)
	assert.Nil(t, got.Find(KindSkill, "missing"))
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	require.NoError(t, err)
	assert.Nil(t, got, "a missing lock is not an error")

	require.NoError(t, os.WriteFile(Path(dir), []byte("version = 99\n"), 0o644))
	_, err = Load(dir)
	assert.ErrorContains(t, err, "unsupported lock version 99")

	require.NoError(t, os.WriteFile(Path(dir), []byte("not toml ["), 0o644))
	_, err = Load(dir)
	assert.Error(t, err)
}

func TestEntryCovers(t *testing.T) {
	e := &Entry{Source: "https://example.com/r", Path: "p", Ref: "main"}
	tests := []struct {
		name string
		e    *Entry
		w    Want
		want bool
	}{
		{"same", e, Want{Source: "https://example.com/r", Path: "p", Ref: "main"}, true},
		{"ref changed", e, Want{Source: "https://example.com/r", Path: "p", Ref: "dev"}, false},
		{"path changed", e, Want{Source: "https://example.com/r", Path: "q", Ref: "main"}, false},
		{"source changed", e, Want{Source: "https://example.com/x", Path: "p", Ref: "main"}, false},
		{"no entry", nil, Want{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.e.Covers(tt.w))
		})
	}
}

func TestIsFullSHA(t *testing.T) {
	assert.True(t, IsFullSHA("0123456789abcdef0123456789abcdef01234567"))
	assert.False(t, IsFullSHA("main"))
	assert.False(t, IsFullSHA("0123456789ABCDEF0123456789abcdef01234567"))
	assert.False(t, IsFullSHA("0123456"))
}

func TestLoadRefusesOtherVersionsWithAnActionableError(t *testing.T) {
	dir := t.TempDir()
	for _, body := range []string{"version = 0\n", "version = 2\n", "version = 99\n", "[[include]]\nname = \"a\"\n"} {
		require.NoError(t, os.WriteFile(Path(dir), []byte(body), 0o644))
		_, err := Load(dir)
		require.Error(t, err, body)
		assert.ErrorContains(t, err, "unsupported lock version")
		assert.ErrorContains(t, err, "run `ai-rulez lock` to regenerate")
	}

	require.NoError(t, os.Remove(Path(dir)))
	got, err := Load(dir)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSaveSortsContentPinsAndIsStable(t *testing.T) {
	dir := t.TempDir()
	f := &File{
		AIRulezVersion: "1.2.3", Tree: "sha256:t",
		Item: []Item{
			{Kind: "skill", ID: "b", Domain: "x", Digest: "sha256:2"},
			{Kind: "rule", ID: "z", Digest: "sha256:1"},
			{Kind: "skill", ID: "a", Domain: "x", Digest: "sha256:3", Owner: "team", Version: "1.0.0"},
		},
		Output: []OutputPin{{Path: "b.md", Digest: "sha256:5"}, {Path: "a.md", Digest: "sha256:4"}},
	}
	require.NoError(t, Save(dir, f))
	first, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	got, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, Save(dir, got))
	second, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
	assert.Equal(t, []string{"z", "a", "b"}, []string{got.Item[0].ID, got.Item[1].ID, got.Item[2].ID})
	assert.Equal(t, "a.md", got.Output[0].Path)
	assert.Equal(t, "team", got.Item[1].Owner)
	assert.NotContains(t, string(first), "Z\n", "no timestamps")
}

func TestServedEntriesAreKeyedByNameAndView(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	f := &File{Version: Version}
	f.Set(KindServed, Entry{Name: "pdf", Digest: "sha256:default"})
	f.Set(KindServed, Entry{Name: "pdf", View: "role:backend", Digest: "sha256:backend"})
	f.Set(KindServed, Entry{Name: "pdf", View: "role:backend", Digest: "sha256:backend2"})

	// Act
	require.NoError(t, Save(dir, f))
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Assert
	require.Len(t, loaded.Served, 2, "setting the same name and view again replaces it")
	assert.Equal(t, "sha256:default", loaded.Find(KindServed, "pdf").Digest, "Find reads the default view")
	assert.Equal(t, "sha256:backend2", loaded.FindView(KindServed, "pdf", "role:backend").Digest)
	assert.Nil(t, loaded.FindView(KindServed, "pdf", "role:frontend"))
	assert.Equal(t, "", loaded.Served[0].View, "entries are written by name, then view")
}

func TestSavedLockWithoutViewsHasNoViewKey(t *testing.T) {
	dir := t.TempDir()
	f := &File{Version: Version}
	f.Set(KindServed, Entry{Name: "pdf", Digest: "sha256:x"})
	require.NoError(t, Save(dir, f))
	data, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "view")
}

func TestEntryCoversVersionConstraints(t *testing.T) {
	pinned := &Entry{Source: "https://example.com/r", Ref: "^1.2", Tag: "v1.2.4", Commit: "c1", Digest: "sha256:1"}
	want := func(mut func(*Want)) Want {
		w := Want{Source: "https://example.com/r", Ref: "^1.2", Constraint: "^1.2"}
		if mut != nil {
			mut(&w)
		}
		return w
	}
	tests := []struct {
		name string
		e    *Entry
		w    Want
		want bool
	}{
		{"the pinned tag satisfies the constraint", pinned, want(nil), true},
		{"a narrower constraint the tag no longer satisfies", pinned, want(func(w *Want) { w.Ref, w.Constraint = "^1.3", "^1.3" }), false},
		{"a constraint edit that keeps the tag valid still changes the recorded ref", pinned, want(func(w *Want) { w.Ref, w.Constraint = "~1.2.0", "~1.2.0" }), false},
		{"a tag prefix the tag does not carry", pinned, want(func(w *Want) { w.TagPrefix = "deploy/v" }), false},
		{"a pin with no tag is not a version pin", &Entry{Source: "https://example.com/r", Ref: "^1.2", Commit: "c1"}, want(nil), false},
		{"a pin whose tag is not a version", &Entry{Source: "https://example.com/r", Ref: "^1.2", Tag: "latest"}, want(nil), false},
		{"a prerelease tag needs include_prerelease", &Entry{Source: "https://example.com/r", Ref: "^1.2", Tag: "v1.3.0-rc.1"}, want(nil), false},
		{"and is covered with it", &Entry{Source: "https://example.com/r", Ref: "^1.2", Tag: "v1.3.0-rc.1"}, want(func(w *Want) { w.IncludePrerelease = true }), true},
		{"a prefixed tag", &Entry{Source: "https://example.com/r", Ref: "~2.1.0", Tag: "deploy/v2.1.3"}, want(func(w *Want) { w.Ref, w.Constraint, w.TagPrefix = "~2.1.0", "~2.1.0", "deploy/v" }), true},
		{"a plain want is unaffected by the tag", &Entry{Source: "https://example.com/r", Ref: "main", Tag: "v1"}, Want{Source: "https://example.com/r", Ref: "main"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.e.Covers(tt.w))
		})
	}
}

func TestTagFieldsRoundTripAndAreOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	f := &File{}
	f.Set(KindInclude, Entry{Name: "shared", Source: "https://example.com/r", Ref: "^1.2", Tag: "v1.2.4", TagObject: "7a9c", Commit: "0f3e", Digest: "sha256:1"})
	f.Set(KindInclude, Entry{Name: "plain", Source: "https://example.com/p", Ref: "main", Commit: "c2", Digest: "sha256:2"})

	require.NoError(t, Save(dir, f))
	data, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	got, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "v1.2.4", got.Find(KindInclude, "shared").Tag)
	assert.Equal(t, "7a9c", got.Find(KindInclude, "shared").TagObject)
	assert.Equal(t, 1, strings.Count(string(data), "tag = "), "an entry without a tag writes no tag key")
	assert.Equal(t, 1, strings.Count(string(data), "tag_object = "))
}
