package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func TestGitIdentity_GivesGitAnIdentityWithoutAnyConfiguration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "author", key: "GIT_AUTHOR_IDENT", want: GitIdentityName + " <" + GitIdentityEmail + ">"},
		{name: "committer", key: "GIT_COMMITTER_IDENT", want: GitIdentityName + " <" + GitIdentityEmail + ">"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: no global or system config, and git may not guess from the host name
			// (a Linux CI runner has no identity to guess from).
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
			GitIdentity(t)

			// Act
			out, err := gitutil.CommandNoContext("", "-c", "user.useConfigOnly=true", "var", tc.key).CombinedOutput()

			// Assert
			if err != nil {
				t.Fatalf("git var %s: %v: %s", tc.key, err, out)
			}
			if got := string(out); !strings.HasPrefix(got, tc.want+" ") {
				t.Fatalf("git var %s = %q, want prefix %q", tc.key, got, tc.want)
			}
		})
	}
}
