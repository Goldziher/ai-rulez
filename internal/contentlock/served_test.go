package contentlock

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func servedLeaf(sourceHash string) []Leaf {
	body := "---\nname: x\ndescription: d\n# Content-Hash: blake3:aaa\n# Source-Hash: " + sourceHash + "\n# Generated: 2026-10-05T10:00:00Z\n---\n\nbody\n"
	return []Leaf{{Path: "SKILL.md", Mode: ModeRegular, Data: []byte(body)}}
}

func TestServedDigest_HeaderIndependentIgnoresOnlyVolatileHeaderLines(t *testing.T) {
	a, err := ServedDigest(servedLeaf("blake3:111"), true)
	require.NoError(t, err)
	b, err := ServedDigest(servedLeaf("blake3:222"), true)
	require.NoError(t, err)
	assert.Equal(t, a, b, "the project-wide Source-Hash and the Generated stamp do not change the pinned digest")
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, a)

	rawA, err := ServedDigest(servedLeaf("blake3:111"), false)
	require.NoError(t, err)
	rawB, err := ServedDigest(servedLeaf("blake3:222"), false)
	require.NoError(t, err)
	assert.NotEqual(t, rawA, rawB, "the served-bytes digest covers the header")
	assert.NotEqual(t, a, rawA)

	changed := servedLeaf("blake3:111")
	changed[0].Data = append(changed[0].Data, []byte("more\n")...)
	c, err := ServedDigest(changed, true)
	require.NoError(t, err)
	assert.NotEqual(t, a, c, "content still changes the digest")
}

func TestServedDigest_UsesTheLockSchemeWithItsOwnDomain(t *testing.T) {
	leaves := []Leaf{{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("# x\n")}}
	served, err := ServedDigest(leaves, false)
	require.NoError(t, err)
	authored, err := TreeDigest("skill", leaves)
	require.NoError(t, err)
	tree, err := TreeDigest(KindServedSkill, leaves)
	require.NoError(t, err)
	assert.Equal(t, tree, served, "one implementation: a served digest is a tree digest of the served-skill kind")
	assert.NotEqual(t, authored, served, "domain separation: a served skill never equals an authored one")
	d := leafDigest(leaves[0])
	assert.Equal(t, "sha256:"+hex.EncodeToString(d[:]), FileDigest(leaves[0]))
}

func TestStripVolatileHeader_OnlyTouchesTheLeadingCommentLines(t *testing.T) {
	deep := []byte("---\nname: x\n---\n\n" + repeatLine("filler\n", 60) + "# Source-Hash: not a header, deep in the body\n")
	assert.Equal(t, deep, StripVolatileHeader(deep))

	body := []byte("Generated: this is prose, not a stamp\nSource-Hash: neither\n")
	assert.Equal(t, body, StripVolatileHeader(body), "only comment lines are header lines")

	html := []byte("<!-- AUTO-GENERATED | Generated: 2026-10-05T10:00:00Z -->\n<!-- Source-Hash: blake3:1 -->\n# Title\n")
	assert.Equal(t, "<!-- AUTO-GENERATED\n# Title\n", string(StripVolatileHeader(html)))
}

func repeatLine(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
