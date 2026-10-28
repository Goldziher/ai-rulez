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
	root      string
	areasFile string
	plugins   string
	asJSON    bool
	withText  bool
}

// exposureInput is what an exposure command reads: the inventory and, when one
// is configured or given, the area map and where it came from.
type exposureInput struct {
	inv       *exposure.Inventory
	areas     *areas.Map
	areasFile string
}

// newExposureCmd groups the static session-exposure commands.
func newExposureCmd() *cobra.Command {
	var f exposureFlags
	group := &cobra.Command{
		Use:   "exposure",
		Short: "Inspect what a Claude Code session loads and which area each artifact belongs to",
		Long: `Inspect the context a Claude Code session started at the repository root sees: the CLAUDE.md
files, rules, skills, agents, MCP servers and plugins, when each one loads (at session start, on demand,
only when the user runs it), and which area of the [exposure] area map it belongs to. The commands are
offline, free and deterministic; see docs/exposure.md.

The repository is the project root (the directory of the configuration, else the working directory) or
--root. The area map is the [exposure] table of the configuration, or --areas, which replaces
[exposure] areas_file. --plugins selects the repository-marketplace plugins whose skills are listed:
none, enabled (the ones the root .claude/settings*.json enable; the default unless [exposure] plugins
says otherwise) or all.

Exit codes: 0 the report was printed, 1 the command could not run (bad flags, an invalid
configuration or area map).`,
		Example: `  ai-rulez exposure inventory
  ai-rulez exposure inventory --format json --with-text
  ai-rulez exposure areas --areas tools/context/areas.toml --format json`,
	}
	inventory := &cobra.Command{
		Use:   "inventory",
		Short: "List every artifact a session can see, with its phase, visibility and parse status",
		Long: `List every native Claude Code artifact of the repository with its kind, source, scope,
visibility (always, path_gated, manual_only), phase (start, on_demand, manual, unknown), listing text,
estimated tokens and frontmatter status, the root project settings that shaped them, and the files that
could not be read. With an area map, each artifact also carries its area and how it was decided.

--format json prints schema/exposure-inventory.v1.schema.json; the artifact texts are left out unless
--with-text is given.`,
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
	addJSONFormat(inventory.Flags(), &f.asJSON, "")
	addJSONFormat(areasCmd.Flags(), &f.asJSON, "")
	inventory.Flags().BoolVar(&f.withText, "with-text", false, "Include each artifact's text in the JSON output")
	group.AddCommand(inventory, areasCmd)
	return group
}

func runExposureInventory(ctx context.Context, w io.Writer, f *exposureFlags) error {
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
	if cfg != nil {
		exp = cfg.Exposure
	}
	if problems := exp.Validate(); len(problems) > 0 {
		return nil, oops.Errorf("the [exposure] table is invalid (AR9W5): %s", strings.Join(problems, "; "))
	}
	mode := exposure.PluginMode(f.plugins)
	if mode == "" && exp != nil {
		mode = exposure.PluginMode(exp.Plugins)
	}
	in := &exposureInput{}
	if in.areas, in.areasFile, err = exposureMap(exp, root, f.areasFile); err != nil {
		return nil, err
	}
	if in.inv, err = exposure.Build(ctx, exposure.Options{Root: root, Plugins: mode}); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return in, nil
}

// exposureProject is the configuration (nil when there is none) and the
// absolute repository root.
func exposureProject(ctx context.Context, rootFlag string) (*config.Config, string, error) {
	if rootFlag != "" {
		root, err := filepath.Abs(rootFlag)
		if err != nil {
			return nil, "", oops.Wrapf(err, "resolve --root")
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return nil, "", oops.Errorf("--root %s is not a directory", rootFlag)
		}
		cfg, err := loadProject(ctx, root, config.WithoutRemote())
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

// exposureMap loads the area map of --areas, else of the [exposure] table; it
// is nil when neither configures one.
func exposureMap(exp *config.ExposureConfig, root, areasFlag string) (*areas.Map, string, error) {
	var (
		m        *areas.Map
		problems []areas.Problem
		file     string
	)
	switch {
	case areasFlag != "":
		file = areasFlag
		m, problems = areas.LoadFile(areasFlag)
	case exp != nil:
		file = exp.AreasFile
		m, problems = areas.Load(exp, root)
	}
	if len(problems) == 0 {
		return m, file, nil
	}
	lines := make([]string, 0, len(problems))
	for _, p := range problems {
		where := "[exposure]"
		if p.File != "" {
			where = fmt.Sprintf("%s:%d", p.File, p.Line)
		}
		lines = append(lines, where+": "+p.Message)
	}
	return nil, "", oops.Errorf("the area map is invalid (AR9W5):\n  %s", strings.Join(lines, "\n  "))
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
	parts := []string{fmt.Sprintf("%d %s (%d tokens)", counts[exposure.PhaseStart], exposure.PhaseStart, tokens[exposure.PhaseStart])}
	for _, p := range []exposure.Phase{exposure.PhaseOnDemand, exposure.PhaseManual, exposure.PhaseUnknown} {
		parts = append(parts, fmt.Sprintf("%d %s", counts[p], p))
	}
	fmt.Fprintf(w, "\n%d %s: %s\n", len(inv.Artifacts), plural(len(inv.Artifacts), "artifact"), strings.Join(parts, ", ")) //nolint:errcheck // stdout write
	for _, p := range inv.Problems {
		fmt.Fprintf(w, "problem: %s: %s\n", p.Path, p.Error) //nolint:errcheck // stdout write
	}
	return nil
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
