package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A regular expression compiled lazily through sync.OnceValue only fails when it
// is first used. A bad pattern must fail in the test run, not in production, so
// every such accessor has to be referenced from a _test.go file of its package
// (the package's TestRegexAccessorsCompile calls each one).
func TestEveryLazyRegexAccessorIsCalledInATest(t *testing.T) {
	root := repoRoot(t)
	accessors := map[string][]string{} // package dir -> accessor names
	walkSources(t, root, func(rel string, file *ast.File) {
		for _, name := range lazyRegexVars(file) {
			dir := filepath.ToSlash(filepath.Dir(rel))
			accessors[dir] = append(accessors[dir], name)
		}
	})
	if len(accessors) == 0 {
		t.Fatal("found no sync.OnceValue regexp accessors: the scan is broken")
	}
	for dir, names := range accessors {
		referenced := identsInTests(t, filepath.Join(root, filepath.FromSlash(dir)))
		for _, name := range names {
			if !referenced[name] {
				t.Errorf("%s: %s is a lazy regexp accessor that no test in the package calls; add it to TestRegexAccessorsCompile", dir, name)
			}
		}
	}
}

// lazyRegexVars lists the package-level vars initialized with
// sync.OnceValue(func() *regexp.Regexp {...}).
func lazyRegexVars(file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, v := range vs.Values {
				if i < len(vs.Names) && isOnceValueRegexp(v) {
					out = append(out, vs.Names[i].Name)
				}
			}
		}
	}
	return out
}

func isOnceValueRegexp(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "OnceValue" {
		return false
	}
	lit, ok := call.Args[0].(*ast.FuncLit)
	if !ok || lit.Type.Results == nil || len(lit.Type.Results.List) != 1 {
		return false
	}
	star, ok := lit.Type.Results.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	typ, ok := star.X.(*ast.SelectorExpr)
	return ok && typ.Sel.Name == "Regexp"
}

// identsInTests returns every identifier used in the _test.go files of dir.
func identsInTests(t *testing.T, dir string) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				used[id.Name] = true
			}
			return true
		})
	}
	return used
}
