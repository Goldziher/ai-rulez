package compression

import (
	"strings"
	"unicode"
)

// Pre-computed importance weights for common analytical words
var importanceWeights = map[string]float64{
	"must":       0.9,
	"shall":      0.9,
	"required":   0.9,
	"critical":   0.95,
	"important":  0.85,
	"essential":  0.85,
	"necessary":  0.8,
	"ensure":     0.8,
	"always":     0.8,
	"never":      0.8,
	"error":      0.85,
	"warning":    0.7,
	"security":   0.9,
	"vulnerable": 0.9,
	"deprecated": 0.8,
	"forbidden":  0.9,
	"mandatory":  0.85,
	"optional":   0.6,
	"default":    0.7,
	"configure":  0.7,
	"parameter":  0.65,
	"function":   0.7,
	"method":     0.7,
	"class":      0.7,
	"interface":  0.7,
	"struct":     0.7,
	"type":       0.65,
	"variable":   0.65,
	"constant":   0.65,
	"return":     0.7,
	"exception":  0.75,
	"validate":   0.7,
	"process":    0.6,
	"execute":    0.7,
	"initialize": 0.7,
	"create":     0.65,
	"delete":     0.65,
	"update":     0.65,
	"read":       0.6,
	"write":      0.6,
}

// Hypernym map: replace specific words with general categories
var hypernymMap = map[string]string{
	"algorithm":      "method",
	"implementation": "approach",
	"optimization":   "improvement",
	"computer":       "device",
	"laptop":         "device",
	"desktop":        "device",
	"server":         "device",
	"database":       "storage",
	"repository":     "storage",
	"directory":      "location",
	"folder":         "location",
	"javascript":     "language",
	"typescript":     "language",
	"python":         "language",
	"golang":         "language",
	"ruby":           "language",
	"java":           "language",
	"framework":      "tool",
	"library":        "tool",
	"package":        "module",
	"dependency":     "module",
	"authentication": "auth",
	"authorization":  "auth",
	"application":    "app",
	"component":      "part",
	"element":        "part",
	"specification":  "spec",
	"configuration":  "config",
	"environment":    "env",
	"development":    "dev",
	"production":     "prod",
	"performance":    "perf",
	"functionality":  "feature",
	"capability":     "feature",
	"requirement":    "need",
	"prerequisite":   "need",
}

// isTechnicalTerm checks if a word looks like a technical term.
func isTechnicalTerm(word string) bool {
	if len(word) <= TechnicalTermMinLength {
		return false
	}
	// Contains underscore (snake_case)
	if strings.Contains(word, "_") {
		return true
	}
	// Has multiple uppercase letters (camelCase, PascalCase, acronyms)
	upperCount := 0
	for _, ch := range word {
		if unicode.IsUpper(ch) {
			upperCount++
		}
	}
	if upperCount >= 2 {
		return true
	}
	// Ends with common suffixes
	lower := strings.ToLower(word)
	if strings.HasSuffix(lower, "tion") || strings.HasSuffix(lower, "ment") || strings.HasSuffix(lower, "ing") {
		return true
	}
	return false
}

// scoreTokenImportance computes the importance of a single token/word.
//
//nolint:gocyclo // multi-factor token scoring algorithm ported from kreuzberg
func scoreTokenImportance(word string, position, totalWords int, wordFreqs map[string]int) float64 {
	lower := strings.ToLower(word)

	// Check pre-computed weights first
	if weight, ok := importanceWeights[lower]; ok {
		return weight
	}

	// Base importance
	importance := SemanticBaseImportance

	// Length bonus (longer words tend to be more meaningful)
	if len(word) > 4 {
		importance += float64(len(word)-4) * 0.02
		if importance > 0.15 {
			importance = SemanticBaseImportance + 0.15
		}
	}

	// Uppercase bonus
	if isAllCaps(word) && len(word) > 1 {
		importance += 0.2
	}

	// Digit bonus
	if containsDigit(word) {
		importance += 0.1
	}

	// Technical term bonus
	if isTechnicalTerm(word) {
		importance += 0.2
	}

	// Position bonus (words at start/end of text tend to be more important)
	if totalWords > 0 {
		relPos := float64(position) / float64(totalWords)
		if relPos < 0.1 || relPos > 0.9 {
			importance += 0.05
		}
	}

	// Frequency-based adjustment: rare words are often more important
	if wordFreqs != nil {
		freq := wordFreqs[lower]
		totalF := 0
		for _, f := range wordFreqs {
			totalF += f
		}
		if totalF > 0 && freq > 0 {
			relFreq := float64(freq) / float64(totalF)
			if relFreq < 0.01 {
				importance += 0.1 // rare word bonus
			}
		}
	}

	return importance
}

// applySemanticTokenScoring filters tokens below the importance threshold.
func applySemanticTokenScoring(text string, threshold float64) string {
	lines := strings.Split(text, "\n")

	// Compute global word frequencies
	wordFreqs := make(map[string]int)
	totalWords := 0
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			wordFreqs[strings.ToLower(word)]++
			totalWords++
		}
	}

	result := make([]string, 0, len(lines))
	wordPosition := 0

	for _, line := range lines {
		// Preserve structure
		if isMarkdownHeader(line) || isMarkdownList(line) || isMarkdownTable(line) ||
			strings.Contains(line, "__CODE_BLOCK_") || strings.TrimSpace(line) == "" {
			result = append(result, line)
			continue
		}

		words := strings.Fields(line)
		filtered := make([]string, 0, len(words))

		for _, word := range words {
			score := scoreTokenImportance(word, wordPosition, totalWords, wordFreqs)
			wordPosition++
			if score >= threshold {
				filtered = append(filtered, word)
			}
		}

		if len(filtered) > 0 {
			result = append(result, strings.Join(filtered, " "))
		}
	}

	return strings.Join(result, "\n")
}

// applyHypernymCompression replaces specific words with general categories.
func applyHypernymCompression(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	// Process word by word, preserving whitespace structure
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		// Preserve structure
		if isMarkdownHeader(line) || isMarkdownTable(line) ||
			strings.Contains(line, "__CODE_BLOCK_") || strings.TrimSpace(line) == "" {
			result = append(result, line)
			continue
		}

		lineWords := strings.Fields(line)
		for i, word := range lineWords {
			lower := strings.ToLower(word)
			// Strip punctuation for lookup
			cleaned := strings.TrimRight(lower, ".,;:!?\"')")
			cleaned = strings.TrimLeft(cleaned, "\"'(")

			if replacement, ok := hypernymMap[cleaned]; ok {
				// Preserve original casing style
				if isAllCaps(word) {
					replacement = strings.ToUpper(replacement)
				} else if word != "" && unicode.IsUpper(rune(word[0])) {
					replacement = strings.ToUpper(replacement[:1]) + replacement[1:]
				}
				// Preserve trailing punctuation
				suffix := word[len(strings.TrimRight(word, ".,;:!?\"')")):]
				prefix := word[:len(word)-len(strings.TrimLeft(word, "\"'("))]
				lineWords[i] = prefix + replacement + suffix
			}
		}

		result = append(result, strings.Join(lineWords, " "))
	}

	return strings.Join(result, "\n")
}
