package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internaltestutil "github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeKeyPair writes a P-256 PKCS#8 private key and its PKIX public key.
func writeKeyPair(t *testing.T, dir, name string) (priv, pub string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	priv, pub = filepath.Join(dir, name+".key"), filepath.Join(dir, name+".pub")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(priv, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(pub, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}), 0o600))
	return priv, pub
}

// signedProject is a locked project whose [signing] trusts keys/release.pub,
// with a second, untrusted key pair next to it. The private keys live outside
// the project.
func signedProject(t *testing.T, env *isoEnv) (root, releaseKey, otherKey string) {
	t.Helper()
	root = minimalProject(t, "\n[signing]\nrequire = [\"lock\"]\ntlog = \"off\"\nkey_file = \"keys/release.pub\"\n")
	keys := t.TempDir()
	releaseKey, releasePub := writeKeyPair(t, keys, "release")
	otherKey, _ = writeKeyPair(t, keys, "other")
	pub, err := os.ReadFile(releasePub) //nolint:gosec // test key
	require.NoError(t, err)
	writeTree(t, root, map[string]string{"keys/release.pub": string(pub)})
	require.Equal(t, 0, env.run(root, "generate", "--yes").ExitCode)
	lock := env.run(root, "lock")
	require.Equal(t, 0, lock.ExitCode, lock.Stderr)
	return root, releaseKey, otherKey
}

type verifyJSON struct {
	Results []struct {
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"results"`
}

func lockSidecar(root string) string {
	return filepath.Join(root, ".ai-rulez", "ai-rulez.lock.sigstore.json")
}

// forgeSignature flips one byte of the DSSE signature in a Sigstore bundle.
func forgeSignature(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test file
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	env := doc["dsseEnvelope"].(map[string]any)          //nolint:forcetypeassert // bundle shape
	sig := env["signatures"].([]any)[0].(map[string]any) //nolint:forcetypeassert // bundle shape
	raw, err := base64.StdEncoding.DecodeString(sig["sig"].(string))
	require.NoError(t, err)
	raw[len(raw)/2] ^= 0xff
	sig["sig"] = base64.StdEncoding.EncodeToString(raw)
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o600))
}

func TestSignLockE2E(t *testing.T) {
	tests := []struct {
		name     string
		act      func(t *testing.T, env *isoEnv, root, releaseKey, otherKey string)
		wantExit int
		wantCode string
	}{
		{name: "a lock signed with the trusted key verifies"},
		{
			name: "a lock changed after signing fails AR724",
			act: func(t *testing.T, env *isoEnv, root, _, _ string) {
				writeTree(t, root, map[string]string{".ai-rulez/rules/local.md": "# Local\n\nChanged after signing.\n"})
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
			},
			wantExit: 2, wantCode: "AR724",
		},
		{
			name: "a lock re-signed with an untrusted key fails AR722",
			act: func(t *testing.T, env *isoEnv, root, _, otherKey string) {
				require.Equal(t, 0, env.run(root, "sign", "--lock", "--key", otherKey).ExitCode)
			},
			wantExit: 2, wantCode: "AR722",
		},
		{
			name:     "a forged signature fails AR721",
			act:      func(t *testing.T, _ *isoEnv, root, _, _ string) { forgeSignature(t, lockSidecar(root)) },
			wantExit: 2, wantCode: "AR721",
		},
		{
			name: "a rolled-back attestation fails AR727",
			act: func(t *testing.T, env *isoEnv, root, releaseKey, _ string) {
				old, err := os.ReadFile(lockSidecar(root))
				require.NoError(t, err)
				require.Equal(t, 0, env.run(root, "verify", "--attestation").ExitCode)
				time.Sleep(1100 * time.Millisecond) // the signer's issued_at has one-second resolution
				require.Equal(t, 0, env.run(root, "sign", "--lock", "--key", releaseKey).ExitCode)
				require.Equal(t, 0, env.run(root, "verify", "--attestation").ExitCode)
				require.NoError(t, os.WriteFile(lockSidecar(root), old, 0o600))
			},
			wantExit: 2, wantCode: "AR727",
		},
		{
			name:     "a missing attestation fails AR720 under require = [\"lock\"]",
			act:      func(t *testing.T, _ *isoEnv, root, _, _ string) { require.NoError(t, os.Remove(lockSidecar(root))) },
			wantExit: 2, wantCode: "AR720",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			root, releaseKey, otherKey := signedProject(t, env)
			sign := env.run(root, "sign", "--lock", "--key", releaseKey)
			require.Equal(t, 0, sign.ExitCode, sign.Stderr)
			if tt.act != nil {
				tt.act(t, env, root, releaseKey, otherKey)
			}

			// Act
			res := env.run(root, "verify", "--attestation", "--format", "json")
			check := env.run(root, "lock", "--check")

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			requireJSONDoc(t, res)
			if tt.wantCode != "" {
				assert.Contains(t, res.Stdout, tt.wantCode)
				if tt.wantCode != "AR727" { // lock --check reads the rollback state but the mark is per verify run
					assert.Equal(t, 2, check.ExitCode, "lock --check enforces [signing] require too: %s", check.Stderr)
				}
			} else {
				var doc verifyJSON
				require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc))
				require.Len(t, doc.Results, 1)
				assert.Equal(t, "valid", doc.Results[0].Status)
				assert.Equal(t, 0, check.ExitCode, check.Stderr)
			}
		})
	}
}

func TestSignArtifactsE2E(t *testing.T) {
	tests := []struct {
		name     string
		subject  string // sbom or bundle
		tamper   func(t *testing.T, target string)
		trusted  bool
		wantExit int
		wantCode string
	}{
		{name: "an sbom round trip verifies", subject: "sbom", trusted: true},
		{
			name: "an edited sbom fails AR724", subject: "sbom", trusted: true,
			tamper: func(t *testing.T, target string) {
				f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test file
				require.NoError(t, err)
				_, _ = f.WriteString(" ") //nolint:errcheck // checked by Close
				require.NoError(t, f.Close())
			},
			wantExit: 2, wantCode: "AR724",
		},
		{name: "an sbom without a trusted signer cannot be verified", subject: "sbom", wantExit: 1},
		{name: "a bundle round trip verifies", subject: "bundle", trusted: true},
		{
			name: "a file added to a signed bundle fails AR724", subject: "bundle", trusted: true,
			tamper: func(t *testing.T, target string) {
				writeTree(t, target, map[string]string{"skills/evil/SKILL.md": "---\nname: evil\ndescription: x\n---\nEvil.\n"})
			},
			wantExit: 2, wantCode: "AR724",
		},
		{
			name: "a forged bundle signature fails AR721", subject: "bundle", trusted: true,
			tamper: func(t *testing.T, target string) {
				forgeSignature(t, filepath.Join(target, ".ai-rulez.sigstore.json"))
			},
			wantExit: 2, wantCode: "AR721",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			work := t.TempDir()
			priv, pub := writeKeyPair(t, t.TempDir(), "release")
			var target string
			if tt.subject == "sbom" {
				target = filepath.Join(work, "sbom.cdx.json")
				writeTree(t, work, map[string]string{"sbom.cdx.json": `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[]}`})
			} else {
				target = filepath.Join(work, "bundle")
				writeTree(t, target, map[string]string{
					".claude-plugin/plugin.json": `{"name":"acme","version":"1.0.0"}`,
					"skills/deploy/SKILL.md":     "---\nname: deploy\ndescription: Deploy.\n---\nDeploy.\n",
				})
			}
			sign := env.run(work, "sign", "--"+tt.subject, target, "--key", priv)
			require.Equal(t, 0, sign.ExitCode, sign.Stderr)
			if tt.tamper != nil {
				tt.tamper(t, target)
			}
			args := []string{"verify", "--" + tt.subject, target, "--format", "json"}
			if tt.trusted {
				args = append(args, "--public-key", pub)
			}

			// Act
			res := env.run(work, args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if tt.wantExit == 1 {
				return
			}
			requireJSONDoc(t, res)
			if tt.wantCode != "" {
				assert.Contains(t, res.Stdout, tt.wantCode)
			} else {
				assert.Contains(t, res.Stdout, `"status": "valid"`)
			}
		})
	}
}

// TestSignBundleRefusesSymlinksE2E: a signature over a directory must not
// cover "where a link pointed", so a symlink in a bundle is refused when
// signing and when verifying.
func TestSignBundleRefusesSymlinksE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	work := t.TempDir()
	priv, pub := writeKeyPair(t, t.TempDir(), "release")
	bundle := filepath.Join(work, "bundle")
	writeTree(t, bundle, map[string]string{"skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploy.\n---\nDeploy.\n"})
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	// Act: sign a clean bundle, then plant a symlink, verify; and try to sign with it.
	require.Equal(t, 0, env.run(work, "sign", "--bundle", bundle, "--key", priv).ExitCode)
	internaltestutil.SymlinkOrSkip(t, outside, filepath.Join(bundle, "skills", "deploy", "notes.txt"))
	verify := env.run(work, "verify", "--bundle", bundle, "--public-key", pub)
	resign := env.run(work, "sign", "--bundle", bundle, "--key", priv)

	// Assert
	assert.NotEqual(t, 0, verify.ExitCode, "a planted symlink must not verify: %s", verify.Stdout)
	assert.Equal(t, 1, resign.ExitCode, "signing a bundle with a symlink is refused: %s", resign.Stdout+resign.Stderr)
	assert.True(t, strings.Contains(resign.Stdout+resign.Stderr, "symlink"), resign.Stderr)
}
