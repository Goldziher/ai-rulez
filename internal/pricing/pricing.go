// Package pricing is the one built-in model price table. internal/llm prices
// model calls with it and internal/evals prices eval runs and their estimates
// with it, so a price is updated in one place and the two never drift.
package pricing

import "strings"

// Version identifies the table. Bump it whenever a price changes: internal/llm
// folds it into its cache identity, so a cost recorded under old prices is never
// replayed under new ones.
const Version = "2026-10-07.1"

// Price is a model price in USD per million tokens.
type Price struct {
	InPerMTok  float64 `json:"in_per_mtok"`
	OutPerMTok float64 `json:"out_per_mtok"`
}

// table is a small, approximate list for common models. It is a convenience for
// budget estimates, not a billing source: prices change, and unknown models have
// no entry. Keys are matched against the model name with any provider prefix
// removed, longest prefix first. The Gemini rows are the paid-tier standard text
// prices from https://ai.google.dev/gemini-api/docs/pricing, checked 2026-10-06
// (gemini-embedding-001 is the figure documented for that model; the page now
// lists its successor first, so re-check it when updating). The OpenAI rows and the Claude rows were
// checked 2026-10-07 against developers.openai.com/api/docs/pricing and
// platform.claude.com/docs/en/about-claude/pricing. The versioned Claude rows (the 4.5 and later Opus
// models, Sonnet 5 and 5.5, Haiku 5.5) are the base prices; Haiku 5.5 is listed at its dearer
// over-100,000-token tier, so an estimate never undershoots. A Claude model without a versioned row
// (Opus 4.1 and earlier, Sonnet 4.x, Haiku 4.x) falls back to the family row, which keeps the old, dearer
// price. The bare haiku, sonnet and opus keys are the short names eval cases and --model use and stay
// at those conservative prices because they do not name a version.
var table = map[string]Price{
	"gpt-4o-mini":            {0.15, 0.60},
	"gpt-4o":                 {2.50, 10.00},
	"gpt-4.1-mini":           {0.40, 1.60},
	"gpt-4.1":                {2.00, 8.00},
	"text-embedding-3-small": {0.02, 0},
	"text-embedding-3-large": {0.13, 0},
	"gemini-2.5-flash-lite":  {0.10, 0.40},
	"gemini-2.5-flash":       {0.30, 2.50},
	"gemini-embedding-001":   {0.15, 0},
	"claude-haiku-5-5":       {0.50, 2.50},
	"claude-haiku":           {1.00, 5.00},
	"claude-sonnet-5":        {2.00, 10.00},
	"claude-sonnet":          {3.00, 15.00},
	"claude-opus-5-5":        {4.00, 20.00},
	"claude-opus-5":          {5.00, 25.00},
	"claude-opus-4-8":        {5.00, 25.00},
	"claude-opus-4-7":        {5.00, 25.00},
	"claude-opus-4-6":        {5.00, 25.00},
	"claude-opus-4-5":        {5.00, 25.00},
	"claude-opus":            {15.00, 75.00},
	"haiku":                  {1.00, 5.00},
	"sonnet":                 {3.00, 15.00},
	"opus":                   {15.00, 75.00},
}

// Lookup returns the built-in price of a model. known is false when no entry
// matches.
//
// The longest table key that prefixes the name wins. What follows the key must be
// a plain snapshot suffix (a date, a version number, "latest" or "preview"); any
// other suffix names a variant (realtime, audio, tts, image, ...) that may cost
// more than the base model, so it is priced at the highest sibling instead.
func Lookup(model string) (price Price, known bool) {
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	bestKey := ""
	for prefix := range table {
		if strings.HasPrefix(name, prefix) && len(prefix) > len(bestKey) {
			bestKey = prefix
		}
	}
	if bestKey == "" {
		return Price{}, false
	}
	if plainSuffix(name[len(bestKey):]) {
		return table[bestKey], true
	}
	return highestSibling(bestKey), true
}

// plainSuffix reports whether rest only versions or dates the base model.
func plainSuffix(rest string) bool {
	if rest == "" {
		return true
	}
	if rest[0] != '-' && rest[0] != '@' {
		return false
	}
	for _, seg := range strings.FieldsFunc(rest, func(r rune) bool { return r == '-' || r == '@' || r == '.' || r == '_' }) {
		if seg == "latest" || seg == "preview" || seg == "exp" {
			continue
		}
		for _, r := range seg {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// highestSibling returns the dearest price among the chat models in key's family
// (the part before its first dash). Embedding rows are not siblings of chat models.
func highestSibling(key string) Price {
	family, _, _ := strings.Cut(key, "-")
	best := table[key]
	for other, p := range table {
		if strings.Contains(other, "embedding") {
			continue
		}
		if f, _, _ := strings.Cut(other, "-"); f == family && p.InPerMTok+p.OutPerMTok > best.InPerMTok+best.OutPerMTok {
			best = p
		}
	}
	return best
}
