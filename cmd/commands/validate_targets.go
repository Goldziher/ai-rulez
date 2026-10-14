package commands

import (
	"context"
	"fmt"
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
	flagLintPath = "path"
	flagLintGlob = "glob"
)

// addTargetFlags registers --path and --glob on validate and scan. They are
// string arrays, not slices: a glob such as "{a,b}/**" holds a comma.
func addTargetFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringArray(flagLintPath, nil, "Also lint this file or directory, which must lie inside the project root (repeatable; a marketplace, plugin, hook file or skill is recognized by its content wherever it is)")
	f.StringArray(flagLintGlob, nil, "Also lint the files matching this glob, relative to the project root (repeatable, for example \"catalog/**/*.json\")")
}

// lintTargetFlags returns the --path and --glob values of the command that is
// running. validate and scan each own the flags; the other command's are never
// parsed and stay empty.
func lintTargetFlags() (paths, globs []string) {
	for _, name := range []string{"validate", "scan"} {
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
	if cfg.Lint != nil {
		for _, p := range cfg.Lint.Paths {
			if !filepath.IsAbs(p) {
				p = filepath.Join(tree.Top, filepath.FromSlash(p))
			}
			paths = append(paths, p)
		}
	}
	spec := lint.TargetSpec{Paths: paths, Globs: globs, Cwd: workingDir()}
	if validateRecursive {
		spec.Under = tree.Rel(cfg.BaseDir)
	}
	if spec.Empty() {
		return nil, nil
	}
	targets, err := lint.ResolveTargets(ctx, tree, spec)
	if err != nil {
		return nil, oops.Hint("--path and --glob must name files inside the project root; [lint] paths are relative to it").Wrap(err)
	}
	for _, g := range targets.Unmatched {
		noteLine(ctx, os.Stderr, fmt.Sprintf("WARN  no file matches --glob %q\n", g))
	}
	return targets, nil
}
