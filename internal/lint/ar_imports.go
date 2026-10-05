package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeImportInvalid reports a Claude memory `@path` import that cannot load.
const CodeImportInvalid = "AR210"

// maxImportHops is how deep Claude Code follows imports: an imported file may
// import another, up to five hops from the file that started the chain.
const maxImportHops = 5

func init() {
	registerRules(RuleInfo{CodeImportInvalid, "import-invalid", SeverityError, "an `@path` memory import points at a missing file, forms a cycle, or sits more than five hops deep, so Claude Code does not load it"})
	registerRunCheck(checkImports)
}

var (
	importRe  = regexp.MustCompile(`(?:^|[\s(\[])@([^\s)\]>"'` + "`" + `]+)`)
	importExt = map[string]bool{".md": true, ".mdc": true, ".mdx": true, ".txt": true, ".markdown": true, ".json": true, ".yaml": true, ".yml": true, ".toml": true, ".rst": true}
)

type importRef struct {
	line int
	path string
}

// importsIn lists the memory imports of a markdown text. Imports inside code
// spans and fenced blocks are not evaluated by Claude Code, nor here.
func importsIn(d doc) []importRef {
	var out []importRef
	for _, l := range d.body() {
		for _, m := range importRe.FindAllStringSubmatch(l.Plain, -1) {
			p := strings.TrimRight(m[1], ".,;:!?")
			if p == "" || strings.Contains(p, "@") || strings.HasPrefix(p, "~") || strings.HasPrefix(p, "/") || strings.Contains(p, "://") || placeholderRe.MatchString(p) {
				continue // home and absolute imports depend on the machine
			}
			if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") && !importExt[strings.ToLower(filepath.Ext(p))] {
				continue
			}
			out = append(out, importRef{line: l.No, path: p})
		}
	}
	return out
}

// resolveImport finds the file an import names: next to the importing file, in
// its skill or command directory, at the root or at the repository top.
func (r *runner) resolveImport(from, itemDir, rel string) string {
	dirs := []string{filepath.Dir(from)}
	if itemDir != "" {
		dirs = append(dirs, itemDir)
	}
	dirs = append(dirs, r.rootAbs(), r.tree.Top)
	for _, dir := range dirs {
		c := filepath.Join(dir, filepath.FromSlash(rel))
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func (r *runner) importsOfFile(abs string) []importRef {
	data, err := readSmallFile(abs)
	if err != nil {
		return nil
	}
	return importsIn(parseDoc(string(data)))
}

func checkImports(r *runner) {
	for i := range r.items {
		it := &r.items[i]
		if !it.owned {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		for _, imp := range importsIn(d) {
			target := r.resolveImport(it.abs, it.itemDir, imp.path)
			if target == "" {
				r.add(CodeImportInvalid, it.abs, imp.line, "import @%s does not resolve to a file, so Claude Code loads nothing for it", imp.path)
				continue
			}
			if msg := r.walkImports(target, it.itemDir, []string{it.abs, target}); msg != "" {
				r.addSev(SeverityWarning, CodeImportInvalid, it.abs, imp.line, "import @%s: %s", imp.path, msg)
			}
		}
	}
}

// walkImports follows the imports of file. stack holds the chain so far (the
// item first); its length minus one is the hop count of file.
func (r *runner) walkImports(file, itemDir string, stack []string) string {
	hops := len(stack) - 1
	for _, imp := range r.importsOfFile(file) {
		next := r.resolveImport(file, itemDir, imp.path)
		if next == "" {
			continue
		}
		for _, seen := range stack {
			if seen == next {
				return "circular import " + r.chain(append(append([]string(nil), stack...), next))
			}
		}
		if hops+1 > maxImportHops {
			return "the chain " + r.chain(append(append([]string(nil), stack...), next)) + " is more than five hops deep; Claude Code stops following imports after five"
		}
		if msg := r.walkImports(next, itemDir, append(append([]string(nil), stack...), next)); msg != "" {
			return msg
		}
	}
	return ""
}

func (r *runner) chain(files []string) string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	return strings.Join(names, " -> ")
}
