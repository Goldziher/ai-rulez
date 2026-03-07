package compression

import (
	"github.com/Goldziher/ai-rulez/internal/config"
)

// Compressor implements multi-level token reduction.
type Compressor struct {
	config *config.CompressionConfig
	stats  *CompressionStats
}

// CompressionStats tracks the reduction metrics.
type CompressionStats struct {
	OriginalSize   int
	CompressedSize int
	Ratio          float64
}

// NewCompressor creates a new Compressor with the given configuration.
func NewCompressor(cfg *config.CompressionConfig) *Compressor {
	return &Compressor{
		config: cfg,
		stats:  &CompressionStats{},
	}
}

// Compress applies token reduction to the content based on the configured level.
func (c *Compressor) Compress(content string) string {
	c.stats.OriginalSize = len(content)

	level := c.config.GetCompressionLevel()
	if level == LevelOff {
		c.stats.CompressedSize = len(content)
		return content
	}

	preserveCode := c.config.ShouldPreserveCode()
	preserveMarkdown := c.config.ShouldPreserveMarkdown()

	// Extract code blocks if preservation is enabled
	var codeBlocks map[string]string
	if preserveCode {
		content, codeBlocks = extractCodeBlocks(content)
	}

	switch level {
	case LevelLight:
		content = c.applyLight(content)
	case LevelModerate:
		content = c.applyModerate(content, preserveMarkdown)
	case LevelAggressive:
		content = c.applyAggressive(content, preserveMarkdown)
	case LevelMaximum:
		content = c.applyMaximum(content, preserveMarkdown)
	}

	// Restore code blocks
	if preserveCode && codeBlocks != nil {
		content = restoreCodeBlocks(content, codeBlocks)
	}

	c.stats.CompressedSize = len(content)
	c.calculateRatio()

	return content
}

// GetStats returns the compression statistics.
func (c *Compressor) GetStats() *CompressionStats {
	return c.stats
}

func (c *Compressor) calculateRatio() {
	if c.stats.OriginalSize > 0 {
		saved := c.stats.OriginalSize - c.stats.CompressedSize
		c.stats.Ratio = float64(saved) / float64(c.stats.OriginalSize) * 100
	}
}

// applyLight: punctuation cleanup, whitespace normalization, HTML comment removal.
func (c *Compressor) applyLight(content string) string {
	content = cleanPunctuation(content)
	content = normalizeSpaces(content)
	content = normalizeNewlines(content)
	content = removeHTMLComments(content)
	return content
}

// applyModerate: light + stopword removal with markdown-aware processing.
func (c *Compressor) applyModerate(content string, preserveMarkdown bool) string {
	content = c.applyLight(content)
	content = removeStopwords(content, englishStopwords, preserveMarkdown)
	return content
}

// applyAggressive: moderate + word filtering by frequency/importance, sentence selection (keep top 40%).
func (c *Compressor) applyAggressive(content string, preserveMarkdown bool) string {
	content = c.applyModerate(content, preserveMarkdown)
	content = filterWordsByImportance(content)
	content = selectSentences(content, SentenceKeepRatio)
	return content
}

// applyMaximum: aggressive + semantic token scoring, hypernym compression.
func (c *Compressor) applyMaximum(content string, preserveMarkdown bool) string {
	content = c.applyAggressive(content, preserveMarkdown)
	content = applySemanticTokenScoring(content, SemanticFilterThreshold)
	content = applyHypernymCompression(content)
	return content
}
