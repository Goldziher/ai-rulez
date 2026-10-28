package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// Column headers of the exposure tables.
const (
	colKind = "KIND"
	colName = "NAME"
	colArea = "AREA"
	colPath = "PATH"
)

// exposureFlags are the flags of the exposure commands.
type exposureFlags struct {
	root          string
	areasFile     string
	plugins       string
	harness       string
	contextWindow int
	asJSON        bool
	withText      bool
}

// exposureInput is what an exposure command reads: the inventory and, when one
// is configured or given, the area map and where it came from.
type exposureInput struct {
	inv       *exposure.Inventory
	exp       *config.ExposureConfig
	areas     *areas.Map
	areasFile string
}

// exposureHeaviest is how many start-of-session artifacts the text report names.
const exposureHeaviest = 5

// newExposureCmd is the start-of-session exposure report and the static
// session-exposure commands below it.
func newExposureCmd() *cobra.Command {
	var f exposureFlags
	group := &cobra.Command{
		Use:   "exposure",
		Short: "Report what a Claude Code session loads at start, and inspect its artifacts and areas",
		Long: `Inspect the context a Claude Code session started at the repository root sees: the CLAUDE.md
files, rules, skills, agents, MCP servers and plugins, when each one loads (at session start, on demand,
only when the user runs it), and which area of the [exposure] area map it belongs to. The commands are
offline, free and deterministic; see docs/exposure.md.

Without a subcommand, exposure prints the start-of-session report: the estimated tokens a session loads
before it touches a file, by kind, the skill listing against Claude Code's listing budget (1% of
--context-window at 4 characters per token, default [exposure] context_window, else 200000), the artifacts
by phase and kind, and the heaviest artifacts at start. A root artifact whose frontmatter does not parse
(phase unknown) counts as start. --format json prints schema/exposure-report.v1.schema.json.

The repository is the project root (the directory of the configuration, else the working directory) or
--root. The configuration is -C/--config when given, else the one discovered from --root (or from the
working directory), with --config-dir and --no-local applied as by every command. The area map is the
[exposure] table of the configuration, or --areas, which replaces [exposure] areas_file. --plugins selects the repository-marketplace plugins whose skills are listed:
none, enabled (the ones the root .claude/settings*.json enable; the default unless [exposure] plugins
says otherwise) or all.

Exit codes: 0 the report was printed, 1 the command could not run (bad flags, an invalid
configuration or area map), 2 (the report only) the start tokens exceed [exposure] start_budget; exposure leakage exits 2 on a
violation or a stale acceptance.`,
		Example: `  ai-rulez exposure
  ai-rulez exposure --plugins all --context-window 1000000 --format json
  ai-rulez exposure inventory
  ai-rulez exposure inventory --format json --with-text
  ai-rulez exposure areas --areas tools/context/areas.toml --format json
  ai-rulez exposure collisions --min-score 0.2
  ai-rulez exposure leakage --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fail(runExposureReport(cmd.Context(), cmd.OutOrStdout(), &f))
		},
	}
	inventory := &cobra.Command{
		Use:   "inventory",
		Short: "List every artifact a session can see, with its phase, visibility and parse status",
		Long: `List every native Claude Code artifact of the repository with its kind, source, scope,
visibility (always, path_gated, manual_only), phase (start, on_demand, manual, unknown), listing text,
estimated tokens and frontmatter status, the root project settings that shaped them, and the files that
could not be read. With an area map, each artifact also carries its area and how it was decided.

--format json prints schema/exposure-inventory.v1.schema.json; the artifact texts are left out unless
--with-text is given (which applies to --format json only).`,
		Example: `  ai-rulez exposure inventory
  ai-rulez exposure inventory --plugins all
  ai-rulez exposure inventory --format json --with-text`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fail(runExposureInventory(cmd.Context(), cmd.OutOrStdout(), &f))
		},
	}
	areasCmd := &cobra.Command{
		Use:   "areas",
		Short: "Show the area map and the area of every artifact, with how it was decided",
		Long: `Show the area map as loaded (normalized terms, path claims, skill_areas) and, for every artifact
of the inventory, its area and its source: skill_areas (an explicit entry), path (a path claim on its
directory), terms (a majority of area terms in its text) or none (unclassified). A plugin skill is owned
by its plugin's directory. Diff the JSON against another classifier to compare the two.

--format json prints schema/exposure-areas.v1.schema.json. Without an area map the command fails.`,
		Example: `  ai-rulez exposure areas
  ai-rulez exposure areas --areas tools/context/areas.toml --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fail(runExposureAreas(cmd.Context(), cmd.OutOrStdout(), &f))
		},
	}
	pf := group.PersistentFlags()
	pf.StringVar(&f.root, "root", "", "Repository root to inspect (default: the project root, else the working directory)")
	pf.StringVar(&f.areasFile, "areas", "", "Area map file to use instead of the [exposure] table (the areas_file format)")
	pf.StringVar(&f.plugins, "plugins", "", "Marketplace plugins whose skills are listed: none, enabled or all (default: [exposure] plugins, else enabled)")
	group.Flags().IntVar(&f.contextWindow, "context-window", 0, "Model context window in tokens, for the skill listing budget (default: [exposure] context_window, else 200000)")
	group.Flags().StringVar(&f.harness, "harness", config.ExposureHarnessClaude, "Harness whose load model applies; only claude")
	addJSONFormat(group.Flags(), &f.asJSON, "")
	addJSONFormat(inventory.Flags(), &f.asJSON, "")
	addJSONFormat(areasCmd.Flags(), &f.asJSON, "")
	inventory.Flags().BoolVar(&f.withText, "with-text", false, "Include each artifact's text in the JSON output")
	group.AddCommand(inventory, areasCmd, newExposureCollisionsCmd(&f), newExposureLeakageCmd(&f))
	return group
}

func runExposureReport(ctx context.Context, w io.Writer, f *exposureFlags) error {
	if f.harness != config.ExposureHarnessClaude {
		return oops.Errorf("unknown --harness %q (only %s)", f.harness, config.ExposureHarnessClaude)
	}
	if f.contextWindow < 0 {
		return oops.Errorf("--context-window must not be negative, got %d", f.contextWindow)
	}
	in, err := loadExposure(ctx, f)
	if err != nil {
		return err
	}
	if in.areas != nil {
		in.inv.Classify(in.areas)
	}
	window := f.contextWindow
	if window == 0 {
		window = in.exp.EffectiveContextWindow()
	}
	var budget int
	if in.exp != nil {
		budget = in.exp.StartBudget
	}
	rep := exposure.NewReport(in.inv, exposure.ReportOptions{ContextWindow: window, StartBudget: budget})
	if f.asJSON {
		err = jsondoc.Write(w, rep)
	} else {
		err = writeReportText(w, rep)
	}
	if err != nil {
		return err //nolint:wrapcheck // stdout write
	}
	if rep.OverStartBudget() {
		return exitStatus(exitFindings)
	}
	return nil
}

func runExposureInventory(ctx context.Context, w io.Writer, f *exposureFlags) error {
	if f.withText && !f.asJSON {
		return oops.Errorf("--with-text applies to --format json")
	}
	in, err := loadExposure(ctx, f)
	if err != nil {
		return err
	}
	if in.areas != nil {
		in.inv.Classify(in.areas)
	}
	if f.asJSON {
		inv := in.inv
		if !f.withText {
			inv = inv.WithoutText()
		}
		return jsondoc.Write(w, inv) //nolint:wrapcheck // stdout write
	}
	return writeInventoryText(w, in.inv, in.areas != nil)
}

func runExposureAreas(ctx context.Context, w io.Writer, f *exposureFlags) error {
	in, err := loadExposure(ctx, f)
	if err != nil {
		return err
	}
	if in.areas == nil {
		return oops.Errorf("no area map: configure [exposure] areas or areas_file, or pass --areas")
	}
	rep := exposure.NewAreasReport(in.inv, in.areas, in.areasFile)
	if f.asJSON {
		return jsondoc.Write(w, rep) //nolint:wrapcheck // stdout write
	}
	return writeAreasText(w, rep)
}

// loadExposure resolves the repository, the plugin mode and the area map, and
// builds the inventory.
func loadExposure(ctx context.Context, f *exposureFlags) (*exposureInput, error) {
	if f.plugins != "" && !exposure.PluginMode(f.plugins).Valid() {
		return nil, oops.Errorf("unknown --plugins %q (use none, enabled or all)", f.plugins)
	}
	cfg, root, err := exposureProject(ctx, f.root)
	if err != nil {
		return nil, err
	}
	var exp *config.ExposureConfig
	base := root
	if cfg != nil {
		exp = cfg.Exposure
		if base, err = filepath.Abs(cfg.BaseDir); err != nil {
			return nil, oops.Wrapf(err, "resolve the project root")
		}
	}
	if problems := exp.Validate(); len(problems) > 0 {
		return nil, oops.Errorf("the [exposure] table is invalid (AR9W5): %s", strings.Join(problems, "; "))
	}
	mode := exposure.PluginMode(f.plugins)
	if mode == "" && exp != nil {
		mode = exposure.PluginMode(exp.Plugins)
	}
	in := &exposureInput{exp: exp}
	if in.areas, in.areasFile, err = exposureMap(exp, base, root, f.areasFile, configFileOf(cfg)); err != nil {
		return nil, err
	}
	if in.inv, err = exposure.Build(ctx, exposure.Options{Root: root, AreaBase: base, Plugins: mode}); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return in, nil
}

// exposureProject is the configuration (nil when there is none) and the
// absolute repository root. With --root the configuration is still -C when
// given (an unreadable or invalid one fails), else the one --config-dir names
// in the root, else the one discovered from the root upwards.
func exposureProject(ctx context.Context, rootFlag string) (*config.Config, string, error) {
	if rootFlag != "" {
		root, err := filepath.Abs(rootFlag)
		if err != nil {
			return nil, "", oops.Wrapf(err, "resolve --root")
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return nil, "", oops.Errorf("--root %s is not a directory", rootFlag)
		}
		cfg, err := exposureConfigFor(ctx, root)
		if errors.Is(err, config.ErrNoConfig) {
			return nil, root, nil
		}
		return cfg, root, err
	}
	cfg, err := loadConfigOrNoConfig(ctx, config.WithoutRemote())
	if errors.Is(err, config.ErrNoConfig) {
		root, absErr := filepath.Abs(workingDir())
		return nil, root, oops.Wrapf(absErr, "resolve the working directory")
	}
	if err != nil {
		return nil, "", err
	}
	root, err := filepath.Abs(cfg.BaseDir)
	return cfg, root, oops.Wrapf(err, "resolve the project root")
}

// exposureConfigFor loads the configuration of an explicit --root, honoring
// -C, --config-dir and --no-local as loadConfigOrNoConfig does for the working
// directory.
func exposureConfigFor(ctx context.Context, root string) (*config.Config, error) {
	opts := []config.LoadOption{config.WithoutRemote()}
	if noLocal {
		opts = append(opts, config.WithoutLocal())
	}
	switch {
	case cfgFile != "":
		return loadProjectFile(ctx, cfgFile, opts...)
	case configDir != "":
		return loadProjectDir(ctx, root, configDir, opts...)
	}
	project, err := config.DiscoverProjectRoot(ctx, root)
	switch {
	case errors.Is(err, config.ErrNoConfig):
		return nil, err //nolint:wrapcheck // the caller checks for ErrNoConfig
	case err != nil:
		project = root
	}
	return loadProject(ctx, project, opts...)
}

// exposureMap loads the area map of --areas, else of the [exposure] table,
// whose areas_file is relative to base, the directory of the configuration; it
// is nil when neither configures one. The file it names is relative to root
// and slash separated, or absolute (slash separated) when it lies outside root.
// A problem is listed with its file and line, as AR9W5 reports it: in the areas
// file, or in configPath (the configuration file, "" when there is none) for
// the inline form, where a problem whose entry is not found names [exposure].
func exposureMap(exp *config.ExposureConfig, base, root, areasFlag, configPath string) (*areas.Map, string, error) {
	var (
		m        *areas.Map
		problems []areas.Problem
		file     string
	)
	switch {
	case areasFlag != "":
		file = rootRelative(root, areasFlag)
		m, problems = areas.LoadFile(areasFlag)
	case exp != nil:
		if exp.AreasFile != "" {
			file = rootRelative(root, filepath.Join(base, filepath.FromSlash(exp.AreasFile)))
		}
		m, problems = areas.Load(exp, base)
	}
	if len(problems) == 0 {
		return m, file, nil
	}
	var cfgLines []string
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil { //nolint:gosec // the project's own configuration file
			cfgLines = strings.Split(string(data), "\n")
		}
	}
	lines := make([]string, 0, len(problems))
	for _, p := range problems {
		where := "[exposure]"
		switch {
		case p.File != "":
			where = fmt.Sprintf("%s:%d", p.File, p.Line)
		case p.LineIn(cfgLines) > 0:
			where = fmt.Sprintf("%s:%d", rootRelative(root, configPath), p.LineIn(cfgLines))
		}
		lines = append(lines, where+": "+p.Message)
	}
	return nil, "", oops.Errorf("the area map is invalid (AR9W5):\n  %s", strings.Join(lines, "\n  "))
}

// configFileOf is the absolute path of the configuration file of cfg, "" when
// there is none.
func configFileOf(cfg *config.Config) string {
	if cfg == nil || cfg.ConfigFile == "" {
		return ""
	}
	p, err := filepath.Abs(cfg.ConfigPath())
	if err != nil {
		return ""
	}
	return p
}

// rootRelative is p relative to root, slash separated, or p made absolute and
// slash separated when it lies outside root.
func rootRelative(root, p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if rel, ok := relativeBelow(root, abs); ok {
		return rel
	}
	// One side may name the other through a symlink (macOS /var is /private/var).
	if realRoot, err := filepath.EvalSymlinks(root); err == nil {
		if realAbs, err := filepath.EvalSymlinks(abs); err == nil {
			if rel, ok := relativeBelow(realRoot, realAbs); ok {
				return rel
			}
		}
	}
	return filepath.ToSlash(abs)
}

// relativeBelow is abs relative to root, slash separated, when abs lies inside root.
func relativeBelow(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// writeTable writes rows as aligned columns, the first row being the header.
func writeTable(w io.Writer, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t")) //nolint:errcheck // flushed below
	}
	return oops.Wrapf(tw.Flush(), "write the table")
}

func writeInventoryText(w io.Writer, inv *exposure.Inventory, withAreas bool) error {
	loaded := "none"
	if len(inv.Plugins.Loaded) > 0 {
		loaded = strings.Join(inv.Plugins.Loaded, ", ")
	}
	fmt.Fprintf(w, "Session inventory of %s (%s; plugins %s, loaded: %s)\n\n", inv.Root, inv.Harness, inv.Plugins.Mode, loaded) //nolint:errcheck // stdout write
	header := []string{"PHASE", colKind, colName, "TOKENS", colPath}
	if withAreas {
		header = []string{"PHASE", colKind, colName, "TOKENS", colArea, colPath}
	}
	rows := [][]string{header}
	tokens, counts := map[exposure.Phase]int{}, map[exposure.Phase]int{}
	for i := range inv.Artifacts {
		a := &inv.Artifacts[i]
		counts[a.Phase]++
		tokens[a.Phase] += a.Tokens
		row := []string{string(a.Phase), string(a.Kind), a.QualifiedName, fmt.Sprint(a.Tokens)}
		if withAreas {
			row = append(row, fmt.Sprintf("%s (%s)", a.Area, a.AreaSource))
		}
		rows = append(rows, append(row, a.Path))
	}
	if err := writeTable(w, rows); err != nil {
		return err
	}
	// The start figures count the unknown artifacts too, as Inventory.Start and
	// Report.StartArtifacts do (fail closed).
	startTokens := tokens[exposure.PhaseStart] + tokens[exposure.PhaseUnknown]
	startCount := counts[exposure.PhaseStart] + counts[exposure.PhaseUnknown]
	parts := []string{fmt.Sprintf("%d %s (%d tokens, unknown included)", startCount, exposure.PhaseStart, startTokens)}
	for _, p := range []exposure.Phase{exposure.PhaseOnDemand, exposure.PhaseManual, exposure.PhaseUnknown} {
		parts = append(parts, fmt.Sprintf("%d %s", counts[p], p))
	}
	fmt.Fprintf(w, "\n%d %s: %s\n", len(inv.Artifacts), plural(len(inv.Artifacts), "artifact"), strings.Join(parts, ", ")) //nolint:errcheck // stdout write
	for _, p := range inv.Problems {
		fmt.Fprintf(w, "problem: %s: %s\n", p.Path, p.Error) //nolint:errcheck // stdout write
	}
	return nil
}

func writeReportText(w io.Writer, rep *exposure.Report) error {
	start := rep.StartArtifacts()
	l := rep.Listing
	listing := fmt.Sprintf("skill listing: %d %s, %d chars of %d budget (%d token window)", l.Skills, plural(l.Skills, "skill"), l.Chars, l.BudgetChars, l.ContextWindow)
	if l.OverBudgetChars > 0 {
		listing += fmt.Sprintf(", OVER by %d: descriptions will be dropped", l.OverBudgetChars)
	}
	if l.DescriptionsCapped > 0 {
		listing += fmt.Sprintf(", %d %s cut at the per-skill cap", l.DescriptionsCapped, plural(l.DescriptionsCapped, "description"))
	}
	lines := []string{fmt.Sprintf("start of session: %d %s, ~%d tokens", start, plural(start, "artifact"), rep.StartTokens)}
	if rep.StartBudget > 0 {
		verdict := "within"
		if rep.OverStartBudget() {
			verdict = fmt.Sprintf("OVER by %d:", rep.StartTokens-rep.StartBudget)
		}
		lines = append(lines, fmt.Sprintf("start budget: %s [exposure] start_budget %d", verdict, rep.StartBudget))
	}
	lines = append(lines, listing, "start tokens by kind:")
	lines = append(lines, countLines(rep.StartTokensByKind)...)
	lines = append(lines, "artifacts by phase and kind:")
	for _, p := range []exposure.Phase{exposure.PhaseStart, exposure.PhaseOnDemand, exposure.PhaseManual, exposure.PhaseUnknown} {
		counts, ok := rep.CountsByPhaseAndKind[p]
		if !ok {
			continue
		}
		parts := make([]string, 0, len(counts))
		for _, kind := range byCountDesc(counts) {
			parts = append(parts, fmt.Sprintf("%s %d", kind, counts[kind]))
		}
		if len(parts) == 0 {
			parts = []string{"none"}
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", p, strings.Join(parts, ", ")))
	}
	lines = append(lines, "tokens if loaded, by phase:")
	for _, p := range []exposure.Phase{exposure.PhaseStart, exposure.PhaseOnDemand, exposure.PhaseManual, exposure.PhaseUnknown} {
		if n, ok := rep.TokensByPhase[p]; ok {
			lines = append(lines, fmt.Sprintf("  %s: %d", p, n))
		}
	}
	lines = append(lines, "heaviest at start:")
	for _, it := range rep.Heaviest(exposureHeaviest) {
		lines = append(lines, fmt.Sprintf("  ~%d %s %s", it.Tokens, it.Kind, it.Path))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return oops.Wrapf(err, "write the exposure report")
}

// countLines renders counts as indented `kind: n` lines, the largest first.
func countLines(counts map[exposure.Kind]int) []string {
	out := make([]string, 0, len(counts))
	for _, kind := range byCountDesc(counts) {
		out = append(out, fmt.Sprintf("  %s: %d", kind, counts[kind]))
	}
	return out
}

// byCountDesc is the kinds of counts, the largest count first, then by name.
func byCountDesc(counts map[exposure.Kind]int) []exposure.Kind {
	kinds := make([]exposure.Kind, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if counts[kinds[i]] != counts[kinds[j]] {
			return counts[kinds[i]] > counts[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	return kinds
}

func writeAreasText(w io.Writer, rep *exposure.AreasReport) error {
	from := "the [exposure] table"
	if rep.AreasFile != "" {
		from = rep.AreasFile
	}
	fmt.Fprintf(w, "Area map of %s: %s (%d %s, %d skill_areas)\n\n", rep.Root, from, len(rep.Areas), plural(len(rep.Areas), "area"), len(rep.SkillAreas)) //nolint:errcheck // stdout write
	rows := [][]string{{colArea, "OWNER", "SHARED", "ARTIFACTS", "PATHS"}}
	for i := range rep.Areas {
		a := &rep.Areas[i]
		shared := "no"
		if a.Shared {
			shared = "yes"
		}
		rows = append(rows, []string{a.Name, a.Owner, shared, fmt.Sprint(rep.Counts[a.Name]), strings.Join(a.Paths, ", ")})
	}
	rows = append(rows, []string{areas.Unclassified, "", "", fmt.Sprint(rep.Counts[areas.Unclassified]), ""})
	if err := writeTable(w, rows); err != nil {
		return err
	}
	if len(rep.SkillAreas) > 0 {
		skills := make([]string, 0, len(rep.SkillAreas))
		for s := range rep.SkillAreas {
			skills = append(skills, s)
		}
		sort.Strings(skills)
		rows = [][]string{{"SKILL", colArea}}
		for _, s := range skills {
			rows = append(rows, []string{s, rep.SkillAreas[s]})
		}
		fmt.Fprintln(w) //nolint:errcheck // stdout write
		if err := writeTable(w, rows); err != nil {
			return err
		}
	}
	rows = [][]string{{colKind, colName, colArea, "SOURCE", colPath}}
	for i := range rep.Artifacts {
		a := &rep.Artifacts[i]
		rows = append(rows, []string{string(a.Kind), a.QualifiedName, a.Area, string(a.AreaSource), a.Path})
	}
	fmt.Fprintln(w) //nolint:errcheck // stdout write
	return writeTable(w, rows)
}
