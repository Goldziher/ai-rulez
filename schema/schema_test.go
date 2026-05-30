package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/schema"
)

func TestSchemaURL(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		file     string
		expected string
	}{
		{
			name:     "dev version uses main branch",
			version:  "dev",
			file:     schema.ConfigSchemaFile,
			expected: "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json",
		},
		{
			name:     "release version uses tag",
			version:  "3.14.2",
			file:     schema.ConfigSchemaFile,
			expected: "https://raw.githubusercontent.com/Goldziher/ai-rulez/v3.14.2/schema/ai-rules.schema.json",
		},
		{
			name:     "mcp schema file",
			version:  "dev",
			file:     schema.MCPSchemaFile,
			expected: "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-mcp.schema.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := schema.Version
			defer func() { schema.Version = original }()

			schema.Version = tt.version
			assert.Equal(t, tt.expected, schema.SchemaURL(tt.file))
		})
	}
}

func TestValidateWithSchema(t *testing.T) {
	t.Run("valid minimal config", func(t *testing.T) {
		cfg := `
version: "3.0"
name: "test-project"
presets:
  - claude
`
		err := schema.ValidateWithSchema([]byte(cfg))
		require.NoError(t, err)
	})

	t.Run("missing name fails", func(t *testing.T) {
		cfg := `
version: "3.0"
presets:
  - claude
`
		err := schema.ValidateWithSchema([]byte(cfg))
		assert.Error(t, err)
	})

	t.Run("missing version fails", func(t *testing.T) {
		cfg := `
name: "test-project"
presets:
  - claude
`
		err := schema.ValidateWithSchema([]byte(cfg))
		assert.Error(t, err)
	})

	t.Run("valid config with inline mcp servers", func(t *testing.T) {
		cfg := `
version: "4.0"
name: "test-project"
presets:
  - claude
mcp_servers:
  - name: grafana
    description: Grafana observability MCP
    command: uvx
    args:
      - mcp-grafana
    env:
      GRAFANA_URL: http://localhost:3000
      GRAFANA_SERVICE_ACCOUNT_TOKEN: ${GRAFANA_SERVICE_ACCOUNT_TOKEN}
    transport: stdio
    enabled: true
  - name: remote-docs
    transport: http
    url: https://mcp.example.com
    enabled: false
`
		err := schema.ValidateWithSchema([]byte(cfg))
		require.NoError(t, err)
	})
}
