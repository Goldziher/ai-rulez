// Package pricing prices model calls and eval estimates. liter-llm's embedded
// model catalog (GetModelInfo, CompletionCostWithCache) is the source of truth;
// this package adds only the fail-closed corrections ai-rulez needs where that
// catalog is missing a model or would charge less than the model really costs
// (see floors and variantFloor). internal/llm prices calls with it and
// internal/evals prices eval runs and their estimates with it, so a price
// comes from one place.
package pricing

import (
	"runtime/debug"
	"strings"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"
)

// floorsRevision is bumped whenever floors or variantFloor change. Version folds
// it into the llm cache identity, so a cost recorded under one set of prices is
// never replayed under another.
const floorsRevision = "1"

const literLLMModule = "github.com/xberg-io/liter-llm/packages/go/v2"

// Version identifies the price source: the liter-llm catalog shipped with the
// linked binding plus the revision of this package's floors.
func Version() string {
	return "literllm-" + LiterLLMVersion() + "+floors-" + floorsRevision
}

// LiterLLMVersion is the version of the linked liter-llm binding module.
func LiterLLMVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == literLLMModule {
				return d.Version
			}
		}
	}
	return "unknown"
}

// Price is a model price in USD per million tokens.
type Price struct {
	InPerMTok  float64 `json:"in_per_mtok"`
	OutPerMTok float64 `json:"out_per_mtok"`
}

// Tokens is the usage a price is applied to. Cached is the part of Prompt the
// provider served from its prompt cache (a subset of Prompt).
type Tokens struct {
	Prompt, Cached, Completion int
}

// floors are consulted only for a model liter-llm 2.2.0 has no catalog row for.
// They are conservative ceilings kept at the old family prices (the short names
// eval cases and --model use, and the Claude families the catalog lacks), not
// billing facts, so an unlisted model never comes out cheaper than its family.
// Keys are bare lowercase names matched by longest prefix. Each row stands
// until liter-llm resolves its issue.
var floors = map[string]Price{
	// TODO(liter-llm#262): the catalog has no claude-haiku-5-5 row (listed at its dearer over-100k tier).
	"claude-haiku-5-5": {0.50, 2.50},
	// TODO(liter-llm#262): no family fallback rows (claude-opus-4-1, claude-sonnet-4, claude-3-5-haiku, ... are missing).
	"claude-haiku":  {1.00, 5.00},
	"claude-sonnet": {3.00, 15.00},
	"claude-opus":   {15.00, 75.00},
	// TODO(liter-llm#262): the short names eval cases and --model use are not model ids in the catalog.
	"haiku":  {1.00, 5.00},
	"sonnet": {3.00, 15.00},
	"opus":   {15.00, 75.00},
}

// variantFloorIn and variantFloorOut are the least a gpt-* realtime, audio or tts variant is priced at.
// TODO(liter-llm#260): the catalog resolves gpt-4o-mini-tts and gpt-4o-mini-realtime-preview to
// the base gpt-4o-mini row by silent prefix fallback (0.15/0.60), which undercharges the variant.
// Remove once GetModelInfo reports whether the match was exact.
const (
	variantFloorIn  = 2.50
	variantFloorOut = 10.00
)

// bare lowercases a model name and drops any provider prefix.
// TODO(liter-llm#260): GetModelInfo("gemini/gemini-2.5-flash") and "vertex_ai/..." find nothing
// although the bare name is listed; only some provider prefixes resolve.
func bare(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func isVariant(name string) bool {
	if !strings.HasPrefix(name, "gpt-") {
		return false
	}
	for _, m := range []string{"realtime", "audio", "tts", "transcribe"} {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

func floorFor(name string) (Price, bool) {
	best, found := "", false
	for key := range floors {
		if strings.HasPrefix(name, key) && len(key) > len(best) {
			best, found = key, true
		}
	}
	if !found {
		return Price{}, false
	}
	return floors[best], true
}

// catalogPrice is the flat per-MTok price liter-llm lists for name. A model with
// context tiers is priced at its dearest tier, so an estimate never undershoots. A listing at
// 0/0 is not a price (TODO(liter-llm#276): paid image, video and audio models carry it).
func catalogPrice(name string) (Price, bool) {
	info := lit.GetModelInfo(name)
	if info == nil {
		return Price{}, false
	}
	p := Price{InPerMTok: info.InputCostPerToken * 1e6, OutPerMTok: info.OutputCostPerToken * 1e6}
	for _, t := range info.Tiers {
		p.InPerMTok = max(p.InPerMTok, t.InputCostPerToken*1e6)
		p.OutPerMTok = max(p.OutPerMTok, t.OutputCostPerToken*1e6)
	}
	return p, p.InPerMTok > 0 || p.OutPerMTok > 0
}

// Lookup returns the flat price of a model. known is false when neither the
// catalog nor a floor has an entry.
func Lookup(model string) (price Price, known bool) {
	name := bare(model)
	price, known = catalogPrice(name)
	if !known {
		price, known = floorFor(name)
	}
	if known && isVariant(name) {
		price.InPerMTok = max(price.InPerMTok, variantFloorIn)
		price.OutPerMTok = max(price.OutPerMTok, variantFloorOut)
	}
	return price, known
}

// Cost prices u on model in USD. Where the catalog lists the model it applies
// the context tier and the cache-read rate for u.Cached; otherwise the floor
// price is applied flat. known is false for a model nothing prices.
func Cost(model string, u Tokens) (usd float64, known bool) {
	name := bare(model)
	prompt, completion := uint64(max(u.Prompt, 0)), uint64(max(u.Completion, 0))
	cached := uint64(max(u.Cached, 0))
	if _, listed := catalogPrice(name); listed {
		if c := lit.CompletionCostWithCache(name, prompt, cached, completion); c != nil {
			usd, known = *c, true
		}
	} else if p, ok := floorFor(name); ok {
		usd, known = (float64(prompt)*p.InPerMTok+float64(completion)*p.OutPerMTok)/1e6, true
	}
	if known && isVariant(name) {
		usd = max(usd, (float64(prompt)*variantFloorIn+float64(completion)*variantFloorOut)/1e6)
	}
	return usd, known
}
