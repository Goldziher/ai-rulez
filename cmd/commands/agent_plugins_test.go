package commands

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
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
