package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
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
				Version:  "5.0",
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

func TestFilterContentFilesByProfile(t *testing.T) {
	t.Parallel()

	files := []config.ContentFile{
		{Name: "everyone"},
		{Name: "backend-only", Profiles: []string{"backend"}},
		{Name: "frontend-only", Profiles: []string{"frontend"}},
	}

	got := func(profile string) []string {
		var names []string
		for _, f := range config.FilterContentFilesByProfile(files, profile) {
			names = append(names, f.Name)
		}
		return names
	}

	assert.Equal(t, []string{"everyone", "backend-only"}, got("backend"))
	assert.Equal(t, []string{"everyone", "frontend-only"}, got("frontend"))
	assert.ElementsMatch(t, []string{"everyone", "backend-only", "frontend-only"}, got(""),
		"an unset profile applies no restriction")
	assert.Equal(t, []string{"everyone", "backend-only"}, got("base,backend"),
		"a composed profile matches any of its elements")
}

func TestResolveProfileDomains(t *testing.T) {
	known := []string{"api", "s", "web"}

	tests := []struct {
		name    string
		entries []string
		want    []string
		wantErr string
	}{
		{name: "exact names are kept in order", entries: []string{"web", "api"}, want: []string{"web", "api"}},
		{name: "unknown exact name is kept as written", entries: []string{"ghost"}, want: []string{"ghost"}},
		{name: "wildcard expands to every known domain sorted", entries: []string{"*"}, want: []string{"api", "s", "web"}},
		{name: "wildcard with an exclusion drops it", entries: []string{"*", "!s"}, want: []string{"api", "web"}},
		{name: "order of positives and negations does not matter", entries: []string{"!s", "*"}, want: []string{"api", "web"}},
		{name: "exclusion after an explicit positive", entries: []string{"api", "web", "!api"}, want: []string{"web"}},
		{name: "unknown exclusion is ignored", entries: []string{"api", "!ghost"}, want: []string{"api"}},
		{name: "wildcard de-duplicates a named positive", entries: []string{"web", "*"}, want: []string{"web", "api", "s"}},
		{name: "exclusion of a builtin reference drops the bare domain", entries: []string{"*", "!builtin:api"}, want: []string{"s", "web"}},
		{name: "only exclusions is an error", entries: []string{"!api"}, wantErr: "excludes domains but selects none"},
		{name: "empty entries resolve to nothing", entries: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ResolveProfileDomains(tt.entries, known)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfig_GetProfileDomains_Wildcard(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string][]string{
			"everything": {"*"},
			"all-but-s":  {"*", "!s"},
			"exact":      {"api"},
		},
		Content: &config.ContentTree{Domains: map[string]*config.Domain{
			"api": {}, "s": {}, "web": {},
		}},
	}

	assert.Equal(t, []string{"api", "s", "web"}, cfg.GetProfileDomains("everything"))
	assert.Equal(t, []string{"api", "web"}, cfg.GetProfileDomains("all-but-s"))
	assert.Equal(t, []string{"api"}, cfg.GetProfileDomains("exact"), "an exact-name profile is unchanged")
}

func TestConfig_SelectContentForProfile_WildcardAndNegation(t *testing.T) {
	tree := &config.ContentTree{Domains: map[string]*config.Domain{
		"api": {Name: "api"}, "s": {Name: "s"}, "web": {Name: "web"},
	}}
	cfg := &config.Config{Profiles: map[string][]string{"all-but-s": {"*", "!s"}}}

	selected, err := cfg.SelectContentForProfile(tree, "all-but-s")
	require.NoError(t, err)

	got := make([]string, 0, len(selected.Domains))
	for name := range selected.Domains {
		got = append(got, name)
	}
	assert.ElementsMatch(t, []string{"api", "web"}, got)
}

func TestConfig_Validate_PureNegationProfile(t *testing.T) {
	cfg := &config.Config{
		Version:  "5.0",
		Name:     "p",
		Presets:  []config.Preset{{BuiltIn: "claude"}},
		Profiles: map[string][]string{"broken": {"!api"}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `profile "broken"`)
	assert.Contains(t, err.Error(), "excludes domains but selects none")
}

func TestValidateWarnsUnknownNegatedProfileDomain(t *testing.T) {
	// Arrange
	rec := &testutil.LogRecorder{}
	cfg := &config.Config{
		Version:  "5.0",
		Name:     "p",
		Presets:  []config.Preset{{BuiltIn: "claude"}},
		Host:     ambient.Host{Log: rec},
		Profiles: map[string][]string{"all-but-ghost": {"*", "!ghost"}},
		Content:  &config.ContentTree{Domains: map[string]*config.Domain{"api": {Name: "api"}}},
	}

	// Act
	err := cfg.Validate()

	// Assert
	require.NoError(t, err, "an unknown negated name is a warning, not an error")
	assert.Contains(t, rec.String(), "profile references non-existent domain")
}
