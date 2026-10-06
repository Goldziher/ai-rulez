package includes

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func authValue(env []string, prefix string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, prefix); ok {
			return v
		}
	}
	return ""
}

func TestWithAuth(t *testing.T) {
	tests := []struct {
		name     string
		hosts    string
		url      string
		token    string
		wantSent bool
		wantOrig string
	}{
		{"github by default", "", "https://github.com/o/r", "tok", true, "https://github.com"},
		{"attacker host withheld", "", "https://attacker.example/x", "tok", false, ""},
		{"lookalike subdomain withheld", "", "https://github.com.evil.example/x", "tok", false, ""},
		{"userinfo trick withheld", "", "https://github.com@attacker.example/x", "tok", false, ""},
		{"allowlisted host", "gitlab.example, github.com", "https://gitlab.example/o/r", "tok", true, "https://gitlab.example"},
		{"allowlist replaces default", "gitlab.example", "https://github.com/o/r", "tok", false, ""},
		{"plain http never", "attacker.example", "http://attacker.example/x", "tok", false, ""},
		{"ssh never", "", "git@github.com:o/r.git", "tok", false, ""},
		{"empty token", "", "https://github.com/o/r", "", false, ""},
		{"git+https accepted", "", "git+https://github.com/o/r", "tok", true, "https://github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv(TokenHostsEnv, tt.hosts)
			base := []string{"A=b"}

			// Act
			env := withAuth(base, tt.url, tt.token)

			// Assert
			if !tt.wantSent {
				assert.Equal(t, base, env)
				return
			}
			assert.Equal(t, "http."+tt.wantOrig+"/.extraHeader", authValue(env, "GIT_CONFIG_KEY_0="))
			want := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("tok:x-oauth-basic"))
			assert.Equal(t, want, authValue(env, "GIT_CONFIG_VALUE_0="))
			assert.Equal(t, "1", authValue(env, "GIT_CONFIG_COUNT="))
		})
	}
}

func TestWithAuthAppendsToExistingGitConfigEnv(t *testing.T) {
	t.Setenv(TokenHostsEnv, "")
	env := withAuth([]string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=1"}, "https://github.com/o/r", "tok")

	assert.Equal(t, "3", authValue(env, "GIT_CONFIG_COUNT="))
	assert.Equal(t, "http.https://github.com/.extraHeader", authValue(env, "GIT_CONFIG_KEY_2="))
}

func TestCloneDoesNotLeakTokenIntoRepoConfig(t *testing.T) {
	// Arrange: a local source repository; a token is passed even though file://
	// never receives it.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@x", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "i"}} {
		out, err := gitutil.CommandNoContext(src, args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	dest := filepath.Join(t.TempDir(), "clone")

	// Act
	err := sparseClone(t.Context(), "file://"+src, "", "", dest, "SECRET-TOKEN")

	// Assert
	require.NoError(t, err)
	cfg, err := os.ReadFile(filepath.Join(dest, ".git", "config"))
	require.NoError(t, err)
	assert.NotContains(t, string(cfg), "SECRET-TOKEN")
}

func TestScrubLegacyCredentials(t *testing.T) {
	// Arrange: a cache whose origin embeds a token, as older versions left it.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://SECRET:x-oauth-basic@github.com/o/r"}} {
		out, err := gitutil.CommandNoContext(dir, args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	// Act
	scrubLegacyCredentials(t.Context(), dir, "https://github.com/o/r")

	// Assert
	cfg, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	require.NoError(t, err)
	assert.NotContains(t, string(cfg), "SECRET")
	assert.Contains(t, string(cfg), "https://github.com/o/r")
}
