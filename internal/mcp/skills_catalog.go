package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Skills extension identifiers (SEP-2640, status Final).
const (
	// SkillsExtensionID is the key servers declare under capabilities.extensions.
	SkillsExtensionID = "io.modelcontextprotocol/skills"
	// SkillURIScheme is the conventional scheme for skill resources.
	SkillURIScheme = "skill://"
	// skillsMetaPrefix is the reverse-domain prefix the extension asks resource
	// _meta keys to use.
	skillsMetaPrefix = "io.modelcontextprotocol.skills/"
	skillMarkdown    = "SKILL.md"
	mimeMarkdown     = "text/markdown"
	mimeText         = "text/plain"
	mimeOctet        = "application/octet-stream"
)

// SkillFilter restricts which skills a serving session exposes. All non-empty
// criteria must hold; Deny always wins over Allow.
type SkillFilter struct {
	// Domains keeps skills owned by one of these domains. "root" names skills
	// that belong to no domain.
	Domains []string
	// Allow keeps skills whose name matches one of these path.Match patterns.
	Allow []string
	// Deny drops skills whose name matches one of these path.Match patterns.
	Deny []string
}

// CatalogFile is one file of a served skill.
type CatalogFile struct {
	URI     string
	RelPath string
	Digest  string // sha256:<hex>
	Size    int
	MIME    string
	Content []byte
}

// CatalogSkill is one served skill with its provenance.
type CatalogSkill struct {
	Name        string
	Description string
	URI         string // skill://<name>/SKILL.md
	Domain      string
	Source      string
	Ref         string
	Pinned      bool
	Keywords    []string
	Frontmatter map[string]any
	Files       []CatalogFile
	// Digest identifies the skill as a whole: sha256 over its sorted file URIs
	// and digests, so one value changes whenever any file does.
	Digest string
}

// Catalog is the immutable set of skills a serving session exposes.
type Catalog struct {
	Profile string
	Preset  string
	skills  []*CatalogSkill
	byName  map[string]*CatalogSkill
	byURI   map[string]*CatalogSkill
	byFile  map[string]*CatalogFile
}

// BuildCatalog converts rendered skills into a catalog, applying the filter.
// Skills whose rendered frontmatter cannot be parsed, or that lack a name or
// description, are rejected: the extension requires both.
func BuildCatalog(profile, preset string, served []generator.ServedSkill, filter SkillFilter) (*Catalog, error) {
	cat := &Catalog{
		Profile: profile,
		Preset:  preset,
		byName:  map[string]*CatalogSkill{},
		byURI:   map[string]*CatalogSkill{},
		byFile:  map[string]*CatalogFile{},
	}
	for i := range served {
		src := &served[i]
		if !filter.allows(src) {
			continue
		}
		skill, err := newCatalogSkill(src)
		if err != nil {
			return nil, err
		}
		if _, dup := cat.byName[skill.Name]; dup {
			return nil, oops.Errorf("two skills are named %q; skill names must be unique to be served", skill.Name)
		}
		cat.skills = append(cat.skills, skill)
		cat.byName[skill.Name] = skill
		cat.byURI[skill.URI] = skill
		for j := range skill.Files {
			cat.byFile[skill.Files[j].URI] = &skill.Files[j]
		}
	}
	sort.Slice(cat.skills, func(i, j int) bool { return cat.skills[i].Name < cat.skills[j].Name })
	return cat, nil
}

func (f SkillFilter) allows(s *generator.ServedSkill) bool {
	if len(f.Domains) > 0 {
		domain := s.Domain
		if domain == "" {
			domain = "root"
		}
		if !containsString(f.Domains, domain) {
			return false
		}
	}
	if matchesAny(f.Deny, s.ID) {
		return false
	}
	return len(f.Allow) == 0 || matchesAny(f.Allow, s.ID)
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := path.Match(strings.TrimSpace(p), name); err == nil && ok {
			return true
		}
	}
	return false
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

func newCatalogSkill(src *generator.ServedSkill) (*CatalogSkill, error) {
	if len(src.Files) == 0 || src.Files[0].RelPath != skillMarkdown {
		return nil, oops.Errorf("skill %q has no SKILL.md", src.ID)
	}
	front, err := parseFrontmatter(src.Files[0].Content)
	if err != nil {
		return nil, oops.Wrapf(err, "skill %q: frontmatter", src.ID)
	}
	name, _ := front["name"].(string)
	if name == "" {
		name = src.ID
		front["name"] = name
	}
	desc, _ := front["description"].(string)
	if strings.TrimSpace(desc) == "" {
		return nil, oops.Errorf("skill %q has no description; the Skills extension requires one", src.ID)
	}
	skill := &CatalogSkill{
		Name:        name,
		Description: desc,
		URI:         SkillURIScheme + name + "/" + skillMarkdown,
		Domain:      src.Domain,
		Source:      src.Source,
		Ref:         src.Ref,
		Pinned:      src.Pinned,
		Keywords:    src.Keywords,
		Frontmatter: front,
	}
	hash := sha256.New()
	for _, f := range src.Files {
		sum := sha256.Sum256(f.Content)
		file := CatalogFile{
			URI:     SkillURIScheme + name + "/" + f.RelPath,
			RelPath: f.RelPath,
			Digest:  "sha256:" + hex.EncodeToString(sum[:]),
			Size:    len(f.Content),
			MIME:    mimeFor(f.RelPath, f.Content),
			Content: f.Content,
		}
		skill.Files = append(skill.Files, file)
	}
	sorted := append([]CatalogFile(nil), skill.Files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].URI < sorted[j].URI })
	for i := range sorted {
		fmt.Fprintf(hash, "%s\x00%s\n", sorted[i].URI, sorted[i].Digest)
	}
	skill.Digest = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return skill, nil
}

func mimeFor(rel string, content []byte) string {
	switch strings.ToLower(path.Ext(rel)) {
	case ".md", ".markdown":
		return mimeMarkdown
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	}
	if utf8.Valid(content) {
		return mimeText
	}
	return mimeOctet
}

// parseFrontmatter decodes the leading YAML frontmatter block of a SKILL.md.
func parseFrontmatter(content []byte) (map[string]any, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, oops.Errorf("SKILL.md must begin with YAML frontmatter")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, oops.Errorf("SKILL.md frontmatter is not closed")
	}
	front := map[string]any{}
	if err := yaml.Unmarshal([]byte(rest[:end]), &front); err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	return front, nil
}

// Skills returns the served skills in name order.
func (c *Catalog) Skills() []*CatalogSkill { return c.skills }

// Lookup resolves a skill by name or by the URI of its SKILL.md.
func (c *Catalog) Lookup(nameOrURI string) (*CatalogSkill, bool) {
	if s, ok := c.byURI[nameOrURI]; ok {
		return s, true
	}
	s, ok := c.byName[nameOrURI]
	return s, ok
}

// File resolves any file of any served skill by resource URI.
func (c *Catalog) File(uri string) (*CatalogFile, bool) {
	f, ok := c.byFile[uri]
	return f, ok
}

// SearchHit is one ranked search result.
type SearchHit struct {
	Skill *CatalogSkill
	Score int
}

// Search ranks skills lexically against the query over name, keywords,
// description and domain. An empty query returns every skill in name order
// with score 0. Results are deterministic: score descending, then name.
func (c *Catalog) Search(query string, limit int) []SearchHit {
	tokens := strings.Fields(strings.ToLower(query))
	var hits []SearchHit
	for _, s := range c.skills {
		score := 0
		if len(tokens) > 0 {
			score = scoreSkill(s, tokens)
			if score == 0 {
				continue
			}
		}
		hits = append(hits, SearchHit{Skill: s, Score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Skill.Name < hits[j].Skill.Name
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func scoreSkill(s *CatalogSkill, tokens []string) int {
	name := strings.ToLower(s.Name)
	desc := strings.ToLower(s.Description)
	domain := strings.ToLower(s.Domain)
	score := 0
	for _, tok := range tokens {
		switch {
		case name == tok:
			score += 10
		case strings.Contains(name, tok):
			score += 5
		}
		for _, kw := range s.Keywords {
			kw = strings.ToLower(kw)
			switch {
			case kw == tok:
				score += 4
			case strings.Contains(kw, tok):
				score += 2
			}
		}
		if containsWord(desc, tok) {
			score += 2
		} else if strings.Contains(desc, tok) {
			score++
		}
		if domain != "" && domain == tok {
			score++
		}
	}
	return score
}

func containsWord(text, word string) bool {
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127)
	}) {
		if w == word {
			return true
		}
	}
	return false
}
