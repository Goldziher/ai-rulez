package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiveKeylessRoundTrip signs with the public-good Fulcio and Rekor and
// verifies offline against the fetched trusted root. It publishes a log entry,
// so it runs only when AI_RULEZ_LIVE_SIGSTORE=1 and a token is given in
// AI_RULEZ_LIVE_SIGSTORE_TOKEN (an OIDC token Fulcio accepts). Never part of the
// default run.
func TestLiveKeylessRoundTrip(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_SIGSTORE") != "1" {
		t.Skip("set AI_RULEZ_LIVE_SIGSTORE=1 (and AI_RULEZ_LIVE_SIGSTORE_TOKEN) to sign against the public Sigstore")
	}
	token := os.Getenv("AI_RULEZ_LIVE_SIGSTORE_TOKEN")
	if token == "" {
		t.Skip("AI_RULEZ_LIVE_SIGSTORE_TOKEN is not set")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	lock := testLock(t, nil)
	signer, err := NewKeylessSigner(KeylessOptions{IDToken: token})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := SignLock(ctx, signer, lock, LockMeta{Version: "live", Now: time.Now()})
	require.NoError(t, err)
	info, err := Inspect(data)
	require.NoError(t, err)
	rootPath, err := UpdateTrustedRoot(nil)
	require.NoError(t, err)
	tr, err := root.NewTrustedRootFromPath(rootPath)
	require.NoError(t, err)
	trust := TrustSet{Entries: []TrustEntry{{Subject: SubjectLock, Identity: info.Identity, Issuer: info.Issuer}}}

	rep, err := VerifyLock(data, lock, LockPolicy{Verifier: Verifier{TrustedRoot: tr, TLog: TLogRequired}, Trust: trust, MaxAge: time.Hour, Now: time.Now()})

	require.NoError(t, err)
	assert.Equal(t, KindKeyless, rep.Result.Signer.Kind)
	assert.True(t, rep.Result.Logged)
	assert.False(t, rep.Result.Weak)
}
