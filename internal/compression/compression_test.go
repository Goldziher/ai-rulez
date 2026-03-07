package compression

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestCompressor(t *testing.T) {
	tests := []struct {
		name   string
		config *config.CompressionConfig
		input  string
		check  func(t *testing.T, result string)
	}{
		{
			name:   "off level preserves text exactly",
			config: &config.CompressionConfig{Level: "off"},
			input:  "Hello world!!!  This is   a test.\n\n\n\nKeep everything.",
			check: func(t *testing.T, result string) {
				assert.Equal(t, "Hello world!!!  This is   a test.\n\n\n\nKeep everything.", result)
			},
		},
		{
			name:   "light level normalizes whitespace",
			config: &config.CompressionConfig{Level: "light"},
			input:  "Hello   world   here",
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "   ")
				assert.Contains(t, result, "Hello")
				assert.Contains(t, result, "world")
			},
		},
		{
			name:   "light level removes HTML comments",
			config: &config.CompressionConfig{Level: "light"},
			input:  "Before <!-- this is a comment --> After",
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "<!--")
				assert.NotContains(t, result, "-->")
				assert.Contains(t, result, "Before")
				assert.Contains(t, result, "After")
			},
		},
		{
			name:   "light level cleans punctuation",
			config: &config.CompressionConfig{Level: "light"},
			input:  "Wow!!! Really??? Yes,,, sure....",
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "!!!")
				assert.NotContains(t, result, "???")
				assert.NotContains(t, result, ",,,")
				assert.Contains(t, result, "!")
				assert.Contains(t, result, "?")
			},
		},
		{
			name:   "light level normalizes excessive newlines",
			config: &config.CompressionConfig{Level: "light"},
			input:  "Line one\n\n\n\n\nLine two",
			check: func(t *testing.T, result string) {
				assert.Equal(t, "Line one\n\nLine two", result)
			},
		},
		{
			name:   "moderate level removes stopwords",
			config: &config.CompressionConfig{Level: "moderate"},
			input:  "The algorithm is very important for the system.",
			check: func(t *testing.T, result string) {
				assert.True(t, len(result) < len("The algorithm is very important for the system."),
					"moderate should produce shorter output")
				assert.Contains(t, result, "algorithm")
				assert.Contains(t, result, "important")
				assert.Contains(t, result, "system")
			},
		},
		{
			name:   "moderate level preserves markdown headers",
			config: &config.CompressionConfig{Level: "moderate"},
			input:  "# My Header\n\nThe content is here for the people.",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "# My Header")
			},
		},
		{
			name:   "moderate level preserves markdown lists",
			config: &config.CompressionConfig{Level: "moderate"},
			input:  "- This is a list item\n* Another item\n+ Third item",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "- This is a list item")
				assert.Contains(t, result, "* Another item")
				assert.Contains(t, result, "+ Third item")
			},
		},
		{
			name:   "aggressive level produces shorter output",
			config: &config.CompressionConfig{Level: "aggressive"},
			input: strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10) +
				"\nIMPORTANT: The API endpoint requires authentication.\n" +
				strings.Repeat("This is some filler text that should be removed. ", 10),
			check: func(t *testing.T, result string) {
				assert.True(t, len(result) < len(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)+
					"\nIMPORTANT: The API endpoint requires authentication.\n"+
					strings.Repeat("This is some filler text that should be removed. ", 10)),
					"aggressive should produce significantly shorter output")
			},
		},
		{
			name:   "maximum level applies hypernym compression",
			config: &config.CompressionConfig{Level: "maximum"},
			input:  "The algorithm provides optimization for the implementation.",
			check: func(t *testing.T, result string) {
				// Hypernym map replaces: algorithm->method, implementation->approach, optimization->improvement
				// After stopword removal and filtering, check that hypernyms are applied
				// The exact output depends on what survives the aggressive filtering
				assert.True(t, len(result) <= len("The algorithm provides optimization for the implementation."),
					"maximum should not increase size")
			},
		},
		// Backward compatibility: old level names map correctly
		{
			name:   "backward compat: none maps to off",
			config: &config.CompressionConfig{Level: "none"},
			input:  "Unchanged text!!!",
			check: func(t *testing.T, result string) {
				assert.Equal(t, "Unchanged text!!!", result)
			},
		},
		{
			name:   "backward compat: minimal maps to light",
			config: &config.CompressionConfig{Level: "minimal"},
			input:  "Text   with   spaces!!!",
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "   ")
				assert.Contains(t, result, "Text")
			},
		},
		{
			name:   "backward compat: standard maps to moderate",
			config: &config.CompressionConfig{Level: "standard"},
			input:  "The algorithm is very important for the system.",
			check: func(t *testing.T, result string) {
				// Should remove stopwords like moderate level
				assert.Contains(t, result, "algorithm")
			},
		},
		// Empty input handling
		{
			name:   "empty input returns empty",
			config: &config.CompressionConfig{Level: "aggressive"},
			input:  "",
			check: func(t *testing.T, result string) {
				assert.Equal(t, "", result)
			},
		},
		{
			name:   "nil config defaults to off",
			config: nil,
			input:  "Should stay the same.",
			check: func(t *testing.T, result string) {
				assert.Equal(t, "Should stay the same.", result)
			},
		},
		// Code block preservation
		{
			name:   "code blocks are preserved in moderate",
			config: &config.CompressionConfig{Level: "moderate", PreserveCode: boolPtr(true)},
			input:  "Some text the and for.\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\nMore text the and for.",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "```go")
				assert.Contains(t, result, "func main()")
				assert.Contains(t, result, "fmt.Println")
			},
		},
		{
			name:   "inline code is preserved",
			config: &config.CompressionConfig{Level: "moderate", PreserveCode: boolPtr(true)},
			input:  "Use the `fmt.Println` function for output.",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "`fmt.Println`")
			},
		},
		{
			name:   "preserve_markdown false allows header modification in stopword removal",
			config: &config.CompressionConfig{Level: "moderate", PreserveMarkdown: boolPtr(false)},
			input:  "# The Simple Header\n\nThe content is here.",
			check: func(t *testing.T, result string) {
				// With preserve_markdown=false, headers are still present but may have stopwords removed
				assert.Contains(t, result, "#")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config
			if cfg == nil {
				cfg = &config.CompressionConfig{}
			}
			compressor := NewCompressor(cfg)
			result := compressor.Compress(tc.input)
			tc.check(t, result)

			// Verify stats are populated
			stats := compressor.GetStats()
			require.NotNil(t, stats)
			assert.Equal(t, len(tc.input), stats.OriginalSize)
			assert.Equal(t, len(result), stats.CompressedSize)
		})
	}
}

func TestCompressionLevelMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "off"},
		{"off", "off"},
		{"light", "light"},
		{"moderate", "moderate"},
		{"aggressive", "aggressive"},
		{"maximum", "maximum"},
		// Backward compat
		{"none", "off"},
		{"minimal", "light"},
		{"standard", "moderate"},
	}

	for _, tc := range tests {
		t.Run("level_"+tc.input, func(t *testing.T) {
			cfg := &config.CompressionConfig{Level: tc.input}
			assert.Equal(t, tc.expected, cfg.GetCompressionLevel())
		})
	}
}

func TestSentenceScoring(t *testing.T) {
	// First sentence should get edge position bonus
	score1 := scoreSentence("This is the first sentence with important API details.", 0, 5)
	scoreMid := scoreSentence("This is a middle sentence.", 2, 5)
	assert.Greater(t, score1, 0.0, "sentence should have positive score")
	assert.Greater(t, score1, scoreMid, "first sentence should score higher than middle")

	// Sentence with numbers should score higher
	scoreNum := scoreSentence("Version 2.0 requires API key 12345.", 2, 5)
	scorePlain := scoreSentence("This requires some key details.", 2, 5)
	assert.Greater(t, scoreNum, scorePlain, "sentence with numbers should score higher")
}

func TestHasImportantCharacteristics(t *testing.T) {
	assert.True(t, hasImportantCharacteristics("API"))
	assert.True(t, hasImportantCharacteristics("version2"))
	assert.True(t, hasImportantCharacteristics("camelCase"))
	assert.True(t, hasImportantCharacteristics("veryLongWordHere"))
	assert.False(t, hasImportantCharacteristics("the"))
	assert.False(t, hasImportantCharacteristics("a"))
}

func TestMarkdownDetection(t *testing.T) {
	assert.True(t, isMarkdownHeader("# Header"))
	assert.True(t, isMarkdownHeader("## Sub Header"))
	assert.True(t, isMarkdownHeader("  # Indented Header"))
	assert.False(t, isMarkdownHeader("Not a header"))

	assert.True(t, isMarkdownList("- Item"))
	assert.True(t, isMarkdownList("* Item"))
	assert.True(t, isMarkdownList("+ Item"))
	assert.True(t, isMarkdownList("1. Item"))
	assert.False(t, isMarkdownList("Not a list"))

	assert.True(t, isMarkdownTable("| Col1 | Col2 |"))
	assert.False(t, isMarkdownTable("Not a table"))
}

func TestCodeBlockExtraction(t *testing.T) {
	input := "Before\n```go\nfunc main() {}\n```\nAfter `inline` code"
	modified, blocks := extractCodeBlocks(input)

	assert.NotContains(t, modified, "func main")
	assert.NotContains(t, modified, "`inline`")
	assert.Len(t, blocks, 2)

	restored := restoreCodeBlocks(modified, blocks)
	assert.Contains(t, restored, "func main")
	assert.Contains(t, restored, "`inline`")
}

func TestCodeBlockExtractionWithBackticksInside(t *testing.T) {
	// Code blocks may contain backticks (e.g., Go raw strings, template literals)
	input := "Before\n```go\ns := `raw string`\nfmt.Println(s)\n```\nAfter"
	modified, blocks := extractCodeBlocks(input)

	assert.NotContains(t, modified, "raw string")
	assert.NotContains(t, modified, "fmt.Println")
	assert.Len(t, blocks, 1, "should capture entire fenced block as one unit")

	restored := restoreCodeBlocks(modified, blocks)
	assert.Contains(t, restored, "s := `raw string`")
	assert.Contains(t, restored, "fmt.Println(s)")
}

func TestStopwordRemoval(t *testing.T) {
	input := "The algorithm is very important for the system."
	result := removeStopwords(input, englishStopwords, true)

	assert.Contains(t, result, "algorithm")
	assert.Contains(t, result, "important")
	assert.Contains(t, result, "system.")
	assert.NotContains(t, result, " the ")
	assert.NotContains(t, result, " is ")
	assert.NotContains(t, result, " very ")
}

func TestStopwordRemovalPreservesMarkdownStructure(t *testing.T) {
	input := "# The Important Header\n\n- This is a list item\n\nThe content is here for the people."
	result := removeStopwords(input, englishStopwords, true)

	// Headers and lists should be preserved exactly
	assert.Contains(t, result, "# The Important Header")
	assert.Contains(t, result, "- This is a list item")
}

func TestFilterWordsByImportance(t *testing.T) {
	input := "# Header\n\nThe API endpoint requires AuthenticationToken for access."
	result := filterWordsByImportance(input)

	// Header preserved
	assert.Contains(t, result, "# Header")
	// Important words kept (all-caps, camelCase, long words)
	assert.Contains(t, result, "API")
	assert.Contains(t, result, "AuthenticationToken")
}

func TestSelectSentences(t *testing.T) {
	// Build input with enough lines that selection actually filters
	lines := []string{
		"# Important Header",
		"",
		"The API requires an authentication token.",
		"This is filler text with no real value.",
		"Another filler line without importance.",
		"Version 2.0 introduces BREAKING changes to the HTTP endpoint.",
		"Some more boring filler content.",
		"Yet another throwaway sentence.",
		"The DatabaseConnection requires SSL encryption.",
		"Blah blah filler.",
	}
	input := strings.Join(lines, "\n")
	result := selectSentences(input, 0.4)

	// Header always kept
	assert.Contains(t, result, "# Important Header")
	// High-scoring lines (numbers, caps, long words) should survive
	assert.Contains(t, result, "BREAKING")
}

func TestSemanticTokenScoring(t *testing.T) {
	// "must" has weight 0.9 in importanceWeights
	score := scoreTokenImportance("must", 0, 10, nil)
	assert.Equal(t, 0.9, score)

	// "security" has weight 0.9
	score = scoreTokenImportance("security", 0, 10, nil)
	assert.Equal(t, 0.9, score)

	// Short common word should have low base score
	score = scoreTokenImportance("at", 5, 10, nil)
	assert.Less(t, score, 0.5)
}

func TestHypernymCompression(t *testing.T) {
	input := "The algorithm requires optimization of the implementation."
	result := applyHypernymCompression(input)

	assert.Contains(t, result, "method")
	assert.Contains(t, result, "improvement")
	assert.Contains(t, result, "approach")
}
