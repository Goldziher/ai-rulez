package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// TestValidateStrictReportsACredentialedSourceAR035 checks that a credential
// already committed to config.toml is an error-level finding in a strict run,
// with the URL redacted.
func TestValidateStrictReportsACredentialedSourceAR035(t *testing.T) {
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	root := t.TempDir()
	body := "version = \"5.0\"\nname = \"p\"\ngitignore = false\nagents_md = false\npresets = [\"claude\"]\n" +
		"\n[[includes]]\nname = \"shared\"\nsource = \"https://user:" + secret + "@github.com/o/r.git\"\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte(body), 0o644))
	// WithoutRemote keeps the declared include unresolved, so no fetch is made.
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)

	report, err := strictLint(context.Background(), cfg)
	require.NoError(t, err)

	var got []lint.Finding
	for _, f := range report.Findings {
		if f.Code == lint.CodeCredentialedSource {
			got = append(got, f)
		}
	}
	require.Len(t, got, 1, "want one AR035")
	assert.Equal(t, lint.SeverityError, got[0].Severity)
	assert.NotContains(t, got[0].Message, secret, "the finding echoes the credential")
}
