package generator

// The always-loaded item listing.
//
// A harness that supports skills does not wait for a skill to be opened: at
// session start it puts a listing of every skill it can load into the prompt,
// one entry of name plus description with some framing around it. That listing
// is paid on every request. Counting only the skill name, as the first cut of
// this report did, understates it several times over.
//
// Measurement that calibrates the constant below (Claude Code 2.1.289, prompt
// size = input + cache_creation + cache_read tokens from
// `claude -p ... --output-format json`, no MCP servers, project settings only):
// 100 skills with 165-character descriptions grew the prompt by 6,402 tokens,
// 64 per skill, against 3 tokens for the name and 34 for the description under
// cl100k_base. The remaining 27 tokens per entry are listing framing and the
// gap between cl100k_base and Claude's tokenizer on that text. A skill with
// disable-model-invocation: true added no measurable tokens. The total listing
// is bounded by a budget on top of the per-entry cap (about 9.6k tokens at 100
// or 200 skills in the same probe), which this model does not apply: it reports
// what the entries would cost, and notes that a harness may shorten or omit
// entries when a very large listing exceeds its budget.

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"gopkg.in/yaml.v3"
)

// ListingEntryOverheadTokens is the estimated framing cost of one listing
// entry, in tokens, on top of the counted name, description and path. It is an
// estimate calibrated against Claude Code (see the file comment), not a
// property of other harnesses.
const ListingEntryOverheadTokens = 27

// listedKinds is the order listing entries are reported in.
var listedKinds = []config.OutputKind{config.OutputKindSkill, config.OutputKindCommand, config.OutputKindAgent}

// listingResult is what listingEntries produces for one runtime.
type listingResult struct {
	entries []Entry
	// replaces marks the item kinds whose separate name and description lines
	// are superseded by a listing entry.
	replaces  map[config.OutputKind]bool
	items     int
	truncated int
	// replacedAlways and replacedConditional are the tokens the pre-listing model
	// charged for the superseded lines.
	replacedAlways      int
	replacedConditional int
}

// assignListed gives every runtime group the item outputs its harness can see.
// The collector keeps one analysis per output path, owned by the first preset to
// write it, but a path shared between presets (.agents/skills) is listed by each
// harness that reads it.
func (b *reportBuilder) assignListed(groups map[string]*runtimeGroup, analyses []*config.OutputAnalysis) {
	for _, analysis := range analyses {
		switch analysis.Kind {
		case config.OutputKindSkill, config.OutputKindCommand, config.OutputKindAgent:
		default:
			continue
		}
		for _, preset := range b.collector.Claimants(analysis.Path) {
			if group, ok := groups[runtimeKey(preset, analysis.Scope)]; ok {
				group.listed = append(group.listed, analysis)
			}
		}
	}
}

// listingEntries builds the listing entries of a runtime: one per listed item
// kind, with the names, descriptions, paths and framing as children.
func (b *reportBuilder) listingEntries(group *runtimeGroup) listingResult {
	result := listingResult{replaces: make(map[config.OutputKind]bool)}
	spec := b.collector.ListingFor(group.preset)
	for _, kind := range listedKinds {
		if !spec.Lists(kind) {
			continue
		}
		var analyses []*config.OutputAnalysis
		for _, analysis := range group.listed {
			if analysis.Kind == kind {
				analyses = append(analyses, analysis)
			}
		}
		slices.SortFunc(analyses, func(left, right *config.OutputAnalysis) int { return cmp.Compare(left.Path, right.Path) })

		names := Entry{Label: "names", Bucket: BucketAlways}
		descriptions := Entry{Label: "descriptions", Bucket: BucketAlways}
		paths := Entry{Label: "paths", Bucket: BucketAlways}
		framing := Entry{Label: "per-entry framing (estimate)", Bucket: BucketAlways}
		for _, analysis := range analyses {
			item, ok := b.listingItem(analysis, spec)
			if !ok {
				continue
			}
			nameTokens := b.counter.Count(item.name)
			descriptionTokens := b.counter.Count(item.description)
			pathTokens := 0
			if spec.IncludePath {
				pathTokens = b.counter.Count(item.path)
			}
			names.Tokens += nameTokens
			descriptions.Tokens += descriptionTokens
			paths.Tokens += pathTokens
			framing.Tokens += ListingEntryOverheadTokens
			b.attribute(analysis.SourcePath, BucketAlways, nameTokens+descriptionTokens+pathTokens+ListingEntryOverheadTokens)
			result.items++
			if item.truncated {
				result.truncated++
			}
			names.Artifacts++
		}
		if names.Artifacts == 0 {
			continue
		}
		descriptions.Artifacts, paths.Artifacts, framing.Artifacts = names.Artifacts, names.Artifacts, names.Artifacts
		entry := Entry{
			Label:     string(kind) + " listing",
			Bucket:    BucketAlways,
			Artifacts: names.Artifacts,
			listing:   true,
		}
		for _, child := range []Entry{names, descriptions, paths, framing} {
			if child.Tokens > 0 {
				entry.Tokens += child.Tokens
				entry.Children = append(entry.Children, child)
			}
		}
		result.entries = append(result.entries, entry)
		result.replaces[kind] = len(group.items[kind]) > 0
	}
	return result
}

// listingItem is one entry of a harness's listing.
type listingItem struct {
	name, description, path string
	truncated               bool
}

// listingItem reads the listed fields off the rendered artifact. It works from
// the finished file rather than the recorded parts because only the provider
// DSL records name and description separately; the hand-written presets record
// the file as a single part. An item marked disable-model-invocation is not
// offered to the model, so it is not listed.
func (b *reportBuilder) listingItem(analysis *config.OutputAnalysis, spec config.ListingSpec) (listingItem, bool) {
	text, ok := b.finalPayload[analysis.Path]
	if !ok {
		var builder strings.Builder
		for _, part := range analysis.Parts {
			builder.WriteString(part.Content)
		}
		text = builder.String()
	}
	fields, body := listingFrontmatter(text)
	if truthy(fields["disable-model-invocation"]) {
		return listingItem{}, false
	}

	item := listingItem{name: scalarString(fields["name"]), description: scalarString(fields["description"])}
	if item.name == "" {
		item.name = analysis.ItemID
	}
	if item.name == "" {
		item.name = itemNameFromPath(analysis)
	}
	if item.description == "" {
		item.description = firstBodyLine(body)
	}
	if when := scalarString(fields["when_to_use"]); when != "" {
		item.description = strings.TrimSpace(item.description + " " + when)
	}
	if limit := spec.DescriptionLimit; limit > 0 {
		if runes := []rune(item.description); len(runes) > limit {
			item.description = string(runes[:limit])
			item.truncated = true
		}
	}
	item.path = filepath.ToSlash(analysis.Path)
	if b.baseDir != "" {
		if rel, err := filepath.Rel(b.baseDir, analysis.Path); err == nil {
			item.path = filepath.ToSlash(rel)
		}
	}
	return item, true
}

// itemNameFromPath derives the name a harness falls back to: the skill
// directory, or the file stem of an agent or command.
func itemNameFromPath(analysis *config.OutputAnalysis) string {
	if analysis.Kind == config.OutputKindSkill {
		return filepath.Base(filepath.Dir(analysis.Path))
	}
	base := filepath.Base(analysis.Path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// listingFrontmatter separates a leading YAML frontmatter block from the body. A
// file without one, or with one that does not parse, has no fields.
func listingFrontmatter(text string) (fields map[string]any, body string) {
	trimmed := strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return nil, text
	}
	rest := trimmed[strings.Index(trimmed, "\n")+1:]
	end := -1
	for offset := 0; offset < len(rest); {
		line := rest[offset:]
		next := strings.Index(line, "\n")
		if next < 0 {
			next = len(line)
		}
		if strings.TrimRight(line[:next], "\r") == frontmatterFence {
			end = offset
			break
		}
		offset += next + 1
	}
	if end < 0 {
		return nil, text
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fields); err != nil {
		return nil, text
	}
	body = rest[end:]
	if newline := strings.Index(body, "\n"); newline >= 0 {
		body = body[newline+1:]
	} else {
		body = ""
	}
	return fields, body
}

func scalarString(value any) string {
	text, _ := value.(string) //nolint:errcheck // a non-string value reads as empty
	return strings.TrimSpace(text)
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

// firstBodyLine is the description a harness falls back to when none is
// declared: the first non-empty line of the body.
func firstBodyLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#")); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// listingNotes explains the listing lines. Empty when no runtime lists anything.
func listingNotes(report *TokenReport) []string {
	listed := false
	truncated := 0
	for _, runtime := range append(append([]RuntimeTokens{}, report.Runtimes...), report.Scoped...) { //nolint:gocritic // small report, copied once
		if runtime.ListedItems > 0 {
			listed = true
		}
		truncated += runtime.TruncatedDescriptions
	}
	if !listed {
		return nil
	}
	notes := []string{
		"Listing lines estimate what a harness puts in the prompt at session start to advertise " +
			"skills, commands and agents: each entry's name and description, its path where the " +
			"harness includes one, and " + strconv.Itoa(ListingEntryOverheadTokens) +
			" tokens of framing per entry. The framing constant is calibrated against Claude Code " +
			"(100 skills measured at 64 tokens each) and is an estimate for every other harness. The " +
			"listing is part of the always-loaded figure and of --budget; always_legacy and " +
			"conditional_legacy keep the numbers the earlier model reported.",
		"A harness bounds its listing (Codex: a share of the context window; Claude Code: a total " +
			"budget on top of the per-entry description cap) and shortens or omits entries beyond it. " +
			"The listing lines do not apply that bound, so a very large skill set is an upper estimate. " +
			"Presets absent from the listing table (amp, antigravity, baz, hermes, xum) " +
			"are not modeled and report no listing. Skills written only to the shared .agents/skills " +
			"tree by agents_md are not attributed to a preset.",
	}
	if truncated > 0 {
		notes = append(notes, "Some descriptions exceed the harness's per-entry limit and were cut in the "+
			"estimate; shorten them or move detail into the skill body.")
	}
	return notes
}
