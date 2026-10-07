package commands

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/project"
)

// TestCommandsNeverLoadOutsidePolicy keeps every load the command line makes
// under the organization policy: a command that passes a bare background
// context to a loader, a CRUD operator or the MCP setup, or that calls
// project.Load* itself, would run without it.
func TestCommandsNeverLoadOutsidePolicy(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	var offenders []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, perr := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, perr)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				pos := fset.Position(call.Pos())
				switch {
				case pkg.Name == "context" && (sel.Sel.Name == "Background" || sel.Sel.Name == "TODO") && fn.Name.Name != "cmdContext":
					offenders = append(offenders, pos.String()+": context."+sel.Sel.Name+"() (use cmdContext())")
				case pkg.Name == "project" && strings.HasPrefix(sel.Sel.Name, "Load") && file != "policy.go":
					offenders = append(offenders, pos.String()+": project."+sel.Sel.Name+" (use loadProject*)")
				}
				return true
			})
		}
	}
	assert.Empty(t, offenders)
}

func TestCmdContextCarriesThePolicyToLoads(t *testing.T) {
	// Arrange
	root := twoRoots(t, policyBadConfig)
	installLoosenBad(t)

	// Act
	cfg, err := project.LoadFile(cmdContext(), filepath.Join(root, "b", ".ai-rulez", "config.toml"))

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.PolicyOutcome)
	assert.NotEmpty(t, cfg.PolicyOutcome.Violations)
}
