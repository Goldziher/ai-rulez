package commands

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Flag names of the lint-target flags of validate and scan.
const (
	flagLintPath    = "path"
	flagLintGlob    = "glob"
	flagLintOutputs = "outputs"
	flagLintExclude = "exclude"

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
	f.StringArray(flagLintPath, nil, "Also lint this file or directory, which must lie inside the project root (repeatable; outside git a path above the configuration's directory widens the root to the nearest directory holding both, if it lies inside the working directory; a marketplace, plugin, hook file or skill is recognized by its content wherever it is). A path that does not exist or leaves the root exits 1, and so does a named file that is unreadable or over 2 MiB. A text file named here that is no content kind (.env, YAML) is scanned for secrets only")
	f.StringArray(flagLintGlob, nil, "Also lint the files matching this glob, relative to the project root (repeatable, for example \"catalog/**/*.json\"). A matched file that is not linted is named on stderr as \"<file>: not linted (<reason>)\" and in the JSON report's notices, and a glob that matches nothing warns")
	f.StringArray(flagLintExclude, nil, "Leave the files matching this glob (relative to the project root, for example \"x/src/public/skills/**\") out of the files found by convention, --path and --glob; works without a configuration. Findings in the configured content tree are dropped with [lint] ignore_paths in config.toml (repeatable)")
	f.Bool(flagLintOutputs, false, "Also lint the generated files (what generate writes, or would write) for secrets, broken references and descriptions; a finding in one is marked as output and a problem the authored source already has is reported once (the same as [lint] lint_outputs)")
}

// lintTargetFlags returns the --path and --glob values of the command that is
// running. validate and scan each own the flags; the other command's are never
// parsed and stay empty.
func lintTargetFlags() (paths, globs []string) {
	for _, set := range lintFlagSets() {
		paths = append(paths, stringArrayFlag(set, flagLintPath)...)
		globs = append(globs, stringArrayFlag(set, flagLintGlob)...)
	}
	return paths, globs
}

// lintExcludeFlags returns the --exclude globs of the command that is running.
func lintExcludeFlags() []string {
	var globs []string
	for _, set := range lintFlagSets() {
		globs = append(globs, stringArrayFlag(set, flagLintExclude)...)
	}
	return globs
}

// lintFlagSets are the flag sets of the commands that own the lint-target flags. The command
// lookup rewrites cobra's flag sets, so the roots of a recursive run, which read the flags
// concurrently, must not each do it: it happens once.
var lintFlagSets = sync.OnceValue(func() []*pflag.FlagSet {
	var sets []*pflag.FlagSet
	for _, name := range lintCommandNames() {
		if c, _, err := RootCmd.Find([]string{name}); err == nil && c != nil {
			sets = append(sets, c.Flags())
		}
	}
	return sets
})

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
	for _, set := range lintFlagSets() {
		if on, gerr := set.GetBool(flagLintOutputs); gerr == nil && on {
			return true
		}
	}
	return false
}

type unmanagedKey struct{}

// withUnmanagedLint marks ctx as a validate run: with a configuration it also lints the harness
// files nobody generated (a hand-written .claude/skills/x/SKILL.md), which the configuration does not list.
// Other commands that lint (catalog, publish) keep to the configured content.
func withUnmanagedLint(ctx context.Context) context.Context {
	return context.WithValue(ctx, unmanagedKey{}, true)
}

func lintsUnmanaged(ctx context.Context) bool {
	on, ok := ctx.Value(unmanagedKey{}).(bool)
	return ok && on
}

// recordedOutputs lists the files a generate run of cfg recorded in its manifest, as slash paths
// relative to the repository top: they are outputs, so a run that lints the files nobody listed leaves them out.
func recordedOutputs(ctx context.Context, cfg *config.Config, top string) []string {
	gen := generator.NewGenerator(cfg)
	gen.SetContext(ctx)
	var rels []string
	for _, abs := range gen.GeneratedPaths() {
		if rel, err := filepath.Rel(top, abs); err == nil {
			rels = append(rels, filepath.ToSlash(rel))
		}
	}
	return rels
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

// baseTargetSpec is the spec of one root before a recursive run narrows it: the named files, the
// [lint] paths of its configuration and the detection of the files that follow a convention.
func baseTargetSpec(ctx context.Context, cfg *config.Config, tree *lint.Tree) lint.TargetSpec {
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
	spec := lint.TargetSpec{Paths: paths, Globs: globs, Exclude: lintExcludeFlags(), Cwd: workingDir(), Detect: (cfg.Implicit || lintsUnmanaged(ctx)) && !explicit, Generated: cfg.Implicit && outputsRequested(cfg)}
	if !cfg.Implicit && spec.Detect {
		spec.ExceptFiles, spec.QuietGenerated = recordedOutputs(ctx, cfg, tree.Top), true
	}
	return spec
}

// resolveLintTargets expands the lint targets of one root: --path and --glob,
// the [lint] paths of its configuration (relative to the project root) and, for
// a configuration-less run, the files that follow a convention. It returns nil
// when nothing is targeted.
func resolveLintTargets(ctx context.Context, cfg *config.Config, tree *lint.Tree) (*lint.Targets, error) {
	spec := baseTargetSpec(ctx, cfg, tree)
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
		return nil, oops.Hint("--path and --glob must name readable files inside the project root; [lint] paths are relative to it. Outside git, run from a directory that holds both the configuration and the --path").Wrap(err)
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
