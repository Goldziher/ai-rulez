package signing_test

import (
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// policyProject writes a lock into a temp project and returns its config.
func policyProject(t *testing.T, s *config.SigningConfig) *config.Config {
	t.Helper()
	setUserDirs(t)
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	lock := testLock(t, nil)
	require.NoError(t, lockfile.Save(cfgDir, lock))
	return &config.Config{BaseDir: root, ConfigDir: cfgDir, Signing: s}
}

func TestPrepareLockCheckRefusesAnEmptyTrustSet(t *testing.T) {
	cfg := policyProject(t, nil)

	_, err := PrepareLockCheck(cfg, VerifyOptions{NoState: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trusted signer")
}

func TestPrepareLockCheckReadsKeysInsideTheProjectOnly(t *testing.T) {
	_, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "outside.pub")
	require.NoError(t, os.WriteFile(outside, pub, 0o644))

	tests := []struct {
		name  string
		setup func(t *testing.T, cfg *config.Config)
		file  string
		want  string
	}{
		{"key inside", func(t *testing.T, cfg *config.Config) {
			require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "release.pub"), pub, 0o644))
		}, "release.pub", ""},
		{"key missing", func(*testing.T, *config.Config) {}, "missing.pub", "read the trusted key"},
		{"symlink out of the project", func(t *testing.T, cfg *config.Config) {
			testutil.SymlinkOrSkip(t, outside, filepath.Join(cfg.BaseDir, "link.pub"))
		}, "link.pub", "read the trusted key"},
		{"dot-dot escape", func(*testing.T, *config.Config) {}, "../outside.pub", "read the trusted key"},
		{"not a key", func(t *testing.T, cfg *config.Config) {
			require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "junk.pub"), []byte("junk"), 0o644))
		}, "junk.pub", "public key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := policyProject(t, &config.SigningConfig{KeyFile: tt.file})
			tt.setup(t, cfg)

			check, err := PrepareLockCheck(cfg, VerifyOptions{NoState: true})

			if tt.want == "" {
				require.NoError(t, err)
				assert.Len(t, check.Policy.Verifier.Keys, 1)
				assert.Equal(t, TLogOff, check.Policy.Verifier.TLog)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPrepareLockCheckTrustedRootResolution(t *testing.T) {
	cfg := policyProject(t, &config.SigningConfig{Identity: "me", Issuer: "https://issuer.example", TrustedRoot: "missing-root.json"})

	_, err := PrepareLockCheck(cfg, VerifyOptions{NoState: true})
	require.Error(t, err)
	assert.Equal(t, CodeRootUnavailable, CodeOf(err), "a root the config names must exist")

	cfg.Signing.TrustedRoot = ""
	check, err := PrepareLockCheck(cfg, VerifyOptions{NoState: true})
	require.NoError(t, err)
	assert.Nil(t, check.Policy.Verifier.TrustedRoot, "without a named or cached root there is none; a keyless bundle then fails with AR725")
	assert.Equal(t, TLogRequired, check.Policy.Verifier.TLog)

	_, err = PrepareLockCheck(cfg, VerifyOptions{NoState: true, TrustedRoot: filepath.Join(t.TempDir(), "nope.json")})
	require.Error(t, err)
	assert.Equal(t, CodeRootUnavailable, CodeOf(err))
}

func TestPrepareLockCheckWarnsWhenTheSignersComeFromTheRepositoryAlone(t *testing.T) {
	_, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "ci.pub")
	require.NoError(t, os.WriteFile(outside, pub, 0o644))
	tests := []struct {
		name     string
		opts     VerifyOptions
		wantWarn bool
	}{
		{"signers only from the repository config", VerifyOptions{NoState: true}, true},
		{"a signer supplied on the command line", VerifyOptions{NoState: true, PublicKeys: []string{outside}}, false},
		{"an identity supplied on the command line", VerifyOptions{NoState: true, Identity: "me", Issuer: "https://issuer.example"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := policyProject(t, &config.SigningConfig{KeyFile: "release.pub"})
			require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "release.pub"), pub, 0o644))

			check, err := PrepareLockCheck(cfg, tt.opts)

			require.NoError(t, err)
			joined := strings.Join(check.Warnings, "\n")
			if tt.wantWarn {
				assert.Contains(t, joined, "this repository's own [signing] configuration")
				return
			}
			assert.NotContains(t, joined, "this repository's own")
		})
	}
}

func TestRequiredLockFindings(t *testing.T) {
	_, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	cfg := policyProject(t, &config.SigningConfig{Require: []string{"lock"}, KeyFile: "release.pub"})
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "release.pub"), pub, 0o644))

	findings := RequiredLockFindings(cfg, nil, testNow)

	require.Len(t, findings, 1)
	assert.Equal(t, CodeMissing, findings[0].Code, "no attestation file")

	cfg.Signing.Require = nil
	assert.Empty(t, RequiredLockFindings(cfg, nil, testNow), "nothing is required")
}

func TestReadBundleFilesRefusesAnAttestationOverTheLimit(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "ai-rulez.lock.sigstore.json")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(" ", MaxBundleBytes+1)), 0o600))

	// Act
	_, err := ReadBundleFiles(path)

	// Assert
	var se *Error
	require.ErrorAs(t, err, &se)
	assert.Equal(t, CodeInvalid, se.Code)
	assert.Contains(t, err.Error(), "size limit")
}
