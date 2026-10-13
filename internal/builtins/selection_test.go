package builtins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onlyAutoExcluded is every auto-include removed, isolating the effect of a
// positive selection.
var onlyAutoExcluded = []string{
	"!ai-governance", "!agent-delegation", "!security", "!git-workflow",
	"!code-quality", "!testing", "!token-efficiency",
}

func TestResolveSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		list        []string
		wantDomains []string
		wantOnly    map[string][]string
		notDomains  []string
	}{
		{
			name:        "positive item of a non-auto pack loads only that item",
			list:        append(append([]string{}, onlyAutoExcluded...), "docker/container-standards"),
			wantDomains: []string{"docker"},
			wantOnly:    map[string][]string{"docker": {"container-standards"}},
		},
		{
			name:        "item of an auto-included pack keeps the whole pack",
			list:        []string{"security/secrets-handling"},
			wantDomains: []string{"security"},
			wantOnly:    map[string][]string{},
		},
		{
			name:        "item then !domain yields only that item",
			list:        []string{"security/secrets-handling", "!security"},
			wantDomains: []string{"security"},
			wantOnly:    map[string][]string{"security": {"secrets-handling"}},
		},
		{
			name:        "!domain then item yields only that item",
			list:        []string{"!security", "security/secrets-handling"},
			wantDomains: []string{"security"},
			wantOnly:    map[string][]string{"security": {"secrets-handling"}},
		},
		{
			name:        "domain wins over domain/name",
			list:        []string{"docker", "docker/container-standards"},
			wantDomains: []string{"docker"},
			wantOnly:    map[string][]string{},
		},
		{
			name:        "unknown pack with an item is not loaded",
			list:        []string{"nope/thing"},
			wantDomains: nil,
			wantOnly:    map[string][]string{},
			notDomains:  []string{"nope"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveSelection(tt.list)
			for _, want := range tt.wantDomains {
				assert.Contains(t, got.Domains, want)
			}
			for _, not := range tt.notDomains {
				assert.NotContains(t, got.Domains, not)
			}
			require.Len(t, got.Only, len(tt.wantOnly))
			for domain, names := range tt.wantOnly {
				var gotNames []string
				for n := range got.Only[domain] {
					gotNames = append(gotNames, n)
				}
				assert.ElementsMatch(t, names, gotNames)
			}
		})
	}
}

func TestResolveSelection_onlyAutoExcludedLoadsNothingElse(t *testing.T) {
	t.Parallel()

	got := ResolveSelection(append(append([]string{}, onlyAutoExcluded...), "docker/container-standards"))
	assert.Equal(t, []string{"docker"}, got.Domains)
}

func TestUnknownSelections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		list           []string
		wantSpecs      []string
		wantSuggestion string
		wantValid      string
	}{
		{name: "valid pack and item", list: []string{"docker", "docker/container-standards"}},
		{name: "exclusions are not checked", list: []string{"!nope", "!security/nope"}},
		{name: "unknown pack", list: []string{"nope"}, wantSpecs: []string{"nope"}, wantValid: "docker"},
		{
			name:           "typo in item names the pack's items",
			list:           []string{"docker/container-standard"},
			wantSpecs:      []string{"docker/container-standard"},
			wantSuggestion: "docker/container-standards",
			wantValid:      "docker/container-standards",
		},
		{name: "extra path segments never resolve", list: []string{"docker/a/b"}, wantSpecs: []string{"docker/a/b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := UnknownSelections(tt.list)
			var specs []string
			for _, u := range got {
				specs = append(specs, u.Spec)
			}
			assert.Equal(t, tt.wantSpecs, specs)
			if len(got) == 0 {
				return
			}
			assert.Equal(t, tt.wantSuggestion, got[0].Suggestion)
			if tt.wantValid != "" {
				assert.Contains(t, got[0].Valid, tt.wantValid)
			}
		})
	}
}

func TestResolvesRefAndRefDomain(t *testing.T) {
	t.Parallel()

	assert.True(t, ResolvesRef("builtin:docker"))
	assert.True(t, ResolvesRef("builtin:docker/container-standards"))
	assert.False(t, ResolvesRef("builtin:docker/nope"))
	assert.False(t, ResolvesRef("builtin:nope"))
	assert.Equal(t, "docker", RefDomain("builtin:docker/container-standards"))
	assert.Equal(t, "docker", RefDomain("builtin:docker"))
}
