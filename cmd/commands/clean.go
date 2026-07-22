package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	cleanDryRun        bool
	cleanForce         bool
	cleanKeepGitignore bool
	cleanKeepManifest  bool
)

var CleanCmd = &cobra.Command{
	Use:   "clean [config-file]",
	Short: "Remove files produced by generate",
	Long: `Remove the files and directories that 'generate' produced — the inverse
of generate. This deletes the generated assistant outputs (CLAUDE.md, AGENTS.md,
GEMINI.md, .claude/, .codex/, generated skills, etc.), the generated manifest,
and the ai-rulez managed block in .gitignore.

The .ai-rulez/ source tree is never touched. Directories are only removed when
they become empty, so files you authored inside a generated directory are kept.

By default clean lists what it will delete and asks for confirmation; use --force
to skip the prompt (required in non-interactive shells) or --dry-run to preview.`,
	Aliases: []string{"clear"},
	Args:    cobra.MaximumNArgs(1),
	Run:     runClean,
}

func init() {
	CleanCmd.Flags().BoolVarP(&cleanDryRun, "dry-run", "d", false, "Show what would be removed without deleting anything")
	CleanCmd.Flags().BoolVarP(&cleanForce, "force", "y", false, "Skip the confirmation prompt")
	CleanCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile whose outputs to remove (default: from config or 'default')")
	CleanCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	CleanCmd.Flags().BoolVar(&cleanKeepGitignore, "keep-gitignore", false, "Leave the ai-rulez managed block in .gitignore in place")
	CleanCmd.Flags().BoolVar(&cleanKeepManifest, "keep-manifest", false, "Leave the generated manifest in place")
}

func runClean(_ *cobra.Command, args []string) {
	ctx := context.Background()

	cfg, err := loadConfigForCommand(ctx, args)
	if err != nil {
		logger.Error("Failed to load config")
		fmtError(err)
		os.Exit(1)
	}

	gen := generator.NewGenerator(cfg)
	opts := generator.CleanOptions{
		DryRun:        true, // compute the plan first; delete only after confirmation
		KeepGitignore: cleanKeepGitignore,
		KeepManifest:  cleanKeepManifest,
	}

	plan, err := gen.Clean(profile, opts)
	if err != nil {
		logger.Error("Failed to compute clean plan")
		fmtError(err)
		os.Exit(1)
	}

	if plan.Empty() {
		logger.Success("Nothing to clean — no generated files found", "profile", plan.Profile)
		return
	}

	printCleanPlan(cfg.BaseDir, plan)

	if cleanDryRun {
		logger.Info("Dry run — no files were removed")
		return
	}

	if !cleanForce {
		total := len(plan.Files) + len(plan.Dirs)
		if !confirmRemoval("", fmt.Sprintf("%d generated file(s) and %d generated director(ies)", len(plan.Files), len(plan.Dirs))) {
			logger.Info("Aborted — nothing removed", "candidates", total)
			return
		}
	}

	opts.DryRun = false
	if _, err := gen.Clean(profile, opts); err != nil {
		logger.Error("Failed to clean generated files")
		fmtError(err)
		os.Exit(1)
	}

	logger.Success("Removed generated files", "files", len(plan.Files), "directories", len(plan.Dirs))
}

func printCleanPlan(baseDir string, plan *generator.CleanPlan) {
	logger.Info("Clean plan", "profile", plan.Profile)
	for _, f := range plan.Files {
		logger.Info("  remove file: " + relOrAbs(baseDir, f))
	}
	for _, d := range plan.Dirs {
		logger.Info("  remove dir (if empty): " + relOrAbs(baseDir, d))
	}
	if plan.ManifestPath != "" {
		logger.Info("  remove manifest: " + relOrAbs(baseDir, plan.ManifestPath))
	}
	if plan.GitignoreEdited {
		logger.Info("  strip ai-rulez block from .gitignore")
	}
}

// relOrAbs renders path relative to baseDir when possible, otherwise returns it
// unchanged — for tidy, project-relative display in the clean plan.
func relOrAbs(baseDir, path string) string {
	if rel, err := filepath.Rel(baseDir, path); err == nil {
		return rel
	}
	return path
}
