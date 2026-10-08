package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePluginAuthoringHermesDependencies(t *testing.T) {
	tests := []struct {
		name         string
		dependencies []string
		wantErr      bool
	}{
		{name: "omitted"},
		{name: "empty list", dependencies: []string{}},
		{name: "named", dependencies: []string{"example-tool"}},
		{name: "extras and versions", dependencies: []string{"example-tool[cli]>=0.1,<1"}},
		{name: "marker", dependencies: []string{`httpx>=0.28; python_version >= "3.11"`}},
		{name: "direct reference", dependencies: []string{"demo @ https://example.com/demo.whl"}},
		{name: "empty requirement", dependencies: []string{""}, wantErr: true},
		{name: "whitespace", dependencies: []string{" \t "}, wantErr: true},
		{name: "bad name", dependencies: []string{"not a package"}, wantErr: true},
		{name: "bad extras", dependencies: []string{"demo[extra"}, wantErr: true},
		{name: "bad specifier", dependencies: []string{"demo>="}, wantErr: true},
		{name: "bad version", dependencies: []string{"demo>=not-a-version"}, wantErr: true},
		{name: "bad marker", dependencies: []string{"demo; python_version >="}, wantErr: true},
		{name: "unknown marker", dependencies: []string{`demo; unknown_variable == "x"`}, wantErr: true},
		{name: "second invalid", dependencies: []string{"demo", ""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Plugin: &PluginAuthoring{
				Name: "example", Version: "1.2.3", Description: "Example plugin.",
				Runtimes: []string{PluginRuntimeHermes},
				Hermes:   &HermesExtras{Dependencies: tt.dependencies},
			}}
			err := cfg.validatePluginAuthoring()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid Hermes dependency")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLoadConfigTOMLHermesDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `version = "5.0"
name = "example"
[plugin]
name = "example"
version = "1.2.3"
description = "Example plugin."
runtimes = ["hermes"]
[plugin.hermes]
dependencies = ["example-tool[cli]>=0.1,<1", "httpx>=0.28; python_version >= '3.11'"]
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	cfg, err := loadConfigTOML(osView(dir), path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Plugin.Hermes)
	assert.Equal(t, []string{"example-tool[cli]>=0.1,<1", "httpx>=0.28; python_version >= '3.11'"},
		cfg.Plugin.Hermes.Dependencies)
}
