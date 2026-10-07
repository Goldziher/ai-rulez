package mcp

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
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
	Triggers    []string
	Delivery    string
	Commit      string
	Trust       string
	// Imported marks a skill whose content comes from an include or from outside
	// the project; it is scanned at the strict level.
	Imported bool
	// LockDigest is the digest ai-rulez.lock pins (see lockDigest).
	LockDigest string
	// Locked reports that ai-rulez.lock records exactly this skill's digest.
	Locked bool
	// Approved and Approvers report the [governance] approvals of this digest (serve_approval.go).
	Approved  bool
	Approvers []string
	// ScanFindings counts security findings that did not block serving.
	ScanFindings int
	// Unscanned lists files that are not served because the security scan cannot
	// read them (binary or over 512 KiB) and the skill's trust level is error.
	Unscanned []string
	// scanLevel and unscannable record the admission scan (see ScanReport).
	scanLevel   string
	unscannable []lint.Finding
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
	refused map[string]Refusal
	reports []ScanReport
}

// BuildCatalog converts rendered skills into a catalog, applying the filter.
// Skills whose rendered frontmatter cannot be parsed, or that lack a description,
// are skipped with a warning: the extension requires one. Two served skills
// with the same name are an error.
func BuildCatalog(profile, preset string, served []generator.ServedSkill, filter SkillFilter) (*Catalog, error) {
	return BuildCatalogIn(nil, profile, preset, served, filter)
}

// BuildCatalogIn is BuildCatalog reporting what it skips to log (nil: the CLI's logger).
func BuildCatalogIn(log logger.Logger, profile, preset string, served []generator.ServedSkill, filter SkillFilter) (*Catalog, error) {
	log = logger.Or(log)
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
		if src.MalformedFrontmatter {
			// validate fails on this; the server must not serve a skill whose
			// frontmatter keys were dropped on load.
			cat.refuseMalformed(log, src)
			continue
		}
		skill, err := newCatalogSkill(log, src)
		if err != nil {
			// One malformed skill must not take the whole server down; the
			// extension cannot represent it (name and description are required
			// and the served bytes must match), so it is left out and reported.
			log.Warn("Not serving a skill the Skills extension cannot represent", "skill", src.ID, "reason", err.Error())
			continue
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

// CodeMalformedFrontmatter is the refusal code of a skill whose SKILL.md
// frontmatter is not valid YAML. It has no AR rule: `validate` rejects the
// project before any rule runs.
const CodeMalformedFrontmatter = "malformed-frontmatter"

func (c *Catalog) refuseMalformed(log logger.Logger, src *generator.ServedSkill) {
	if c.refused == nil {
		c.refused = map[string]Refusal{}
	}
	reason := "the SKILL.md frontmatter is not valid YAML (check for unquoted values containing ': '); fix it and run `ai-rulez validate`"
	c.refused[src.ID] = Refusal{Name: src.ID, Code: CodeMalformedFrontmatter, Reason: reason}
	log.Warn("Not serving a skill with malformed frontmatter", "skill", src.ID, "source", src.Source)
}

func (f SkillFilter) allows(s *generator.ServedSkill) bool {
	if len(f.Domains) > 0 {
		domain := s.Domain
		if domain == "" {
			domain = domainRoot
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

func newCatalogSkill(log logger.Logger, src *generator.ServedSkill) (*CatalogSkill, error) {
	if len(src.Files) == 0 || src.Files[0].RelPath != skillMarkdown {
		return nil, oops.Errorf("skill %q has no SKILL.md", src.ID)
	}
	front, err := parseFrontmatter(src.Files[0].Content)
	if err != nil {
		return nil, oops.Wrapf(err, "skill %q: frontmatter", src.ID)
	}
	name := stringField(front, "name")
	if name == "" {
		name = src.ID
		front["name"] = name
	}
	if !validSkillSegment(name) {
		return nil, oops.Errorf("skill %q has the name %q, which is not a valid skill:// path segment", src.ID, name)
	}
	desc := stringField(front, "description")
	if strings.TrimSpace(desc) == "" {
		// The extension requires a description. Falling back to the name keeps the
		// skill findable; the bytes served are untouched, so digests still match.
		log.Warn("A served skill has no description; using its name (add a description frontmatter field so find_skill can rank it)", "skill", src.ID)
		desc = name
		front["description"] = desc
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
		Triggers:    src.Triggers,
		Delivery:    string(src.Delivery),
		Commit:      src.Commit,
		Trust:       src.Trust,
		Imported:    src.Imported,
		Frontmatter: front,
	}
	if len(skill.Triggers) == 0 {
		skill.Triggers = listField(front, "triggers")
	}
	if len(skill.Keywords) == 0 {
		skill.Keywords = listField(front, "keywords")
	}
	leaves := make([]contentlock.Leaf, 0, len(src.Files))
	for _, f := range src.Files {
		if signing.IsSignatureFile(f.RelPath) {
			// A publisher's attestation of the skill (serve_signing.go) is not skill
			// content: it is neither served, scanned nor part of the digests.
			continue
		}
		leaf := contentlock.Leaf{Path: f.RelPath, Mode: contentlock.ModeRegular, Data: f.Content}
		leaves = append(leaves, leaf)
		skill.Files = append(skill.Files, CatalogFile{
			URI:     SkillURIScheme + name + "/" + f.RelPath,
			RelPath: f.RelPath,
			Digest:  contentlock.FileDigest(leaf),
			Size:    len(f.Content),
			MIME:    mimeFor(f.RelPath, f.Content),
			Content: f.Content,
		})
	}
	// Two digests of the same files under the one scheme of ai-rulez.lock: Digest
	// covers the bytes as served; LockDigest leaves out the header lines that
	// change without this skill changing, and is what the lock pins.
	if skill.Digest, err = contentlock.ServedDigest(leaves, false); err != nil {
		return nil, oops.With("skill", name).Wrapf(err, "digest skill files")
	}
	if skill.LockDigest, err = contentlock.ServedDigest(leaves, !src.Verbatim); err != nil {
		return nil, oops.With("skill", name).Wrapf(err, "digest skill files")
	}
	return skill, nil
}

// skillSegmentRe is a single URI path segment with no separators, whitespace or
// control characters; the name becomes the first segment of every skill:// URI.
var skillSegmentRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validSkillSegment(name string) bool {
	return skillSegmentRe.MatchString(name) && !strings.Contains(name, "..")
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
		isWord := r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127
		return !isWord
	}) {
		if w == word {
			return true
		}
	}
	return false
}
