package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

// keyFixture is a signing key and the public key file a machine trusts it by.
type keyFixture struct {
	signer  *sigstore.KeySigner
	pubPath string
}

func newKey(t *testing.T) keyFixture {
	t.Helper()
	priv, pub, err := sigstore.GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := sigstore.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "signer.pub")
	require.NoError(t, os.WriteFile(path, pub, 0o600))
	return keyFixture{signer: ks, pubPath: path}
}

// attest signs data as a policy attestation claimed to be issued at at.
func (k keyFixture) attest(t *testing.T, data string, at time.Time) []byte {
	t.Helper()
	bundle, err := SignPolicy(context.Background(), k.signer, []byte(data), at)
	require.NoError(t, err)
	return bundle
}

// blob signs the exact bytes with a message signature, as cosign sign-blob does.
func (k keyFixture) blob(t *testing.T, data string) []byte {
	t.Helper()
	pb, err := k.signer.Bundle(context.Background(), &sigstore.PlainData{Data: []byte(data)})
	require.NoError(t, err)
	out, err := protojson.Marshal(pb)
	require.NoError(t, err)
	return out
}

func signedOpts(t *testing.T, flag string, mutate func(*DiscoverOptions)) DiscoverOptions {
	t.Helper()
	home := t.TempDir()
	o := DiscoverOptions{
		Flag: flag, Clock: ambient.Fixed(epoch), Env: ambient.MapEnv{Vars: map[string]string{}, Home: home},
		ManagedPaths: []string{filepath.Join(home, "none.toml")},
	}
	if mutate != nil {
		mutate(&o)
	}
	return o
}

func writeSigned(t *testing.T, dir, body string, bundle []byte) string {
	t.Helper()
	path := writePolicy(t, dir, "policy.toml", body)
	if bundle != nil {
		require.NoError(t, os.WriteFile(path+SidecarSuffix, bundle, 0o600))
	}
	return path
}

func TestSignedFilePolicy(t *testing.T) {
	key, other := newKey(t), newKey(t)
	body := "policy_version = 1\nname = \"signed\"\n[lock]\nenforce = true\n"
	trust := func(k keyFixture) func(*DiscoverOptions) {
		return func(o *DiscoverOptions) { o.Signature.KeyFiles = []string{k.pubPath} }
	}
	tests := []struct {
		name       string
		bundle     func() []byte
		file       string
		mutate     func(*DiscoverOptions)
		wantSigner bool
		wantErr    string
	}{
		{"a trusted signature verifies", func() []byte { return key.attest(t, body, epoch) }, body, trust(key), true, ""},
		{"a cosign blob signature verifies", func() []byte { return key.blob(t, body) }, body, trust(key), true, ""},
		{"signed by an untrusted key", func() []byte { return other.attest(t, body, epoch) }, body, trust(key), false, "AR746"},
		{"the policy changed after signing", func() []byte { return key.attest(t, body, epoch) }, body + "[guard]\ngenerated = true\n", trust(key), false, "changed since it was signed"},
		{"a blob signature over other bytes", func() []byte { return key.blob(t, body) }, body + "# edit\n", trust(key), false, "AR746"},
		{"a garbage signature", func() []byte { return []byte(`{"not":"a bundle"}`) }, body, trust(key), false, "AR746"},
		{"signed in the future", func() []byte { return key.attest(t, body, epoch.Add(2*time.Hour)) }, body, trust(key), false, "future"},
		{"unsigned and not required is fine", nil, body, trust(key), false, ""},
		{"unsigned and required", nil, body, func(o *DiscoverOptions) { trust(key)(o); o.Signature.Require = true }, false, "AR746"},
		{"required without any trusted signer", nil, body, func(o *DiscoverOptions) { o.Signature.Require = true }, false, "no trusted signer is configured"},
		{"a signature is ignored when no trust is configured", func() []byte { return key.attest(t, body, epoch) }, body, nil, false, ""},
		{"a bad trusted key file fails closed", nil, body, func(o *DiscoverOptions) { o.Signature.KeyFiles = []string{filepath.Join(t.TempDir(), "gone.pub")} }, false, "cannot read the trusted signer key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var bundle []byte
			if tt.bundle != nil {
				bundle = tt.bundle()
			}
			path := writeSigned(t, t.TempDir(), tt.file, bundle)
			// Act
			layers, err := Discover(signedOpts(t, path, tt.mutate))
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, layers, 1)
			assert.Equal(t, tt.wantSigner, layers[0].Signer != "", "signer %q", layers[0].Signer)
		})
	}
}

func TestSignedFilePolicyRollback(t *testing.T) {
	// Arrange: one machine verifies a newer policy, then is handed an older signed one.
	key := newKey(t)
	body := "policy_version = 1\n[lock]\nenforce = true\n"
	home := t.TempDir()
	opts := func(path string) DiscoverOptions {
		o := signedOpts(t, path, func(o *DiscoverOptions) { o.Signature.KeyFiles = []string{key.pubPath} })
		o.Env = ambient.MapEnv{Vars: map[string]string{}, Home: home}
		return o
	}
	dir := t.TempDir()
	path := writeSigned(t, dir, body, key.attest(t, body, epoch.Add(-time.Hour)))
	// Act
	_, newerErr := Discover(opts(path))
	require.NoError(t, os.WriteFile(path+SidecarSuffix, key.attest(t, body, epoch.Add(-48*time.Hour)), 0o600))
	_, olderErr := Discover(opts(path))
	require.NoError(t, os.WriteFile(path+SidecarSuffix, key.attest(t, body, epoch.Add(-time.Hour)), 0o600))
	_, againErr := Discover(opts(path))
	// Assert
	require.NoError(t, newerErr)
	require.Error(t, olderErr)
	assert.Contains(t, olderErr.Error(), "AR746")
	assert.Contains(t, olderErr.Error(), "AR727")
	require.NoError(t, againErr, "the same signing time is not a rollback")
}

func TestSignedBlobPolicyWithoutSigningTimeCannotBeRolledBack(t *testing.T) {
	// Arrange: a key-signed blob bundle has no signing time, so time cannot order
	// versions; the machine must still refuse a version it has already replaced.
	key := newKey(t)
	v1 := "policy_version = 1\n[lock]\nenforce = true\n"
	v2 := v1 + "[guard]\ngenerated = true\n"
	home := t.TempDir()
	opts := func(path string) DiscoverOptions {
		o := signedOpts(t, path, func(o *DiscoverOptions) { o.Signature.KeyFiles = []string{key.pubPath} })
		o.Env = ambient.MapEnv{Vars: map[string]string{}, Home: home}
		return o
	}
	path := writeSigned(t, t.TempDir(), v1, key.blob(t, v1))
	// Act
	_, firstErr := Discover(opts(path))
	require.NoError(t, os.WriteFile(path, []byte(v2), 0o600))
	require.NoError(t, os.WriteFile(path+SidecarSuffix, key.blob(t, v2), 0o600))
	_, newerErr := Discover(opts(path))
	require.NoError(t, os.WriteFile(path, []byte(v1), 0o600))
	require.NoError(t, os.WriteFile(path+SidecarSuffix, key.blob(t, v1), 0o600))
	_, rollbackErr := Discover(opts(path))
	// Assert
	require.NoError(t, firstErr)
	require.NoError(t, newerErr)
	require.Error(t, rollbackErr)
	assert.Contains(t, rollbackErr.Error(), "AR727")
}

func TestSignedPolicyIdentityTrustNeedsAnIssuer(t *testing.T) {
	// Arrange
	path := writePolicy(t, t.TempDir(), "p.toml", minimalPolicy)
	// Act
	_, err := Discover(signedOpts(t, path, func(o *DiscoverOptions) { o.Signature.Identity = "ci@example.org" }))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go together")
}

func TestSignedPolicyConfigFromTheEnvironmentAndUserConfig(t *testing.T) {
	key := newKey(t)
	body := "policy_version = 1\n[lock]\nenforce = true\n"
	signed := writeSigned(t, t.TempDir(), body, key.attest(t, body, epoch))
	unsigned := writeSigned(t, t.TempDir(), body, nil)
	t.Run("signer key and requirement from the environment", func(t *testing.T) {
		o := signedOpts(t, "", nil)
		o.Env = ambient.MapEnv{Vars: map[string]string{EnvPolicy: unsigned, EnvSignerKey: key.pubPath, EnvRequireSigned: "1"}, Home: t.TempDir()}
		_, err := Discover(o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
		o.Env = ambient.MapEnv{Vars: map[string]string{EnvPolicy: signed, EnvSignerKey: key.pubPath, EnvRequireSigned: "1"}, Home: t.TempDir()}
		layers, err := Discover(o)
		require.NoError(t, err)
		assert.NotEmpty(t, layers[0].Signer)
	})
	t.Run("signers and the requirement from the user config", func(t *testing.T) {
		// A literal string: a Windows path's backslashes are not TOML escapes.
		cfg := "[policy]\nrequire_signature = true\n[[policy.signers]]\nkey_file = '" + key.pubPath + "'\n"
		o := signedOpts(t, signed, nil)
		o.Env = userEnv(t, cfg)
		layers, err := Discover(o)
		require.NoError(t, err)
		assert.NotEmpty(t, layers[0].Signer)
		o = signedOpts(t, unsigned, nil)
		o.Env = userEnv(t, cfg)
		_, err = Discover(o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
}

func TestUserSignersAreValidated(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"neither identity nor key", "[[policy.signers]]\nissuer = \"i\"\n", "needs identity"},
		{"identity without issuer", "[[policy.signers]]\nidentity = \"a@b.c\"\n", "needs an issuer"},
		{"both forms", "[[policy.signers]]\nidentity = \"a\"\nkey_file = \"k.pem\"\nissuer = \"i\"\n", "not both"},
		{"unanchored regexp", "[[policy.signers]]\nidentity_regexp = \"a.*\"\nissuer = \"i\"\n", "anchored"},
		{"key file with issuer", "[[policy.signers]]\nkey_file = \"k.pem\"\nissuer = \"i\"\n", "no issuer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := LoadUserSettings(userEnv(t, tt.body))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// signedRemote is a policy server that publishes a signature next to the policy.
type signedRemote struct {
	srv    *httptest.Server
	body   atomic.Value
	bundle atomic.Value
	down   atomic.Bool
	noSig  atomic.Bool
	sigErr atomic.Bool
}

func newSignedRemote(t *testing.T, body string, bundle []byte) *signedRemote {
	t.Helper()
	f := &signedRemote{}
	f.body.Store(body)
	f.bundle.Store(bundle)
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case f.down.Load():
			http.Error(w, "down", http.StatusServiceUnavailable)
		case r.URL.Path == "/policy.toml":
			_, _ = w.Write([]byte(f.body.Load().(string)))
		case r.URL.Path == "/policy.toml"+SidecarSuffix && f.sigErr.Load():
			http.Error(w, "boom", http.StatusInternalServerError)
		case r.URL.Path == "/policy.toml"+SidecarSuffix && !f.noSig.Load():
			_, _ = w.Write(f.bundle.Load().([]byte))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *signedRemote) opts(t *testing.T, flag string, key keyFixture, mutate func(*DiscoverOptions)) DiscoverOptions {
	t.Helper()
	return signedOpts(t, flag, func(o *DiscoverOptions) {
		o.HTTPClient = f.srv.Client()
		o.Signature.KeyFiles = []string{key.pubPath}
		if mutate != nil {
			mutate(o)
		}
	})
}

func TestSignedURLPolicyNeedsNoDigest(t *testing.T) {
	// Arrange
	key := newKey(t)
	body := "policy_version = 1\nname = \"signed\"\n[lock]\nenforce = true\n"
	f := newSignedRemote(t, body, key.attest(t, body, epoch))
	url := f.srv.URL + "/policy.toml"

	t.Run("a verified signature replaces the pin", func(t *testing.T) {
		o := f.opts(t, url, key, nil)
		layers, err := Discover(o)
		require.NoError(t, err)
		require.Len(t, layers, 1)
		assert.NotEmpty(t, layers[0].Signer)
		assert.True(t, layers[0].Policy.Lock.Enforce)
	})
	t.Run("without a signature the missing pin is the error", func(t *testing.T) {
		f.noSig.Store(true)
		t.Cleanup(func() { f.noSig.Store(false) })
		_, err := Discover(f.opts(t, url, key, nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
	})
	t.Run("a content change that the signature does not cover is refused", func(t *testing.T) {
		f.body.Store(body + "[guard]\ngenerated = true\n")
		t.Cleanup(func() { f.body.Store(body) })
		_, err := Discover(f.opts(t, url, key, nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
	t.Run("an untrusted signer is refused", func(t *testing.T) {
		_, err := Discover(f.opts(t, url, newKey(t), nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
}

func TestSignedURLPolicyCacheKeepsTheSignature(t *testing.T) {
	// Arrange: a first online load caches body and bundle; the server then goes down.
	key := newKey(t)
	body := "policy_version = 1\n[lock]\nenforce = true\n"
	f := newSignedRemote(t, body, key.attest(t, body, epoch))
	url := f.srv.URL + "/policy.toml"
	home := t.TempDir()
	withHome := func(o *DiscoverOptions) { o.Env = ambient.MapEnv{Vars: map[string]string{}, Home: home} }
	_, err := Discover(f.opts(t, url, key, withHome))
	require.NoError(t, err)
	f.down.Store(true)

	t.Run("the cached copy is verified again", func(t *testing.T) {
		layers, err := Discover(f.opts(t, url, key, func(o *DiscoverOptions) { withHome(o); o.Clock = ambient.Fixed(epoch.Add(time.Hour)) }))
		require.NoError(t, err)
		assert.NotEmpty(t, layers[0].Signer, "the signature traveled with the cached body")
		assert.Contains(t, layers[0].Note, "cached copy")
	})
	t.Run("a cached copy checked against another trusted key fails", func(t *testing.T) {
		_, err := Discover(f.opts(t, url, newKey(t), func(o *DiscoverOptions) { withHome(o); o.Clock = ambient.Fixed(epoch.Add(time.Hour)) }))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
	t.Run("past max_stale it fails closed", func(t *testing.T) {
		_, err := Discover(f.opts(t, url, key, func(o *DiscoverOptions) { withHome(o); o.Clock = ambient.Fixed(epoch.Add(30 * 24 * time.Hour)) }))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742")
	})
}

func TestPinnedURLWithSignatureRules(t *testing.T) {
	key := newKey(t)
	body := "policy_version = 1\n[lock]\nenforce = true\n"
	f := newSignedRemote(t, body, key.attest(t, body, epoch))
	pinned := f.srv.URL + "/policy.toml@" + digest([]byte(body))
	t.Run("a pinned and signed policy records its signer", func(t *testing.T) {
		layers, err := Discover(f.opts(t, pinned, key, nil))
		require.NoError(t, err)
		assert.NotEmpty(t, layers[0].Signer)
	})
	t.Run("required but unsigned", func(t *testing.T) {
		f.noSig.Store(true)
		t.Cleanup(func() { f.noSig.Store(false) })
		_, err := Discover(f.opts(t, pinned, key, func(o *DiscoverOptions) { o.Signature.Require = true }))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
	t.Run("not required and unsigned loads on the pin alone", func(t *testing.T) {
		f.noSig.Store(true)
		t.Cleanup(func() { f.noSig.Store(false) })
		layers, err := Discover(f.opts(t, pinned, key, nil))
		require.NoError(t, err)
		assert.Empty(t, layers[0].Signer)
	})
	t.Run("an unreachable signature is tolerated only when not required", func(t *testing.T) {
		f.sigErr.Store(true)
		t.Cleanup(func() { f.sigErr.Store(false) })
		layers, err := Discover(f.opts(t, pinned, key, nil))
		require.NoError(t, err)
		assert.Empty(t, layers[0].Signer)
		_, err = Discover(f.opts(t, pinned, key, func(o *DiscoverOptions) { o.Signature.Require = true }))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742", "required: with no cached signature the run fails closed")
	})
	t.Run("a bad signature is refused even though the pin matches", func(t *testing.T) {
		f.bundle.Store(newKey(t).attest(t, body, epoch))
		t.Cleanup(func() { f.bundle.Store(key.attest(t, body, epoch)) })
		_, err := Discover(f.opts(t, pinned, key, nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR746")
	})
}

func TestExtendedPoliciesMustBeSignedToo(t *testing.T) {
	// Arrange: signatures are required; the team policy is signed but its parent is not.
	key := newKey(t)
	dir := t.TempDir()
	parent := chainFile(t, dir, "org.toml", nil, "[lock]\nenforce = true\n")
	childBody := "policy_version = 1\nname = \"team\"\nextends = [\"org.toml\"]\n"
	child := writeSigned(t, dir, childBody, key.attest(t, childBody, epoch))
	mutate := func(o *DiscoverOptions) { o.Signature.KeyFiles = []string{key.pubPath}; o.Signature.Require = true }
	// Act
	_, err := Discover(signedOpts(t, child, mutate))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR746")
	assert.Contains(t, err.Error(), "org.toml")
	// And signing the parent fixes it.
	parentBody, rerr := os.ReadFile(parent)
	require.NoError(t, rerr)
	require.NoError(t, os.WriteFile(parent+SidecarSuffix, key.attest(t, string(parentBody), epoch), 0o600))
	layers, err := Discover(signedOpts(t, child, mutate))
	require.NoError(t, err)
	assert.Len(t, layers, 2)
	assert.NotEmpty(t, layers[1].Signer)
}

func TestSidecarNames(t *testing.T) {
	assert.Equal(t, "/etc/p.toml.sigstore.json", sidecarRef(Ref{Location: "/etc/p.toml"}).Location)
	got := sidecarRef(Ref{Location: "https://p.example.org/a/p.toml?x=1", Remote: true})
	assert.Equal(t, "https://p.example.org/a/p.toml.sigstore.json?x=1", got.Location)
	assert.True(t, got.Remote)
}

func TestPolicyStatementCoversTheNormalizedDigest(t *testing.T) {
	// Arrange: the same policy with CRLF line endings has the same digest.
	lf := "policy_version = 1\nname = \"x\"\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	// Act
	a, errA := PolicyStatement([]byte(lf), epoch)
	b, errB := PolicyStatement([]byte(crlf), epoch)
	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, a.Subject, b.Subject)
	assert.Equal(t, PredicatePolicy, a.PredicateType)
	_, err := PolicyStatement([]byte("not a policy"), epoch)
	require.Error(t, err, "only a parseable policy is signed")
}
