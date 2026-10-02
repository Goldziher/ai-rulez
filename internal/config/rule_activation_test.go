package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGlobs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"src/**"}, []string{"src/**"}},
		{"comma split", []string{"a, b"}, []string{"a", "b"}},
		{"braces kept", []string{"*.{ts,tsx}"}, []string{"*.{ts,tsx}"}},
		{"braces and commas", []string{"*.{ts,tsx}, docs/**"}, []string{"*.{ts,tsx}", "docs/**"}},
		{"whitespace trimmed", []string{"  a  ", " b"}, []string{"a", "b"}},
		{"empties dropped", []string{"", " , ,a,,"}, []string{"a"}},
		{"dedupe first seen", []string{"b", "a,b", "a"}, []string{"b", "a"}},
		{"unbalanced close brace", []string{"a},b"}, []string{"a}", "b"}},
		{"only empties", []string{"", " "}, nil},
		{"brackets kept", []string{"src/[a,b]*.ts"}, []string{"src/[a,b]*.ts"}},
		{"brackets then split", []string{"src/[a,b]*.ts, c"}, []string{"src/[a,b]*.ts", "c"}},
		{"escaped comma kept verbatim", []string{`a\,b`}, []string{`a\,b`}},
		{"escaped comma then split", []string{`a\,b, c`}, []string{`a\,b`, "c"}},
		{"nested braces", []string{"{a,{b,c}}, d"}, []string{"{a,{b,c}}", "d"}},
		{"unbalanced open brace stops splitting", []string{"{a,b, c"}, []string{"{a,b, c"}},
		{"unbalanced open bracket stops splitting", []string{"[a,b, c"}, []string{"[a,b, c"}},
		{"trailing backslash", []string{`a\`}, []string{`a\`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeGlobs(tt.in))
		})
	}
}

func TestResolveActivation_NormalizesValue(t *testing.T) {
	got := (&Metadata{Activation: " Manual "}).ResolveActivation()

	assert.Equal(t, ActivationManual, got.Mode)
	assert.Equal(t, ActivationSourceActivation, got.Source)
}

func TestResolveActivation(t *testing.T) {
	tests := []struct {
		name string
		meta *Metadata
		want Activation
	}{
		{
			name: "nil metadata",
			meta: nil,
			want: Activation{Mode: ActivationAlways, Source: ActivationSourceDerived},
		},
		{
			name: "empty metadata is always",
			meta: &Metadata{},
			want: Activation{Mode: ActivationAlways, Source: ActivationSourceDerived},
		},
		{
			name: "globs derive glob",
			meta: &Metadata{Globs: []string{"src/**"}},
			want: Activation{Mode: ActivationGlob, Globs: []string{"src/**"}, Source: ActivationSourceDerived},
		},
		{
			name: "paths derive glob",
			meta: &Metadata{Paths: []string{"docs/**, src/**"}},
			want: Activation{Mode: ActivationGlob, Globs: []string{"docs/**", "src/**"}, Source: ActivationSourceDerived},
		},
		{
			name: "description alone stays always",
			meta: &Metadata{Extra: map[string]string{"description": "Use for tests"}},
			want: Activation{Mode: ActivationAlways, Description: "Use for tests", Source: ActivationSourceDerived},
		},
		{
			name: "explicit activation wins over trigger",
			meta: &Metadata{Activation: "manual", Extra: map[string]string{"trigger": "always_on"}},
			want: Activation{Mode: ActivationManual, Source: ActivationSourceActivation},
		},
		{
			name: "explicit activation wins over alwaysApply",
			meta: &Metadata{Activation: "auto", Extra: map[string]string{"alwaysApply": "true", "description": "d"}},
			want: Activation{Mode: ActivationAuto, Description: "d", Source: ActivationSourceActivation},
		},
		{
			name: "trigger always_on",
			meta: &Metadata{Extra: map[string]string{"trigger": "always_on"}},
			want: Activation{Mode: ActivationAlways, Source: ActivationSourceTrigger},
		},
		{
			name: "trigger glob merges legacy glob",
			meta: &Metadata{Globs: []string{"a"}, Extra: map[string]string{"trigger": "glob", "glob": "b, a"}},
			want: Activation{Mode: ActivationGlob, Globs: []string{"a", "b"}, Source: ActivationSourceTrigger},
		},
		{
			name: "trigger model_decision",
			meta: &Metadata{Extra: map[string]string{"trigger": "model_decision", "description": "d"}},
			want: Activation{Mode: ActivationAuto, Description: "d", Source: ActivationSourceTrigger},
		},
		{
			name: "trigger manual",
			meta: &Metadata{Extra: map[string]string{"trigger": "manual"}},
			want: Activation{Mode: ActivationManual, Source: ActivationSourceTrigger},
		},
		{
			name: "unknown trigger falls through",
			meta: &Metadata{Extra: map[string]string{"trigger": "bogus"}},
			want: Activation{Mode: ActivationAlways, Source: ActivationSourceDerived},
		},
		{
			name: "trigger wins over alwaysApply",
			meta: &Metadata{Extra: map[string]string{"trigger": "manual", "alwaysApply": "true"}},
			want: Activation{Mode: ActivationManual, Source: ActivationSourceTrigger},
		},
		{
			name: "alwaysApply true",
			meta: &Metadata{Globs: []string{"a"}, Extra: map[string]string{"alwaysApply": "true"}},
			want: Activation{Mode: ActivationAlways, Globs: []string{"a"}, Source: ActivationSourceAlwaysApply},
		},
		{
			name: "alwaysApply false with globs",
			meta: &Metadata{Globs: []string{"a"}, Extra: map[string]string{"alwaysApply": "false"}},
			want: Activation{Mode: ActivationGlob, Globs: []string{"a"}, Source: ActivationSourceAlwaysApply},
		},
		{
			name: "alwaysApply false with description",
			meta: &Metadata{Extra: map[string]string{"alwaysApply": "false", "description": "d"}},
			want: Activation{Mode: ActivationAuto, Description: "d", Source: ActivationSourceAlwaysApply},
		},
		{
			name: "alwaysApply false bare",
			meta: &Metadata{Extra: map[string]string{"alwaysApply": "false"}},
			want: Activation{Mode: ActivationManual, Source: ActivationSourceAlwaysApply},
		},
		{
			name: "unparseable alwaysApply falls through",
			meta: &Metadata{Extra: map[string]string{"alwaysApply": "maybe"}},
			want: Activation{Mode: ActivationAlways, Source: ActivationSourceDerived},
		},
		{
			name: "invalid explicit activation falls through",
			meta: &Metadata{Activation: "sometimes", Extra: map[string]string{"trigger": "manual"}},
			want: Activation{Mode: ActivationManual, Source: ActivationSourceTrigger},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.meta.ResolveActivation())
		})
	}
}

func TestParseFrontmatter_Activation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		activation string
		paths      []string
		globs      []string
		extraKeys  []string
	}{
		{
			name:       "yaml activation and globs",
			input:      "---\nactivation: glob\nglobs:\n  - \"*.go\"\n---\nbody",
			activation: "glob",
			globs:      []string{"*.go"},
		},
		{
			name:  "paths comma string",
			input: "---\npaths: \"src/**, docs/**\"\n---\nbody",
			paths: []string{"src/**", "docs/**"},
		},
		{
			name:  "paths comma string with braces",
			input: "---\npaths: \"*.{ts,tsx}, lib/**\"\n---\nbody",
			paths: []string{"*.{ts,tsx}", "lib/**"},
		},
		{
			name:       "fallback parser keeps activation out of extra",
			input:      "---\nactivation: auto\ndescription: d\nnested:\n  a: b\n---\nbody",
			activation: "auto",
			extraKeys:  []string{"description", "nested"},
		},
		{
			name:       "fallback parser splits comma globs",
			input:      "---\nactivation: glob\nglobs: \"a, b\"\nnested:\n  a: b\n---\nbody",
			activation: "glob",
			globs:      []string{"a", "b"},
			extraKeys:  []string{"nested"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, _, malformed := parseFrontmatter(tt.input)

			require.False(t, malformed)
			require.NotNil(t, meta)
			assert.Equal(t, tt.activation, meta.Activation)
			if tt.paths != nil {
				assert.Equal(t, tt.paths, meta.PathScope())
			}
			if tt.globs != nil {
				assert.Equal(t, tt.globs, meta.PathScope())
			}
			assert.NotContains(t, meta.Extra, "activation")
			for _, k := range tt.extraKeys {
				assert.Contains(t, meta.Extra, k)
			}
		})
	}
}

func TestValidate_RuleActivation(t *testing.T) {
	tests := []struct {
		name    string
		meta    *Metadata
		wantErr string
	}{
		{"no metadata", nil, ""},
		{"no activation", &Metadata{Globs: []string{"a"}}, ""},
		{"always bare", &Metadata{Activation: "always"}, ""},
		{"glob with globs", &Metadata{Activation: "glob", Globs: []string{"a"}}, ""},
		{"glob with paths", &Metadata{Activation: "glob", Paths: []string{"a"}}, ""},
		{"glob with legacy glob", &Metadata{Activation: "glob", Extra: map[string]string{"glob": "a"}}, ""},
		{"auto with description", &Metadata{Activation: "auto", Extra: map[string]string{"description": "d"}}, ""},
		{"manual", &Metadata{Activation: "manual"}, ""},
		{"manual with globs", &Metadata{Activation: "manual", Globs: []string{"a"}}, ""},
		{"mixed case value", &Metadata{Activation: " Always "}, ""},
		{"always with legacy glob", &Metadata{Activation: "always", Extra: map[string]string{"glob": "a"}}, "conflicts with globs"},
		{"unknown value", &Metadata{Activation: "sometimes"}, "unknown activation"},
		{"glob without globs", &Metadata{Activation: "glob"}, "requires globs"},
		{"auto without description", &Metadata{Activation: "auto"}, "requires a description"},
		{"always with globs", &Metadata{Activation: "always", Globs: []string{"a"}}, "conflicts with globs"},
		{
			"contradicting legacy only warns",
			&Metadata{Activation: "manual", Extra: map[string]string{"trigger": "always_on", "alwaysApply": "true"}},
			"",
		},
	}

	place := map[string]func(ContentFile) *Config{
		"root rule": func(f ContentFile) *Config {
			return &Config{Content: &ContentTree{Rules: []ContentFile{f}}}
		},
		"root context": func(f ContentFile) *Config {
			return &Config{Content: &ContentTree{Context: []ContentFile{f}}}
		},
		"domain rule": func(f ContentFile) *Config {
			return &Config{Content: &ContentTree{Domains: map[string]*Domain{"d": {Rules: []ContentFile{f}}}}}
		},
		"domain context": func(f ContentFile) *Config {
			return &Config{Content: &ContentTree{Domains: map[string]*Domain{"d": {Context: []ContentFile{f}}}}}
		},
		"local rule": func(f ContentFile) *Config {
			return &Config{LocalContent: &ContentTree{Rules: []ContentFile{f}}}
		},
		"local domain context": func(f ContentFile) *Config {
			return &Config{LocalContent: &ContentTree{Domains: map[string]*Domain{"d": {Context: []ContentFile{f}}}}}
		},
	}

	for placeName, build := range place {
		for _, tt := range tests {
			t.Run(placeName+"/"+tt.name, func(t *testing.T) {
				cfg := build(ContentFile{Name: "r", Path: "rules/r.md", Metadata: tt.meta})

				err := cfg.validateRuleActivation()

				if tt.wantErr == "" {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), "rules/r.md")
			})
		}
	}

	t.Run("nil trees", func(t *testing.T) {
		require.NoError(t, (&Config{}).validateRuleActivation())
	})
}

func TestValidate_RuleActivation_Wiring(t *testing.T) {
	rule := func(meta *Metadata) []ContentFile {
		return []ContentFile{{Name: "r", Path: "rules/r.md", Metadata: meta}}
	}
	base := func(tree *ContentTree) *Config {
		return &Config{Version: "4.0", Name: "x", Presets: []Preset{{BuiltIn: "claude"}}, Content: tree}
	}

	t.Run("Validate surfaces the activation error", func(t *testing.T) {
		cfg := base(&ContentTree{Rules: rule(&Metadata{Activation: "glob"})})

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires globs")
	})

	t.Run("included domain only warns", func(t *testing.T) {
		cfg := base(&ContentTree{Domains: map[string]*Domain{
			"inc": {FromInclude: true, Rules: rule(&Metadata{Activation: "bogus"})},
			"bi":  {Builtin: true, Context: rule(&Metadata{Activation: "auto"})},
		}})

		require.NoError(t, cfg.validateRuleActivation())
	})

	t.Run("project domain is an error", func(t *testing.T) {
		cfg := base(&ContentTree{Domains: map[string]*Domain{
			"mine": {Rules: rule(&Metadata{Activation: "bogus"})},
		}})

		require.Error(t, cfg.validateRuleActivation())
	})
}
