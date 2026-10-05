package schema_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/schema"
)

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

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

func TestValidateWithSchema_Rules(t *testing.T) {
	const head = "version: \"4.0\"\nname: \"test-project\"\npresets:\n  - claude\n"

	t.Run("valid rules mode and mode_by_preset", func(t *testing.T) {
		cfg := head + "rules:\n  mode: split\n  mode_by_preset:\n    cursor: inline\n    my-custom: split\n"
		require.NoError(t, schema.ValidateWithSchema([]byte(cfg)))
	})

	t.Run("baz preset and baz_scoped", func(t *testing.T) {
		require.NoError(t, schema.ValidateWithSchema([]byte(head+"  - baz\nrules:\n  baz_scoped: root\n")))
		assert.Error(t, schema.ValidateWithSchema([]byte(head+"rules:\n  baz_scoped: deep\n")))
	})

	t.Run("invalid mode fails", func(t *testing.T) {
		assert.Error(t, schema.ValidateWithSchema([]byte(head+"rules:\n  mode: both\n")))
	})

	t.Run("invalid mode_by_preset value fails", func(t *testing.T) {
		assert.Error(t, schema.ValidateWithSchema([]byte(head+"rules:\n  mode_by_preset:\n    cursor: both\n")))
	})

	t.Run("unknown rules key fails", func(t *testing.T) {
		assert.Error(t, schema.ValidateWithSchema([]byte(head+"rules:\n  bogus: true\n")))
	})
}

func TestValidateFile_TOML(t *testing.T) {
	t.Run("consumer plugins and marketplaces validate", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]
schema = "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json"

[[plugins]]
marketplace = "official"
name = "my-plugin"
scope = "project"
enabled = true

[[marketplaces]]
name = "official"
source = "https://github.com/org/marketplace"
type = "github"
`)
		require.NoError(t, schema.ValidateFile(path))
	})

	t.Run("mcp self_server options validate", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[mcp]
self_server = true
self_server_version = "4.19.0"
`)
		require.NoError(t, schema.ValidateFile(path))
	})

	t.Run("mcp server headers validate", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${API_TOKEN}", X-Team = "core" }
`)
		require.NoError(t, schema.ValidateFile(path))
	})

	t.Run("invalid mcp header name is rejected", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { "Bad Header" = "v" }
`)
		assert.Error(t, schema.ValidateFile(path))
	})

	t.Run("unknown mcp key is rejected", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[mcp]
self_servers = true
`)
		assert.Error(t, schema.ValidateFile(path))
	})

	t.Run("unknown key is rejected", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]
presetz = ["claude"]
`)
		err := schema.ValidateFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("bad include content type is rejected", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[[includes]]
name = "shared"
source = "./shared"
include = ["rules", "mcp"]
`)
		assert.Error(t, schema.ValidateFile(path))
	})

	t.Run("valid include with commands and include-override", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]

[[includes]]
name = "shared"
source = "./shared"
include = ["rules", "commands"]
merge_strategy = "include-override"
`)
		require.NoError(t, schema.ValidateFile(path))
	})

	t.Run("all builtin names validate", func(t *testing.T) {
		path := writeTOML(t, `version = "4.0"
name = "x"
presets = ["claude"]
builtins = ["docker", "cicd", "observability", "polyglot-bindings", "vite-plus", "!ai-governance"]
`)
		require.NoError(t, schema.ValidateFile(path))
	})
}

func TestLintSection(t *testing.T) {
	const head = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"full section", head + "[lint]\nfail_on = \"warning\"\nignore = [\"AR401\"]\n[lint.severity]\nAR101 = \"off\"\n[lint.budgets.skill]\nmax_lines = 10\n[lint.description]\nmin_length = 5\n", false},
		{"bad fail_on", head + "[lint]\nfail_on = \"loud\"\n", true},
		{"unknown key", head + "[lint]\nbogus = true\n", true},
		{"bad severity", head + "[lint.severity]\nAR101 = \"loud\"\n", true},
		{"unknown budget key", head + "[lint.budgets.skill]\nmax_words = 3\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateFile(writeTOML(t, tt.body))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFile_LintSecurityAndFollowups(t *testing.T) {
	good := `version = "4.0"
name = "x"
presets = ["claude"]

[lint]
allow_overrides = ["dup", "backend/dup"]
allowed_keys = ["team"]

[lint.metadata.last_verified]
type = "date"
max_age_days = 90
required = true
kinds = ["skill"]

[lint.security]
scan_imports = "error"
allowed_hosts = ["github.com", "*.example.org"]
allowed_tools = ["Bash"]
secret_patterns = [{ name = "corp", regex = "corp_[a-z0-9]{10}" }]
injection_phrases = ["as root"]

[[lint.external]]
name = "scanner"
command = ["scan", "--sarif"]
format = "sarif"
`
	bad := map[string]string{
		"unknown security key":  "\n[lint.security]\nbogus = 1\n",
		"bad scan_imports":      "\n[lint.security]\nscan_imports = \"sometimes\"\n",
		"bad metadata type":     "\n[lint.metadata.a]\ntype = \"number\"\n",
		"external needs name":   "\n[[lint.external]]\ncommand = [\"x\"]\n",
		"pattern needs a regex": "\n[lint.security]\nsecret_patterns = [{ name = \"n\" }]\n",
	}
	header := "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "config.toml")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	require.NoError(t, schema.ValidateFile(write(good)))
	for name, extra := range bad {
		assert.Error(t, schema.ValidateFile(write(header+extra)), name)
	}
}

func TestVerifiersSection(t *testing.T) {
	const head = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", head + "[[verifiers]]\nname = \"a\"\ntype = \"glob_count\"\nglob = \"*.go\"\nmin = 1\nexclude = [\"vendor/**\"]\n", false},
		{"missing type", head + "[[verifiers]]\nname = \"a\"\n", true},
		{"unknown type", head + "[[verifiers]]\nname = \"a\"\ntype = \"command\"\n", true},
		{"unknown key", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\nbogus = 1\n", true},
		{"bad severity", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\nseverity = \"loud\"\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateFile(writeTOML(t, tt.body))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
