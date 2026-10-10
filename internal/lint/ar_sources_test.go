package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TestCredentialInSourceURLAR035 flags a source already committed to config.toml
// that carries a credential in its URL, and stays quiet for the URL forms an SSH
// remote uses (which carry a login name, not a secret).
func TestCredentialInSourceURLAR035(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
		secret string // must never appear in the finding's message
	}{
		{
			name:   "include with user and password",
			config: "\n[[includes]]\nname = \"shared\"\nsource = \"https://user:ghp_abcdefghijklmnopqrstuvwxyz0123456789@github.com/o/r.git\"\n",
			want:   true,
			secret: "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		},
		{
			name:   "installed skill with a token as the user",
			config: "\n[[installed_skills]]\nname = \"sk\"\nsource = \"https://ghp_abcdefghijklmnopqrstuvwxyz0123456789@github.com/o/r.git\"\n",
			want:   true,
			secret: "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		},
		{
			name:   "skill source with a credential",
			config: "\n[[skill_sources]]\nname = \"vendor\"\nurl = \"git+https://user:token@git.example.com/o/r\"\n",
			want:   true,
			secret: "token",
		},
		{
			name:   "plain https include",
			config: "\n[[includes]]\nname = \"shared\"\nsource = \"https://github.com/o/r.git\"\n",
			want:   false,
		},
		{
			name:   "scp-style ssh remote",
			config: "\n[[installed_skills]]\nname = \"sk\"\nsource = \"git@github.com:o/r.git\"\n",
			want:   false,
		},
		{
			name:   "ssh protocol remote",
			config: "\n[[includes]]\nname = \"shared\"\nsource = \"ssh://git@github.com/o/r.git\"\n",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{".ai-rulez/config.toml": baseConfig + tt.config})
			gitAdd(t, dir)
			// WithoutRemote loads the declared sources without fetching them.
			cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutRemote())
			require.NoError(t, err)
			tree, err := LoadTree(dir)
			require.NoError(t, err)

			// Act
			report, err := Run(cfg, tree)
			require.NoError(t, err)

			// Assert
			var got []Finding
			for _, f := range report.Findings {
				if f.Code == CodeCredentialedSource {
					got = append(got, f)
				}
			}
			if !tt.want {
				assert.Empty(t, got, "unexpected AR035\n%s", dump(report.Findings))
				return
			}
			require.Len(t, got, 1, "want one AR035\n%s", dump(report.Findings))
			assert.Equal(t, SeverityError, got[0].Severity)
			assert.True(t, strings.HasSuffix(got[0].File, "config.toml"), "file = %q", got[0].File)
			assert.Contains(t, got[0].Message, "credential")
			assert.Contains(t, got[0].Message, "AI_RULEZ_GIT_TOKEN")
			if tt.secret != "" {
				assert.NotContains(t, got[0].Message, tt.secret, "the finding echoes the credential")
			}
		})
	}
}
