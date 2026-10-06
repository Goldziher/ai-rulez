// Package pricing is the one built-in model price table. internal/llm prices
// model calls with it and internal/evals prices eval runs and their estimates
// with it, so a price is updated in one place and the two never drift.
package pricing

import "strings"

// Version identifies the table. Bump it whenever a price changes: internal/llm
// folds it into its cache identity, so a cost recorded under old prices is never
// replayed under new ones.
const Version = "2026-07"

// Price is a model price in USD per million tokens.
type Price struct {
	InPerMTok  float64 `json:"in_per_mtok"`
	OutPerMTok float64 `json:"out_per_mtok"`
}

// table is a small, approximate list for common models. It is a convenience for
// budget estimates, not a billing source: prices change, and unknown models have
// no entry. Keys are matched against the model name with any provider prefix
// removed, longest prefix first. The bare haiku, sonnet and opus keys are the
// short names eval cases and --model use.
var table = map[string]Price{
	"gpt-4o-mini":            {0.15, 0.60},
	"gpt-4o":                 {2.50, 10.00},
	"gpt-4.1-mini":           {0.40, 1.60},
	"gpt-4.1":                {2.00, 8.00},
	"text-embedding-3-small": {0.02, 0},
	"text-embedding-3-large": {0.13, 0},
	"claude-haiku":           {1.00, 5.00},
	"claude-sonnet":          {3.00, 15.00},
	"claude-opus":            {15.00, 75.00},
	"haiku":                  {1.00, 5.00},
	"sonnet":                 {3.00, 15.00},
	"opus":                   {15.00, 75.00},
}

// Lookup returns the built-in price of a model. known is false when no entry
// matches.
func Lookup(model string) (price Price, known bool) {
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	bestLen := 0
	for prefix, p := range table {
		if strings.HasPrefix(name, prefix) && len(prefix) > bestLen {
			price, bestLen, known = p, len(prefix), true
		}
	}
	return price, known
}
