package review

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Item is one piece of content a rubric can score.
type Item struct {
	// ID is kind:name, or kind:domain/name for domain content.
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
	// Path is the repository-relative slash path of the item file.
	Path string `json:"path"`
	// Digest is the sha256 of the item file ("" when it could not be read).
	Digest string `json:"digest,omitempty"`
	// Owned is false for content from includes, installed skills and builtins.
	Owned bool `json:"owned"`

	// Abs is the absolute path of the item file.
	Abs         string   `json:"-"`
	Description string   `json:"-"`
	Keys        []string `json:"-"`
	Body        string   `json:"-"`
	// Frontmatter is the raw YAML block between the --- lines ("" when there is none).
	Frontmatter string `json:"-"`
	// Raw is the whole item file, which the withholding checks read.
	Raw       string `json:"-"`
	ReadError string `json:"-"`
}

// Collect lists the skills, agents, commands and rules of cfg. rel maps an
// absolute path to a repository-relative one ("" when outside the repository).
// Items are sorted by ID.
func Collect(cfg *config.Config, rel func(abs string) string) []Item {
	if cfg == nil || cfg.Content == nil {
		return nil
	}
	configDir, _ := filepath.Abs(cfg.ConfigDir) //nolint:errcheck // falls back to empty
	var items []Item
	addAll := func(domain string, rules, skills, agents, commands []config.ContentFile) {
		for _, group := range []struct {
			kind  string
			files []config.ContentFile
		}{{KindRule, rules}, {KindSkill, skills}, {KindAgent, agents}, {KindCommand, commands}} {
			for _, cf := range group.files {
				items = append(items, newItem(group.kind, domain, cf, configDir, rel))
			}
		}
	}
	c := cfg.Content
	addAll("", c.Rules, c.Skills, c.Agents, c.Commands)
	domains := make([]string, 0, len(c.Domains))
	for n := range c.Domains {
		domains = append(domains, n)
	}
	sort.Strings(domains)
	for _, n := range domains {
		if d := c.Domains[n]; d != nil {
			addAll(n, d.Rules, d.Skills, d.Agents, d.Commands)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func newItem(kind, domain string, cf config.ContentFile, configDir string, rel func(string) string) Item {
	abs, _ := filepath.Abs(cf.Path) //nolint:errcheck // keeps the raw path
	name := cf.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(cf.Path), filepath.Ext(cf.Path))
	}
	id := kind + ":" + name
	if domain != "" {
		id = kind + ":" + domain + "/" + name
	}
	relToConfig, err := filepath.Rel(configDir, abs)
	owned := err == nil && !strings.HasPrefix(relToConfig, "..") && !strings.Contains(cf.Path, "://")
	it := Item{ID: id, Kind: kind, Name: name, Domain: domain, Owned: owned, Abs: abs}
	if rel != nil {
		it.Path = rel(abs)
	}
	if it.Path == "" {
		it.Path = filepath.ToSlash(cf.Path)
	}
	if !owned {
		// Third-party content is never read for review: it is not sent and not scored.
		return it
	}
	data, _, rerr := safefs.ReadRegularKeepMode(abs) // a review never changes a file's mode
	if rerr != nil {
		it.ReadError = rerr.Error()
		return it
	}
	return it.WithText(string(data))
}

// WithText returns the item with its content read from text: the digest, the raw file, the
// frontmatter (name, description, keys) and the body. The id, kind, path and ownership stay.
func (it Item) WithText(text string) Item {
	it.Digest = textDigest(text)
	it.Raw = text
	fm, body := splitFrontmatter(text)
	it.Body = body
	it.Frontmatter = frontmatterBlock(text)
	if n, ok := fm["name"].(string); ok && strings.TrimSpace(n) != "" {
		it.Name = strings.TrimSpace(n)
	}
	it.Description = ""
	if d, ok := fm["description"].(string); ok {
		it.Description = strings.TrimSpace(d)
	}
	it.Keys = nil
	for k := range fm {
		it.Keys = append(it.Keys, k)
	}
	sort.Strings(it.Keys)
	return it
}

// splitFrontmatter separates a leading ---/--- YAML block from the body. A
// missing or malformed block yields an empty map and the whole text.
func splitFrontmatter(raw string) (map[string]any, string) {
	text := strings.TrimPrefix(raw, "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return map[string]any{}, text
	}
	block, body, found := strings.Cut(rest, "\n---")
	if !found {
		return map[string]any{}, text
	}
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil || fm == nil {
		return map[string]any{}, text
	}
	return fm, body
}

// TextDigest is the digest an item carries for its file text ("sha256:<hex>").
func TextDigest(text string) string { return textDigest(text) }

// textDigest is the sha256 of text in the "sha256:<hex>" form items carry.
func textDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// frontmatterBlock returns the raw text of a leading ---/--- block, "" when the
// file has no well-formed one (the same rule splitFrontmatter applies).
func frontmatterBlock(raw string) string {
	text := strings.ReplaceAll(strings.TrimPrefix(raw, "\xef\xbb\xbf"), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	block, _, found := strings.Cut(rest, "\n---")
	if !found {
		return ""
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil || fm == nil {
		return ""
	}
	return block
}

// Selected filters items by the command-line selectors: an id, a name, or a
// repository-relative path. No selector keeps everything.
func Selected(items []Item, selectors []string) []Item {
	if len(selectors) == 0 {
		return items
	}
	var out []Item
	for _, it := range items {
		for _, s := range selectors {
			s = strings.TrimSuffix(filepath.ToSlash(s), "/")
			if s == it.ID || s == it.Name || s == it.Path || strings.HasSuffix(it.Path, "/"+s) || strings.HasPrefix(it.Path, s+"/") {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// Unmatched returns the selectors that matched no item, so a typo is reported
// instead of reviewing nothing.
func Unmatched(items []Item, selectors []string) []string {
	var out []string
	for _, s := range selectors {
		if len(Selected(items, []string{s})) == 0 {
			out = append(out, s)
		}
	}
	return out
}

// excludedBy returns the [review] exclude glob that matches the item, if any.
func excludedBy(it Item, globs []string) (string, bool) {
	for _, g := range globs {
		for _, cand := range []string{it.Name, it.ID, it.Path} {
			if ok, err := path.Match(g, cand); err == nil && ok {
				return g, true
			}
		}
	}
	return "", false
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

// wordSet is the lower-case word set of text.
func wordSet(text string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
		set[w] = struct{}{}
	}
	return set
}

// jaccard is the word-set Jaccard similarity, the measure AR702 uses.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if _, ok := b[w]; ok {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// ChangedItems returns the ids of the items whose files are among changed, the paths a
// revision diff lists relative to the repository root. baseRel is the project directory relative
// to that root ("" when they are the same). A skill or command directory counts as changed when
// any file below it did, so an edited reference marks its skill.
func ChangedItems(items []Item, changed []string, baseRel string) map[string]bool {
	set := make(map[string]bool, len(changed))
	for _, c := range changed {
		set[filepath.ToSlash(c)] = true
	}
	out := map[string]bool{}
	for _, it := range items {
		file := path.Join(baseRel, it.Path)
		if set[file] {
			out[it.ID] = true
			continue
		}
		if base := strings.ToUpper(path.Base(it.Path)); base == "SKILL.MD" || base == "COMMAND.MD" {
			dir := path.Dir(file) + "/"
			for c := range set {
				if strings.HasPrefix(c, dir) {
					out[it.ID] = true
					break
				}
			}
		}
	}
	return out
}
