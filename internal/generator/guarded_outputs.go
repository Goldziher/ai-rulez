package generator

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// hasLocalOutputs reports whether any output is machine-local.
func (g *Generator) hasLocalOutputs(outputs []config.OutputFile) bool {
	for _, output := range outputs {
		if output.LocalOnly {
			return true
		}
	}
	return false
}

// guardedOutputs lists the project-relative files that must never be committable:
// machine-local outputs and MCP configs holding resolved secrets.
func (g *Generator) guardedOutputs(outputs []config.OutputFile) []string {
	var rels []string
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if output.LocalOnly || (output.Sensitive && isMCPConfigOutput(rel)) {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	return rels
}

func (g *Generator) hasGuardedSecretOutputs(outputs []config.OutputFile) bool {
	for _, output := range outputs {
		if output.IsDir || !output.Sensitive {
			continue
		}
		if isMCPConfigOutput(filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))) {
			return true
		}
	}
	return false
}

// verifyGuardedOutputsIgnored asks git (or, outside a repository, the project's
// .gitignore) about the real paths of the guarded outputs, after the ignore
// entries are written. The entries were chosen from probes and patterns; a user
// rule that un-ignores a path, a rule that only looked like it covered a pattern,
// or an ignore file git does not read would otherwise leave local or secret
// content committable without any message. It names paths only.
func (g *Generator) verifyGuardedOutputsIgnored(outputs []config.OutputFile) error {
	rels := append(g.guardedOutputs(outputs), g.localInputRels()...)
	if len(rels) == 0 {
		return nil
	}
	ignored := g.ignoredSet(rels, nil)
	var open []string
	for _, rel := range rels {
		if !ignored[rel] {
			open = append(open, rel)
		}
	}
	if len(open) == 0 {
		return nil
	}
	return oops.
		With("paths", open).
		Hint("Machine-local and secret-bearing files must be git-ignored. A .gitignore rule probably un-ignores them "+
			"(for example \"!.claude/skills/**\"): narrow that rule, or run with --no-local to generate the shared view").
		Errorf("generated machine-local or secret outputs are not git-ignored: %s", strings.Join(open, ", "))
}
