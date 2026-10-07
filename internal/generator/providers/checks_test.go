package providers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

func TestSanitizeCheckName(t *testing.T) {
	tests := map[string]string{
		"lint":         "lint",
		"a.b_c-d":      "a.b_c-d",
		"x --> y":      "x-----y",
		"-->":          "---",
		"sp ace/slash": "sp-ace-slash",
		"":             "check",
	}
	for in, want := range tests {
		assert.Equal(t, want, sanitizeCheckName(in), in)
	}
}

func TestRenderAggregate_NameCannotCloseTheMarker(t *testing.T) {
	// Arrange
	spec := &ProviderSpec{Name: "t"}
	g := New(spec)
	cfg := &config.Config{}
	items := []config.ContentFile{{Name: "evil --> <script>", Content: "body"}}

	// Act
	out, err := g.renderAggregate("checks", &OutputSpec{File: "CHECKS.md"}, items, "/base", cfg)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, out)
	text := string(out.RawContent)
	marker := strings.SplitN(text, "\n", 3)[1]
	assert.Equal(t, 1, strings.Count(marker, "-->"), "only the marker's own terminator may appear")
	assert.Contains(t, text, "<!-- ai-rulez:check:evil------script- -->")
	assert.Contains(t, text, "## evil------script-")
}

func TestRenderAggregate_DescriptionFallbackAndTargets(t *testing.T) {
	// Arrange
	spec, err := LoadProviderSpec([]byte(`
name = "kilo"
[outputs.checks]
mode = "aggregate"
file = "REVIEW.md"
filter = "include_if_targeting_provider"
`), "k.toml", FormatAuto)
	require.NoError(t, err)
	orig := checkItems
	t.Cleanup(func() { checkItems = orig })
	checkItems = func(*config.Config, *config.ContentTree) []config.ContentFile {
		return []config.ContentFile{
			{Name: "empty-body", Metadata: &config.Metadata{Extra: map[string]string{"description": "From description."}}},
			{Name: "other-tool", Content: "Not for kilo.", Metadata: &config.Metadata{Targets: []string{"cursor"}}},
		}
	}

	// Act
	outputs, err := New(spec).Generate(&config.ContentTree{}, absSlash("/proj"), &config.Config{Name: "t"})

	// Assert
	require.NoError(t, err)
	var got string
	for _, o := range outputs {
		if o.Path == filepath.Join(absSlash("/proj"), "REVIEW.md") {
			got = string(o.RawContent)
		}
	}
	assert.Equal(t, "<!-- ai-rulez:checks:begin -->\n<!-- ai-rulez:check:empty-body -->\n\n## empty-body\n\nFrom description.\n"+
		"<!-- ai-rulez:checks:end -->\n", got)
}

func TestRenderChecks_PerItemRenamesSeverity(t *testing.T) {
	// Arrange
	spec, err := LoadProviderSpec([]byte(`
name = "x"
[outputs.checks]
mode = "per_item_file"
dir = ".x/checks"
filename = "{id}.md"
[outputs.checks.body]
sections = ["frontmatter", "content"]
[outputs.checks.frontmatter]
fields = ["description", "severity"]
tools = true
renames = { severity = "severity-default" }
`), "x.toml", FormatAuto)
	require.NoError(t, err)
	orig := checkItems
	t.Cleanup(func() { checkItems = orig })
	checkItems = func(*config.Config, *config.ContentTree) []config.ContentFile {
		return []config.ContentFile{{Name: "sec", Content: "Body.", Metadata: &config.Metadata{
			Tools: []string{"Read"}, Extra: map[string]string{"description": "D", "severity": "high"}}}}
	}

	// Act
	outputs, err := New(spec).Generate(&config.ContentTree{}, absSlash("/proj"), &config.Config{Name: "t"})

	// Assert
	require.NoError(t, err)
	var got string
	for _, o := range outputs {
		if o.Path == filepath.Join(absSlash("/proj"), ".x", "checks", "sec.md") {
			got = o.Content
		}
	}
	assert.Contains(t, got, "severity-default: high")
	assert.NotContains(t, got, "\nseverity:")
	assert.Contains(t, got, "name: sec")
}

func TestLoadProviderSpec_ChecksSidecarValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"ok augment", "[[sidecars]]\nkind = \"checks\"\npath = \"a.yaml\"\ndialect = \"augment\"", ""},
		{"missing dialect", "[[sidecars]]\nkind = \"checks\"\npath = \"a.yaml\"", "needs dialect"},
		{"unknown dialect", "[[sidecars]]\nkind = \"checks\"\npath = \"a.yaml\"\ndialect = \"standard\"", "needs dialect"},
		{"json document", "[[sidecars]]\nkind = \"checks\"\npath = \"a.json\"\ndialect = \"augment\"", "yaml document"},
		{"checks dialect on mcp", "[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\ndialect = \"augment\"", "dialect"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadProviderSpec([]byte("name = \"t\"\n"+tt.body), "t.toml", FormatAuto)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRenderAggregate_WarnsWhenKiloWouldTruncateTheFile(t *testing.T) {
	// Arrange
	spec, err := LoadProviderSpec([]byte(`
name = "kilo"
[outputs.checks]
mode = "aggregate"
file = "REVIEW.md"
`), "kilo.toml", FormatAuto)
	require.NoError(t, err)
	var warnings []string
	restore := diag.SetDefaultSink(func(msg string, _ ...any) { warnings = append(warnings, msg) })
	t.Cleanup(restore)
	g := New(spec)
	cfg := &config.Config{}

	// Act: a short file is quiet, a long one is reported once.
	_, err = g.renderAggregate("checks", &OutputSpec{File: "REVIEW.md"}, []config.ContentFile{{Name: "a", Content: "short"}}, t.TempDir(), cfg)
	require.NoError(t, err)
	quiet := len(warnings)
	_, err = g.renderAggregate("checks", &OutputSpec{File: "REVIEW.md"},
		[]config.ContentFile{{Name: "a", Content: strings.Repeat("x", 10001)}}, t.TempDir(), cfg)
	require.NoError(t, err)

	// Assert
	assert.Zero(t, quiet)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "10000")
	assert.Contains(t, warnings[0], "REVIEW.md")
}

func TestComputeItemID_ChecksUseTheSanitizedName(t *testing.T) {
	assert.Equal(t, "a-b", computeItemID(OutputTypeChecks, config.ContentFile{Name: "A b"}))
	assert.Equal(t, "x-----y", computeItemID(OutputTypeChecks, config.ContentFile{Name: "x --> y"}))
}
