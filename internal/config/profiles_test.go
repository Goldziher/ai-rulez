package config_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitProfileNames(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "single name", value: "backend", want: []string{"backend"}},
		{name: "single name is trimmed", value: "  backend  ", want: []string{"backend"}},
		{name: "empty value has no names", value: "", want: nil},
		{name: "blank value has no names", value: "   ", want: nil},
		{name: "two names compose", value: "base,backend", want: []string{"base", "backend"}},
		{name: "elements are trimmed", value: "base , backend", want: []string{"base", "backend"}},
		{name: "empty elements are ignored", value: "base,,backend,", want: []string{"base", "backend"}},
		{name: "order is preserved", value: "backend,base", want: []string{"backend", "base"}},
		{name: "repeated name is kept as written", value: "base,base", want: []string{"base", "base"}},
		{name: "separators only yield no names", value: ", ,", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, config.SplitProfileNames(tt.value))
		})
	}
}

func TestCanonicalProfile(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "single name unchanged", value: "backend", want: "backend"},
		{name: "whitespace normalized", value: "base , backend", want: "base,backend"},
		{name: "trailing separator dropped", value: "base,backend,", want: "base,backend"},
		{
			// Returned verbatim so an error can quote what was typed instead of
			// an empty string.
			name:  "unusable value returned as written",
			value: ",",
			want:  ",",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, config.CanonicalProfile(tt.value))
		})
	}
}

// profilesConfig is the shape composition exists for: one profile everybody
// installs, plus role profiles that add to it.
func profilesConfig() *config.Config {
	return &config.Config{
		Profiles: map[string][]string{
			"base":     {"shared"},
			"backend":  {"shared", "api"},
			"frontend": {"web"},
		},
	}
}

func TestConfig_GetProfileDomains_Composed(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    []string
	}{
		{name: "single name behaves as before", profile: "backend", want: []string{"shared", "api"}},
		{name: "unknown single name has no domains", profile: "nope", want: nil},
		{name: "two names union", profile: "base,frontend", want: []string{"shared", "web"}},
		{
			name:    "overlapping domains are de-duplicated",
			profile: "base,backend",
			want:    []string{"shared", "api"},
		},
		{
			name:    "union order follows first mention",
			profile: "frontend,backend",
			want:    []string{"web", "shared", "api"},
		},
		{name: "unknown element contributes nothing", profile: "base,nope", want: []string{"shared"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, profilesConfig().GetProfileDomains(tt.profile))
		})
	}
}

func TestConfig_GetProfileDomains_ComposedDefault(t *testing.T) {
	cfg := profilesConfig()
	cfg.Default = "base,frontend"

	assert.Equal(t, []string{"shared", "web"}, cfg.GetProfileDomains(""),
		"an empty profile falls back to the default, which may itself be composed")
}

func TestConfig_HasProfile_Composed(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    bool
	}{
		{name: "defined name", profile: "base", want: true},
		{name: "unknown name", profile: "nope", want: false},
		{name: "empty value", profile: "", want: false},
		{name: "every element defined", profile: "base,backend", want: true},
		{name: "one element unknown", profile: "base,nope", want: false},
		{name: "first element unknown", profile: "nope,base", want: false},
		{name: "separators only", profile: ",", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, profilesConfig().HasProfile(tt.profile))
		})
	}
}

func TestConfig_UnknownProfileNames(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    []string
	}{
		{name: "all known", profile: "base,backend", want: nil},
		{name: "names the bad element only", profile: "base,nope,frontend", want: []string{"nope"}},
		{name: "names every bad element", profile: "nope,base,bad", want: []string{"nope", "bad"}},
		{name: "single unknown is the value itself", profile: "nope", want: []string{"nope"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, profilesConfig().UnknownProfileNames(tt.profile))
		})
	}
}

func TestConfig_Validate_ComposedDefault(t *testing.T) {
	tests := []struct {
		name        string
		defaultName string
		profiles    map[string][]string
		wantErr     string
	}{
		{
			name:        "composed default accepted",
			defaultName: "base,backend",
			profiles:    map[string][]string{"base": {"shared"}, "backend": {"api"}},
		},
		{
			name:        "single default accepted",
			defaultName: "base",
			profiles:    map[string][]string{"base": {"shared"}},
		},
		{
			name:        "unknown element is named, not the whole value",
			defaultName: "base,nope",
			profiles:    map[string][]string{"base": {"shared"}},
			wantErr:     `default profile "nope" does not exist in profiles`,
		},
		{
			name:        "comma in a profile name is rejected",
			defaultName: "",
			profiles:    map[string][]string{"base,backend": {"shared"}},
			wantErr:     `profile name "base,backend" contains ","`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Version:  "3.0",
				Name:     "composed",
				Presets:  []config.Preset{{BuiltIn: "claude"}},
				Default:  tt.defaultName,
				Profiles: tt.profiles,
			}
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
