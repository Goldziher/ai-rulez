package ard

import "strings"

// maxQueryLength bounds a derived query: an eval prompt longer than this is a
// task description, not something a user would type into a search box.
const maxQueryLength = 200

// QuerySources are the places representative queries come from, in priority
// order.
type QuerySources struct {
	// Explicit are the skill's own `representative_queries`.
	Explicit []string
	// EvalPrompts are prompts of eval cases that expect the skill to trigger
	// (expect_trigger: true). Callers must not pass near-miss or negative
	// prompts: they describe what the skill does not serve.
	EvalPrompts []string
	// Triggers are the skill frontmatter `triggers`.
	Triggers []string
}

// DeriveQueries picks up to MaxQueries representative queries: explicit ones
// first, then positive eval prompts, then triggers. Each is trimmed with its
// whitespace collapsed; empty ones, eval prompts longer than 200 characters
// and case-insensitive duplicates are dropped. Fewer than MinQueries may come
// back; Build reports that as a finding.
func DeriveQueries(src QuerySources) []string {
	var prompts []string
	for _, p := range src.EvalPrompts {
		if len(collapse(p)) <= maxQueryLength {
			prompts = append(prompts, p)
		}
	}
	all := make([]string, 0, len(src.Explicit)+len(prompts)+len(src.Triggers))
	all = append(all, src.Explicit...)
	all = append(all, prompts...)
	all = append(all, src.Triggers...)
	out := normalizeQueries(all)
	if len(out) > MaxQueries {
		out = out[:MaxQueries]
	}
	return out
}

// normalizeQueries trims and collapses whitespace, drops empty values and
// case-insensitive duplicates, and keeps the order.
func normalizeQueries(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, q := range in {
		q = collapse(q)
		key := strings.ToLower(q)
		if q == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, q)
	}
	return out
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
