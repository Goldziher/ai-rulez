package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestLazyTablesBuild forces every package-level sync.OnceValue table that does
// not return a *regexp.Regexp (those are in TestRegexAccessorsCompile). Several of
// them call regexp.MustCompile inside, so a bad pattern would panic at first use in
// production rather than in a test. The test also parses the package, so a table
// added without an entry here fails it.
func TestLazyTablesBuild(t *testing.T) {
	tables := map[string]func() any{
		"ambiguousExt":        func() any { return ambiguousExt() },
		"antiRefusalPatterns": func() any { return antiRefusalPatterns() },
		"antiRefusalStems":    func() any { return antiRefusalStems() },
		"bypassPatterns":      func() any { return bypassPatterns() },
		"bytecodeExt":         func() any { return bytecodeExt() },
		"bytecodeSkipDirs":    func() any { return bytecodeSkipDirs() },
		"confusableLetters":   func() any { return confusableLetters() },
		"defaultSourceHosts":  func() any { return defaultSourceHosts() },
		"deserRules":          func() any { return deserRules() },
		"deserStems":          func() any { return deserStems() },
		"exfilDestStems":      func() any { return exfilDestStems() },
		"exfilProseStems":     func() any { return exfilProseStems() },
		"exfilStems":          func() any { return exfilStems() },
		"fileExts":            func() any { return fileExts() },
		"grantWrappers":       func() any { return grantWrappers() },
		"internalStems":       func() any { return internalStems() },
		"localSpecPrefixes":   func() any { return localSpecPrefixes() },
		"lookalikeExtras":     func() any { return lookalikeExtras() },
		"malwareDocs":         func() any { return malwareDocs() },
		"malwareRules":        func() any { return malwareRules() },
		"malwareStems":        func() any { return malwareStems() },
		"mcpLaunchKeys":       func() any { return mcpLaunchKeys() },
		"mcpMetaKeys":         func() any { return mcpMetaKeys() },
		"npmLocks":            func() any { return npmLocks() },
		"pythonLocks":         func() any { return pythonLocks() },
		"npmUniverse":         func() any { return npmUniverse() },
		"packageJSONSections": func() any { return packageJSONSections() },
		"persistRules":        func() any { return persistRules() },
		"persistStems":        func() any { return persistStems() },
		"phishingFormRe":      func() any { return phishingFormRe() },
		"proseCommandWords":   func() any { return proseCommandWords() },
		"pyPIUniverse":        func() any { return pyPIUniverse() },
		"redirectSourceStems": func() any { return redirectSourceStems() },
		"reservedAliases":     func() any { return reservedAliases() },
		"reservedNames":       func() any { return reservedNames() },
		"scriptRes":           func() any { return scriptRes() },
	}
	for name, build := range tables {
		t.Run(name, func(t *testing.T) {
			if build() == nil {
				t.Errorf("%s built nil", name)
			}
		})
	}
	for _, name := range lazyTableVars(t) {
		if _, ok := tables[name]; !ok {
			t.Errorf("%s is a lazy sync.OnceValue table that TestLazyTablesBuild does not force; add it to the list", name)
		}
	}
}

// lazyTableVars lists the package-level vars of the non-test files initialized with
// sync.OnceValue(func() T {...}) where T is not *regexp.Regexp.
func lazyTableVars(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, e.Name(), nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		files = append(files, file)
	}
	var out []string
	for _, file := range files {
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
					if i < len(vs.Names) && isOnceValueTable(v) {
						out = append(out, vs.Names[i].Name)
					}
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no sync.OnceValue tables: the scan is broken")
	}
	return out
}

func isOnceValueTable(expr ast.Expr) bool {
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
		return true
	}
	sx, ok := star.X.(*ast.SelectorExpr)
	return !ok || sx.Sel.Name != "Regexp"
}
