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

// codeBlock is one row of the "Code ranges" table in docs/strict-validation.md.
type codeBlock struct {
	from, to, owner, status string
}

func parseCodeBlocks(t *testing.T) []codeBlock {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "strict-validation.md"))
	require.NoError(t, err)
	_, section, found := strings.Cut(string(doc), "### Code ranges")
	require.True(t, found, "docs/strict-validation.md has no Code ranges section")
	section, _, _ = strings.Cut(section, "\n## ")
	row := regexp.MustCompile("(?m)^\\| `(AR[0-9A-Z]{3})`-`(AR[0-9A-Z]{3})` \\| ([^|]+) \\| (\\w+) \\|$")
	var blocks []codeBlock
	for _, m := range row.FindAllStringSubmatch(section, -1) {
		blocks = append(blocks, codeBlock{from: m[1], to: m[2], owner: strings.TrimSpace(m[3]), status: m[4]})
	}
	require.NotEmpty(t, blocks)
	return blocks
}

func blockFor(blocks []codeBlock, code string) (codeBlock, bool) {
	for _, b := range blocks {
		if code >= b.from && code <= b.to {
			return b, true
		}
	}
	return codeBlock{}, false
}

func TestCodeBlocksAreWellFormedAndDoNotOverlap(t *testing.T) {
	blocks := parseCodeBlocks(t)
	statuses := map[string]bool{"allocated": true, "reserved": true, "proposed": true}
	for i, b := range blocks {
		assert.LessOrEqual(t, b.from, b.to, "block %s-%s is reversed", b.from, b.to)
		assert.True(t, statuses[b.status], "block %s-%s has unknown status %q", b.from, b.to, b.status)
		for _, o := range blocks[i+1:] {
			assert.True(t, b.to < o.from || o.to < b.from, "blocks %s-%s and %s-%s overlap", b.from, b.to, o.from, o.to)
		}
	}
}

func TestCodeBlocksAreAllocated(t *testing.T) {
	blocks := parseCodeBlocks(t)
	for _, r := range Rules() {
		b, ok := blockFor(blocks, r.Code)
		if assert.True(t, ok, "%s (%s) is outside every block of the Code ranges table", r.Code, r.Name) {
			assert.Equal(t, "allocated", b.status, "%s (%s) is registered in a block that is %s for %q", r.Code, r.Name, b.status, b.owner)
		}
	}
}

// codeLiteral matches a string literal that is exactly one rule code.
var codeLiteral = regexp.MustCompile(`^AR[0-9][0-9A-Z]{2}$`)

// TestAllocatedBlocksCoverLiteralsInOtherPackages catches a code another
// package declares (internal/okf, internal/mcp, internal/importer, ...) that the
// lint registry does not know or that sits in a block reserved for another
// feature: the registry test alone cannot see them.
func TestAllocatedBlocksCoverLiteralsInOtherPackages(t *testing.T) {
	blocks := parseCodeBlocks(t)
	fset := token.NewFileSet()
	root := filepath.Join("..", "..")
	found := 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "lint" || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				code, _ := strconv.Unquote(lit.Value)
				if !codeLiteral.MatchString(code) {
					return true
				}
				found++
				_, registered := lookupRule(code)
				assert.True(t, registered, "%s declares %s, which the lint registry does not know", path, code)
				if b, ok := blockFor(blocks, code); assert.True(t, ok, "%s declares %s outside every block", path, code) {
					assert.Equal(t, "allocated", b.status, "%s declares %s in a %s block", path, code, b.status)
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, found, 12, "the scan found too few code literals: the walk is broken")
}
