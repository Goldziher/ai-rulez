package providers

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	out := g.renderAggregate("checks", &OutputSpec{File: "CHECKS.md"}, items, "/base", cfg)

	// Assert
	require.NotNil(t, out)
	marker := strings.SplitN(out.Content, "\n", 2)[0]
	assert.Equal(t, 1, strings.Count(marker, "-->"), "only the marker's own terminator may appear")
	assert.Contains(t, out.Content, "<!-- ai-rulez:check:evil------script- -->")
}
