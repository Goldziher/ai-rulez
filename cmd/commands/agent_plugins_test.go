package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
)

const agentPluginsBaseConfig = `version = "5.0"
name = "acme"
presets = ["claude"]

[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["agent-plugins"]

[plugin.author]
name = "Jane"
`

const agentPluginsProjectConfig = agentPluginsBaseConfig + `
[[plugin.mcp]]
name = "keyed"
command = "acme-server"
args = ["--key", "${API_KEY}"]
`

func strictFindings(t *testing.T) []lint.Finding {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
	require.NoError(t, err)
	report, err := strictLint(t.Context(), cfg)
	require.NoError(t, err)
	return report.Findings
}

func TestStrictValidateReportsAgentPluginsPlaceholders(t *testing.T) {
	// Arrange: a plugin MCP server whose args use a ${VAR} Agent Plugins never expands.
	publishProjectWith(t, agentPluginsProjectConfig)

	// Act
	findings := strictFindings(t)

	// Assert
	var got *lint.Finding
	for i := range findings {
		if findings[i].Code == lint.CodeAgentPluginPlaceholder {
			got = &findings[i]
		}
	}
	require.NotNil(t, got, "%v", findings)
	assert.Equal(t, lint.SeverityError, got.Severity)
	assert.Contains(t, got.Message, "${API_KEY}")
	assert.Equal(t, "plugin.json", filepath.Base(got.File))
}

func TestStrictValidateIsQuietForAValidAgentPluginsPackage(t *testing.T) {
	publishProjectWith(t, agentPluginsBaseConfig)
	for _, f := range strictFindings(t) {
		assert.NotContains(t, f.Code, "AR9O", "%v", f)
	}
}

var agentPluginsPublishConfig = strings.Replace(agentPluginsBaseConfig, "runtimes = [\"agent-plugins\"]\n",
	"runtimes = [\"agent-plugins\"]\nspec = \"1.1.0\"\n", 1) + `
[[plugin.mcp]]
name = "docs"
transport = "http"
url = "https://docs.acme.test/mcp"
`

func TestPublish_EmitsAValidatedAgentPluginsDirectory(t *testing.T) {
	// Arrange
	root := publishProjectWith(t, agentPluginsPublishConfig)
	publishEmit = []string{"agent-plugins"}

	// Act
	_, err := runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	assert.Contains(t, dist["emit/agent-plugins/acme/plugin.json"], "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json")
	assert.Contains(t, dist["emit/agent-plugins/acme/mcp.json"], "https://agent-plugins.org/schemas/1.1.0/mcp.schema.json")
	assert.Contains(t, dist, "emit/agent-plugins/acme/skills/deploy/SKILL.md")
	assert.NotContains(t, dist, "emit/agent-plugins/acme/.ai-rulez-generated.json")
	var verifyOut bytes.Buffer
	_, _ = capture(t, func() { err = runPublishVerify(context.Background(), &verifyOut, filepath.Join(root, "dist")) })
	require.NoError(t, err)
}

func TestPublish_RefusesAnAgentPluginsPackageClientsWouldSkip(t *testing.T) {
	// Arrange: the server's placeholder is never expanded, so generate leaves the
	// server out and the strict gate names it.
	publishProjectWith(t, agentPluginsProjectConfig)
	publishDryRun = true

	// Act
	_, err := runPublishCapture(t)

	// Assert
	requirePublishError(t, err, publish.CodePreflight, publish.ExitGate)
}

func TestVerifyPluginCoversTheAgentPluginsPackage(t *testing.T) {
	root := publishProjectWith(t, agentPluginsPublishConfig)
	cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).VerifyPlugin(""))

	// A hand edit of mcp.json is drift: the content hash of the sidecar covers it.
	writeFile(t, filepath.Join(root, "mcp.json"), "{}\n")

	err = generator.NewGenerator(cfg).VerifyPlugin("")
	require.ErrorIs(t, err, generator.ErrPluginDrift)
}
