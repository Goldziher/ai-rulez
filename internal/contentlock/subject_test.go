package contentlock

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
)

// The vectors were computed with an independent Python implementation of the
// formula in docs/lockfile.md ("Signing the lock").
func TestSubjectDigestVectors(t *testing.T) {
	const (
		emptyTree = "sha256:e8c93a22e1ed47e16dd881a55dc4fbc5ba685af20083b93469265da901784029"
		tree      = "sha256:6cd1d810fce0e91263b3ebfa3610821a820a07b415a8f2a4b9415de191b6c24e"
	)
	tests := []struct {
		name string
		in   Subject
		want string
	}{
		{"empty tree", Subject{Tree: emptyTree, HashVersion: 1, Scope: "all"},
			"sha256:e2aa7b2316bd0afce50b14562f66085c3cdccd202e192bf86ec13dd4ccbe5960"},
		{"outputs pinned", Subject{Tree: tree, HashVersion: 1, Scope: "all", OutputsPinned: true},
			"sha256:60e6900f7b4f0dfa733cc2ac84f7c8de74f7a2a5724cf0951ce4af4b24e39cb9"},
		{"scope skills", Subject{Tree: tree, HashVersion: 1, Scope: "skills"},
			"sha256:42b272daba65257a8d6f8e67ac77cad298bb52abdaf3ae92a44189ba73558a9e"},
		{"approvals digest set", Subject{Tree: tree, ApprovalsDigest: "sha256:" + repeat("cd", 32), HashVersion: 1, Scope: "all", OutputsPinned: true},
			"sha256:84e7e405f01af2e4dfbad51474518623622d9dcf7d03ea71e7b2b948e20ae51f"},
		{"hash version 2", Subject{Tree: tree, HashVersion: 2, Scope: "all", OutputsPinned: true},
			"sha256:03c00956aa0f7b777ec4ab615791587a0fff2b0c7882903d92234874ffec0b0e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.Digest())
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

func TestSubjectOfRecomputesTreeAndDefaultsScope(t *testing.T) {
	f := &lockfile.File{
		Version: 1, Tree: "sha256:forged", OutputsPinned: true,
		Item: []lockfile.Item{{Kind: "skill", ID: "deploy", Digest: "sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05"}},
	}

	got := SubjectOf(f)

	assert.Equal(t, TreeOf(f), got.Tree, "the stored tree must not be trusted")
	assert.NotEqual(t, "sha256:forged", got.Tree)
	assert.Equal(t, "all", got.Scope)
	assert.Equal(t, 1, got.HashVersion)
	assert.True(t, got.OutputsPinned)
	assert.Empty(t, got.ApprovalsDigest)
}
