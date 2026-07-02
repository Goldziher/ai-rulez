package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ruleNames extracts the Name of each ContentFile, preserving order.
func ruleNames(files []config.ContentFile) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	return names
}

func TestDedupeByName_keepsFirstAndReportsDropped(t *testing.T) {
	t.Parallel()

	input := []config.ContentFile{
		{Name: "a", Content: "first-a"},
		{Name: "b", Content: "b"},
		{Name: "a", Content: "second-a"},
		{Name: "a", Content: "third-a"},
	}

	kept, dropped := dedupeByName(input)

	require.Len(t, kept, 2)
	assert.Equal(t, []string{"a", "b"}, ruleNames(kept))
	assert.Equal(t, "first-a", kept[0].Content, "first occurrence must win")
	assert.Equal(t, []string{"a", "a"}, dropped)
}

func TestCombineDedupedContentFiles_precedenceThenAlphabetical(t *testing.T) {
	t.Parallel()

	// Higher-precedence slices are passed first; keep-first wins, output sorted.
	high := []config.ContentFile{{Name: "commit-messages", Content: "LOCAL"}}
	low := []config.ContentFile{
		{Name: "commit-messages", Content: "BUILTIN"},
		{Name: "atomic-commits", Content: "BUILTIN"},
	}

	got := combineDedupedContentFiles(high, low)

	assert.Equal(t, []string{"atomic-commits", "commit-messages"}, ruleNames(got), "output is alphabetical")
	for _, f := range got {
		if f.Name == "commit-messages" {
			assert.Equal(t, "LOCAL", f.Content, "highest-precedence source must win")
		}
	}
}

// TestAllInlineRules_sourcePrecedence covers the core fix: the same rule name
// defined by root, an on-disk domain, an include-sourced domain, and a builtin
// domain collapses to a single entry, with precedence
// root > on-disk domain > include > builtin.
func TestAllInlineRules_sourcePrecedence(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTree{
		Rules: []config.ContentFile{{Name: "shared", Content: "ROOT"}},
		Domains: map[string]*config.Domain{
			"ondisk": {
				Name:  "ondisk",
				Rules: []config.ContentFile{{Name: "shared", Content: "ONDISK"}, {Name: "ondisk-only", Content: "x"}},
			},
			"included": {
				Name:        "included",
				FromInclude: true,
				Rules:       []config.ContentFile{{Name: "shared", Content: "INCLUDE"}, {Name: "include-only", Content: "x"}},
			},
			"git-workflow": {
				Name:    "git-workflow",
				Builtin: true,
				Rules:   []config.ContentFile{{Name: "shared", Content: "BUILTIN"}, {Name: "builtin-only", Content: "x"}},
			},
		},
	}

	got := allInlineRules(tree)

	// Each name appears exactly once.
	assert.Equal(t, []string{"builtin-only", "include-only", "ondisk-only", "shared"}, ruleNames(got))

	// Root wins the "shared" collision.
	for _, f := range got {
		if f.Name == "shared" {
			assert.Equal(t, "ROOT", f.Content)
		}
	}
}

func TestAllInlineRules_ondiskBeatsBuiltin(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"git-workflow": {
				Name:    "git-workflow",
				Builtin: true,
				Rules:   []config.ContentFile{{Name: "commit-messages", Content: "BUILTIN"}},
			},
			"team": {
				Name:  "team",
				Rules: []config.ContentFile{{Name: "commit-messages", Content: "ONDISK"}},
			},
		},
	}

	got := allInlineRules(tree)
	require.Len(t, got, 1)
	assert.Equal(t, "ONDISK", got[0].Content, "on-disk domain must beat builtin")
}

func TestDomainNamesByPrecedence_ordersOnDiskIncludeBuiltin(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"zeta-builtin":  {Name: "zeta-builtin", Builtin: true},
			"alpha-include": {Name: "alpha-include", FromInclude: true},
			"mid-ondisk":    {Name: "mid-ondisk"},
		},
	}

	// On-disk (rank 0) first, then include (rank 1), then builtin (rank 2) —
	// regardless of alphabetical name order across ranks.
	assert.Equal(t, []string{"mid-ondisk", "alpha-include", "zeta-builtin"}, domainNamesByPrecedence(tree))
}

func TestFindDuplicateContent_reportsWinnerAndLosers(t *testing.T) {
	t.Parallel()

	root := []config.ContentFile{{Name: "commit-messages", Path: "local://commit-messages.md"}}
	domains := []config.ContentFile{
		{Name: "commit-messages", Path: "builtin://git-workflow/commit-messages.md"},
		{Name: "unique", Path: "builtin://x.md"},
	}

	dups := FindDuplicateContent(root, domains)

	require.Len(t, dups, 1)
	assert.Equal(t, "commit-messages", dups[0].Name)
	assert.Equal(t, "local://commit-messages.md", dups[0].Winner)
	assert.Equal(t, []string{"builtin://git-workflow/commit-messages.md"}, dups[0].Losers)
}
