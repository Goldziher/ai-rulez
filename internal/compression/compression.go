package compression

import (
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type Compressor struct {
	config *config.CompressionConfig
	stats  *CompressionStats
}

type CompressionStats struct {
	OriginalSize   int
	CompressedSize int
	Ratio          float64
}

func NewCompressor(cfg *config.CompressionConfig) *Compressor {
	return &Compressor{
		config: cfg,
		stats:  &CompressionStats{},
	}
}

func (c *Compressor) Compress(content string) string {
	c.stats.OriginalSize = len(content)

	if c.config.ShouldPreserveFormatting() {
		return content
	}

	level := c.config.GetCompressionLevel()

	switch level {
	case "none":
		return content
	case "minimal":
		content = c.applyMinimalCompression(content)
	case "standard":
		content = c.applyStandardCompression(content)
	case "aggressive":
		content = c.applyAggressiveCompression(content)
	}

	c.stats.CompressedSize = len(content)
	c.calculateRatio()

	return content
}

func (c *Compressor) GetStats() *CompressionStats {
	return c.stats
}

func (c *Compressor) calculateRatio() {
	if c.stats.OriginalSize > 0 {
		saved := c.stats.OriginalSize - c.stats.CompressedSize
		c.stats.Ratio = float64(saved) / float64(c.stats.OriginalSize) * 100
	}
}

// Compression methods
func (c *Compressor) applyMinimalCompression(content string) string {
	content = c.trimTrailingWhitespace(content)
	content = c.removeExcessiveBlankLines(content)
	return content
}

func (c *Compressor) applyStandardCompression(content string) string {
	content = c.applyMinimalCompression(content)
	content = c.compactPriorityLabels(content)

	// Future: implement content deduplication
	// if c.config.ShouldRemoveDuplicates() {
	//     content = c.deduplicateContent(content)
	// }

	return content
}

func (c *Compressor) applyAggressiveCompression(content string) string {
	content = c.applyStandardCompression(content)

	if c.config.ShouldUseAbbreviations() {
		content = c.applyAbbreviations(content)
	}

	return content
}

func (c *Compressor) trimTrailingWhitespace(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}

func (c *Compressor) removeExcessiveBlankLines(content string) string {
	re := regexp.MustCompile(`\n{3,}`)
	return re.ReplaceAllString(content, "\n\n")
}

func (c *Compressor) compactPriorityLabels(content string) string {
	content = strings.ReplaceAll(content, "**Priority:**", "Priority:")
	return content
}

func (c *Compressor) applyAbbreviations(content string) string {
	content = strings.ReplaceAll(content, "Priority:", "P:")
	content = strings.ReplaceAll(content, "Description:", "Desc:")
	return content
}
