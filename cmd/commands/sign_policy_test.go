package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

func policyFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ai-rulez-policy.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func TestSignPolicyWritesASidecarThePolicyLoaderVerifies(t *testing.T) {
	// Arrange
	f := newSignFixture(t, "")
	file := policyFile(t, "policy_version = 1\nname = \"org\"\n[lock]\nenforce = true\n")
	signKey, signPolicy = f.privKey, file
	t.Cleanup(func() { signPolicy = "" })

	// Act
	var code int
	capture(t, func() { code = runSign(context.Background(), nil, nil) })

	// Assert
	require.Equal(t, 0, code)
	info, err := os.Stat(file + ".sigstore.json")
	require.NoError(t, err, "the signature goes next to the policy")
	assertFileMode(t, info, 0o644, "a policy signature is public")
	load := func() error {
		_, err := policy.Discover(policy.DiscoverOptions{
			Flag: file, Env: ambient.MapEnv{Vars: map[string]string{}, Home: t.TempDir()},
			ManagedPaths: []string{filepath.Join(t.TempDir(), "none.toml")},
			Signature:    policy.SignatureOptions{Require: true, KeyFiles: []string{f.pubKey}},
		})
		return err
	}
	require.NoError(t, load(), "the signer's key verifies the policy")
	require.NoError(t, os.WriteFile(file, []byte("policy_version = 1\nname = \"org\"\n"), 0o644))
	err = load()
	require.Error(t, err, "an edited policy no longer matches its signature")
	assert.Contains(t, err.Error(), "AR746")
}

func TestSignPolicyRefusesWhatIsNotAPolicy(t *testing.T) {
	// Arrange
	f := newSignFixture(t, "")
	file := policyFile(t, "this is not a policy")
	signKey, signPolicy = f.privKey, file
	t.Cleanup(func() { signPolicy = "" })

	// Act
	var code int
	_, stderr := capture(t, func() { code = runSign(context.Background(), nil, nil) })

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "AR743")
	_, err := os.Stat(file + ".sigstore.json")
	assert.True(t, os.IsNotExist(err), "nothing is signed that the loader would reject")
}

func TestSignPolicyIsASubjectOfItsOwn(t *testing.T) {
	// Arrange
	newSignFixture(t, "")
	signPolicy, signLock = "p.toml", true
	t.Cleanup(func() { signPolicy, signLock = "", false })

	// Act
	err := validateSignFlags()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestSignPolicyRejectsTheBundleOnlyFlags(t *testing.T) {
	// Arrange
	f := newSignFixture(t, "")
	signKey, signPolicy, signProvenance = f.privKey, policyFile(t, "policy_version = 1\n"), true
	t.Cleanup(func() { signPolicy, signProvenance = "", false })

	// Act
	err := validateSignFlags()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--provenance applies to --bundle")
}
