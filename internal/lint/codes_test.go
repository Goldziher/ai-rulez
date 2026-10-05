package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_CodesAndNamesAreUnique(t *testing.T) {
	codes, names := map[string]string{}, map[string]string{}
	for _, r := range Rules() {
		prev, dup := codes[r.Code]
		assert.False(t, dup, "code %s is registered for both %q and %q", r.Code, prev, r.Name)
		codes[r.Code] = r.Name
		prev, dup = names[r.Name]
		assert.False(t, dup, "name %s is registered for both %s and %s", r.Name, prev, r.Code)
		names[r.Name] = r.Code
	}
}

func TestRegistry_EveryCodeConstantIsRegisteredOnce(t *testing.T) {
	registered := map[string]int{}
	for _, r := range registry {
		registered[r.Code]++
	}
	seen := map[string]string{}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range spec.Names {
				if !strings.HasPrefix(name.Name, "Code") || i >= len(spec.Values) {
					continue
				}
				lit, ok := spec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				code, _ := strconv.Unquote(lit.Value)
				if !strings.HasPrefix(code, "AR") {
					continue
				}
				prev, dup := seen[code]
				assert.False(t, dup, "%s and %s both declare %s", prev, name.Name, code)
				seen[code] = name.Name
				assert.Equal(t, 1, registered[code], "%s (%s) must be registered exactly once", name.Name, code)
			}
			return true
		})
	}
	assert.Len(t, seen, len(registry), "a registered rule has no Code constant, or the reverse")
}

func TestRegistry_EveryCodeIsDocumented(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "strict-validation.md"))
	require.NoError(t, err)
	row := regexp.MustCompile("(?m)^\\| (AR[0-9A-Z]+) \\| `([a-z0-9-]+)` \\|")
	documented := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(string(doc), -1) {
		_, dup := documented[m[1]]
		assert.False(t, dup, "%s is documented twice in docs/strict-validation.md", m[1])
		documented[m[1]] = m[2]
	}
	for _, r := range Rules() {
		name, ok := documented[r.Code]
		if assert.True(t, ok, "%s (%s) is not in the docs/strict-validation.md table", r.Code, r.Name) {
			assert.Equal(t, r.Name, name, "docs name for %s", r.Code)
		}
	}
	for code := range documented {
		_, ok := lookupRule(code)
		assert.True(t, ok, "%s is documented but not registered", code)
	}
}
