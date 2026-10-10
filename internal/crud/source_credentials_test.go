package crud

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourcesRefuseACredentialedURL checks that neither `include add` nor
// `skill install` writes a URL with userinfo into the config, and that the
// refusal names the environment variable to use instead.
func TestSourcesRefuseACredentialedURL(t *testing.T) {
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{"user and password", "https://user:" + secret + "@github.com/o/r.git", true},
		{"token as the user", "https://" + secret + "@github.com/o/r.git", true},
		{"git+https with a credential", "git+https://user:token@git.example.com/o/r", true},
		{"token in the query string", "https://github.com/o/r.git?access_token=" + secret, true},
		{"plain https", "https://github.com/o/r.git", false},
		{"scp-style ssh", "git@github.com:o/r.git", false},
	}
	for _, tt := range tests {
		t.Run("include/"+tt.name, func(t *testing.T) {
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)

			err = op.AddInclude(context.Background(), &AddIncludeRequest{Name: "inc", Source: tt.source})

			assertSourceOutcome(t, dir, err, tt.wantErr, secret)
		})
		t.Run("skill/"+tt.name, func(t *testing.T) {
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)

			err = op.InstallSkill(context.Background(), &InstallSkillRequest{Name: "sk", Source: tt.source})

			assertSourceOutcome(t, dir, err, tt.wantErr, secret)
		})
	}
}

// TestLocalSourcesRefuseACredentialedURL checks the machine-local overlay path
// (`--local`) refuses the same URL: the check runs before the overlay is opened.
func TestLocalSourcesRefuseACredentialedURL(t *testing.T) {
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	source := "https://user:" + secret + "@github.com/o/r.git"
	tests := []struct {
		name string
		run  func(op *OperatorImpl) error
	}{
		{"include", func(op *OperatorImpl) error {
			return op.AddInclude(context.Background(), &AddIncludeRequest{Name: "inc", Source: source})
		}},
		{"skill", func(op *OperatorImpl) error {
			return op.InstallSkill(context.Background(), &InstallSkillRequest{Name: "sk", Source: source})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)

			err = tt.run(op.Local())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "embeds a credential")
			assertNoSecretOnDisk(t, dir, secret)
		})
	}
}

// assertSourceOutcome asserts the refusal (with a hint naming the environment
// variable) or success, and that the credential is never written under dir.
func assertSourceOutcome(t *testing.T, dir string, err error, wantErr bool, secret string) {
	t.Helper()
	if wantErr {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embeds a credential")
		assert.NotContains(t, err.Error(), secret, "the refusal echoes the credential")
		oopsErr, ok := oops.AsOops(err)
		require.True(t, ok, "the refusal is an oops error")
		assert.Contains(t, oopsErr.Hint(), "AI_RULEZ_GIT_TOKEN")
		assert.Contains(t, oopsErr.Hint(), "git credential helper")
	} else {
		require.NoError(t, err)
	}
	assertNoSecretOnDisk(t, dir, secret)
}

// assertNoSecretOnDisk fails when secret appears in any file the operator wrote.
func assertNoSecretOnDisk(t *testing.T, dir, secret string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // a path the test just created
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(data), secret) {
			t.Errorf("credential written to %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}
