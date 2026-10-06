package lint

import (
	_ "embed" // the trap table
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

// Harness traps (AR9C1...): files a harness silently ignores. The traps are
// rows of an embedded table with a closed predicate vocabulary, so adding one
// is data plus a fixture, and every row carries its evidence and the date it
// was checked. They run only for harnesses that are configured as presets (or
// named in [lint.traps] extra_harnesses), over the repository's tracked files.

//go:embed traps.toml
var trapsTOML []byte

// Predicate kinds of the closed vocabulary.
const (
	predExtNotIn           = "ext-not-in"
	predNameSuffixRequired = "name-suffix-required"
	predFrontmatterEnum    = "frontmatter-enum"
	predFrontmatterMissing = "frontmatter-missing-all"
	predKeyMisspelt        = "key-misspelt"
	predSizeOver           = "size-over"
)

// File kinds a trap can apply to.
const (
	kindGenerated   = "generated"
	kindHandwritten = "handwritten"
)

// maxTrapFileBytes bounds how much of a file a predicate reads.
const maxTrapFileBytes = 1 << 20

type trapTable struct {
	Schema int    `toml:"schema"`
	Trap   []Trap `toml:"trap"`
}

// Trap is one row of the harness trap table.
type Trap struct {
	Code           string        `toml:"code"`
	Name           string        `toml:"name"`
	Harness        string        `toml:"harness"`
	Message        string        `toml:"message"`
	Scope          TrapScope     `toml:"scope"`
	Predicate      TrapPredicate `toml:"predicate"`
	CertainlyInert bool          `toml:"certainly_inert"`
	Hint           string        `toml:"hint"`
	Source         string        `toml:"source"`
	Quote          string        `toml:"quote"`
	VerifiedOn     string        `toml:"verified_on"`
	HarnessVersion string        `toml:"harness_version"`
}

// TrapScope selects the files a trap looks at: files below Dir (at the repo or
// root level, or in a nested package), optionally ending in Suffix, of the
// listed Kinds ("generated" carries the ai-rulez banner).
type TrapScope struct {
	Dir    string   `toml:"dir"`
	Suffix string   `toml:"suffix"`
	Kinds  []string `toml:"kinds"`
}

// TrapPredicate is the condition of a trap; only the fields of its Kind are read.
type TrapPredicate struct {
	Kind        string   `toml:"kind"`
	Allowed     []string `toml:"allowed"`
	IgnoreNames []string `toml:"ignore_names"`
	Suffix      string   `toml:"suffix"`
	Key         string   `toml:"key"`
	Keys        []string `toml:"keys"`
	Unless      []string `toml:"unless"`
	// Canonical lists the real key spellings of key-misspelt: a key that equals
	// one ignoring case, hyphens and underscores, but not exactly, is flagged.
	Canonical []string `toml:"canonical"`
	// LimitID names the row of limits.toml a size-over predicate measures against.
	LimitID string `toml:"limit_id"`
	// Measure is what size-over counts: frontmatter-chars (the values of Keys),
	// file-chars, file-bytes, or chain-bytes (the AGENTS.md files from the lint
	// root down to the file).
	Measure string `toml:"measure"`

	limit harnesslimits.Limit // resolved from LimitID when the table loads
}

var (
	trapsOnce   sync.Once
	trapsLoaded []Trap
	trapsErr    error
)

// Traps returns the embedded trap table.
func Traps() ([]Trap, error) {
	trapsOnce.Do(func() {
		var t trapTable
		if err := toml.Unmarshal(trapsTOML, &t); err != nil {
			trapsErr = fmt.Errorf("parse traps.toml: %w", err)
			return
		}
		if err := resolveLimits(t.Trap); err != nil {
			trapsErr = err
			return
		}
		trapsLoaded = t.Trap
	})
	return trapsLoaded, trapsErr
}

// trapHit is one predicate match.
type trapHit struct {
	line   int
	detail string
}

// matches reports whether rel (slash path from the lint root) is inside the
// scope directory, at the root or in a nested package, and has the suffix.
func (s TrapScope) matches(rel string) bool {
	dir := strings.Trim(s.Dir, "/")
	if dir != "" && !(strings.HasPrefix(rel, dir+"/") || strings.Contains(rel, "/"+dir+"/")) {
		return false
	}
	return s.Suffix == "" || strings.HasSuffix(strings.ToLower(rel), strings.ToLower(s.Suffix))
}

func (s TrapScope) hasKind(kind string) bool {
	return len(s.Kinds) == 0 || slices.Contains(s.Kinds, kind)
}

// eval runs the predicate against one file.
func (p TrapPredicate) eval(rel string, content []byte) []trapHit {
	name := path.Base(rel)
	if slices.ContainsFunc(p.IgnoreNames, func(n string) bool { return strings.EqualFold(n, name) }) {
		return nil
	}
	switch p.Kind {
	case predExtNotIn:
		ext := strings.ToLower(path.Ext(name))
		if !slices.Contains(p.Allowed, ext) {
			return []trapHit{{line: 1, detail: fmt.Sprintf("extension %q", ext)}}
		}
	case predNameSuffixRequired:
		if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(p.Suffix)) {
			return []trapHit{{line: 1}}
		}
	case predFrontmatterEnum:
		return p.evalEnum(content)
	case predFrontmatterMissing:
		return p.evalMissing(content)
	case predKeyMisspelt:
		return p.evalMisspelt(content)
	case predSizeOver:
		return p.evalSize(content)
	}
	return nil
}

func (p TrapPredicate) evalEnum(content []byte) []trapHit {
	d := parseDoc(string(content))
	k, ok := parseFrontmatterDoc(d).top(p.Key)
	if !ok {
		return nil
	}
	values := []string{scalar(k.Value)}
	if list, isList := k.Value.([]any); isList {
		values = values[:0]
		for _, v := range list {
			values = append(values, scalar(v))
		}
	}
	for _, v := range values {
		if !slices.Contains(p.Allowed, v) {
			return []trapHit{{line: k.Line, detail: fmt.Sprintf("%s is %q", p.Key, sanitizeScannerText(v))}}
		}
	}
	return nil
}

func (p TrapPredicate) evalMissing(content []byte) []trapHit {
	d := parseDoc(string(content))
	fm := parseFrontmatterDoc(d)
	for _, key := range p.Keys {
		if k, ok := fm.top(key); ok && frontmatterSet(k.Value) {
			return nil
		}
	}
	for _, key := range p.Unless {
		if k, ok := fm.top(key); ok && frontmatterTruthy(k.Value) {
			return nil
		}
	}
	return []trapHit{{line: 1}}
}

// frontmatterSet reports a value that says something: not empty, not false.
func frontmatterSet(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	case bool:
		return t
	}
	return scalar(v) != ""
}

func frontmatterTruthy(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return strings.EqualFold(scalar(v), "true")
}

// activeHarnesses lists the harnesses whose traps run: configured presets plus
// [lint.traps] extra_harnesses.
func (r *runner) activeHarnesses() map[string]bool {
	active := map[string]bool{}
	for _, p := range r.cfg.Presets {
		for _, n := range []string{p.BuiltIn, p.Name} {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				active[n] = true
			}
		}
	}
	if r.lc.Traps != nil {
		for _, h := range r.lc.Traps.ExtraHarnesses {
			active[strings.ToLower(strings.TrimSpace(h))] = true
		}
	}
	return active
}

// checkTraps runs the trap table over the tracked files under the lint root.
// It reads the files on disk, so a generated file is seen as last written.
func (r *runner) checkTraps() {
	r.checkTableAge()
	traps, err := Traps()
	if err != nil || len(traps) == 0 {
		return
	}
	active := r.activeHarnesses()
	var relevant []Trap
	for _, t := range traps {
		if active[t.Harness] {
			relevant = append(relevant, t)
		}
	}
	if len(relevant) == 0 {
		return
	}
	paths := r.tree.Paths()
	paths = append(paths, r.ignoredGeneratedPaths(relevant, paths)...)
	sort.Strings(paths)
	chains := map[string][]chainFile{}
	for _, f := range paths {
		rel, ok := r.underRoot(f)
		if !ok {
			continue
		}
		var content []byte
		var read, generated bool
		for _, t := range relevant {
			if !t.Scope.matches(rel) {
				continue
			}
			if !read {
				content, generated = readTrapFile(filepath.Join(r.tree.Top, filepath.FromSlash(f)))
				read = true
			}
			kind := kindHandwritten
			if generated {
				kind = kindGenerated
			}
			if !t.Scope.hasKind(kind) {
				continue
			}
			abs := filepath.Join(r.tree.Top, filepath.FromSlash(f))
			if t.Predicate.isChain() {
				chains[t.Code+"/"+t.Harness] = append(chains[t.Code+"/"+t.Harness], chainFile{rel: rel, abs: abs, size: len(content), generated: generated})
				continue
			}
			for _, hit := range t.Predicate.eval(rel, content) {
				r.addTrap(t, abs, hit, generated)
			}
		}
	}
	r.checkChains(relevant, chains)
}

// maxIgnoredWalk bounds the files visited when looking for generated outputs
// that git does not list.
const maxIgnoredWalk = 20000

// ignoredGeneratedPaths finds files below the scope directories of the
// generated-kind traps that the tree does not list. A harness such as claude
// gitignores its generated skills and agents, so the tracked-file index never
// sees them; only files carrying the ai-rulez banner are returned, so a
// handwritten ignored file stays out of scope. Paths are relative to the tree top.
func (r *runner) ignoredGeneratedPaths(relevant []Trap, known []string) []string {
	have := make(map[string]bool, len(known))
	for _, k := range known {
		have[k] = true
	}
	dirs := map[string]bool{}
	for _, t := range relevant {
		if d := strings.Trim(t.Scope.Dir, "/"); d != "" && slices.Contains(t.Scope.Kinds, kindGenerated) {
			dirs[d] = true
		}
	}
	var out []string
	visited := 0
	for d := range dirs {
		start := filepath.Join(r.tree.Top, filepath.FromSlash(path.Join(r.baseRel, d)))
		_ = filepath.WalkDir(start, func(p string, e fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
			if err != nil || visited > maxIgnoredWalk {
				return nil //nolint:nilerr // best effort
			}
			if e.IsDir() || !e.Type().IsRegular() {
				return nil
			}
			visited++
			rel, rerr := filepath.Rel(r.tree.Top, p)
			if rerr != nil {
				return nil //nolint:nilerr // best effort
			}
			rel = filepath.ToSlash(rel)
			if have[rel] {
				return nil
			}
			if _, generated := readTrapFile(p); generated {
				have[rel] = true
				out = append(out, rel)
			}
			return nil
		})
	}
	return out
}

// underRoot returns f relative to the lint root, or false when f is outside it.
func (r *runner) underRoot(f string) (string, bool) {
	if r.baseRel == "" {
		return f, true
	}
	rel, ok := strings.CutPrefix(f, r.baseRel+"/")
	return rel, ok
}

// readTrapFile reads up to maxTrapFileBytes and reports whether the file
// carries the ai-rulez generated banner near its top.
func readTrapFile(abs string) (content []byte, generated bool) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, false
	}
	defer file.Close() //nolint:errcheck // read only
	buf := make([]byte, maxTrapFileBytes)
	n, _ := file.Read(buf) //nolint:errcheck // a short or failed read is treated as what was read
	content = buf[:n]
	head := content
	if len(head) > 2048 {
		head = head[:2048]
	}
	return content, strings.Contains(string(head), "Generated by ai-rulez")
}

// addTrap records one trap finding with its provenance. A file ai-rulez
// generated that the harness certainly ignores is an error (a preset bug),
// unless the severity was set in [lint.severity].
func (r *runner) addTrap(t Trap, abs string, hit trapHit, generated bool) {
	n := len(r.findings)
	msg := t.Message
	if hit.detail != "" {
		msg += " (" + hit.detail + ")"
	}
	if generated {
		msg += "; ai-rulez generated this file, so this is a bug in the preset"
	}
	r.add(t.Code, abs, hit.line, "%s", msg)
	if len(r.findings) == n {
		return
	}
	f := &r.findings[n]
	f.Trap = &TrapInfo{Harness: t.Harness, Evidence: t.Source, VerifiedOn: t.VerifiedOn, Hint: t.Hint}
	if generated && t.CertainlyInert && !r.severityConfigured(t.Code) {
		f.Severity = SeverityError
	}
}

// severityConfigured reports whether [lint.severity] sets the code.
func (r *runner) severityConfigured(code string) bool {
	for key := range r.lc.Severity {
		if rule, ok := lookupRule(key); ok && rule.Code == code {
			return true
		}
	}
	return false
}

// trapHarnesses lists the harness names the lint settings may list.
func trapHarnesses() []string {
	traps, _ := Traps() //nolint:errcheck // an unreadable table has no harnesses
	set := map[string]bool{}
	for _, t := range traps {
		set[t.Harness] = true
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// validateTraps checks [lint.traps].
func validateTraps(tr *config.LintTraps) []string {
	if tr == nil {
		return nil
	}
	known := trapHarnesses()
	var problems []string
	for _, h := range tr.ExtraHarnesses {
		if !slices.Contains(known, strings.ToLower(strings.TrimSpace(h))) {
			problems = append(problems, fmt.Sprintf("lint.traps.extra_harnesses: unknown harness %q (known: %s)", h, strings.Join(known, ", ")))
		}
	}
	return problems
}

// TrapsMarkdown renders the trap table as the Markdown table docs/harness-traps.md embeds.
func TrapsMarkdown() (string, error) {
	traps, err := Traps()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("| Code | Name | Harness | Inert | Fires when | Fix | Evidence |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, t := range traps {
		inert := "no"
		if t.CertainlyInert {
			inert = "yes"
		}
		fmt.Fprintf(&sb, "| %s | `%s` | %s | %s | %s | %s | [%s](%s) `%s` (verified %s) |\n",
			t.Code, t.Name, t.Harness, inert, mdCell(t.Message), mdCell(t.Hint), t.Harness, t.Source, strings.ReplaceAll(t.Quote, "|", `\|`), t.VerifiedOn)
	}
	return sb.String(), nil
}

// mdCell escapes text for a Markdown table cell.
func mdCell(s string) string {
	return strings.NewReplacer("|", `\|`, "<", "&lt;", ">", "&gt;", "*", `\*`).Replace(s)
}
