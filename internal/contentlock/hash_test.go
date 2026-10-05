package contentlock

import (
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vectors below were computed with an independent Python implementation of
// the scheme in docs/lockfile.md. If one changes, lockfile.Version must change.
func TestVectors(t *testing.T) {
	skill := []Leaf{
		{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("---\nname: deploy\n---\nDeploy.\n")},
		{Path: "scripts/run.sh", Mode: ModeExecutable, Data: []byte("#!/bin/sh\necho hi\n")},
		{Path: "assets/logo.bin", Mode: ModeRegular, Data: []byte("\x00\x01\r\n\x02")},
	}
	tests := []struct {
		name   string
		kind   string
		leaves []Leaf
		want   string
	}{
		{"rule", "rule", []Leaf{{Path: "style.md", Mode: ModeRegular, Data: []byte("# Style\n")}},
			"sha256:f9c0b1ef53a34e543828ff3459f4f117e08edc2976c22a3ee32d0a65ee212c9c"},
		{"rule with CRLF is the same", "rule", []Leaf{{Path: "style.md", Mode: ModeRegular, Data: []byte("# Style\r\n")}},
			"sha256:f9c0b1ef53a34e543828ff3459f4f117e08edc2976c22a3ee32d0a65ee212c9c"},
		{"empty file", "context", []Leaf{{Path: "empty.md", Mode: ModeRegular}},
			"sha256:6e09fc1d734e8a2db85a25265407b4e78e44e45e712d4f60d2682c121f95c40b"},
		{"skill with resources, binary bytes kept", "skill", skill,
			"sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05"},
		{"script LF", "skill", []Leaf{{Path: "run.sh", Mode: ModeExecutable, Data: []byte("echo hi\n")}},
			"sha256:d7aec0e55512be14e8c20554ee6da9911881211fec26eec3927bb18b21212b54"},
		{"script CRLF is a different file (issue 233)", "skill", []Leaf{{Path: "run.sh", Mode: ModeExecutable, Data: []byte("echo hi\r\n")}},
			"sha256:5b9fcff355356346b64a0f8146b7c65b765f377d1deddf29318ef9e686e7ee92"},
		{"installed skill", KindInstalledSkill, []Leaf{{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("# S\n")}},
			"sha256:018df4aac28d9eca573a05cb491c6704407d9b5de8ca10e0dfe20300928140db"},
		{"skill source", KindSkillSource, []Leaf{{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("# S\n")}},
			"sha256:cf90b42b8c9bbb2e9bc94075e4d8a183117f45b46d0b746b7bc617587f64bc7b"},
		{"served skill", KindServedSkill, []Leaf{{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("# x\n")}},
			"sha256:2225664563ac4bc0affa10d8ca1ea4dcd5ed2a61792b124824005493dffb458c"},
		{"no files", "rule", nil,
			"sha256:c36b2dc178586a67dfcf7bc1e18ab82e0aa267eb5c5a7d7cba0a6ee7b062ec26"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TreeDigest(tt.kind, tt.leaves)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	leaf := leafDigest(Leaf{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("# Title\nbody\n")})
	assert.Equal(t, "cbac10c14a090ab0301ad17b0013e8c960c5960d5574dcabf38867b98987c621", hex.EncodeToString(leaf[:]))

	top := TopDigest([]Entry{
		{Kind: "output", Key: "CLAUDE.md", Digest: "sha256:" + "abababababababababababababababababababababababababababababababab"},
		{Kind: "item/skill", Key: "\x00deploy", Digest: "sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05"},
	})
	assert.Equal(t, "sha256:6cd1d810fce0e91263b3ebfa3610821a820a07b415a8f2a4b9415de191b6c24e", top)
	assert.Equal(t, "sha256:e8c93a22e1ed47e16dd881a55dc4fbc5ba685af20083b93469265da901784029", TopDigest(nil))
}

func TestTreeDigestIsOrderIndependent(t *testing.T) {
	leaves := []Leaf{
		{Path: "SKILL.md", Mode: ModeRegular, Data: []byte("a")},
		{Path: "references/a.md", Mode: ModeRegular, Data: []byte("b")},
		{Path: "references/b.md", Mode: ModeRegular, Data: []byte("c")},
		{Path: "scripts/x.sh", Mode: ModeExecutable, Data: []byte("d")},
	}
	want, err := TreeDigest("skill", leaves)
	require.NoError(t, err)
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic shuffle
	for range 25 {
		shuffled := append([]Leaf(nil), leaves...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got, err := TreeDigest("skill", shuffled)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestTreeDigestDistinguishes(t *testing.T) {
	base := []Leaf{{Path: "a.md", Mode: ModeRegular, Data: []byte("x")}}
	ref, err := TreeDigest("rule", base)
	require.NoError(t, err)
	variants := map[string][]Leaf{
		"content":       {{Path: "a.md", Mode: ModeRegular, Data: []byte("y")}},
		"mode":          {{Path: "a.md", Mode: ModeExecutable, Data: []byte("x")}},
		"rename":        {{Path: "b.md", Mode: ModeRegular, Data: []byte("x")}},
		"extra file":    append(append([]Leaf(nil), base...), Leaf{Path: "c.md", Mode: ModeRegular}),
		"bare CR kept":  {{Path: "a.md", Mode: ModeRegular, Data: []byte("x\r")}},
		"non-text CRLF": {{Path: "a.bin", Mode: ModeRegular, Data: []byte("x\r\n")}},
	}
	seen := map[string]string{ref: "base"}
	for name, leaves := range variants {
		got, err := TreeDigest("rule", leaves)
		require.NoError(t, err)
		assert.NotContains(t, seen, got, name+" collides with "+seen[got])
		seen[got] = name
	}
	other, err := TreeDigest("agent", base)
	require.NoError(t, err)
	assert.NotEqual(t, ref, other, "kinds are domain separated")

	// the split between path and data cannot be shifted
	a, _ := TreeDigest("rule", []Leaf{{Path: "ab", Mode: ModeRegular, Data: []byte("c")}})
	b, _ := TreeDigest("rule", []Leaf{{Path: "a", Mode: ModeRegular, Data: []byte("bc")}})
	assert.NotEqual(t, a, b)
}

func TestTreeDigestRejectsBadPaths(t *testing.T) {
	for _, p := range []string{"", "/abs.md", "a\\b.md", "../x.md", "a/./b.md", "a//b.md"} {
		_, err := TreeDigest("rule", []Leaf{{Path: p, Mode: ModeRegular}})
		assert.Error(t, err, p)
	}
	_, err := TreeDigest("rule", []Leaf{{Path: "a.md", Mode: ModeRegular}, {Path: "a.md", Mode: ModeRegular}})
	assert.Error(t, err, "duplicate paths")
	_, err = TreeDigest("rule", []Leaf{{Path: "a.md", Mode: "777"}})
	assert.Error(t, err)
}

func TestModeFor(t *testing.T) {
	assert.Equal(t, ModeRegular, ModeFor(0o644))
	assert.Equal(t, ModeRegular, ModeFor(0o600))
	assert.Equal(t, ModeExecutable, ModeFor(0o755))
	assert.Equal(t, ModeExecutable, ModeFor(0o100))
}

func TestScriptsAreHashedByteForByte(t *testing.T) {
	for _, name := range []string{"a.sh", "a.bash", "a.zsh", "a.py", "a.js", "a.mjs", "a.cjs", "a.ts", "A.SH"} {
		assert.False(t, IsTextPath(name), name)
		lf, err := TreeDigest("skill", []Leaf{{Path: name, Mode: ModeExecutable, Data: []byte("x\n")}})
		require.NoError(t, err)
		crlf, err := TreeDigest("skill", []Leaf{{Path: name, Mode: ModeExecutable, Data: []byte("x\r\n")}})
		require.NoError(t, err)
		assert.NotEqual(t, lf, crlf, name)
	}
	assert.True(t, IsTextPath("a.md"))
	assert.Equal(t, []string{".json", ".jsonc", ".markdown", ".md", ".mdc", ".mdx", ".toml", ".txt", ".yaml", ".yml"}, TextExtensions())
}
