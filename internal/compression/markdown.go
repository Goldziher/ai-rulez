package compression

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Matches fenced code blocks: ```...``` or ~~~...~~~
	fencedCodeBlockRe = regexp.MustCompile("(?s)(```.*?```|~~~.*?~~~)")
	// Matches inline code: `...`
	inlineCodeRe = regexp.MustCompile("`[^`]+`")
)

// extractCodeBlocks replaces code blocks and inline code with placeholders,
// returning the modified text and a map of placeholder -> original content.
func extractCodeBlocks(text string) (processed string, blockMap map[string]string) {
	blocks := make(map[string]string)
	counter := 0

	// Replace fenced code blocks first
	text = fencedCodeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("__CODE_BLOCK_%d__", counter)
		counter++
		blocks[placeholder] = match
		return placeholder
	})

	// Replace inline code
	text = inlineCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("__CODE_BLOCK_%d__", counter)
		counter++
		blocks[placeholder] = match
		return placeholder
	})

	return text, blocks
}

// restoreCodeBlocks replaces placeholders back with original code blocks.
func restoreCodeBlocks(text string, blocks map[string]string) string {
	for placeholder, original := range blocks {
		text = strings.ReplaceAll(text, placeholder, original)
	}
	return text
}

// isMarkdownHeader detects lines starting with # (markdown headers).
func isMarkdownHeader(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "#")
}

// isMarkdownList detects lines starting with list markers (-, *, +, or numbered).
func isMarkdownList(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return true
	}
	// Check for numbered lists like "1. "
	for i, ch := range trimmed {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' && i > 0 && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
			return true
		}
		break
	}
	return false
}

// isMarkdownTable detects lines that look like markdown table rows (| ... |).
func isMarkdownTable(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}
