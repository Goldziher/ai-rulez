package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Why a convention is attached to a unit.
const (
	WhyMatchesFile      = "matches-file"
	WhyMatchesReference = "matches-referenced-path"
	WhyUnconditional    = "unconditional"
	WhyAncestor         = "ancestor-instructions"
	WhyImported         = "imported"
	WhyOverlap          = "overlapping-scope"
)

// Limits of the attachment.
const (
	// UnconditionalFullTokens is the largest unconditional rule sent in full; a larger one is sent as an index.
	UnconditionalFullTokens = 6000
	// MaxImportDepth is how deep @imports are followed.
	MaxImportDepth = 5
	// maxReferencedTokens bounds the path tokens read from a unit to find referenced paths.
	maxReferencedTokens = 300
	// maxConventionTokens bounds the conventions of one unit; later ones are sent as an index.
	maxConventionTokens = 20000
)

// Repo is the project as the attachment sees it: every candidate file and a reader.
type Repo struct {
	// Files are the slash paths relative to the project root.
	Files []string
	// Read returns a file's text; ok is false when it cannot be read safely.
	Read func(rel string) (text string, ok bool)
}

// Convention is a written standard attached to a unit: a path-scoped rule or an instruction file.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Convention struct {
	// ID is "rule:<path>" or "claude-md:<path>".
	ID string `json:"id"`
	// Path is the file.
	Path string `json:"path"`
	// Paths are the globs of a path-scoped rule; empty for an unconditional rule or an instruction file.
	Paths []string `json:"paths,omitempty"`
	// Why is one of the Why constants.
	Why string `json:"why"`
	// SHA256 is the hash of the file.
	SHA256 string `json:"sha256"`
	// Lines are the numbered lines of the file as the reviewer sees them; empty for an index-only convention.
	Lines []string `json:"-"`
	// Index lists the headings (with line numbers) of a convention too large to send in full.
	Index []string `json:"-"`
	// Full is true when Lines hold the whole file.
	Full bool `json:"-"`
	// Base is the directory the globs are relative to.
	Base string `json:"-"`
}

// Attachment is everything attached to one unit.
type Attachment struct {
	Conventions []Convention
}

// IDs returns the convention ids in order.
func (a Attachment) IDs() []string {
	out := make([]string, len(a.Conventions))
	for i := range a.Conventions {
		out[i] = a.Conventions[i].ID
	}
	return out
}

// AttachOptions switches the two kinds of attachment.
type AttachOptions struct {
	Rules    bool
	ClaudeMD bool
	// Secrets are values that no convention text may carry; they are masked like secret-looking text.
	Secrets []string
}

var (
	ruleFileRe = regexp.MustCompile(`^(?:(.*)/)?\.claude/rules/(?:.+/)?[^/]+\.md$`)
	// pathTokRe finds path-like tokens: at least one directory separator, so a bare file name in
	// prose ("setup.py", "README.md") does not pull in every rule scoped to that extension.
	pathTokRe = regexp.MustCompile(`[A-Za-z0-9_@.~-]+(?:/[A-Za-z0-9_@.~*-]+)+`)
	importRe  = regexp.MustCompile(`(?:^|[\s(])@([A-Za-z0-9_./~-]+(?:\.[A-Za-z0-9]+)?)`)
)

// Attach resolves the conventions that apply to a unit by the rules Claude Code itself uses.
//
// Path-scoped rules: for every ancestor directory D of a file of the unit, the Markdown files below
// D/.claude/rules/ (at any depth) are candidates. A rule with a `paths:` list attaches when one glob,
// taken relative to D, matches a file of the unit (matches-file) or a path the unit mentions
// (matches-referenced-path); a rule without `paths:` attaches always (unconditional). A rule unit never
// attaches itself, and attaches the rules whose scope overlaps its own (overlapping-scope).
//
// Instruction files: CLAUDE.md, CLAUDE.local.md, AGENTS.md (and .claude/CLAUDE.md) of every ancestor
// directory, root first and nearest last, with the files they @import to depth MaxImportDepth. The unit's
// own files are excluded.
func Attach(repo Repo, u Unit, opts AttachOptions) Attachment {
	own := map[string]bool{}
	var sent []string
	for _, f := range u.Files {
		own[f.Path] = true
		if len(f.Lines) > 0 {
			sent = append(sent, f.Path)
		}
	}
	if len(sent) == 0 {
		sent = []string{u.Primary}
	}
	var att Attachment
	seen := map[string]bool{}
	add := func(c Convention) {
		if !seen[c.ID] {
			seen[c.ID] = true
			att.Conventions = append(att.Conventions, c)
		}
	}
	if opts.ClaudeMD {
		files := attachInstructionFiles(repo, u, own, opts)
		for i := range files {
			add(files[i])
		}
	}
	if opts.Rules {
		rules := attachRules(repo, u, own, sent, opts)
		for i := range rules {
			add(rules[i])
		}
	}
	budgetConventions(att.Conventions)
	return att
}

// ancestorDirs returns "", then every directory on the way to the file's own directory.
func ancestorDirs(p string) []string {
	dirs := []string{""}
	dir := path.Dir(p)
	if dir == "." {
		return dirs
	}
	acc := ""
	for _, seg := range strings.Split(dir, "/") {
		acc = path.Join(acc, seg)
		dirs = append(dirs, acc)
	}
	return dirs
}

func isUnder(p, dir string) bool { return dir == "" || p == dir || strings.HasPrefix(p, dir+"/") }

func relTo(p, dir string) string {
	if dir == "" {
		return p
	}
	return strings.TrimPrefix(strings.TrimPrefix(p, dir), "/")
}

type ruleDoc struct {
	path, base string
	globs      []Glob
	rawGlobs   []string
	text       string
}

func attachRules(repo Repo, u Unit, own map[string]bool, sent []string, opts AttachOptions) []Convention {
	var rules []ruleDoc
	for _, f := range repo.Files {
		m := ruleFileRe.FindStringSubmatch(f)
		if m == nil || own[f] {
			continue
		}
		base := m[1]
		if !anyUnder(sent, base) {
			continue
		}
		text, ok := repo.Read(f)
		if !ok {
			continue
		}
		rd := ruleDoc{path: f, base: base, text: text}
		rd.rawGlobs = ruleGlobs(text)
		for _, g := range rd.rawGlobs {
			if cg, ok := CompileGlob(g); ok {
				rd.globs = append(rd.globs, cg)
			}
		}
		rules = append(rules, rd)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].path < rules[j].path })
	refs := referencedPaths(u)
	var out []Convention
	for _, r := range rules {
		why := ""
		switch {
		case len(r.rawGlobs) == 0:
			why = WhyUnconditional
		case matchesAny(r, sent):
			why = WhyMatchesFile
		case matchesAnyRef(r, refs):
			why = WhyMatchesReference
		}
		if why == "" {
			continue
		}
		out = append(out, conventionOf("rule", r.path, r.base, r.rawGlobs, why, r.text, opts))
	}
	if u.Kind == KindRule {
		out = append(out, overlappingRules(repo, u, own, opts)...)
	}
	return out
}

func anyUnder(files []string, base string) bool {
	for _, f := range files {
		if isUnder(f, base) {
			return true
		}
	}
	return false
}

func matchesAny(r ruleDoc, files []string) bool {
	for _, f := range files {
		if !isUnder(f, r.base) {
			continue
		}
		rel := relTo(f, r.base)
		for _, g := range r.globs {
			if g.Match(rel) {
				return true
			}
		}
	}
	return false
}

func matchesAnyRef(r ruleDoc, refs []string) bool {
	for _, ref := range refs {
		for _, cand := range []string{ref, relTo(ref, r.base)} {
			for _, g := range r.globs {
				if g.Match(cand) {
					return true
				}
			}
		}
	}
	return false
}

// referencedPaths returns the path-like tokens a unit's text mentions, in order of first appearance.
func referencedPaths(u Unit) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range u.Files {
		for _, line := range f.Lines {
			for _, tok := range pathTokRe.FindAllString(line, -1) {
				tok = strings.TrimLeft(strings.Trim(tok, ".,;:)(\"'`"), "./")
				if tok == "" || seen[tok] || strings.Contains(tok, "://") || strings.HasPrefix(tok, "@") {
					continue
				}
				seen[tok] = true
				out = append(out, tok)
				if len(out) >= maxReferencedTokens {
					return out
				}
			}
		}
	}
	return out
}

// ruleGlobs reads the `paths:` frontmatter of a rule: a list, or one string.
func ruleGlobs(text string) []string {
	fm := frontmatter.SplitString(text)
	if !fm.Closed {
		return nil
	}
	var doc map[string]any
	if yaml.Unmarshal([]byte(fm.Raw), &doc) != nil {
		return nil
	}
	switch v := doc["paths"].(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

func overlappingRules(repo Repo, u Unit, own map[string]bool, opts AttachOptions) []Convention {
	self, _ := repoRule(repo, u.Primary)
	if len(self.rawGlobs) == 0 {
		return nil
	}
	var out []Convention
	for _, f := range repo.Files {
		m := ruleFileRe.FindStringSubmatch(f)
		if m == nil || own[f] || !sharesAncestry(m[1], self.base) {
			continue
		}
		other, ok := repoRule(repo, f)
		if !ok {
			continue
		}
		if len(other.rawGlobs) == 0 || !globsOverlap(self, other) {
			continue
		}
		out = append(out, conventionOf("rule", f, other.base, other.rawGlobs, WhyOverlap, other.text, opts))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func repoRule(repo Repo, p string) (ruleDoc, bool) {
	m := ruleFileRe.FindStringSubmatch(p)
	if m == nil {
		return ruleDoc{}, false
	}
	text, ok := repo.Read(p)
	if !ok {
		return ruleDoc{}, false
	}
	rd := ruleDoc{path: p, base: m[1], text: text, rawGlobs: ruleGlobs(text)}
	for _, g := range rd.rawGlobs {
		if cg, ok := CompileGlob(g); ok {
			rd.globs = append(rd.globs, cg)
		}
	}
	return rd, true
}

func sharesAncestry(a, b string) bool { return isUnder(a, b) || isUnder(b, a) }

// globsOverlap reports whether some path plausibly matches a pattern of both rules: a sample path
// built from one pattern is tried against the other's patterns, both ways round.
func globsOverlap(a, b ruleDoc) bool {
	try := func(x, y ruleDoc) bool {
		for _, raw := range x.rawGlobs {
			for _, sample := range globSamples(raw) {
				full := path.Join(x.base, sample)
				if !isUnder(full, y.base) {
					continue
				}
				rel := relTo(full, y.base)
				for _, g := range y.globs {
					if g.Match(rel) {
						return true
					}
				}
			}
		}
		return false
	}
	if len(a.globs) == 0 || len(b.globs) == 0 {
		return false
	}
	return try(a, b) || try(b, a)
}

// globSamples builds concrete paths a pattern matches.
func globSamples(pattern string) []string {
	var out []string
	for _, alt := range expandGlobBraces(strings.TrimPrefix(strings.TrimSpace(pattern), "./")) {
		s := strings.ReplaceAll(alt, "**/", "x/")
		s = strings.ReplaceAll(s, "**", "x/y")
		s = strings.ReplaceAll(s, "*", "a")
		s = strings.ReplaceAll(s, "?", "a")
		out = append(out, s)
	}
	return out
}

func attachInstructionFiles(repo Repo, u Unit, own map[string]bool, opts AttachOptions) []Convention {
	have := make(map[string]bool, len(repo.Files))
	for _, f := range repo.Files {
		have[f] = true
	}
	var out []Convention
	seen := map[string]bool{}
	var visit func(rel, why string, depth int)
	visit = func(rel, why string, depth int) {
		if seen[rel] || own[rel] || depth > MaxImportDepth {
			return
		}
		text, ok := repo.Read(rel)
		if !ok {
			return
		}
		seen[rel] = true
		out = append(out, conventionOf("claude-md", rel, path.Dir(rel), nil, why, text, opts))
		for _, imp := range importsOf(text) {
			target := path.Clean(path.Join(path.Dir(rel), imp))
			if strings.HasPrefix(target, "..") || !have[target] {
				continue
			}
			visit(target, WhyImported, depth+1)
		}
	}
	for _, d := range ancestorDirs(u.Primary) {
		for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", ".claude/CLAUDE.md"} {
			rel := path.Join(d, name)
			if have[rel] {
				visit(rel, WhyAncestor, 1)
			}
		}
	}
	return out
}

// importsOf lists the @path imports of an instruction file, outside code fences and code spans.
func importsOf(text string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") || strings.HasPrefix(strings.TrimSpace(line), "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line = stripCodeSpans(line)
		for _, m := range importRe.FindAllStringSubmatch(line, -1) {
			p := m[1]
			if strings.HasPrefix(p, "~") || !strings.Contains(p, ".") && !strings.Contains(p, "/") {
				continue
			}
			out = append(out, p)
		}
	}
	return out
}

var codeSpanRe = regexp.MustCompile("`[^`]*`")

func stripCodeSpans(s string) string { return codeSpanRe.ReplaceAllString(s, " ") }

func conventionOf(kind, p, base string, globs []string, why, text string, opts AttachOptions) Convention {
	sum := sha256.Sum256([]byte(text))
	c := Convention{ID: kind + ":" + p, Path: p, Paths: globs, Why: why, SHA256: hex.EncodeToString(sum[:8]), Base: base}
	lines := splitLines(defuseMarkers(llm.RedactSecrets(maskSecretValues(text, opts.Secrets))))
	limit := UnconditionalFullTokens * 2
	if why == WhyUnconditional {
		limit = UnconditionalFullTokens
	}
	if tokens.Estimate(text) <= limit {
		c.Lines, c.Full = lines, true
		return c
	}
	c.Index = indexOf(lines)
	return c
}

// budgetConventions turns the conventions past the total token budget into indexes.
func budgetConventions(cs []Convention) {
	total := 0
	for i := range cs {
		n := 0
		for _, l := range cs[i].Lines {
			n += tokens.Estimate(l) + 1
		}
		if total+n > maxConventionTokens && cs[i].Full {
			cs[i].Full = false
			cs[i].Index = indexOf(cs[i].Lines)
			cs[i].Lines = nil
			continue
		}
		total += n
	}
}

func indexOf(lines []string) []string {
	var out []string
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			out = append(out, "L"+itoa(i+1)+"| "+strings.TrimSpace(l))
		}
	}
	return out
}
