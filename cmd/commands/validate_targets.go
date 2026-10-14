package commands

import (
	"context"
	"os"
	"path/filepath"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Flag names of the lint-target flags of validate and scan.
const (
	flagLintPath    = "path"
	flagLintGlob    = "glob"
	flagLintOutputs = "outputs"

	// cmdNameValidate and cmdNameScan name the commands that own those flags;
	// the flags are found through the command tree, which the commands cannot
	// refer to by variable (an initialization cycle).
	cmdNameValidate = "validate"
	cmdNameScan     = "scan"
)

// addTargetFlags registers --path, --glob and --outputs on validate and scan. The first two are
// string arrays, not slices: a glob such as "{a,b}/**" holds a comma.
func addTargetFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringArray(flagLintPath, nil, "Also lint this file or directory, which must lie inside the project root (repeatable; a marketplace, plugin, hook file or skill is recognized by its content wherever it is). A path that does not exist or leaves the root exits 1, and so does a named file that is unreadable or over 2 MiB. A text file named here that is no content kind (.env, YAML) is scanned for secrets only")
	f.StringArray(flagLintGlob, nil, "Also lint the files matching this glob, relative to the project root (repeatable, for example \"catalog/**/*.json\"). A matched file that is not linted is named on stderr as \"<file>: not linted (<reason>)\" and in the JSON report's notices, and a glob that matches nothing warns")
	f.Bool(flagLintOutputs, false, "Also lint the generated files (what generate writes, or would write) for secrets, broken references and descriptions; a finding in one is marked as output and a problem the authored source already has is reported once (the same as [lint] lint_outputs)")
}

// lintTargetFlags returns the --path and --glob values of the command that is
// running. validate and scan each own the flags; the other command's are never
// parsed and stay empty.
func lintTargetFlags() (paths, globs []string) {
	for _, name := range lintCommandNames() {
		c, _, err := RootCmd.Find([]string{name})
		if err != nil || c == nil {
			continue
		}
		paths = append(paths, stringArrayFlag(c.Flags(), flagLintPath)...)
		globs = append(globs, stringArrayFlag(c.Flags(), flagLintGlob)...)
	}
	return paths, globs
}

func stringArrayFlag(set *pflag.FlagSet, name string) []string {
	v, err := set.GetStringArray(name)
	if err != nil {
		return nil
	}
	return v
}

// lintCommandNames are the commands that own the lint-target flags.
func lintCommandNames() []string { return []string{cmdNameValidate, cmdNameScan} }

// lintInputFlagsSet reports whether a flag that changes which files are linted
// (--path, --glob, --outputs) was given.
func lintInputFlagsSet() bool { return targetFlagsSet() || outputsFlag() }

// outputsFlag reports whether --outputs was given to the command that is running.
func outputsFlag() bool {
	for _, name := range lintCommandNames() {
		if c, _, err := RootCmd.Find([]string{name}); err == nil && c != nil {
			if on, gerr := c.Flags().GetBool(flagLintOutputs); gerr == nil && on {
				return true
			}
		}
	}
	return false
}

// outputsRequested reports whether the generated files are linted: --outputs or
// [lint] lint_outputs.
func outputsRequested(cfg *config.Config) bool {
	return outputsFlag() || (cfg != nil && cfg.Lint != nil && cfg.Lint.LintOutputs)
}

// targetFlagsSet reports whether --path or --glob was given.
func targetFlagsSet() bool {
	paths, globs := lintTargetFlags()
	return len(paths)+len(globs) > 0
}

// resolveLintTargets expands the lint targets of one root: --path and --glob,
// the [lint] paths of its configuration (relative to the project root) and, for
// a configuration-less run, the files that follow a convention. It returns nil
// when nothing is targeted.
func resolveLintTargets(ctx context.Context, cfg *config.Config, tree *lint.Tree) (*lint.Targets, error) {
	paths, globs := lintTargetFlags()
	explicit := len(paths)+len(globs) > 0
	if cfg.Lint != nil {
		for _, p := range cfg.Lint.Paths {
			if !filepath.IsAbs(p) {
				p = filepath.Join(tree.Top, filepath.FromSlash(p))
			}
			paths = append(paths, p)
		}
	}
	spec := lint.TargetSpec{Paths: paths, Globs: globs, Cwd: workingDir(), Detect: cfg.Implicit && !explicit, Generated: cfg.Implicit && outputsRequested(cfg)}
	roots, orphans := orphanRootsFrom(ctx)
	if orphans {
		covered, ok := rootsRel(tree, roots)
		if !ok {
			return nil, nil // a root at the repository top lints every pick
		}
		spec.Except, spec.Lenient = covered, true
	} else if validateRecursive {
		spec.Under = recursiveUnder(cfg, tree)
	}
	if spec.Empty() {
		return nil, nil
	}
	targets, err := lint.ResolveTargets(ctx, tree, spec)
	if err != nil {
		return nil, oops.Hint("--path and --glob must name readable files inside the project root; [lint] paths are relative to it").Wrap(err)
	}
	if orphans && !noteOrphans(targets) {
		return nil, nil // every pick belongs to a root
	}
	for _, n := range targets.Notices() {
		noteLine(ctx, os.Stderr, "WARN  "+n+"\n")
	}
	return targets, nil
}

// orphanKey carries the root directories of a recursive run into the pass that
// lints the picks no root covers.
type orphanKey struct{}

// withOrphanRoots marks ctx as the pass over the --path and --glob picks that lie
// outside every root; roots are the absolute directories of the roots.
func withOrphanRoots(ctx context.Context, roots []string) context.Context {
	return context.WithValue(ctx, orphanKey{}, roots)
}

func orphanRootsFrom(ctx context.Context) (roots []string, ok bool) {
	roots, ok = ctx.Value(orphanKey{}).([]string)
	return roots, ok
}

// rootsRel returns the roots as slash paths relative to the tree top. ok is false
// when one of them is the top itself, which covers every file.
func rootsRel(tree *lint.Tree, roots []string) (rels []string, ok bool) {
	for _, root := range roots {
		rel := tree.Rel(root)
		if rel == "." {
			return nil, false
		}
		rels = append(rels, rel)
	}
	return rels, true
}

func plainRels(t *lint.Targets) []string {
	out := make([]string, len(t.Plain))
	for i := range t.Plain {
		out[i] = t.Plain[i].Rel
	}
	return out
}

// recursiveUnder is the directory a root of a recursive run keeps its picks
// below; the repository root keeps every file ("").
func recursiveUnder(cfg *config.Config, tree *lint.Tree) string {
	if under := tree.Rel(cfg.BaseDir); under != "." {
		return under
	}
	return ""
}

// noteOrphans names each file linted because no root covers it. It reports false
// when there is none.
func noteOrphans(targets *lint.Targets) bool {
	if len(targets.Files)+len(targets.Plain)+len(targets.Skipped) == 0 {
		return false
	}
	for _, rel := range append(targets.Rels(), plainRels(targets)...) {
		targets.AddNotice(rel + ": outside every --recursive root, linted at the repository root")
	}
	return true
}
