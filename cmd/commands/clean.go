package commands

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
	"github.com/spf13/cobra"
)

var (
	cleanDryRun        bool
	cleanForce         bool
	cleanIncludeEdited bool
	cleanKeepGitignore bool
	cleanKeepManifest  bool
	cleanFormat        string
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

By default clean lists what it will delete and asks for confirmation; use --yes
to skip the prompt (required in non-interactive shells; declining exits 1) or --dry-run
to preview. Generated files you edited by hand are kept with a warning; --include-edited
removes them too.`,
	Aliases: []string{"clear"},
	Args:    cobra.MaximumNArgs(1),
	RunE:    runClean,
}

func init() {
	CleanCmd.Flags().BoolVarP(&cleanDryRun, "dry-run", "d", false, "Show what would be removed without deleting anything")
	addYesFlag(CleanCmd.Flags(), &cleanForce, "Skip the confirmation prompt")
	CleanCmd.Flags().BoolVar(&cleanIncludeEdited, "include-edited", false, "Also remove generated files whose body was edited by hand (otherwise they are kept with a warning)")
	CleanCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile whose outputs to remove, or a comma-separated list to compose several (default: from config or 'default')")
	CleanCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	CleanCmd.Flags().BoolVar(&userScope, "user", false, "Remove the files 'generate --user' wrote into the home directories, as recorded in the user manifest")
	CleanCmd.Flags().BoolVar(&cleanKeepGitignore, "keep-gitignore", false, "Leave the ai-rulez managed block in .gitignore in place")
	addFormatFlag(CleanCmd.Flags(), &cleanFormat, formatText, formatText, formatText, formatJSON)
	CleanCmd.Flags().BoolVar(&cleanKeepManifest, "keep-manifest", false, "Leave the generated manifest in place")
}

func runClean(cmd *cobra.Command, args []string) error {
	ctx := cmdContext()
	if err := checkFormatFlag(cleanFormat); err != nil {
		return fail(err)
	}
	out := outFor(cmd)

	if handled, err := handleUserClean(out, args); handled {
		return err
	}

	cfg, err := loadConfigForCommand(ctx, args)
	if err != nil {
		return failMsg("Failed to load config", err)
	}

	gen := generator.NewGenerator(cfg)
	// Compute the plan first; delete only after confirmation.
	opts := cleanOptions(cleanKeepGitignore)

	plan, err := gen.Clean(profile, opts)
	if err != nil {
		return failMsg("Failed to compute clean plan", err)
	}

	if plan.Empty() {
		return writeCleanResult(out, cfg.BaseDir, plan, cleanDryRun, "Nothing to clean: no generated files found")
	}

	if cleanDryRun {
		return writeCleanResult(out, cfg.BaseDir, plan, true, "Dry run: no files were removed")
	}
	if cleanFormat != formatJSON {
		printCleanPlan(out.Stdout(), cfg.BaseDir, plan)
	}

	if err := confirmRemovalUnlessYes(cleanForce, "", fmt.Sprintf("%d generated file(s) and %d generated director(ies)", len(plan.Files), len(plan.Dirs)),
		fmt.Sprintf("Aborted (%d candidates)", len(plan.Files)+len(plan.Dirs))); err != nil {
		return fail(err)
	}

	opts.DryRun = false
	if _, err := gen.Clean(profile, opts); err != nil {
		return failMsg("Failed to clean generated files", err)
	}

	if cleanFormat == formatJSON {
		return writeCleanDocument(out.Stdout(), cfg.BaseDir, plan, false)
	}
	out.Info("Removed generated files: %d files, %d directories\n", len(plan.Files), len(plan.Dirs))
	return nil
}

// writeCleanResult prints the plan of a run that removes nothing (a dry run, or
// an empty plan) as the command's result and a note on stderr.
func writeCleanResult(out render.Out, baseDir string, plan *generator.CleanPlan, dryRun bool, note string) error {
	if cleanFormat == formatJSON {
		return writeCleanDocument(out.Stdout(), baseDir, plan, dryRun)
	}
	if !plan.Empty() {
		printCleanPlan(out.Stdout(), baseDir, plan)
	}
	out.Info("%s\n", note)
	return nil
}

// cleanDocument is the `clean --format json` result.
type cleanDocument struct {
	Status      string   `json:"status"`
	DryRun      bool     `json:"dry_run"`
	Profile     string   `json:"profile"`
	Files       []string `json:"files"`
	Directories []string `json:"directories"`
}

func writeCleanDocument(w io.Writer, baseDir string, plan *generator.CleanPlan, dryRun bool) error {
	status := "removed"
	switch {
	case plan.Empty():
		status = "nothing_to_clean"
	case dryRun:
		status = "planned"
	}
	doc := cleanDocument{Status: status, DryRun: dryRun, Profile: plan.Profile, Files: []string{}, Directories: []string{}}
	for _, f := range plan.Files {
		doc.Files = append(doc.Files, relOrAbs(baseDir, f))
	}
	for _, d := range plan.Dirs {
		doc.Directories = append(doc.Directories, relOrAbs(baseDir, d))
	}
	return jsondoc.Write(w, doc) //nolint:wrapcheck // already contextual
}

// cleanOptions builds the options of a clean run from the flags. --include-edited also
// removes generated files whose body was edited by hand; without it they are kept
// with a warning, because the edit cannot be recreated.
func cleanOptions(keepGitignore bool) generator.CleanOptions {
	return generator.CleanOptions{
		DryRun:        true,
		KeepGitignore: keepGitignore,
		KeepManifest:  cleanKeepManifest,
		RemoveEdited:  cleanIncludeEdited,
	}
}

// printCleanPlan writes the plan, the result of a clean run, to w.
func printCleanPlan(w io.Writer, baseDir string, plan *generator.CleanPlan) {
	writef(w, "Clean plan (profile %s)\n", plan.Profile)
	for _, f := range plan.Files {
		writeln(w, "  remove file: "+relOrAbs(baseDir, f))
	}
	for _, f := range plan.Unmerged {
		writeln(w, "  remove ai-rulez keys from: "+relOrAbs(baseDir, f))
	}
	for _, f := range plan.Restored {
		writeln(w, "  restore imported command: "+relOrAbs(baseDir, f))
	}
	for _, d := range plan.Dirs {
		writeln(w, "  remove dir (if empty): "+relOrAbs(baseDir, d))
	}
	if plan.ManifestPath != "" {
		writeln(w, "  remove manifest: "+relOrAbs(baseDir, plan.ManifestPath))
	}
	if plan.LocalManifestPath != "" {
		writeln(w, "  remove local manifest: "+relOrAbs(baseDir, plan.LocalManifestPath))
	}
	if plan.GitignoreEdited {
		writeln(w, "  strip ai-rulez block from .gitignore")
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
