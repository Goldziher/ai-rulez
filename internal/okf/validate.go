package okf

import (
	"fmt"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Finding is one problem in a bundle. Path is bundle-relative.
type Finding struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Line     int      `json:"line,omitempty"`
	Message  string   `json:"message"`
}

// NewFinding builds a finding with the code's default severity.
func NewFinding(code, p string, line int, format string, args ...any) Finding {
	return Finding{
		Code: code, Name: RuleName(code), Severity: DefaultSeverity(code),
		Path: p, Line: line, Message: fmt.Sprintf(format, args...),
	}
}

var (
	versionRe  = regexp.MustCompile(`^\d+\.\d+$`)
	logDateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	linkRe     = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	codeSpanRe = regexp.MustCompile("`[^`\n]*`")
)

// Validate checks the bundle against the conformance rules of SPEC section 11
// plus the softer hygiene checks. The result is sorted by path, line and code
// so it can be diffed.
func (b *Bundle) Validate() []Finding {
	var out []Finding
	out = append(out, b.checkPaths()...)
	out = append(out, b.checkConcepts()...)
	out = append(out, b.checkReserved()...)
	out = append(out, b.checkIndexes()...)
	out = append(out, b.checkLinksAndOrphans()...)
	out = append(out, b.checkTitles()...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// CheckRoot reports an error when the bundle has no root index.md naming
// okf_version. Reading tolerates that (SPEC section 11), but a directory
// without one is not a bundle, and import okf refuses it the same way, so
// validate must not pass it. Validate does not include this check: the
// importers and project lint load partial trees on purpose.
func (b *Bundle) CheckRoot() []Finding {
	if idx, ok := b.Indexes[IndexFile]; ok && idx.Frontmatter.Present && idx.Frontmatter.Lookup(keyOKFVersion) != nil {
		return nil
	}
	f := NewFinding(CodeVersionInvalid, IndexFile, 0, "not an OKF bundle: expected an index.md naming okf_version at the bundle root")
	f.Severity = SeverityError
	return []Finding{f}
}

func (b *Bundle) checkPaths() []Finding {
	var out []Finding
	for _, p := range b.Problems {
		out = append(out, NewFinding(CodePathUnsafe, p.Path, 0, "%s", p.Message))
	}
	seen := map[string]string{}
	all := make([]string, 0, len(b.Files)+len(b.Dirs))
	for p := range b.Files {
		all = append(all, p)
	}
	for p := range b.Dirs {
		if p != "" {
			all = append(all, p)
		}
	}
	sort.Strings(all)
	for _, p := range all {
		key := strings.ToLower(p)
		if prev, ok := seen[key]; ok {
			out = append(out, NewFinding(CodePathUnsafe, p, 0, "differs only in case from %s", prev))
			continue
		}
		seen[key] = p
	}
	return out
}

func (b *Bundle) checkConcepts() []Finding {
	var out []Finding
	for _, p := range b.ConceptPaths() {
		c := b.Concepts[p]
		switch {
		case c.Frontmatter.Err != nil:
			out = append(out, NewFinding(CodeTypeInvalid, p, 1, "unparseable frontmatter: %v", c.Frontmatter.Err))
		case !c.Frontmatter.Present:
			out = append(out, NewFinding(CodeTypeInvalid, p, 1, "no frontmatter block; every concept needs at least `type`"))
		case c.Type() == "":
			out = append(out, NewFinding(CodeTypeInvalid, p, 1, "frontmatter has no non-empty `type`"))
		}
	}
	return out
}

func (b *Bundle) checkReserved() []Finding {
	var out []Finding
	for _, p := range sortedMapKeys(b.Indexes) {
		idx := b.Indexes[p]
		fm := idx.Frontmatter
		if fm.Err != nil {
			out = append(out, NewFinding(CodeReservedStructure, p, 1, "unparseable frontmatter: %v", fm.Err))
			continue
		}
		if !fm.Present {
			continue
		}
		if idx.Style == StyleFrontmatter {
			out = append(out, frontmatterStyleFindings(p, idx)...)
			continue
		}
		if p != IndexFile {
			out = append(out, NewFinding(CodeReservedStructure, p, 1, "only the bundle-root index may carry frontmatter (SPEC section 8)"))
			continue
		}
		for _, k := range fm.Keys() {
			if k != keyOKFVersion {
				out = append(out, NewFinding(CodeReservedStructure, p, 1, "root index frontmatter may only contain okf_version, found %q", k))
			}
		}
		if v := fm.Lookup(keyOKFVersion); v != nil {
			out = append(out, versionFinding(p, strings.TrimSpace(v.Value))...)
		}
	}
	for _, p := range sortedMapKeys(b.Logs) {
		log := b.Logs[p]
		for i, line := range strings.Split(log.Body, "\n") {
			if h, ok := strings.CutPrefix(line, "## "); ok && !logDateRe.MatchString(strings.TrimSpace(h)) {
				f := NewFinding(CodeReservedStructure, p, i+1, "log heading %q is not an ISO YYYY-MM-DD date", strings.TrimSpace(h))
				f.Severity = SeverityWarning
				out = append(out, f)
			}
		}
	}
	return out
}

// frontmatterStyleFindings checks an index in the frontmatter style: only title,
// version and entries (plus okf_version at the root), and an info note that the
// bundle uses the scheme OKF 0.2 does not describe.
func frontmatterStyleFindings(p string, idx *IndexFileDoc) []Finding {
	var out []Finding
	for _, k := range idx.Frontmatter.Keys() {
		if frontmatterStyleKeys[k] || (k == keyOKFVersion && p == IndexFile) {
			continue
		}
		out = append(out, NewFinding(CodeReservedStructure, p, 1, "index frontmatter may only contain title, version, entries and (at the root) okf_version, found %q", k))
	}
	if v := idx.Frontmatter.Lookup(keyOKFVersion); v != nil && p == IndexFile {
		out = append(out, versionFinding(p, strings.TrimSpace(v.Value))...)
	}
	if p == IndexFile {
		f := NewFinding(CodeVersionInvalid, p, 1, "the index uses the frontmatter style (title, version %s, entries), not the body listing of OKF %s", idx.Frontmatter.Scalar("version"), SpecVersion)
		f.Severity = SeverityInfo
		out = append(out, f)
	}
	return out
}

func versionFinding(p, v string) []Finding {
	switch {
	case !versionRe.MatchString(v):
		return []Finding{NewFinding(CodeVersionInvalid, p, 1, "okf_version %q is not MAJOR.MINOR", v)}
	case v != SpecVersion:
		f := NewFinding(CodeVersionInvalid, p, 1, "okf_version %s is not the version ai-rulez implements (%s); reading best-effort", v, SpecVersion)
		f.Severity = SeverityInfo
		return []Finding{f}
	}
	return nil
}

// resolve maps a link target found in a file inside dir to a bundle path. ok is
// false for external URLs and pure anchors; inside is false when the target
// escapes the bundle.
func resolve(dir, target string) (p string, ok, inside bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return "", false, true
	}
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		target = target[:i]
	}
	if un, err := url.PathUnescape(target); err == nil {
		target = un
	}
	var joined string
	if rest, abs := strings.CutPrefix(target, "/"); abs {
		joined = rest
	} else {
		joined = path.Join(dir, target)
	}
	cleaned := path.Clean(joined)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", true, false
	}
	if cleaned == "." {
		cleaned = ""
	}
	return cleaned, true, true
}

func dirOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

func (b *Bundle) exists(p string) bool { return b.Files[p] || b.Dirs[p] }

func (b *Bundle) checkIndexes() []Finding {
	var out []Finding
	conceptsByDir, subdirsByDir := b.conceptLayout()
	for _, p := range sortedMapKeys(b.Indexes) {
		idx := b.Indexes[p]
		dir := dirOf(p)
		listed := map[string]bool{}
		for _, e := range idx.Entries {
			target, ok, inside := resolve(dir, e.Target)
			if !ok {
				continue
			}
			line := e.fileLine(idx.BodyOffset)
			if !inside || !b.exists(target) {
				out = append(out, NewFinding(CodeIndexMismatch, p, line, "entry %q points at %s, which is not in the bundle", e.Title, e.Target))
				continue
			}
			listed[target] = true
			if b.Dirs[target] {
				listed[path.Join(target, IndexFile)] = true
			}
			if strings.HasSuffix(target, "/"+IndexFile) || target == IndexFile {
				listed[dirOf(target)] = true
			}
		}
		for _, c := range conceptsByDir[dir] {
			if !listed[c] {
				out = append(out, NewFinding(CodeIndexMismatch, p, 1, "concept %s is not listed", path.Base(c)))
			}
		}
		for _, sub := range subdirsByDir[dir] {
			if !listed[sub] {
				out = append(out, NewFinding(CodeIndexMismatch, p, 1, "subdirectory %s/ is not listed", path.Base(sub)))
			}
		}
	}
	return out
}

// conceptLayout groups the concepts by directory and lists, per directory, the
// direct subdirectories that contain a concept at any depth. Computed once, so
// checking many indexes stays linear in the number of concepts.
func (b *Bundle) conceptLayout() (concepts map[string][]string, subdirs map[string][]string) {
	concepts = map[string][]string{}
	subSets := map[string]map[string]bool{}
	for _, c := range b.ConceptPaths() {
		dir := dirOf(c)
		concepts[dir] = append(concepts[dir], c)
		for d := dir; d != ""; d = dirOf(d) {
			parent := dirOf(d)
			if subSets[parent] == nil {
				subSets[parent] = map[string]bool{}
			}
			subSets[parent][d] = true
		}
	}
	subdirs = make(map[string][]string, len(subSets))
	for dir, set := range subSets {
		subdirs[dir] = slices.Sorted(maps.Keys(set))
	}
	return concepts, subdirs
}

func (b *Bundle) checkLinksAndOrphans() []Finding {
	var out []Finding
	referenced := map[string]bool{}
	for _, p := range sortedMapKeys(b.Indexes) {
		for _, e := range b.Indexes[p].Entries {
			if target, ok, inside := resolve(dirOf(p), e.Target); ok && inside {
				referenced[target] = true
				if b.Dirs[target] {
					referenced[path.Join(target, IndexFile)] = true
				}
			}
		}
	}
	for _, p := range b.ConceptPaths() {
		c := b.Concepts[p]
		for _, l := range extractLinks(c.Body) {
			target, ok, inside := resolve(dirOf(p), l.target)
			if !ok {
				continue
			}
			line := c.BodyOffset + l.line
			if !inside || !b.exists(target) {
				out = append(out, NewFinding(CodeLinkBroken, p, line, "link %s does not resolve to a file in the bundle", l.target))
				continue
			}
			if target != p {
				referenced[target] = true
			}
		}
	}
	if len(b.Indexes) > 0 {
		for _, p := range b.ConceptPaths() {
			if !referenced[p] {
				out = append(out, NewFinding(CodeOrphan, p, 1, "no index entry or concept links to this file"))
			}
		}
	}
	return out
}

type link struct {
	target string
	line   int
}

// extractLinks returns inline markdown links outside fenced code and code spans.
func extractLinks(body string) []link {
	var out []link
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line = codeSpanRe.ReplaceAllString(line, "")
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			out = append(out, link{target: m[1], line: i + 1})
		}
	}
	return out
}

func (b *Bundle) checkTitles() []Finding {
	byDir := map[string]map[string]string{}
	var out []Finding
	for _, p := range b.ConceptPaths() {
		c := b.Concepts[p]
		if c.Frontmatter.Scalar(keyTitle) == "" {
			continue
		}
		dir := dirOf(p)
		if byDir[dir] == nil {
			byDir[dir] = map[string]string{}
		}
		key := strings.ToLower(oneLine(c.Title()))
		if prev, ok := byDir[dir][key]; ok {
			out = append(out, NewFinding(CodeTitleDuplicate, p, 1, "title %q is also used by %s", c.Title(), path.Base(prev)))
			continue
		}
		byDir[dir][key] = p
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
