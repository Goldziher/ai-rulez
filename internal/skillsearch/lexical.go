// Package skillsearch ranks skills against a free-text query. The ranker is
// lexical (BM25F), deterministic and offline; MCP find_skill, `ai-rulez search`
// and `search --eval` all call it so there is one ranking.
package skillsearch

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Ranking is BM25F over four fields of a skill: its name, its declared
// triggers, its keywords and its description. Order is score descending, then
// name; embeddings may be added later behind the same function.

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

// Doc is the searchable text of one skill.
type Doc struct {
	Name        string
	Description string
	Triggers    []string
	Keywords    []string
}

// Hit is one ranked result: the index of the document in the slice given to
// Rank and its score.
type Hit struct {
	Index int
	Score float64
}

// Tokenize lowercases s, splits it on anything that is not a letter or digit,
// drops stopwords and applies a light suffix stemmer so "migrations" matches
// "migration" and "refunding" matches "refund".
func Tokenize(s string) []string {
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

// stem strips an inflectional suffix and a trailing silent "e", so the forms of
// one word meet: image/images, cache/cached/caching, release/releases/releasing.
func stem(w string) string {
	for _, suf := range []string{"ing", "ied", "ies", "ed", "es", "s"} {
		if len(w) <= len(suf)+3 || !strings.HasSuffix(w, suf) {
			continue
		}
		base := w[:len(w)-len(suf)]
		switch suf {
		case "ies", "ied":
			return base + "y"
		case "es":
			// "es" is a plural ending only after a sibilant (matches, boxes, processes);
			// elsewhere the "e" belongs to the word (images, services).
			if !endsSibilant(base) {
				base = w[:len(w)-1]
			}
		case "s":
			if strings.HasSuffix(w, "ss") {
				return w // class, access: not a plural
			}
		}
		return dropSilentE(base)
	}
	return dropSilentE(w)
}

func endsSibilant(s string) bool {
	return strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") ||
		strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh")
}

// dropSilentE removes a final "e" from a word of five letters or more, so "cache"
// and the stem left by "cached" and "caching" agree.
func dropSilentE(w string) string {
	if len(w) > 4 && strings.HasSuffix(w, "e") {
		return w[:len(w)-1]
	}
	return w
}

type weightedField struct {
	weight float64
	tokens []string
}

type bm25Doc struct {
	tf     map[string]float64 // weighted term frequency across fields
	length float64            // weighted length
}

// Rank scores docs against the query. Documents with no matching term are
// omitted. An empty query (or one that is only stopwords) returns nothing.
// Hits are ordered by score descending, then by document name.
func Rank(docs []Doc, query string) []Hit {
	terms := uniqueTokens(Tokenize(query))
	if len(terms) == 0 || len(docs) == 0 {
		return nil
	}
	scored := make([]bm25Doc, len(docs))
	df := map[string]int{}
	var total float64
	for i := range docs {
		d := bm25Doc{tf: map[string]float64{}}
		for _, f := range docFields(&docs[i]) {
			for _, tok := range f.tokens {
				d.tf[tok] += f.weight
				d.length += f.weight
			}
		}
		for tok := range d.tf {
			df[tok]++
		}
		total += d.length
		scored[i] = d
	}
	avg := total / float64(len(scored))
	if avg == 0 {
		return nil
	}
	n := float64(len(scored))
	var hits []Hit
	for i := range scored {
		d := &scored[i]
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
			hits = append(hits, Hit{Index: i, Score: math.Round(score*1e6) / 1e6})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return docs[hits[i].Index].Name < docs[hits[j].Index].Name
	})
	return hits
}

func docFields(d *Doc) []weightedField {
	return []weightedField{
		{weightName, Tokenize(d.Name)},
		{weightTriggers, Tokenize(strings.Join(d.Triggers, " "))},
		{weightKeywords, Tokenize(strings.Join(d.Keywords, " "))},
		{weightDescription, Tokenize(d.Description)},
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
