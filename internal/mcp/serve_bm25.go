package mcp

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// find_skill ranks skills with BM25F over four fields of the skill: its name,
// its declared triggers, its keywords and its description. The ranking is
// lexical and deterministic (score descending, then name); embeddings may be
// added later behind the same function.

// BM25 parameters and per-field weights. A match in the name or a trigger says
// more about intent than a word somewhere in the description.
const (
	bm25K1 = 1.2
	bm25B  = 0.75

	weightName        = 3.0
	weightTriggers    = 2.5
	weightKeywords    = 2.0
	weightDescription = 1.0
)

var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true, "do": true,
	"for": true, "from": true, "how": true, "i": true, "in": true, "is": true, "it": true, "me": true, "my": true,
	"of": true, "on": true, "or": true, "that": true, "the": true, "this": true, "to": true, "use": true, "want": true,
	"we": true, "with": true, "you": true, "need": true, "when": true, "should": true,
}

// tokenize lowercases s, splits it on anything that is not a letter or digit,
// drops stopwords and applies a light suffix stemmer so "migrations" matches
// "migration" and "refunding" matches "refund".
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := fields[:0]
	for _, f := range fields {
		if stopwords[f] {
			continue
		}
		out = append(out, stem(f))
	}
	return out
}

func stem(w string) string {
	for _, suf := range []string{"ing", "ied", "ies", "ed", "es", "s"} {
		if len(w) > len(suf)+3 && strings.HasSuffix(w, suf) {
			if suf == "ies" || suf == "ied" {
				return w[:len(w)-len(suf)] + "y"
			}
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

// FindHit is one ranked find_skill result.
type FindHit struct {
	Skill *CatalogSkill
	Score float64
}

type weightedField struct {
	weight float64
	tokens []string
}

type bm25Doc struct {
	skill  *CatalogSkill
	tf     map[string]float64 // weighted term frequency across fields
	length float64            // weighted length
}

// bm25Rank scores skills against the query. Skills with no matching term are
// omitted. An empty query (or one that is only stopwords) returns nothing: use
// list_skill_resources or skills/list to enumerate.
func bm25Rank(skills []*CatalogSkill, query string) []FindHit {
	terms := uniqueTokens(tokenize(query))
	if len(terms) == 0 || len(skills) == 0 {
		return nil
	}
	docs := make([]bm25Doc, 0, len(skills))
	df := map[string]int{}
	var total float64
	for _, s := range skills {
		d := bm25Doc{skill: s, tf: map[string]float64{}}
		for _, f := range skillFields(s) {
			for _, tok := range f.tokens {
				d.tf[tok] += f.weight
				d.length += f.weight
			}
		}
		for tok := range d.tf {
			df[tok]++
		}
		total += d.length
		docs = append(docs, d)
	}
	avg := total / float64(len(docs))
	if avg == 0 {
		return nil
	}
	n := float64(len(docs))
	var hits []FindHit
	for i := range docs {
		d := &docs[i]
		score := 0.0
		for _, t := range terms {
			tf := d.tf[t]
			if tf == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[t])+0.5)/(float64(df[t])+0.5))
			score += idf * tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*d.length/avg))
		}
		if score > 0 {
			hits = append(hits, FindHit{Skill: d.skill, Score: math.Round(score*1e6) / 1e6})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Skill.Name < hits[j].Skill.Name
	})
	return hits
}

func skillFields(s *CatalogSkill) []weightedField {
	return []weightedField{
		{weightName, tokenize(s.Name)},
		{weightTriggers, tokenize(strings.Join(s.Triggers, " "))},
		{weightKeywords, tokenize(strings.Join(s.Keywords, " "))},
		{weightDescription, tokenize(s.Description)},
	}
}

func uniqueTokens(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, t := range in {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// listField reads a frontmatter value that is a list of strings or one
// comma-separated string.
func listField(front map[string]any, key string) []string {
	var out []string
	switch v := front[key].(type) {
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
