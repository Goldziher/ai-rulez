package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// Exit codes of the okf commands: 0 success, 1 the command could not run, 2 it
// ran and found problems (drift, lint findings at the failing severity, a
// refused import).
const (
	exitOKFProblems  = 2
	exitOKFCannotRun = 1
)

const okfFailNone = "none"

var (
	okfOut        string
	okfProfile    string
	okfRole       string
	okfInclude    []string
	okfCheck      bool
	okfFormat     string
	okfFailOn     string
	okfInto       string
	okfDomain     string
	okfForce      bool
	okfDryRun     bool
	okfIndexStyle string
)

// OKFCmd groups the commands that work on OKF bundles directly.
var OKFCmd = &cobra.Command{
	Use:   "okf",
	Short: "Work with OKF (Open Knowledge Format) bundles",
	Long: `Lint OKF bundles. OKF is a directory of markdown files with YAML frontmatter
(https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
ai-rulez implements spec 0.2. See also "ai-rulez export okf" and "ai-rulez import okf".`,
}

var okfValidateCmd = &cobra.Command{
	Use:   "validate <dir|git-url[@ref][#subdir]>",
	Short: "Lint an OKF bundle (works on third-party bundles)",
	Long: `Check a bundle against the OKF v0.2 conformance rules and the hygiene checks
AR9B0-AR9B9. The bundle does not need to come from ai-rulez.

Exit codes: 0 no findings at --fail-on or above, 2 findings, 1 the bundle could
not be read.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runOKFValidate(watchParentContext(cmd), args[0], os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

// ExportCmd groups the export formats.
var ExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export ai-rulez content to another format",
}

var exportOKFCmd = &cobra.Command{
	Use:   "okf [config-file]",
	Short: "Export rules, context, skills and more as an OKF bundle",
	Long: `Write the project's content as an OKF v0.2 bundle: one concept per rule, context
file, skill, agent, command and check, a root index.md with okf_version, and an
index.md per directory. The output is deterministic (sorted, no timestamps), so it
diffs cleanly in git. Links between exported items point at bundle paths.
--index-style frontmatter writes index.md as title, version and entries in the
frontmatter instead of the OKF 0.2 body listing (default, okf.index_style).

Without --out the bundle goes to okf.dir (default docs/okf). --out replaces the
contents of that directory, but only when it is empty or already an OKF bundle.
--check writes nothing and exits 2 when the bundle on disk differs.
--role exports only the content a role selects (see "ai-rulez roles list").

The okf preset ("presets = [\"claude\", \"okf\"]") runs the same export inside
generate, so the bundle stays in sync and generate --check detects drift.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runOKFExport(watchParentContext(cmd), args, os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

// ImportCmd groups the import formats.
var ImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import content from another format into .ai-rulez/",
}

var importOKFCmd = &cobra.Command{
	Use:   "okf <dir|git-url[@ref][#subdir]>",
	Short: "Import an OKF bundle into .ai-rulez/",
	Long: `Convert the concepts of an OKF bundle into ai-rulez rules, context and skills
(a concept whose x-ai-rulez.kind is agent, command or check becomes that kind).

Each concept lands by its x-ai-rulez metadata when it has it (a bundle made by
"export okf" round-trips losslessly), otherwise by its type: decision-like types
become rules, how-to-like types become skills, everything else becomes context.
--into forces one kind, --domain places everything in a domain.

Nothing is overwritten unless --force is given; a file that exists with the same
bytes is reported as unchanged, so importing twice is a no-op. Imported text is
checked by the security scan (AR001-AR011) first, and the import is refused, with
nothing written, when it finds an error. Paths that leave the target directory
are rejected. Use --dry-run to see what would happen.

Exit codes: 0 done, 2 refused (security findings) or files skipped because they
differ, 1 the bundle could not be read.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runOKFImport(watchParentContext(cmd), args[0], os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	addFormatFlag(okfValidateCmd.Flags(), &okfFormat, formatText, formatText, formatText, formatJSON)
	okfValidateCmd.Flags().StringVar(&okfFailOn, "fail-on", "error", "Lowest severity that fails the run: error, warning, info or none")
	OKFCmd.AddCommand(okfValidateCmd)

	exportOKFCmd.Flags().StringVarP(&okfOut, "out", "o", "", "Bundle directory (default: okf.dir, docs/okf)")
	exportOKFCmd.Flags().StringVarP(&okfProfile, "profile", "p", "", "Profile to export (default: from config or 'default')")
	exportOKFCmd.Flags().StringVar(&okfRole, "role", "", "Export the slice of content a role selects (see 'ai-rulez roles list'); mutually exclusive with --profile")
	exportOKFCmd.Flags().StringSliceVar(&okfInclude, "include", nil, "Kinds to export: rules,context,skills,agents,commands,checks (default: okf.include or all)")
	exportOKFCmd.Flags().StringVar(&okfIndexStyle, "index-style", "", "index.md scheme: body (OKF 0.2 listing, default) or frontmatter (title, version, entries); default: okf.index_style")
	exportOKFCmd.Flags().BoolVar(&okfCheck, "check", false, "Write nothing; exit 2 when the bundle on disk differs")
	exportOKFCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	ExportCmd.AddCommand(exportOKFCmd)

	importOKFCmd.Flags().StringVar(&okfInto, "into", "", "Force every concept into one kind: rules, context or skills")
	importOKFCmd.Flags().StringVar(&okfDomain, "domain", "", "Place the imported content in this domain")
	importOKFCmd.Flags().BoolVar(&okfDryRun, "dry-run", false, "Report what would happen without writing")
	importOKFCmd.Flags().BoolVar(&okfForce, "force", false, "Overwrite files that exist and differ")
	addFormatFlag(importOKFCmd.Flags(), &okfFormat, formatText, formatText, formatText, formatJSON)
	importOKFCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	ImportCmd.AddCommand(importOKFCmd)
	includes.OKFScan = okfScanner(nil)
}

func loadBundle(ctx context.Context, spec string) (*okf.Bundle, func(), error) {
	src, err := okfbridge.ParseSource(spec)
	if err != nil {
		return nil, func() {}, err
	}
	dir, cleanup, err := src.Fetch(ctx)
	if err != nil {
		return nil, func() {}, err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		cleanup()
		return nil, func() {}, fmt.Errorf("%s is not a directory", spec)
	}
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return b, cleanup, nil
}

func runOKFValidate(ctx context.Context, spec string, out io.Writer) int {
	switch okfFormat {
	case "", formatText, formatJSON:
	default:
		fmtError(oops.Errorf("unknown --format %q (use text or json)", okfFormat))
		return exitOKFCannotRun
	}
	failOn := okf.Severity(okfFailOn)
	if okfFailOn != okfFailNone && failOn != okf.SeverityError && failOn != okf.SeverityWarning && failOn != okf.SeverityInfo {
		fmtError(oops.Errorf("unknown --fail-on %q (use error, warning, info or none)", okfFailOn))
		return exitOKFCannotRun
	}
	b, cleanup, err := loadBundle(ctx, spec)
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	defer cleanup()
	findings := b.Validate()
	if err := writeOKFFindings(out, spec, b, findings, okfFormat == formatJSON); err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	if okfFailOn != okfFailNone && okfFails(findings, failOn) {
		return exitOKFProblems
	}
	return 0
}

func okfRank(s okf.Severity) int {
	switch s {
	case okf.SeverityError:
		return 3
	case okf.SeverityWarning:
		return 2
	case okf.SeverityInfo:
		return 1
	}
	return 0
}

func okfFails(findings []okf.Finding, threshold okf.Severity) bool {
	for i := range findings {
		if okfRank(findings[i].Severity) >= okfRank(threshold) {
			return true
		}
	}
	return false
}

func writeOKFFindings(out io.Writer, spec string, b *okf.Bundle, findings []okf.Finding, asJSON bool) error {
	if findings == nil {
		findings = []okf.Finding{}
	}
	if asJSON {
		return jsondoc.Write(out, map[string]any{
			"bundle": spec, "okf_spec": okf.SpecVersion,
			"concepts": len(b.Concepts), "index_style": b.IndexStyle(), "findings": findings,
		})
	}
	w := reportWriter{out}
	counts := map[okf.Severity]int{}
	for i := range findings {
		f := &findings[i]
		counts[f.Severity]++
		loc := f.Path
		if loc == "" {
			loc = "."
		}
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		w.printf("%s: %s %s (%s) %s\n", loc, f.Severity, f.Code, f.Name, f.Message)
	}
	w.printf("%d concepts, %d errors, %d warnings, %d info (OKF spec %s, index style: %s)\n",
		len(b.Concepts), counts[okf.SeverityError], counts[okf.SeverityWarning], counts[okf.SeverityInfo], okf.SpecVersion, indexStyleLabel(b.IndexStyle()))
	return nil
}

func runOKFExport(ctx context.Context, args []string, out io.Writer) int {
	cfg, err := loadConfigForCommand(ctx, args, config.WithoutLocal())
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	includeList := okfInclude
	if len(includeList) == 0 {
		includeList = cfg.OKFInclude()
	}
	kinds, err := okfbridge.ParseKinds(includeList)
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	tree, err := okfExportTree(cfg)
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	style := okfIndexStyle
	if style == "" {
		style = cfg.OKFIndexStyle()
	}
	if style != okf.StyleBody && style != okf.StyleFrontmatter {
		fmtError(oops.Errorf("unknown --index-style %q (use %s or %s)", style, okf.StyleBody, okf.StyleFrontmatter))
		return exitOKFCannotRun
	}
	res, err := okfbridge.Export(tree, okfbridge.ExportOptions{Include: kinds, IndexStyle: style, LocalDir: cfg.ConfigDir})
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	dir := okfOut
	if dir == "" {
		dir = filepath.Join(cfg.BaseDir, filepath.FromSlash(cfg.OKFDir()))
	}
	w := reportWriter{out}
	for _, note := range res.Notes {
		fmt.Fprintln(os.Stderr, "note:", note)
	}
	if okfCheck {
		drift, err := okf.Compare(dir, res.Files)
		if err != nil {
			fmtError(err)
			return exitOKFCannotRun
		}
		if drift.Empty() {
			w.printf("%s is up to date (%d files)\n", dir, len(res.Files))
			return 0
		}
		for _, group := range []struct {
			label string
			paths []string
		}{{"missing", drift.Missing}, {"changed", drift.Changed}, {"extra", drift.Extra}} {
			for _, p := range group.paths {
				w.printf("%s: %s\n", group.label, p)
			}
		}
		w.printf("%s differs from the sources; run ai-rulez export okf (or generate) to update it\n", dir)
		return exitOKFProblems
	}
	if err := okf.WriteFiles(dir, res.Files, true); err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	w.printf("Wrote %d files to %s (%s)\n", len(res.Files), dir, kindCounts(res.Counts))
	return 0
}

// okfExportTree selects the content to export: a role's slice with --role, else
// the profile's.
func okfExportTree(cfg *config.Config) (*config.ContentTree, error) {
	if okfRole != "" && okfProfile != "" {
		return nil, oops.Hint("A role replaces the profile selection; pass only one").Errorf("--role and --profile are mutually exclusive")
	}
	gen := generator.NewGenerator(cfg)
	if okfRole != "" {
		return gen.ContentForRole(okfRole) //nolint:wrapcheck // already contextual
	}
	return gen.ContentForProfile(okfProfile) //nolint:wrapcheck // already contextual
}

func kindCounts(counts map[okfbridge.Kind]int) string {
	var parts []string
	for _, k := range okfbridge.AllKinds {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	if len(parts) == 0 {
		return "no content"
	}
	return strings.Join(parts, ", ")
}

func runOKFImport(ctx context.Context, spec string, out io.Writer) int {
	into, err := parseImportInto()
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	targetDir := configDir
	if targetDir == "" {
		targetDir = defaultConfigDirName
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	if info, statErr := os.Stat(absTarget); statErr != nil || !info.IsDir() {
		fmtError(oops.Hint("Run `ai-rulez init` first, or pass --config-dir.").Errorf("%s does not exist", targetDir))
		return exitOKFCannotRun
	}
	b, cleanup, err := loadBundle(ctx, spec)
	if err != nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	defer cleanup()

	lintCfg := importLintConfig(ctx)
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{
		ConfigDir: absTarget, Into: into, Domain: okfDomain, Force: okfForce, DryRun: okfDryRun,
		Scan: okfScanner(lintCfg),
	})
	var secErr *okfbridge.SecurityError
	if err != nil && res == nil {
		fmtError(err)
		return exitOKFCannotRun
	}
	if writeErr := writeOKFImport(out, spec, targetDir, res); writeErr != nil {
		fmtError(writeErr)
		return exitOKFCannotRun
	}
	switch {
	case errors.As(err, &secErr):
		fmt.Fprintln(os.Stderr, err)
		return exitOKFProblems
	case err != nil:
		fmtError(err)
		return exitOKFCannotRun
	case res.Count(okfbridge.StatusConflict) > 0:
		fmt.Fprintf(os.Stderr, "%d files exist and differ; nothing was overwritten (use --force)\n", res.Count(okfbridge.StatusConflict))
		return exitOKFProblems
	}
	return 0
}

// okfScanner adapts the AR0xx security scan to the bridge's Scanner.
func okfScanner(lc *config.LintConfig) okfbridge.Scanner {
	return func(texts map[string]string) []okfbridge.SecurityFinding {
		var found []okfbridge.SecurityFinding
		for _, f := range lint.ScanTexts(lc, texts) {
			found = append(found, okfbridge.SecurityFinding{Code: f.Code, Severity: string(f.Severity), File: f.File, Line: f.Line, Message: f.Message})
		}
		return found
	}
}

func parseImportInto() (okfbridge.Kind, error) {
	if okfInto == "" {
		return "", nil
	}
	k, err := okfbridge.ParseKinds([]string{okfInto})
	if err != nil || len(k) != 1 {
		return "", oops.Errorf("--into must be rules, context or skills, got %q", okfInto)
	}
	return k[0], nil
}

// importLintConfig returns the project's [lint] settings for the pre-write
// scan, or nil (defaults) when the configuration does not load.
func importLintConfig(ctx context.Context) *config.LintConfig {
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return nil
	}
	return cfg.Lint
}

func writeOKFImport(out io.Writer, spec, targetDir string, res *okfbridge.ImportResult) error {
	if okfFormat == formatJSON {
		return jsondoc.Write(out, map[string]any{
			keySource: spec, "target": targetDir, "dry_run": okfDryRun,
			"actions": nonNilActions(res.Actions), "findings": res.Findings,
			"security": res.Security, "skipped": res.Skipped, "index_style": res.IndexStyle,
		})
	}
	w := reportWriter{out}
	verb := map[string]string{
		okfbridge.StatusCreated: "created", okfbridge.StatusUnchanged: "unchanged",
		okfbridge.StatusOverwritten: "overwritten", okfbridge.StatusConflict: "exists, differs",
	}
	if okfDryRun {
		verb[okfbridge.StatusCreated], verb[okfbridge.StatusOverwritten] = "would create", "would overwrite"
	}
	for _, a := range res.Actions {
		w.printf("%-16s %s/%s\n", verb[a.Status], targetDir, a.Path)
	}
	for _, s := range res.Skipped {
		w.printf("%-16s %s\n", "skipped", s)
	}
	for i := range res.Findings {
		f := &res.Findings[i]
		w.printf("note: %s %s %s\n", f.Code, f.Path, f.Message)
	}
	for _, f := range res.Security {
		w.printf("security: %s %s:%d %s (%s)\n", f.Code, f.File, f.Line, f.Message, f.Severity)
	}
	if res.IndexStyle != "" {
		w.printf("index style: %s\n", res.IndexStyle)
	}
	w.printf("Summary: %d created, %d overwritten, %d unchanged, %d conflicts, %d skipped\n",
		res.Count(okfbridge.StatusCreated), res.Count(okfbridge.StatusOverwritten),
		res.Count(okfbridge.StatusUnchanged), res.Count(okfbridge.StatusConflict), len(res.Skipped))
	return nil
}

func nonNilActions(a []okfbridge.Action) []okfbridge.Action {
	if a == nil {
		return []okfbridge.Action{}
	}
	return a
}

func indexStyleLabel(style string) string {
	if style == "" {
		return "none"
	}
	return style
}
