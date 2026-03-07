package compression

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// scoreSentence computes a multi-factor importance score for a sentence.
//
//nolint:gocyclo // multi-factor scoring algorithm ported from kreuzberg
func scoreSentence(sentence string, position, totalSentences int) float64 {
	words := strings.Fields(sentence)
	wordCount := len(words)
	if wordCount == 0 {
		return 0
	}

	score := 0.0

	// Edge position bonus (first or last sentence)
	if position == 0 || position == totalSentences-1 {
		score += EdgePositionBonus
	}

	// Ideal word count bonus (3-25 words)
	if wordCount >= IdealWordCountMin && wordCount <= IdealWordCountMax {
		score += IdealWordCountBonus
	}

	// Numeric content weight
	numericCount := 0
	for _, word := range words {
		for _, ch := range word {
			if ch >= '0' && ch <= '9' {
				numericCount++
				break
			}
		}
	}
	if wordCount > 0 {
		score += NumericContentWeight * float64(numericCount) / float64(wordCount)
	}

	// Caps/acronym weight
	capsCount := 0
	for _, word := range words {
		if isAllCaps(word) && len(word) > 1 {
			capsCount++
		}
	}
	if wordCount > 0 {
		score += CapsAcronymWeight * float64(capsCount) / float64(wordCount)
	}

	// Long word weight
	longWordCount := 0
	for _, word := range words {
		if len(word) > LongWordThreshold {
			longWordCount++
		}
	}
	if wordCount > 0 {
		score += LongWordWeight * float64(longWordCount) / float64(wordCount)
	}

	// Punctuation density weight
	punctCount := 0
	totalChars := 0
	for _, ch := range sentence {
		totalChars++
		if unicode.IsPunct(ch) {
			punctCount++
		}
	}
	if totalChars > 0 {
		score += PunctuationDensWeight * float64(punctCount) / float64(totalChars)
	}

	// Diversity ratio weight (unique words / total words)
	uniqueWords := make(map[string]struct{})
	for _, word := range words {
		uniqueWords[strings.ToLower(word)] = struct{}{}
	}
	score += DiversityRatioWeight * float64(len(uniqueWords)) / float64(wordCount)

	// Character entropy weight
	score += CharEntropyWeight * charEntropy(sentence)

	return score
}

// charEntropy computes the Shannon entropy of characters in a string, normalized to [0,1].
func charEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	freq := make(map[rune]int)
	total := 0
	for _, ch := range s {
		freq[ch]++
		total++
	}
	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / float64(total)
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	// Normalize by max possible entropy (log2 of number of unique chars)
	maxEntropy := math.Log2(float64(len(freq)))
	if maxEntropy > 0 {
		return entropy / maxEntropy
	}
	return 0
}

// isAllCaps returns true if the word contains only uppercase letters (and possibly digits).
func isAllCaps(word string) bool {
	hasLetter := false
	for _, ch := range word {
		if unicode.IsLetter(ch) {
			hasLetter = true
			if !unicode.IsUpper(ch) {
				return false
			}
		}
	}
	return hasLetter
}

// isCamelCase detects camelCase or PascalCase words.
func isCamelCase(word string) bool {
	hasUpper := false
	hasLower := false
	for _, ch := range word {
		if unicode.IsUpper(ch) {
			hasUpper = true
		}
		if unicode.IsLower(ch) {
			hasLower = true
		}
	}
	// Must have both, and not be all caps
	return hasUpper && hasLower && !isAllCaps(word) && len(word) > 2
}

// containsDigit returns true if the word contains at least one digit.
func containsDigit(word string) bool {
	for _, ch := range word {
		if ch >= '0' && ch <= '9' {
			return true
		}
	}
	return false
}

// hasImportantCharacteristics detects important words (all-caps, contains digits, camelCase, long words).
func hasImportantCharacteristics(word string) bool {
	if isAllCaps(word) && len(word) > 1 {
		return true
	}
	if containsDigit(word) {
		return true
	}
	if isCamelCase(word) {
		return true
	}
	if len(word) > ImportantWordMinLength {
		return true
	}
	return false
}

// countAlpha returns the number of alphabetic characters in a word.
func countAlpha(word string) int {
	count := 0
	for _, ch := range word {
		if unicode.IsLetter(ch) {
			count++
		}
	}
	return count
}

// selectSentences keeps the top keepRatio fraction of sentences by score.
func selectSentences(text string, keepRatio float64) string {
	// Split into sentences (simple heuristic: split on ". ", "! ", "? ", or newlines)
	lines := strings.Split(text, "\n")

	// Treat each non-empty line as a "sentence" for scoring purposes
	type scoredLine struct {
		index int
		line  string
		score float64
	}

	var scored []scoredLine
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Preserve markdown structure lines with a high score
		if isMarkdownHeader(line) || isMarkdownTable(line) || strings.Contains(line, "__CODE_BLOCK_") {
			scored = append(scored, scoredLine{index: i, line: line, score: 1000}) // always keep
			continue
		}
		s := scoreSentence(trimmed, i, len(lines))
		scored = append(scored, scoredLine{index: i, line: line, score: s})
	}

	if len(scored) == 0 {
		return text
	}

	// Determine how many to keep
	keepCount := int(math.Ceil(float64(len(scored)) * keepRatio))
	if keepCount < 1 {
		keepCount = 1
	}
	if keepCount >= len(scored) {
		return text
	}

	// Sort by score descending to find threshold
	sortedByScore := make([]scoredLine, len(scored))
	copy(sortedByScore, scored)
	sort.Slice(sortedByScore, func(i, j int) bool {
		return sortedByScore[i].score > sortedByScore[j].score
	})

	// Mark which lines to keep
	keepSet := make(map[int]bool)
	for i := 0; i < keepCount; i++ {
		keepSet[sortedByScore[i].index] = true
	}

	// Rebuild text preserving original order, including empty lines between kept lines
	var result []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Keep blank lines if adjacent to kept content
			result = append(result, line)
			continue
		}
		if keepSet[i] {
			result = append(result, line)
		}
	}

	// Clean up trailing empty lines
	output := strings.Join(result, "\n")
	output = normalizeNewlines(output)
	return output
}

// filterWordsByImportance applies aggressive word filtering based on frequency and importance.
//
//nolint:gocyclo // frequency-based filtering algorithm ported from kreuzberg
func filterWordsByImportance(text string) string {
	lines := strings.Split(text, "\n")

	// Compute global word frequencies
	wordFreqs := make(map[string]int)
	totalWords := 0
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			lower := strings.ToLower(word)
			wordFreqs[lower]++
			totalWords++
		}
	}

	// Compute average word length
	totalLen := 0
	uniqueCount := 0
	for word := range wordFreqs {
		totalLen += len(word)
		uniqueCount++
	}
	avgLen := 0.0
	if uniqueCount > 0 {
		avgLen = float64(totalLen) / float64(uniqueCount)
	}

	avgFreq := 0.0
	if uniqueCount > 0 {
		avgFreq = float64(totalWords) / float64(uniqueCount)
	}

	result := make([]string, 0, len(lines))
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
			lower := strings.ToLower(word)
			freq := float64(wordFreqs[lower])

			keep := false
			switch {
			case hasImportantCharacteristics(word):
				keep = true
			case freq < avgFreq && float64(len(word)) > avgLen:
				// Low frequency + above average length -> likely important
				keep = true
			case countAlpha(word) >= ImportantCharMinAlpha && hasImportantCharacteristics(word):
				keep = true
			}

			if keep {
				filtered = append(filtered, word)
			}
		}

		if len(filtered) > 0 {
			result = append(result, strings.Join(filtered, " "))
		}
		// If nothing survived, drop the line entirely (aggressive)
	}

	return strings.Join(result, "\n")
}
