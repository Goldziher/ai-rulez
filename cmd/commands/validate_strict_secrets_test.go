package commands

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictLint_SecretReferencesSurviveTheDeliveryRender(t *testing.T) {
	// Arrange: the delivery check renders the presets, which resolves ${VAR} in
	// the servers it holds; the secret scan must still see the references.
	t.Setenv("GITHUB_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz0123456789")
	t.Setenv("API_KEY", "abcdef1234567890abcdef")
	cfg := deliveryProject(t, `["claude"]`, `
[[mcp_servers]]
name = "gh"
command = "node"
args = ["s.js"]
env = { GITHUB_TOKEN = "${GITHUB_TOKEN}" }

[[mcp_servers]]
name = "web"
transport = "http"
url = "https://x.test/mcp"
headers = { Authorization = "Bearer ${API_KEY}" }
`, servedSkillFiles)
	_ = deliveryFindings(cfg)

	// Act
	report, err := strictLint(cfg)

	// Assert
	require.NoError(t, err)
	for _, f := range report.Findings {
		assert.NotEqual(t, lint.CodeSecretInConfig, f.Code, "reference reported as a literal credential: %s", f.Message)
	}
}

func TestStrictLint_UnscannableAuthoredSkillMDIsAnError(t *testing.T) {
	// Arrange
	cfg := deliveryProject(t, `["claude"]`,
		"\n[[mcp_servers]]\nname = \"skills\"\ncommand = \"ai-rulez\"\nargs = [\"mcp\", \"--serve-skills\"]\n",
		map[string]string{"skills/nulmd/SKILL.md": "---\ndescription: NUL in SKILL.md\ndelivery: served\n---\nBody\x00\n"})

	// Act
	report, err := strictLint(cfg)

	// Assert
	require.NoError(t, err)
	var got []lint.Finding
	for _, f := range report.Findings {
		if f.Code == lint.CodeServedUnscannable {
			got = append(got, f)
		}
	}
	require.Len(t, got, 1)
	assert.Equal(t, lint.SeverityError, got[0].Severity)
}
