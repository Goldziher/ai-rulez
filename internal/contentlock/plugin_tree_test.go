package contentlock

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestTreeOf_CoversMarketplacesAndPlugins(t *testing.T) {
	base := func() *lockfile.File {
		f := &lockfile.File{}
		f.Set(lockfile.KindMarketplace, lockfile.Entry{Name: "m", Source: "a/b", Commit: "c1", Digest: "sha256:1"})
		f.Set(lockfile.KindPlugin, lockfile.Entry{Name: "p", Source: "marketplace:m", Commit: "c2", Digest: "sha256:2", Origin: "https://e.com/x.git"})
		return f
	}
	tests := []struct {
		name   string
		change func(f *lockfile.File)
	}{
		{"marketplace commit", func(f *lockfile.File) { f.Marketplace[0].Commit = "c9" }},
		{"marketplace digest", func(f *lockfile.File) { f.Marketplace[0].Digest = "sha256:9" }},
		{"plugin commit", func(f *lockfile.File) { f.Plugin[0].Commit = "c9" }},
		{"plugin digest", func(f *lockfile.File) { f.Plugin[0].Digest = "sha256:9" }},
		{"plugin origin", func(f *lockfile.File) { f.Plugin[0].Origin = "https://e.com/y.git" }},
		{"plugin removed", func(f *lockfile.File) { f.Plugin = nil }},
		{"marketplace removed", func(f *lockfile.File) { f.Marketplace = nil }},
	}
	want := TreeOf(base())
	assert.Equal(t, want, TreeOf(base()), "the tree digest is deterministic")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := base()
			tt.change(f)
			assert.NotEqual(t, want, TreeOf(f))
		})
	}
}

func TestTreeOf_PluginsAndSkillsAreDistinctKinds(t *testing.T) {
	// Arrange: the same entry as a skill and as a plugin must not hash alike
	entry := lockfile.Entry{Name: "x", Source: "a/b", Commit: "c", Digest: "sha256:1"}
	asSkill, asPlugin := &lockfile.File{}, &lockfile.File{}
	asSkill.Set(lockfile.KindSkill, entry)
	asPlugin.Set(lockfile.KindPlugin, entry)

	// Act / Assert
	assert.NotEqual(t, TreeOf(asSkill), TreeOf(asPlugin))
}
