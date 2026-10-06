package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verifiersHead = "version = \"4.0\"\nname = \"proj\"\npresets = [\"claude\"]\n"

func TestVerifiers_TOMLRoundTripAndLoad(t *testing.T) {
	src := verifiersHead + `
[[verifiers]]
name = "readme"
description = "A README exists"
type = "file_exists"
path = "README.md"

[[verifiers]]
name = "few-todos"
type = "glob_count"
glob = "**/*.go"
exclude = ["vendor/**"]
min = 1
max = 500
severity = "warning"

[[verifiers]]
name = "go-version"
type = "key_equals"
path = "package.json"
key = "engines.node"
equals = ">=20"
`
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	path := filepath.Join(dir, ".ai-rulez", "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	cfg, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, cfg.Verifiers, 3)
	assert.Equal(t, "file_exists", cfg.Verifiers[0].Type)
	assert.Equal(t, "README.md", cfg.Verifiers[0].Path)
	assert.Equal(t, "warning", cfg.Verifiers[1].Severity)
	require.NotNil(t, cfg.Verifiers[1].Max)
	assert.Equal(t, 500, *cfg.Verifiers[1].Max)
	require.NotNil(t, cfg.Verifiers[2].Equals)
	assert.Equal(t, ">=20", *cfg.Verifiers[2].Equals)

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
	again, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, cfg.Verifiers, again.Verifiers, "[[verifiers]] must survive a config rewrite")
}

func TestLoadConfig_LocalOverlayMergesVerifiersByName(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML+"\n[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"A\"\n\n[[verifiers]]\nname = \"b\"\ntype = \"file_exists\"\npath = \"B\"\n")
	writeProjectFile(t, dir, "config.local.toml", "[[verifiers]]\nname = \"b\"\ntype = \"file_absent\"\npath = \"B\"\n\n[[verifiers]]\nname = \"c\"\ntype = \"file_exists\"\npath = \"C\"\n")

	cfg, err := LoadConfig(context.Background(), base)

	require.NoError(t, err)
	require.Len(t, cfg.Verifiers, 3)
	byName := map[string]string{}
	for _, v := range cfg.Verifiers {
		byName[v.Name] = v.Type
	}
	assert.Equal(t, map[string]string{"a": "file_exists", "b": "file_absent", "c": "file_exists"}, byName)
}

func TestValidateVerifiers(t *testing.T) {
	two := 2
	one := 1
	val := "x"
	neg := -1
	tests := []struct {
		name    string
		v       []VerifierConfig
		wantErr string
	}{
		{"valid file_exists", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "README.md"}}, ""},
		{"valid regex", []VerifierConfig{{Name: "a", Type: "regex", Glob: "**/*.go", Pattern: `^package `}}, ""},
		{"valid key_equals", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Key: "a.b", Equals: &val}}, ""},
		{"valid generated_in_sync", []VerifierConfig{{Name: "a", Type: "generated_in_sync"}}, ""},
		{"missing name", []VerifierConfig{{Type: "file_exists", Path: "x"}}, "name"},
		{"bad name", []VerifierConfig{{Name: "a b", Type: "file_exists", Path: "x"}}, "name"},
		{"duplicate name", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "x"}, {Name: "a", Type: "file_exists", Path: "y"}}, "duplicate"},
		{"unknown type", []VerifierConfig{{Name: "a", Type: "bogus"}}, "type"},
		{"command reserved", []VerifierConfig{{Name: "a", Type: "command"}}, "type"},
		{"bad severity", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "x", Severity: "loud"}}, "severity"},
		{"missing path", []VerifierConfig{{Name: "a", Type: "file_absent"}}, "path"},
		{"absolute path", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "/etc/passwd"}}, "path"},
		{"escaping path", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "../x"}}, "path"},
		{"glob_count without bounds", []VerifierConfig{{Name: "a", Type: "glob_count", Glob: "*"}}, "min or max"},
		{"glob_count min over max", []VerifierConfig{{Name: "a", Type: "glob_count", Glob: "*", Min: &two, Max: &one}}, "min"},
		{"regex missing pattern", []VerifierConfig{{Name: "a", Type: "regex", Glob: "*"}}, "pattern"},
		{"regex invalid pattern", []VerifierConfig{{Name: "a", Type: "forbid", Glob: "*", Pattern: "("}}, "pattern"},
		{"regex missing glob", []VerifierConfig{{Name: "a", Type: "forbid", Pattern: "x"}}, "glob"},
		{"key_equals missing key", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Equals: &val}}, "key"},
		{"glob with exponential braces", []VerifierConfig{{Name: "a", Type: "glob_count", Glob: strings.Repeat("{a,b}", 30), Min: &one}}, "alternatives"},
		{"exclude with exponential braces", []VerifierConfig{{Name: "a", Type: "forbid", Glob: "*", Pattern: "x", Exclude: []string{strings.Repeat("{a,b}", 30)}}}, "exclude"},
		{"negative min", []VerifierConfig{{Name: "a", Type: "glob_count", Glob: "*", Min: &neg}}, "negative"},
		{"key_equals unsupported extension", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "README.md", Key: "k", Equals: &val}}, "unsupported format"},
		{"key_equals bad key", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Key: "a..b", Equals: &val}}, "key"},
		{"key_equals escaped key", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Key: `dependencies.lodash\.merge`, Equals: &val}}, ""},
		{"inapplicable field on file_exists", []VerifierConfig{{Name: "a", Type: "file_exists", Path: "x", Pattern: "y"}}, "does not apply"},
		{"inapplicable glob on key_equals", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Key: "k", Equals: &val, Glob: "*"}}, "glob"},
		{"inapplicable path on glob_count", []VerifierConfig{{Name: "a", Type: "glob_count", Glob: "*", Min: &one, Path: "x"}}, "path"},
		{"key_equals missing equals", []VerifierConfig{{Name: "a", Type: "key_equals", Path: "p.json", Key: "k"}}, "equals"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Verifiers: tt.v}

			err := cfg.validateVerifiers()

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateVerifiers_GeneratedInSyncProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantErr bool
	}{
		{"none", "", false},
		{"defined", "backend", false},
		{"implicit default", "default", false},
		{"composed", "backend,default", false},
		{"unknown", "nope", true},
		{"unknown in composed", "backend,nope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Profiles:  map[string][]string{"backend": {"b"}},
				Verifiers: []VerifierConfig{{Name: "s", Type: "generated_in_sync", Profile: tt.profile}},
			}

			err := cfg.validateVerifiers()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "profile")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestVerifiers_SpecFormInline(t *testing.T) {
	const spec = verifiersHead + `
[[verifiers]]
name = "no-todo"
rule = "style"
severity = "warning"
fix = "Remove the TODO."
when_changed = ["src/**/*.go"]
exclude = ["src/gen/**"]

[verifiers.require.all]
[[verifiers.require.all]]
[verifiers.require.all.forbid]
regex = "TODO"
`
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"spec form loads", strings.Replace(spec, "[verifiers.require.all]\n[[verifiers.require.all]]", "[[verifiers.require.all]]", 1), ""},
		{"flat form still loads", verifiersHead + "\n[[verifiers]]\nname = \"r\"\ntype = \"file_exists\"\npath = \"README.md\"\n", ""},
		{"no type and no target", verifiersHead + "\n[[verifiers]]\nname = \"r\"\n[verifiers.require.regex]\nregex = \"x\"\n", "exactly one of rule, skill, agent or command"},
		{"no predicate", verifiersHead + "\n[[verifiers]]\nname = \"r\"\nrule = \"style\"\n", "needs a require predicate"},
		{"flat field without type", verifiersHead + "\n[[verifiers]]\nname = \"r\"\nrule = \"style\"\npath = \"x\"\n[verifiers.require.regex]\nregex = \"x\"\n", "does not apply without a type"},
		{"spec field with type", verifiersHead + "\n[[verifiers]]\nname = \"r\"\ntype = \"file_exists\"\npath = \"x\"\nfix = \"y\"\n", "fix, which does not apply to type"},
		{"bad glob", verifiersHead + "\n[[verifiers]]\nname = \"r\"\nrule = \"s\"\nwhen_changed = [\"{a,b}{c,d}{e,f}{g,h}{i,j}{k,l}{m,n}\"]\n[verifiers.require.regex]\nregex = \"x\"\n", "invalid verifier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			path := filepath.Join(dir, ".ai-rulez", "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))

			// Act
			cfg, err := LoadConfigFromFile(context.Background(), path)
			if err == nil {
				err = cfg.validateVerifiers()
			}

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if strings.Contains(tt.body, "no-todo") {
				v := cfg.Verifiers[0]
				assert.True(t, v.IsSpec())
				assert.Equal(t, "style", v.Rule)
				assert.Equal(t, []string{"src/**/*.go"}, v.WhenChanged)
				require.NotNil(t, v.Require)
				require.Len(t, v.Require.All, 1)
				require.NotNil(t, v.Require.All[0].Forbid)
				out, err := MarshalTOML(cfg)
				require.NoError(t, err)
				assert.NotContains(t, string(out), `type = ""`)
				assert.Contains(t, string(out), "forbid")
			}
		})
	}
}
