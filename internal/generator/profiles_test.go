package generator

import (
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// domainNames returns the sorted domain names of a content tree. Sorted because
// the tree stores domains in a map — the union's order is asserted where it is
// actually observable, in config.GetProfileDomains.
func domainNames(tree *config.ContentTree) []string {
	names := make([]string, 0, len(tree.Domains))
	for name := range tree.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestComposedProfile_MatchesHandWrittenUnion is the point of composition: the
// tokens fixture defines "full" as [backend, frontend], which is exactly the
// combinatorial profile a user has to hand-write today. Composing the two role
// profiles has to produce the same content tree, or composition is not a
// replacement for it.
func TestComposedProfile_MatchesHandWrittenUnion(t *testing.T) {
	gen := NewGenerator(tokensFixture(t))

	composed, err := gen.getContentForProfile("backend,frontend")
	require.NoError(t, err)
	union, err := gen.getContentForProfile("full")
	require.NoError(t, err)

	assert.Equal(t, []string{"backend", "frontend"}, domainNames(composed))
	assert.Equal(t, domainNames(union), domainNames(composed))
	assert.Equal(t, union.Rules, composed.Rules)
	assert.Equal(t, union.Context, composed.Context)
}

func TestComposedProfile_ContentSelection(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		domains []string
	}{
		{name: "single name is unchanged", profile: "backend", domains: []string{"backend"}},
		{name: "two names union", profile: "backend,frontend", domains: []string{"backend", "frontend"}},
		{
			// "full" already covers "backend", so the overlap must not appear twice.
			name:    "overlapping profiles de-duplicate",
			profile: "full,backend",
			domains: []string{"backend", "frontend"},
		},
		{name: "whitespace around an element is ignored", profile: " backend , frontend ", domains: []string{"backend", "frontend"}},
		{name: "an empty element is ignored", profile: "backend,,frontend", domains: []string{"backend", "frontend"}},
	}

	gen := NewGenerator(tokensFixture(t))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := gen.getContentForProfile(tt.profile)
			require.NoError(t, err)
			assert.Equal(t, tt.domains, domainNames(tree))
		})
	}
}

func TestComposedProfile_UnknownElement(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantErr string
	}{
		{name: "single unknown name", profile: "nope", wantErr: "profile not found: nope"},
		{name: "second element unknown", profile: "backend,nope", wantErr: "profile not found: nope"},
		{name: "first element unknown", profile: "nope,backend", wantErr: "profile not found: nope"},
		{name: "two unknown elements", profile: "nope,backend,bad", wantErr: "profile not found: nope, bad"},
		{name: "value is only separators", profile: ",", wantErr: "profile not found: ,"},
	}

	gen := NewGenerator(tokensFixture(t))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gen.getContentForProfile(tt.profile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr,
				"the error names the element that is wrong, not the whole composed value")

			var oopsErr oops.OopsError
			require.ErrorAs(t, err, &oopsErr)
			assert.Contains(t, oopsErr.Hint(), "backend",
				"the available-profiles hint is unchanged for a composed value")
		})
	}
}

func TestResolveProfile_Composed(t *testing.T) {
	tests := []struct {
		name           string
		configDefault  string
		profile        string
		wantActive     string
		editConfigOnly bool
	}{
		{name: "explicit single name", profile: "backend", wantActive: "backend"},
		{name: "explicit composed value", profile: "backend,frontend", wantActive: "backend,frontend"},
		{name: "composed value is canonicalized", profile: " backend , frontend ", wantActive: "backend,frontend"},
		{name: "empty falls back to the config default", configDefault: "frontend", wantActive: "frontend"},
		{
			name:          "a composed default is honored",
			configDefault: "backend,frontend",
			wantActive:    "backend,frontend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tokensFixture(t)
			if tt.configDefault != "" {
				cfg.Default = tt.configDefault
			}
			assert.Equal(t, tt.wantActive, NewGenerator(cfg).resolveProfile(tt.profile))
		})
	}
}

// TestTokenReport_ComposedProfile checks the cost report end to end for a composed
// value: it is reported under the composed name and costs what the equivalent
// hand-written profile costs.
func TestTokenReport_ComposedProfile(t *testing.T) {
	composed, err := NewGenerator(tokensFixture(t)).TokenReport(TokenReportOptions{
		Profile: "backend,frontend",
		Counter: tokens.CL100KBase(),
	})
	require.NoError(t, err)

	union := tokensReport(t, "full")

	assert.Equal(t, "backend,frontend", composed.Profile)
	assert.Equal(t, union.HeadlineAlways, composed.HeadlineAlways)
	assert.Equal(t, union.HeadlinePreset, composed.HeadlinePreset)
}
