package gitutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// directGitTestAllowList holds the test files that run git directly to build
// fixture repositories. Production code never does: it goes through Command (or
// CommandNoContext) so the environment a git hook inherits cannot redirect git
// to the hook's repository. Do not add a non-test file here.
var directGitTestAllowList = map[string]bool{
	"internal/config/bundle_exclude_test.go":          true,
	"internal/doctor/doctor_test.go":                  true,
	"internal/generator/generic_sidecars_e2e_test.go": true,
	"internal/generator/gitignore_matrix_test.go":     true,
	"internal/generator/local_content_test.go":        true,
	"internal/generator/local_drift_test.go":          true,
	"internal/generator/local_ignore_verify_test.go":  true,
	"internal/generator/plugin_drift_test.go":         true,
	"internal/generator/gitignore_needed_test.go":     true,
	"internal/gitignore/symlink_test.go":              true,
	"internal/skillsource/skillsource_test.go":        true,
	"internal/includes/git_test.go":                   true,
	"internal/includes/gitops_test.go":                true,
	"internal/includes/pin_test.go":                   true,
	"internal/includes/skill_source_test.go":          true,
	"internal/lint/lint_test.go":                      true,
	"internal/mcp/serve_build_test.go":                true,
	"internal/okfbridge/source_test.go":               true,
}

func TestNoDirectGitExecOutsideGitutil(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	for _, top := range []string{"cmd", "internal", "schema"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			if strings.HasPrefix(rel, "internal/gitutil/") {
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") && directGitTestAllowList[rel] {
				return nil
			}
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok && runsGit(call) {
					offenders = append(offenders, rel)
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}
	sort.Strings(offenders)
	require.Empty(t, offenders, "run git through gitutil.Command (or CommandNoContext), never exec.Command(\"git\", ...)")
}

// runsGit reports whether call is exec.Command("git", ...) or
// exec.CommandContext(ctx, "git", ...).
func runsGit(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return false
	}
	idx := -1
	switch sel.Sel.Name {
	case "Command":
		idx = 0
	case "CommandContext":
		idx = 1
	}
	if idx < 0 || len(call.Args) <= idx {
		return false
	}
	lit, ok := call.Args[idx].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	name, err := strconv.Unquote(lit.Value)
	return err == nil && name == "git"
}
