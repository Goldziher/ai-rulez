package skillsearch

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// conceptEmbedder is a bag-of-concepts embedder: every word that belongs to a
// concept adds to that concept's dimension, so synonyms land close together
// without a model. It counts calls and can be told to fail.
type conceptEmbedder struct {
	mu       sync.Mutex
	concepts map[string]int
	dims     int
	calls    int
	texts    []string
	err      error
	fp, name string
}

func newConceptEmbedder(groups ...[]string) *conceptEmbedder {
	e := &conceptEmbedder{concepts: map[string]int{}, dims: len(groups) + 1, fp: "test@local", name: "concepts-v1"}
	for i, g := range groups {
		for _, w := range g {
			e.concepts[w] = i
		}
	}
	return e
}

func (e *conceptEmbedder) Embed(_ context.Context, texts []string) (Embedding, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.err != nil {
		return Embedding{}, e.err
	}
	e.texts = append(e.texts, texts...)
	out := Embedding{CostKnown: true}
	for _, t := range texts {
		v := make([]float32, e.dims)
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return r < 'a' || r > 'z' }) {
			if c, ok := e.concepts[w]; ok {
				v[c]++
			} else {
				v[e.dims-1] += 0.05 // unknown words are weak noise
			}
		}
		out.Vectors = append(out.Vectors, v)
		out.Tokens += len(strings.Fields(t))
	}
	return out, nil
}

func (e *conceptEmbedder) Fingerprint() string { return e.fp }
func (e *conceptEmbedder) Model() string       { return e.name }

var errProvider = errors.New("provider down")

// refundCatalog is a small catalog where the query "customer wants money back"
// shares no word with the right skill's text but one concept.
func refundCatalog() []Item {
	mk := func(name, desc string, kw ...string) Item {
		return Item{ID: name, Digest: "sha256:" + name, Doc: Doc{Name: name, Description: desc, Keywords: kw}}
	}
	return []Item{
		mk("issue-refund", "Reimburse a customer for a returned purchase", "reimburse"),
		mk("deploy-staging", "Roll a service out to the staging cluster", "rollout"),
		mk("dispute-charge", "Handle a chargeback dispute from a bank"),
		mk("git-workflow", "Branching and pull request conventions"),
	}
}

func refundEmbedder() *conceptEmbedder {
	return newConceptEmbedder(
		[]string{"refund", "reimburse", "money", "back", "returned", "purchase", "customer"},
		[]string{"deploy", "rollout", "roll", "staging", "cluster", "service", "out"},
		[]string{"dispute", "chargeback", "bank", "charge"},
		[]string{"git", "branching", "pull", "request", "conventions"},
	)
}

func onceReset() sync.Once { return sync.Once{} }
