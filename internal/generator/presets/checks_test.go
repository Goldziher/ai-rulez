package presets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestAllChecks_RootBeatsDomainAndSortsByName(t *testing.T) {
	// Arrange
	content := &config.ContentTree{
		Checks: []config.ContentFile{{Name: "b", Content: "root b"}},
		Domains: map[string]*config.Domain{
			"d": {Name: "d", Checks: []config.ContentFile{{Name: "b", Content: "domain b"}, {Name: "a", Content: "domain a"}}},
		},
	}

	// Act
	got := AllChecks(nil, content)

	// Assert
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Name)
	assert.Equal(t, "root b", got[1].Content)
}

func TestRenderCheckSections(t *testing.T) {
	tests := []struct {
		name   string
		checks []config.ContentFile
		header string
		want   string
	}{
		{"none", nil, "# H", ""},
		{"body", []config.ContentFile{{Name: "x", Content: "Body.\n"}}, "# H\n", "# H\n\n<!-- ai-rulez:check:x -->\n\n## x\n\nBody.\n"},
		{"description fallback", []config.ContentFile{{Name: "x", Metadata: &config.Metadata{Extra: map[string]string{"description": "Desc"}}}}, "",
			"<!-- ai-rulez:check:x -->\n\n## x\n\nDesc\n"},
		{"nothing to say", []config.ContentFile{{Name: "x"}}, "", "<!-- ai-rulez:check:x -->\n\n## x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RenderCheckSections(tt.checks, tt.header))
		})
	}
}

func TestCursor_ChecksCollapseIntoBugbotFile(t *testing.T) {
	// Arrange
	content := &config.ContentTree{Checks: []config.ContentFile{
		{Name: "security", Content: "Flag injection."},
		{Name: "kilo-only", Content: "Not here.", Metadata: &config.Metadata{Targets: []string{"kilo"}}},
	}}

	// Act
	outputs, err := (&CursorPresetGenerator{}).Generate(content, "/proj", &config.Config{Name: "t", BaseDir: "/proj"})

	// Assert
	require.NoError(t, err)
	var got config.OutputFile
	for _, o := range outputs {
		if o.Path == filepath.Join("/proj", ".cursor", "BUGBOT.md") {
			got = o
		}
	}
	assert.Equal(t, "<!-- ai-rulez:checks:begin -->\n<!-- ai-rulez:check:security -->\n\n## security\n\nFlag injection.\n"+
		"<!-- ai-rulez:checks:end -->\n", string(got.RawContent))
	assert.True(t, got.Committed)
	assert.False(t, got.PartiallyOwned)
	assert.NotEmpty(t, got.MergeClaims)
}

func TestCursor_NoChecksNoBugbotFile(t *testing.T) {
	outputs, err := (&CursorPresetGenerator{}).Generate(&config.ContentTree{}, "/proj", &config.Config{Name: "t", BaseDir: "/proj"})

	require.NoError(t, err)
	for _, o := range outputs {
		assert.NotEqual(t, filepath.Join("/proj", ".cursor", "BUGBOT.md"), o.Path)
	}
}

func TestCursor_ChecksKeepAHandWrittenBugbotFile(t *testing.T) {
	// Arrange
	base := t.TempDir()
	path := filepath.Join(base, ".cursor", "BUGBOT.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("# Team Bugbot rules\n\nNever approve force pushes.\n"), 0o600))
	content := &config.ContentTree{Checks: []config.ContentFile{{Name: "security", Content: "Flag injection."}}}

	// Act
	outputs, err := (&CursorPresetGenerator{}).Generate(content, base, &config.Config{Name: "t", BaseDir: base})

	// Assert
	require.NoError(t, err)
	var got config.OutputFile
	for _, o := range outputs {
		if o.Path == path {
			got = o
		}
	}
	assert.True(t, strings.HasPrefix(string(got.RawContent), "# Team Bugbot rules\n\nNever approve force pushes.\n"))
	assert.Contains(t, string(got.RawContent), "Flag injection.")
	assert.True(t, got.PartiallyOwned, "a file with the user's text is shared, never deleted or ignored whole")
}

func TestCursor_ChecksSkippedInUserScope(t *testing.T) {
	content := &config.ContentTree{Checks: []config.ContentFile{{Name: "security", Content: "Flag injection."}}}

	outputs, err := (&CursorPresetGenerator{}).Generate(content, "/home/u", &config.Config{Name: "t", BaseDir: "/home/u", UserScope: true})

	require.NoError(t, err)
	for _, o := range outputs {
		assert.NotContains(t, o.Path, "BUGBOT.md", "no per-user Bugbot file is documented")
	}
}

func TestAllChecks_SkipsInvalidNamesAndCaseCollisions(t *testing.T) {
	// Arrange
	content := &config.ContentTree{
		Checks: []config.ContentFile{
			{Name: "Security", Content: "root"},
			{Name: "../escape", Content: "traversal"},
			{Name: "has space", Content: "space"},
		},
		Domains: map[string]*config.Domain{
			"d": {Name: "d", Checks: []config.ContentFile{{Name: "security", Content: "domain, other case"}, {Name: "perf", Content: "ok"}}},
		},
	}

	// Act
	got := AllChecks(nil, content)

	// Assert
	names := make([]string, len(got))
	for i, c := range got {
		names[i] = c.Name
	}
	assert.Equal(t, []string{"Security", "perf"}, names)
	assert.Equal(t, "root", got[0].Content, "the higher-precedence check wins the case-insensitive collision")
}

func TestRenderCheckSections_UsesTheSanitizedNameInHeadings(t *testing.T) {
	got := RenderCheckSections([]config.ContentFile{{Name: "a b\n# injected", Content: "x"}}, "")

	assert.NotContains(t, got, "\n# injected")
	assert.Contains(t, got, "## a-b---injected")
}
