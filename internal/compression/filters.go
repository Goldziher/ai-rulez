package compression

import (
	"regexp"
	"strings"
)

var (
	repeatedExclamation = regexp.MustCompile(`!{2,}`)
	repeatedQuestion    = regexp.MustCompile(`\?{2,}`)
	repeatedComma       = regexp.MustCompile(`,{2,}`)
	repeatedPeriod      = regexp.MustCompile(`\.{4,}`) // preserve ... (ellipsis)
	multipleSpaces      = regexp.MustCompile(` {2,}`)
	excessiveNewlines   = regexp.MustCompile(`\n{3,}`)
	htmlCommentRe       = regexp.MustCompile(`<!--[\s\S]*?-->`)
)

// cleanPunctuation collapses repeated punctuation marks to single instances.
func cleanPunctuation(text string) string {
	text = repeatedExclamation.ReplaceAllString(text, "!")
	text = repeatedQuestion.ReplaceAllString(text, "?")
	text = repeatedComma.ReplaceAllString(text, ",")
	text = repeatedPeriod.ReplaceAllString(text, "...")
	return text
}

// normalizeSpaces collapses 2+ consecutive spaces to a single space.
func normalizeSpaces(text string) string {
	return multipleSpaces.ReplaceAllString(text, " ")
}

// normalizeNewlines collapses 3+ consecutive newlines to 2.
func normalizeNewlines(text string) string {
	return excessiveNewlines.ReplaceAllString(text, "\n\n")
}

// removeHTMLComments strips <!-- ... --> comments from text.
func removeHTMLComments(text string) string {
	return htmlCommentRe.ReplaceAllString(text, "")
}

// removeStopwords removes stopwords from text while preserving markdown structure.
func removeStopwords(text string, stopwords map[string]struct{}, preserveMarkdown bool) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		if preserveMarkdown {
			// Preserve markdown structure lines
			if isMarkdownHeader(line) || isMarkdownList(line) || isMarkdownTable(line) {
				result = append(result, line)
				continue
			}
			// Preserve empty lines
			if strings.TrimSpace(line) == "" {
				result = append(result, line)
				continue
			}
			// Check for code block placeholders
			if strings.Contains(line, "__CODE_BLOCK_") {
				result = append(result, line)
				continue
			}
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, line)
			continue
		}

		filtered := make([]string, 0, len(words))
		for _, word := range words {
			lower := strings.ToLower(word)
			// Strip trailing punctuation for lookup
			cleaned := strings.TrimRight(lower, ".,;:!?\"')")
			cleaned = strings.TrimLeft(cleaned, "\"'(")
			if _, isStop := stopwords[cleaned]; !isStop {
				filtered = append(filtered, word)
			}
		}

		if len(filtered) > 0 {
			result = append(result, strings.Join(filtered, " "))
		} else {
			// If all words were stopwords, keep the original line to avoid losing meaning
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
