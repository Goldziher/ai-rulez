package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// testLock returns a lock whose tree matches its entries.
func testLock(t *testing.T, mutate func(*lockfile.File)) *lockfile.File {
	t.Helper()
	f := &lockfile.File{
		Version: lockfile.Version, OutputsPinned: true,
		Item: []lockfile.Item{
			{Kind: "rule", ID: "style", Path: "rules/style.md", Digest: "sha256:" + hexRepeat("ab")},
			{Kind: "skill", ID: "deploy", Domain: "backend", Path: "domains/backend/skills/deploy", Digest: "sha256:" + hexRepeat("cd")},
		},
	}
	if mutate != nil {
		mutate(f)
	}
	f.Tree = contentlock.TreeOf(f)
	return f
}

func hexRepeat(s string) string {
	out := ""
	for range 32 {
		out += s
	}
	return out
}

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func signKeyed(t *testing.T, lock *lockfile.File, meta LockMeta) (bundle, pubPEM []byte) {
	t.Helper()
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	if meta.Now.IsZero() {
		meta.Now = testNow
	}
	if meta.Version == "" {
		meta.Version = "5.0.0"
	}
	data, err := SignLock(context.Background(), signer, lock, meta)
	require.NoError(t, err)
	return data, pub
}

func keyTrust(t *testing.T, pubPEM []byte) TrustSet {
	t.Helper()
	pub, err := ParsePublicKey(pubPEM)
	require.NoError(t, err)
	return TrustSet{Entries: []TrustEntry{{Subject: SubjectLock, Key: pub}}}
}
