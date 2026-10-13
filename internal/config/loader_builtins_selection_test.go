package config

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const selectionHeader = `version = "5.0"
name = "t"
presets = ["claude"]
`

// loadSelectionConfig writes body under the shared header and loads it.
func loadSelectionConfig(t *testing.T, body string) *Config {
	t.Helper()
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, aiRulezDirName)
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename), []byte(selectionHeader+body), 0o644))
	cfg, err := LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	return cfg
}

// domainItemNames lists every content entry name of a loaded domain, sorted.
func domainItemNames(cfg *Config, domain string) []string {
	d, ok := cfg.Content.Domains[domain]
	if !ok {
		return nil
	}
	var names []string
	for _, group := range [][]ContentFile{d.Rules, d.Context, d.Skills, d.Agents, d.Commands} {
		for i := range group {
			names = append(names, group[i].Name)
		}
	}
	sort.Strings(names)
	return names
}

const allAutoExcluded = `"!ai-governance","!agent-delegation","!security","!git-workflow","!code-quality","!testing","!token-efficiency"`

// TestLoadConfig_BuiltinItemSelection covers issue #399: one row per repro case.
func TestLoadConfig_BuiltinItemSelection(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantItems    map[string][]string // exact items per domain
		wantWhole    []string            // domains loaded with more than one item
		wantAbsent   []string            // domains that must not be loaded
		wantScoped   map[string]bool
		wantValidate string // substring of the Validate error, "" for none
	}{
		{
			name:       "1 positive item loads only that item",
			body:       "builtins = [" + allAutoExcluded + `,"docker/container-standards"]`,
			wantItems:  map[string][]string{"docker": {"container-standards"}},
			wantAbsent: []string{"security", "git-workflow", "testing"},
		},
		{
			name:      "2a item of an auto-included pack keeps the whole pack",
			body:      `builtins = ["security/secrets-handling"]`,
			wantWhole: []string{"security"},
		},
		{
			name:      "2b item then !pack keeps only the item",
			body:      `builtins = ["security/secrets-handling", "!security"]`,
			wantItems: map[string][]string{"security": {"secrets-handling"}},
		},
		{
			name:      "2c !pack then item keeps only the item",
			body:      `builtins = ["!security", "security/secrets-handling"]`,
			wantItems: map[string][]string{"security": {"secrets-handling"}},
		},
		{
			name:         "3 unknown positive entry fails validation naming the choices",
			body:         `builtins = ["nope"]`,
			wantValidate: "docker",
		},
		{
			name:         "3 typo in an item names the pack's items",
			body:         `builtins = ["docker/container-standard"]`,
			wantValidate: "docker/container-standards",
		},
		{
			name:       "1b positive item of a multi-item pack loads only that item",
			body:       "builtins = [" + allAutoExcluded + `,"cicd/pipeline-standards"]`,
			wantItems:  map[string][]string{"cicd": {"pipeline-standards"}},
			wantAbsent: []string{"security"},
		},
		{
			name: "4 root !domain/name is honored for a builtin:-scoped pack",
			body: `builtins = ["!cicd/github-workflow"]

[profiles]
default = ["builtin:cicd"]
`,
			wantItems:  map[string][]string{"cicd": {"devops-engineer", "pipeline-standards", "release-engineer"}},
			wantScoped: map[string]bool{"cicd": true},
		},
		{
			name: "5 builtin:domain/name loads only that item, scoped",
			body: `builtins = ["!security"]

[profiles]
default = ["builtin:docker/container-standards", "builtin:cicd/pipeline-standards"]
`,
			wantItems: map[string][]string{
				"docker": {"container-standards"},
				"cicd":   {"pipeline-standards"},
			},
			wantScoped: map[string]bool{"docker": true, "cicd": true},
		},
		{
			name: "profile ref to the pack wins over an item ref",
			body: `[profiles]
default = ["builtin:cicd/pipeline-standards", "builtin:cicd"]
`,
			wantWhole: []string{"cicd"},
		},
		{
			name: "global item selection wins over a profile ref for the same pack",
			body: `builtins = ["cicd/pipeline-standards"]

[profiles]
default = ["builtin:cicd"]
`,
			wantItems:  map[string][]string{"cicd": {"pipeline-standards"}},
			wantScoped: map[string]bool{"cicd": false},
		},
		{
			name:       "!domain/name beats a positive item",
			body:       `builtins = ["docker/container-standards", "!docker/container-standards"]`,
			wantAbsent: []string{"docker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadSelectionConfig(t, tt.body)

			for domain, want := range tt.wantItems {
				assert.Equal(t, want, domainItemNames(cfg, domain), domain)
			}
			for _, domain := range tt.wantWhole {
				assert.Greater(t, len(domainItemNames(cfg, domain)), 1, domain)
			}
			for _, domain := range tt.wantAbsent {
				assert.NotContains(t, cfg.Content.Domains, domain)
			}
			for domain, scoped := range tt.wantScoped {
				require.Contains(t, cfg.Content.Domains, domain)
				assert.Equal(t, scoped, cfg.Content.Domains[domain].BuiltinScoped, domain)
			}

			err := cfg.Validate()
			if tt.wantValidate != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantValidate)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidate_BuiltinProfileRefs(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{"pack", "builtin:docker", false},
		{"pack item", "builtin:docker/container-standards", false},
		{"unknown item", "builtin:docker/nope", true},
		{"unknown pack", "builtin:nope", true},
		{"extra segments", "builtin:docker/a/b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadSelectionConfig(t, "[profiles]\ndefault = [\""+tt.ref+"\"]\n")
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
