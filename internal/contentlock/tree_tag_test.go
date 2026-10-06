package contentlock

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
)

func TestTreeOfCoversTheResolvedTag(t *testing.T) {
	base := lockfile.Entry{Name: "shared", Source: "https://example.com/r", Ref: "^1.2", Commit: "c1", Digest: "sha256:1"}
	withTag := base
	withTag.Tag, withTag.TagObject = "v1.2.4", "7a9c"
	otherTag := withTag
	otherTag.Tag = "v1.2.5"
	otherObject := withTag
	otherObject.TagObject = "beef"

	tree := func(e lockfile.Entry) string { return TreeOf(&lockfile.File{Include: []lockfile.Entry{e}}) }

	assert.NotEqual(t, tree(base), tree(withTag), "recording a tag changes the tree")
	assert.NotEqual(t, tree(withTag), tree(otherTag), "so does a different tag")
	assert.NotEqual(t, tree(withTag), tree(otherObject), "and a different tag object")
	assert.Equal(t, tree(withTag), tree(withTag), "stable")
}

func TestTreeOfIsUnchangedForEntriesWithoutATag(t *testing.T) {
	// The vector pins the scheme of locks written before version constraints.
	f := &lockfile.File{Include: []lockfile.Entry{{Name: "a", Source: "s", Ref: "main", Commit: "c", Digest: "sha256:d"}}}

	got := TreeOf(f)

	want := TopDigest([]Entry{{Kind: "include", Key: "a", Digest: "c\x00sha256:d\x00s\x00main\x00"}})
	assert.Equal(t, want, got)
}
