package exposure

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// maxImportHops is how deep Claude Code follows a memory import: an imported
// file may import another, up to five hops from the file that started the
// chain. The same limit as AR210 (internal/lint) so both describe one loader.
const maxImportHops = 5

var (
	importRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[\s(\[])@([^\s)\]>"'` + "`" + `]+)`)
	})
)

// importExtension reports whether ext is a text file a memory import can bring
// into context. Anything else is not modeled, which fails a configured start
// budget closed.
func importExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".md", ".mdc", ".mdx", ".markdown", ".txt", ".json", ".yaml", ".yml", ".toml", ".rst":
		return true
	}
	return false
}

// importRef is one `@path` directive: the path as written and its 1-based line
// within the text it was read from.
type importRef struct {
	line int
	path string
}

// Import is a local file a CLAUDE.md or rule includes with `@path`, and the
// cost it adds to the importing artifact.
type Import struct {
	// Path is relative to the repository root, slash separated.
	Path      string `json:"path"`
	TextBytes int    `json:"text_bytes"`
	Tokens    int    `json:"tokens"`
}

// ImportProblem is an import of a start-of-session instruction file that the
// inventory could not model: an absolute or home path, a path that resolves
// outside the root, an unreadable target, or an importing file that could not
// be read. A start budget fails closed on one (see Report.OverStartBudget).
type ImportProblem struct {
	// Path is the directive as written, or the importing file when the file
	// itself could not be read.
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// instructionKind reports whether a kind is a memory file Claude Code evaluates
// `@path` imports in. A skill, agent or command body is plain text.
func instructionKind(kind Kind) bool { return kind == KindClaudeMD || kind == KindRule }

// resolveImports follows the `@path` imports of an instruction artifact,
// recursively, and adds their token cost to it. Imports resolve beside the file
// that writes them, as Claude Code resolves them. A path that is absolute,
// starts with `~`, or leaves the repository root is not followed and becomes an
// ImportProblem; so is a target that exists but cannot be read. A missing target
// loads nothing, and a file already visited in the chain is not counted twice.
func (b *builder) resolveImports(a *Artifact) {
	if !instructionKind(a.Kind) {
		return
	}
	if reason := a.UnreadReason(); reason != "" {
		a.ImportProblems = append(a.ImportProblems, ImportProblem{
			Path: a.Path, Reason: "the importing file was not read: " + reason,
		})
		return
	}
	seen := map[string]bool{a.Path: true}
	problems := map[ImportProblem]bool{}
	var imports []Import
	var walk func(from, text string, hops int)
	walk = func(from, text string, hops int) {
		for _, ref := range importRefs(text) {
			if hops >= maxImportHops {
				// Claude Code stops after five hops, so this directive loads
				// nothing and costs nothing.
				continue
			}
			rel, reason := resolveImportPath(from, ref.path)
			if reason != "" {
				problems[ImportProblem{Path: ref.path, Reason: reason}] = true
				continue
			}
			if rel == "" || seen[rel] {
				continue
			}
			seen[rel] = true
			im, problem, ok, imported := b.readImport(rel)
			if problem != "" {
				problems[ImportProblem{Path: ref.path, Reason: problem}] = true
				continue
			}
			if !ok {
				continue
			}
			imports = append(imports, im)
			if imported != "" {
				walk(rel, imported, hops+1)
			}
		}
	}
	walk(a.Path, a.Text, 0)
	sort.Slice(imports, func(i, j int) bool { return imports[i].Path < imports[j].Path })
	a.Imports = imports
	for _, im := range imports {
		a.ImportTokens += im.Tokens
	}
	// Tokens is what the artifact brings into context: its own text and every
	// imported file's text.
	a.Tokens = EstimateTokens(a.TextBytes) + a.ImportTokens
	for p := range problems {
		a.ImportProblems = append(a.ImportProblems, p)
	}
	sort.Slice(a.ImportProblems, func(i, j int) bool {
		if a.ImportProblems[i].Path != a.ImportProblems[j].Path {
			return a.ImportProblems[i].Path < a.ImportProblems[j].Path
		}
		return a.ImportProblems[i].Reason < a.ImportProblems[j].Reason
	})
}

// readImport reads one resolved import: its cost, an unmodeled problem (empty
// when the file is modelable), whether it contributed an entry, and its text
// when there is one to follow further.
func (b *builder) readImport(rel string) (im Import, problem string, ok bool, text string) {
	r := b.read(rel)
	switch {
	case r.status == readGone:
		return Import{}, "", false, ""
	case r.status == readRefused && r.oversized == 0:
		return Import{}, r.reason, false, ""
	}
	bytes := len(r.text)
	if r.oversized > 0 {
		// A file over the read limit is counted by its size, as an unread
		// artifact is.
		bytes = r.oversized
	}
	return Import{Path: rel, TextBytes: bytes, Tokens: EstimateTokens(bytes)}, "", true, r.text
}

// importRefs lists the `@path` directives of a markdown text. Directives inside
// fenced code and inline code spans are not evaluated, as Claude Code does not
// evaluate them there.
func importRefs(text string) []importRef {
	var out []importRef
	var open string
	for i, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if fence, rest, ok := fenceOpen(trimmed); ok {
			switch {
			case open == "":
				open = fence
			case fence == open || rest == "":
				open = ""
			}
			continue
		}
		if open != "" {
			continue
		}
		plain := codeSpanRe.ReplaceAllStringFunc(line, func(s string) string { return strings.Repeat(" ", len(s)) })
		for _, m := range importRe().FindAllStringSubmatch(plain, -1) {
			p := strings.TrimRight(m[1], ".,;:!?")
			if p == "" || strings.Contains(p, "@") || strings.Contains(p, "://") || placeholderRe.MatchString(p) {
				continue
			}
			if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") && !importExtension(path.Ext(p)) {
				continue
			}
			out = append(out, importRef{line: i + 1, path: p})
		}
	}
	return out
}

var codeSpanRe = regexp.MustCompile("`[^`\n]*`")

var placeholderRe = regexp.MustCompile(`\{\{|\$\{`)

// fenceOpen reports whether a trimmed line opens or closes a fenced block: the
// opening run, and the text after it.
func fenceOpen(line string) (fence, rest string, ok bool) {
	for _, mark := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, mark) {
			run := 0
			for run < len(line) && line[run] == mark[0] {
				run++
			}
			return line[:run], strings.TrimSpace(line[run:]), true
		}
	}
	return "", "", false
}

// resolveImportPath turns a directive into a path relative to the root. reason
// is set for a directive the inventory refuses to model (an absolute or home
// path, or one that leaves the root); rel is empty for a directive that names
// no local file (a URL, an email, a package name).
func resolveImportPath(from, ref string) (rel, reason string) {
	if strings.HasPrefix(ref, "~") || strings.HasPrefix(ref, "/") || path.IsAbs(ref) {
		return "", "an absolute or home path is not modeled"
	}
	dir := path.Dir(from)
	if dir == "." {
		dir = ""
	}
	clean := path.Clean(path.Join(dir, ref))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "the path resolves outside the repository root"
	}
	return clean, ""
}
