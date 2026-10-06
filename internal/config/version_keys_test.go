package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsVersionSugar(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"^1.2", true},
		{"~2.1.0", true},
		{"1.x || 3", true},
		{">= 1.4.0", true},
		{"*", true},
		{"1.2.*", true},
		{"main", false},
		{"v1.2.3", false},
		{"1.2", false}, // legal branch name: stays a ref
		{">=1", false}, // legal branch name: stays a ref
		{"release/1.x", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			assert.Equal(t, tt.want, IsVersionSugar(tt.ref))
		})
	}
}

func TestVersionSpecAndRequestedRef(t *testing.T) {
	tests := []struct {
		name         string
		inc          IncludeConfig
		wantSpec     VersionSpec
		wantRequired string
	}{
		{"plain ref", IncludeConfig{Ref: "main"}, VersionSpec{}, "main"},
		{"no ref", IncludeConfig{}, VersionSpec{}, ""},
		{"version key", IncludeConfig{Version: " ^1.2 "}, VersionSpec{Constraint: "^1.2"}, "^1.2"},
		{"ref sugar", IncludeConfig{Ref: "~2.1.0"}, VersionSpec{Constraint: "~2.1.0"}, "~2.1.0"},
		{"refinements travel with the constraint", IncludeConfig{Version: "^1", TagPrefix: "deploy/v", IncludePrerelease: true}, VersionSpec{Constraint: "^1", TagPrefix: "deploy/v", IncludePrerelease: true}, "^1"},
		{"refinements without a constraint are inert", IncludeConfig{Ref: "main", TagPrefix: "x"}, VersionSpec{}, "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantSpec, tt.inc.VersionSpec())
			assert.Equal(t, tt.wantRequired, tt.inc.RequestedRef())
		})
	}
	sk := InstalledSkillConfig{Name: "d", Ref: "^1"}
	assert.Equal(t, "^1", sk.RequestedRef())
	src := SkillSourceConfig{Name: "s", Version: ">=1.4.0 <2.0.0"}
	assert.Equal(t, ">=1.4.0 <2.0.0", src.RequestedRef())
	assert.True(t, src.VersionSpec().Active())
}

func TestValidateVersionKeys(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		version string
		prefix  string
		pre     bool
		wantErr string
	}{
		{name: "plain ref"},
		{name: "plain ref with a SHA", ref: "0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e"},
		{name: "version", version: "^1.2"},
		{name: "ref sugar", ref: "^1.2"},
		{name: "range", version: ">=1.4.0 <2.0.0", prefix: "deploy/v", pre: true},
		{name: "both ref and version", ref: "main", version: "^1", wantErr: "AR731"},
		{name: "ref sugar and version", ref: "^1", version: "^1", wantErr: "cannot be combined"},
		{name: "unparsable version", version: "^^1", wantErr: "not a valid version constraint"},
		{name: "unparsable sugar", ref: "~~1", wantErr: "ref is not a valid version constraint"},
		{name: "tag_prefix needs a constraint", ref: "main", prefix: "v", wantErr: "tag_prefix needs version"},
		{name: "include_prerelease needs a constraint", pre: true, wantErr: "include_prerelease needs version"},
		{name: "prefix with whitespace", version: "^1", prefix: "a b", wantErr: "tag_prefix"},
		{name: "prefix that looks like an option", version: "^1", prefix: "--x", wantErr: "tag_prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVersionKeys("includes", "shared", tt.ref, tt.version, tt.prefix, tt.pre, "")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "AR731")
		})
	}
}

func TestConfigValidateVersionKeysCoversEverySourceKind(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"include", Config{Includes: []IncludeConfig{{Name: "a", Source: "https://x/y", Ref: "main", Version: "^1"}}}},
		{"installed skill", Config{InstalledSkills: []InstalledSkillConfig{{Name: "a", Source: "https://x/y", Version: "nope"}}}},
		{"skill source", Config{SkillSources: []SkillSourceConfig{{Name: "a", URL: "https://x/y", Ref: "v1", Version: "^1"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validateVersionKeys()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR731")
		})
	}
}

func TestInstalledSkillAcceptsRefSugar(t *testing.T) {
	// ValidateInstalledSkills refuses '^' and ' ' in a git ref; the shorthand for a constraint is exempt.
	err := ValidateInstalledSkills([]InstalledSkillConfig{{Name: "a", Source: "https://github.com/o/r", Ref: "^1.2"}})
	require.NoError(t, err)
	err = ValidateInstalledSkills([]InstalledSkillConfig{{Name: "a", Source: "https://github.com/o/r", Ref: "a b:c"}})
	require.Error(t, err, "a ref that is neither a ref nor sugar is still refused")
}

func TestVersionKeysDecodeFromTOML(t *testing.T) {
	data := []byte(`
version = "4.0"
name = "p"

[[presets]]
name = "claude"

[[includes]]
name = "shared"
source = "https://github.com/example-org/ai-rules"
version = "^1.2"
tag_prefix = "v"
include_prerelease = true

[[installed_skills]]
name = "deploy"
source = "https://github.com/example-org/skills"
ref = "~2.1.0"

[[skill_sources]]
name = "acme"
url = "https://github.com/example-org/skills"
version = ">=1.4.0 <2.0.0"
`)

	cfg, err := DecodeTOMLConfig(data, "config.toml")

	require.NoError(t, err)
	assert.Equal(t, VersionSpec{Constraint: "^1.2", TagPrefix: "v", IncludePrerelease: true}, cfg.Includes[0].VersionSpec())
	assert.Equal(t, "~2.1.0", cfg.InstalledSkills[0].RequestedRef())
	assert.Equal(t, ">=1.4.0 <2.0.0", cfg.SkillSources[0].RequestedRef())
	require.NoError(t, cfg.validateVersionKeys())
}
