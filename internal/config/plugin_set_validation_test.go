package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const setValidationHeader = `version = "5.0"
name = "proj"
presets = ["claude"]

[[mcp_servers]]
name = "moon-ear"
command = "moon-ear"
`

func TestValidate_PluginSet(t *testing.T) {
	tests := []struct {
		name    string
		tail    string
		files   map[string]string
		wantErr []string
	}{
		{
			name: "two plugins and two marketplaces are valid",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["a-*"]
mcp_servers = ["moon-ear"]

[[plugins_authoring]]
name = "b"
skills = ["b-*"]

[[marketplaces_authoring]]
name = "pub"
plugins = ["a", "b"]
output_dir = "out/pub"

[[marketplaces_authoring]]
name = "int"
plugins = ["b"]
output_dir = "out/int"
`,
		},
		{
			name: "plugin and plugins_authoring together point at the migration",
			tail: `
[plugin]
name = "one"
version = "1.0.0"
description = "d"

[[plugins_authoring]]
name = "a"
skills = ["a-*"]
`,
			wantErr: []string{"[plugin]", "[[plugins_authoring]]", "migrate"},
		},
		{
			name: "marketplace and marketplaces_authoring together",
			tail: `
[marketplace]
name = "m"
members = ["x"]

[[marketplaces_authoring]]
name = "pub"
plugins = ["a"]
`,
			wantErr: []string{"[marketplace]", "[[marketplaces_authoring]]"},
		},
		{
			name: "a plugin defined in marketplace.plugins and plugins_authoring",
			tail: `
[marketplace]
name = "m"
[[marketplace.plugins]]
name = "a"
skills = ["x"]

[[plugins_authoring]]
name = "a"
skills = ["y"]
`,
			wantErr: []string{`plugin "a"`, "[[marketplace.plugins]]", "[[plugins_authoring]]"},
		},
		{
			name: "duplicate marketplace name",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[marketplaces_authoring]]
name = "pub"
plugins = ["a"]
output_dir = "one"

[[marketplaces_authoring]]
name = "pub"
plugins = ["a"]
output_dir = "two"
`,
			wantErr: []string{`marketplace "pub"`, "#1", "#2"},
		},
		{
			name: "a marketplace lists a plugin no definition provides",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[marketplaces_authoring]]
name = "pub"
plugins = ["a", "ghost"]
`,
			wantErr: []string{`marketplace "pub" lists plugin "ghost"`, "a"},
		},
		{
			name: "a marketplace that lists nothing names every way to add a plugin",
			tail: `
[[marketplaces_authoring]]
name = "pub"
`,
			wantErr: []string{`marketplace "pub" lists no plugin`, "[[marketplace.plugins]]", "members", "from_domains", "plugins"},
		},
		{
			name: "a marketplace block with no plugin source",
			tail: `
[marketplace]
name = "m"
`,
			wantErr: []string{`marketplace "m" lists no plugin`, "[[plugins_authoring]]"},
		},
		{
			name: "a marketplace listing the same plugin twice",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[marketplaces_authoring]]
name = "pub"
plugins = ["a", "a"]
`,
			wantErr: []string{`marketplace "pub" lists plugin "a" twice`},
		},
		{
			name: "two marketplaces with one output_dir",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[marketplaces_authoring]]
name = "one"
plugins = ["a"]
output_dir = "out"

[[marketplaces_authoring]]
name = "two"
plugins = ["a"]
output_dir = "out/"
`,
			wantErr: []string{`marketplaces "one" and "two"`, "output_dir"},
		},
		{
			name: "an unsafe marketplace output_dir",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[marketplaces_authoring]]
name = "one"
plugins = ["a"]
output_dir = "../escape"
`,
			wantErr: []string{`marketplace "one"`, "output_dir"},
		},
		{
			name: "a plugin definition that selects nothing",
			tail: `
[[plugins_authoring]]
name = "a"
description = "empty"
`,
			wantErr: []string{`plugin "a" selects no content`},
		},
		{
			name: "a plugin definition bundling an undefined MCP server",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]
mcp_servers = ["ghost"]
`,
			wantErr: []string{`plugin "a" bundles MCP server "ghost"`, "moon-ear"},
		},
		{
			name: "an invalid plugin definition name",
			tail: `
[[plugins_authoring]]
name = "Bad Name"
skills = ["x"]
`,
			wantErr: []string{"Bad Name"},
		},
		{
			name: "a plugin definition with an invalid hook",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["x"]

[[plugins_authoring.hooks]]
event = "SessionStart"
[[plugins_authoring.hooks.hooks]]
type = "command"
`,
			wantErr: []string{`plugin "a" hook SessionStart[0] requires either 'command' or 'script'`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{"config.toml": setValidationHeader + tt.tail}
			for k, v := range tt.files {
				files[k] = v
			}
			root := definitionProject(t, files)
			cfg, err := LoadConfig(context.Background(), root)
			require.NoError(t, err)

			// Act
			err = cfg.Validate()

			// Assert
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, part := range tt.wantErr {
				assert.Contains(t, err.Error(), part)
			}
		})
	}
}

func TestSchemaFindings_UnknownKeyInADefinition(t *testing.T) {
	tests := []struct {
		name string
		tail string
		want string
	}{
		{name: "plugins_authoring", tail: "\n[[plugins_authoring]]\nname = \"a\"\nskills = [\"x\"]\nbogus = 1\n", want: "bogus"},
		{name: "marketplaces_authoring", tail: "\n[[marketplaces_authoring]]\nname = \"a\"\nplugins = [\"x\"]\nbogus = 1\n", want: "bogus"},
		{name: "marketplace.plugins", tail: "\n[marketplace]\nname = \"m\"\n[[marketplace.plugins]]\nname = \"a\"\nskills = [\"x\"]\nbogus = 1\n", want: "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := definitionProject(t, map[string]string{"config.toml": setValidationHeader + tt.tail})
			cfg, err := LoadConfig(context.Background(), root)
			require.NoError(t, err)

			// Act
			findings, err := SchemaFindings(cfg)

			// Assert
			require.NoError(t, err)
			require.NotEmpty(t, findings)
			assert.Contains(t, findings[0].String(), tt.want)
		})
	}
}
