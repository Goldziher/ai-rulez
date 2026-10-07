package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listingFixtureReport renders the tokens fixture with the given presets and
// extra skills added to the source tree.
func listingFixtureReport(t *testing.T, presets []string, skills map[string]string) *TokenReport {
	t.Helper()
	dir := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "config", "tokens"), dir)
	configPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	original, err := os.ReadFile(configPath)
	require.NoError(t, err)
	edited := strings.Replace(string(original), "presets = [\"claude\"]\n", "presets = [\""+strings.Join(presets, "\", \"")+"\"]\n", 1)
	edited += "\n[header]\nhashes = \"full\"\n"
	require.NoError(t, os.WriteFile(configPath, []byte(edited), 0o600))
	for name, text := range skills {
		skillDir := filepath.Join(dir, ".ai-rulez", "skills", name)
		require.NoError(t, os.MkdirAll(skillDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(text), 0o600))
	}
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	report, err := NewGenerator(cfg).TokenReport(TokenReportOptions{Profile: "backend", Counter: tokens.CL100KBase()})
	require.NoError(t, err)
	return report
}

func skillText(name, description, extra string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n" + extra + "---\nbody of " + name + "\n"
}

func TestTokenReport_ListingCoversEverySkillHarness(t *testing.T) {
	report := listingFixtureReport(t, []string{"claude", "codex", "cursor", "gemini", "opencode", "copilot", "devin", "cline", "pi", "junie"}, nil)

	for _, preset := range []string{"claude", "codex", "cursor", "gemini", "opencode", "copilot", "devin", "cline", "pi", "junie"} {
		runtime := findRuntime(t, report, preset)
		assert.Positive(t, runtime.Listing, "%s lists skills, so it has a listing cost", preset)
		assert.Positive(t, runtime.ListedItems, preset)
		assert.LessOrEqual(t, runtime.Listing, runtime.Always, "%s: the listing is part of the always-loaded figure", preset)
	}
}

func TestTokenReport_ListingNotModeledForUnlistedPresets(t *testing.T) {
	report := listingFixtureReport(t, []string{"claude", "amp"}, nil)
	amp := findRuntime(t, report, "amp")
	assert.Zero(t, amp.Listing)
	assert.Equal(t, amp.LegacyAlways, amp.Always, "an unmodeled preset keeps the legacy figure")
}

func TestTokenReport_ListingPerEntryMath(t *testing.T) {
	report := listingFixtureReport(t, []string{"claude"}, nil)
	runtime := findRuntime(t, report, "claude")
	framing := 0
	items := 0
	for _, entry := range runtime.Entries {
		if !strings.HasSuffix(entry.Label, " listing") {
			continue
		}
		items += entry.Artifacts
		framing += findChild(t, entry, "per-entry framing (estimate)").Tokens
	}
	assert.Equal(t, runtime.ListedItems, items)
	assert.Equal(t, items*ListingEntryOverheadTokens, framing)
}

func TestTokenReport_ListingSkipsModelDisabledSkillsAndLegacyStaysComparable(t *testing.T) {
	long := strings.Repeat("alpha beta gamma ", 200)
	with := listingFixtureReport(t, []string{"claude"}, map[string]string{
		"visible": skillText("visible", "Use when x.", ""),
	})
	without := listingFixtureReport(t, []string{"claude"}, map[string]string{
		"visible": skillText("visible", "Use when x.", ""),
		"hidden":  skillText("hidden", long, "disable-model-invocation: true\n"),
	})
	assert.Equal(t, findRuntime(t, with, "claude").Listing, findRuntime(t, without, "claude").Listing,
		"a skill with disable-model-invocation is not offered to the model and costs no listing")
}

func TestTokenReport_ListingDescriptionLimit(t *testing.T) {
	long := strings.Repeat("alpha beta gamma ", 200) // 3,400 characters
	report := listingFixtureReport(t, []string{"claude", "gemini"}, map[string]string{
		"wordy": skillText("wordy", long, ""),
	})
	claude := findRuntime(t, report, "claude")
	gemini := findRuntime(t, report, "gemini")
	assert.Equal(t, 1, claude.TruncatedDescriptions, "claude cuts a listing entry at 1,536 characters")
	assert.Zero(t, gemini.TruncatedDescriptions, "no limit is known for gemini")
	assert.Greater(t, gemini.Listing, claude.Listing-ListingEntryOverheadTokens*claude.ListedItems+gemini.ListedItems,
		"the untruncated gemini listing is the larger one for the same skill")
}

func TestTokenReport_HeadlineAndBudgetUseTheListing(t *testing.T) {
	report := listingFixtureReport(t, []string{"claude"}, nil)
	runtime := findRuntime(t, report, "claude")
	assert.Equal(t, runtime.Always, report.HeadlineAlways)
	assert.Equal(t, runtime.Listing, report.HeadlineListing)
	assert.Equal(t, runtime.LegacyAlways, report.HeadlineAlwaysLegacy)
	assert.Greater(t, report.HeadlineAlways, report.HeadlineAlwaysLegacy)
	assert.Equal(t, ListingEntryOverheadTokens, report.ListingEntryOverhead)

	// A ceiling between the legacy and the real figure is exceeded only because
	// of the listing.
	ceiling := report.HeadlineAlwaysLegacy + 1
	require.Less(t, ceiling, report.HeadlineAlways)
	cfg := tokensFixture(t)
	gated, err := NewGenerator(cfg).TokenReport(TokenReportOptions{Profile: "backend", Counter: tokens.CL100KBase(), Budget: ceiling})
	require.NoError(t, err)
	require.NotNil(t, gated.Budget)
	assert.True(t, gated.Budget.Exceeded)
}

func TestListingFrontmatter(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantFields map[string]any
		wantBody   string
	}{
		{"no frontmatter", "just body\n", nil, "just body\n"},
		{"basic", "---\nname: a\n---\nbody\n", map[string]any{"name": "a"}, "body\n"},
		{"crlf", "---\r\nname: a\r\n---\r\nbody\r\n", map[string]any{"name": "a"}, "body\r\n"},
		{"unterminated", "---\nname: a\nbody\n", nil, "---\nname: a\nbody\n"},
		{"invalid yaml", "---\nname: [a\n---\nbody\n", nil, "---\nname: [a\n---\nbody\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, body := listingFrontmatter(tt.text)
			assert.Equal(t, tt.wantFields, fields)
			assert.Equal(t, tt.wantBody, body)
		})
	}
}

func TestListingNotes_DoNotMentionRemovedPresets(t *testing.T) {
	report := &TokenReport{Runtimes: []RuntimeTokens{{ListedItems: 1}}}

	notes := strings.Join(listingNotes(report), "\n")

	assert.NotEmpty(t, notes)
	assert.NotContains(t, notes, "continue-dev", "continue-dev was removed in v5")
}
