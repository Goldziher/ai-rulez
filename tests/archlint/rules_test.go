package archlint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Structural rules beyond ambient authority. Like the ambient check they are a
// ratchet: today's violations are listed in rules_allowlist.txt as
// "<rule> <file> <count>", a new one fails, and an entry whose sites are gone
// must be removed (UPDATE_RULES_ALLOWLIST=1 rewrites the list).
//
//	init        no new init() function
//	global      no new package-level mutable variable (sentinel errors and
//	            compiled regular expressions are not counted)
//	background  no context.Background() or context.TODO() outside package main
//	print       no fmt.Print* outside cmd/ (library code writes through a logger or writer)
//
// The layering rule (internal/lint and internal/config never import cmd/) has
// no allowlist: it holds today and must keep holding.
const (
	ruleInit       = "init"
	ruleGlobal     = "global"
	ruleBackground = "background"
	rulePrint      = "print"

	modulePath = "github.com/Goldziher/ai-rulez/v5/"
)

// layering lists the package trees that must not import the CLI layer.
var layering = []string{"internal/lint/", "internal/config/"}

// sourceDirs are the trees the rules look at.
var sourceDirs = []string{"internal", "cmd", "pkg"}

// walkSources calls fn for every parsed non-test Go file below the source dirs.
func walkSources(t *testing.T, root string, fn func(rel string, file *ast.File)) {
	t.Helper()
	for _, dir := range sourceDirs {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path) //nolint:errcheck // path is below root
			rel = filepath.ToSlash(rel)
			if d.IsDir() || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse %s: %w", rel, err)
			}
			fn(rel, file)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// importNames maps the local name of each import of path-set members to its path.
func importNames(file *ast.File, want func(path string) bool) map[string]string {
	names := map[string]string{}
	for _, imp := range file.Imports {
		p, _ := strconv.Unquote(imp.Path.Value) //nolint:errcheck // a parsed import path is quoted
		if !want(p) {
			continue
		}
		local := filepath.Base(p)
		if imp.Name != nil {
			local = imp.Name.Name
		}
		names[local] = p
	}
	return names
}

// selectorCalls counts pkg.Name(...) calls where pkg is a local name bound to
// importPath and match accepts Name.
func selectorCalls(file *ast.File, importPath string, match func(name string) bool) int {
	names := importNames(file, func(p string) bool { return p == importPath })
	if len(names) == 0 {
		return 0
	}
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Obj != nil {
			return true
		}
		if _, bound := names[id.Name]; bound && match(sel.Sel.Name) {
			n++
		}
		return true
	})
	return n
}

// sentinelValue reports whether a package-level initializer builds an error, a
// compiled regular expression, or a sync.OnceValue that compiles one on first
// use (lazy, and immutable in practice).
func sentinelValue(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		switch fn.Sel.Name {
		case "New", "Errorf", "MustCompile", "Join", "OnceValue", "OnceValues":
			return true
		}
	case *ast.Ident:
		return fn.Name == "MustCompile"
	}
	return false
}

func countGlobals(file *ast.File) int {
	n := 0
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name == "_" || strings.HasPrefix(name.Name, "err") || strings.HasPrefix(name.Name, "Err") {
					continue
				}
				if len(vs.Values) == len(vs.Names) && sentinelValue(vs.Values[i]) {
					continue
				}
				n++
			}
		}
	}
	return n
}

func countInits(file *ast.File) int {
	n := 0
	for _, decl := range file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "init" {
			n++
		}
	}
	return n
}

// scanRules returns "<rule> <file>" -> count, and the layering violations.
func scanRules(t *testing.T, root string) (found map[string]int, layer []string) {
	t.Helper()
	found = map[string]int{}
	add := func(rule, rel string, n int) {
		if n > 0 {
			found[rule+" "+rel] += n
		}
	}
	walkSources(t, root, func(rel string, file *ast.File) {
		add(ruleInit, rel, countInits(file))
		add(ruleGlobal, rel, countGlobals(file))
		if file.Name.Name != "main" {
			add(ruleBackground, rel, selectorCalls(file, "context", func(n string) bool { return n == "Background" || n == "TODO" }))
		}
		if !strings.HasPrefix(rel, "cmd/") {
			add(rulePrint, rel, selectorCalls(file, "fmt", func(n string) bool { return strings.HasPrefix(n, "Print") }))
		}
		for _, prefix := range layering {
			if !strings.HasPrefix(rel, prefix) {
				continue
			}
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value) //nolint:errcheck // a parsed import path is quoted
				if strings.HasPrefix(p, modulePath+"cmd/") || p == modulePath+"cmd" {
					layer = append(layer, rel+" imports "+p)
				}
			}
		}
	})
	sort.Strings(layer)
	return found, layer
}

func readRulesAllowlist(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // repository file
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("rules allowlist line %q: want \"<rule> <file> <count>\"", line)
		}
		n, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("rules allowlist line %q: %v", line, err)
		}
		out[fields[0]+" "+fields[1]] = n
	}
	return out
}

func writeRulesAllowlist(t *testing.T, path string, found map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Structural-rule violations that exist today (see rules_test.go).\n")
	b.WriteString("# <rule> <file> <count>. The list only shrinks: do not add a line, fix the code.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, found[k])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil { //nolint:gosec // repository file
		t.Fatal(err)
	}
}

func TestStructuralRules(t *testing.T) {
	root := repoRoot(t)
	list := filepath.Join(root, "tests", "archlint", "rules_allowlist.txt")
	found, layer := scanRules(t, root)
	if os.Getenv("UPDATE_RULES_ALLOWLIST") != "" {
		writeRulesAllowlist(t, list, found)
		return
	}
	allowed := readRulesAllowlist(t, list)
	var problems []string
	for key, n := range found {
		if n > allowed[key] {
			problems = append(problems, fmt.Sprintf("NEW   %s: %d site(s), allowlist has %d", key, n, allowed[key]))
		}
	}
	for key, n := range allowed {
		if found[key] < n {
			problems = append(problems, fmt.Sprintf("STALE %s: allowlist has %d, found %d: lower or remove the entry (UPDATE_RULES_ALLOWLIST=1)", key, n, found[key]))
		}
	}
	for _, v := range layer {
		problems = append(problems, "LAYER "+v+": internal/lint and internal/config must not import cmd/")
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d problem(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// TestRulesFlagASeededViolation proves each rule sees its violation, so an
// allowlist cannot hide a scanner that finds nothing.
func TestRulesFlagASeededViolation(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil { //nolint:gosec // scratch file
			t.Fatal(err)
		}
	}
	write("internal/config/seeded.go", "package config\n\nimport (\n\t\"context\"\n\t\"errors\"\n\t\"fmt\"\n\n\t_ \""+modulePath+"cmd/commands\"\n)\n\n"+
		"var counter int\n\nvar errGone = errors.New(\"gone\")\n\nfunc init() {}\n\n"+
		"func F() { _ = context.Background(); fmt.Println(\"x\") }\n")
	write("cmd/tool/main.go", "package main\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\nfunc main() { _ = context.Background(); fmt.Println(\"x\") }\n")
	found, layer := scanRules(t, dir)
	want := map[string]int{
		"init internal/config/seeded.go":       1,
		"global internal/config/seeded.go":     1,
		"background internal/config/seeded.go": 1,
		"print internal/config/seeded.go":      1,
	}
	if len(found) != len(want) {
		t.Fatalf("scan found %v, want %v", found, want)
	}
	for k, n := range want {
		if found[k] != n {
			t.Fatalf("scan found %v, want %v", found, want)
		}
	}
	if len(layer) != 1 {
		t.Fatalf("layering found %v, want one violation", layer)
	}
}
