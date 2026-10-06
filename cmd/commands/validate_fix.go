package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	validateFix       bool
	validateFixUnsafe bool
	validateDryRun    bool
)

func addFixFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVar(&validateFix, "fix", false, "With --strict, apply the safe automatic fixes to authored sources: executable bits (AR502, AR503, AR505), frontmatter key renames (AR303), quoted booleans (AR304), unclosed code fences (AR806) and missing final newlines (AR807). Harness trap fixes (AR9C7 and project AR9CA key-misspelt rows) also rewrite a misspelt frontmatter key in hand-written harness files outside .ai-rulez/. Never touches generated outputs or security findings")
	f.BoolVar(&validateFixUnsafe, "fix-unsafe", false, "With --strict, also apply fixes that can change meaning: skill name normalization (AR804). Implies --fix")
	f.BoolVar(&validateDryRun, "dry-run", false, "With --fix or --fix-unsafe, print the unified diff and change nothing")
}

func fixRequested() bool { return validateFix || validateFixUnsafe }

// applyFixes runs the fixers over each report, prints a summary, and removes the
// fixed findings from the reports so the exit code reflects what is left.
func applyFixes(reports []*lint.Report, cfgs []*config.Config) error {
	var totalApplied, totalOutside, totalSkipped int
	var diffs strings.Builder
	var skipped []lint.FixSkipped
	for i, r := range reports {
		cfg := cfgAt(cfgs, i)
		opts := lint.FixOptions{Unsafe: validateFixUnsafe, DryRun: validateDryRun}
		if cfg != nil {
			configDir, err := filepath.Abs(cfg.ConfigDir)
			if err == nil {
				opts.EditRoot = gitutil.Resolve(configDir)
				opts.DiffRoot = opts.EditRoot
				if top := gitutil.TopLevel(configDir); top != "" {
					opts.DiffRoot = gitutil.Resolve(top)
				}
			}
			generated := map[string]bool{}
			for _, p := range generator.NewGenerator(cfg).GeneratedPaths() {
				generated[gitutil.Resolve(p)] = true
			}
			opts.Refuse = func(abs string) string {
				if generated[gitutil.Resolve(abs)] {
					return "the file is a generated output; fix its source instead"
				}
				if filepath.Base(abs) == lockfile.FileName {
					return "the lock is written by `ai-rulez lock`, never by a fix"
				}
				return ""
			}
		}
		res, err := lint.ApplyFixes(r.Findings, opts)
		if err != nil {
			return oops.Wrapf(err, "apply fixes")
		}
		totalApplied += len(res.Applied)
		totalOutside += appliedOutside(res.Applied, opts.EditRoot)
		totalSkipped += len(res.Skipped)
		skipped = append(skipped, res.Skipped...)
		diffs.WriteString(res.Diff)
		res.DropFixed(r, validateDryRun)
	}
	printFixSummary(os.Stderr, totalApplied, totalOutside, skipped)
	if validateDryRun && diffs.Len() > 0 {
		var w io.Writer = os.Stdout
		if structuredFormat(validateFormat) {
			w = os.Stderr // stdout carries the report alone
		}
		if _, err := io.WriteString(w, diffs.String()); err != nil {
			return oops.Wrap(err)
		}
	}
	return nil
}

// appliedOutside counts the fixes that edited a file outside the configuration
// directory (hand-written harness files; trap fixes do this). An empty root
// counts none.
func appliedOutside(applied []lint.FixApplied, root string) int {
	if root == "" {
		return 0
	}
	n := 0
	for _, a := range applied {
		if rel, err := filepath.Rel(root, gitutil.Resolve(a.File)); err != nil || strings.HasPrefix(rel, "..") {
			n++
		}
	}
	return n
}

func printFixSummary(w io.Writer, applied, outside int, skipped []lint.FixSkipped) {
	verb, done := "fixed", "fixed"
	if validateDryRun {
		verb, done = "would fix", "would be fixed"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %d finding(s)", verb, applied)
	if len(skipped) > 0 {
		fmt.Fprintf(&sb, ", skipped %d", len(skipped))
	}
	sb.WriteString("\n")
	for i := range skipped {
		s := &skipped[i]
		fmt.Fprintf(&sb, "  skipped %s %s:%d: %s\n", s.Code, s.File, s.Line, s.Reason)
	}
	if outside > 0 {
		fmt.Fprintf(&sb, "%d in hand-written files outside the configuration directory; review them with your version control\n", outside)
	}
	if applied > outside && !validateDryRun {
		fmt.Fprintf(&sb, "the fixed findings are %s; run `ai-rulez generate` to propagate source changes to generated files\n", done)
	}
	_, _ = io.WriteString(w, sb.String()) //nolint:errcheck // a diagnostic on stderr
}
