package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

var (
	catalogDiffFormat   string
	catalogDiffExitCode bool
)

// maxCatalogJSONBytes bounds a catalog JSON file read for a diff.
const maxCatalogJSONBytes = 256 << 20

// workingTreeLabel names the current project on the right-hand side of a diff.
const workingTreeLabel = "working tree"

var catalogDiffCmd = &cobra.Command{
	Use:   "diff <from> [<to>]",
	Short: "Compare two catalogs: JSON files or git revisions",
	Long: `Compare two catalogs and print what was added, removed or changed: items (by
digest, description, owner, version, load cost, lint status, approval and roles),
MCP servers, roles, skill dependencies and the lint totals.

Each argument is a catalog JSON file written by ` + "`catalog --format json --schema-version 2`" + ` (or the
catalog.json of a ` + "`--html`" + ` site), or a git revision. A revision is read from git without
touching the working tree: the tracked files of the configuration directory at that
commit are extracted to a temporary directory and a catalog is built from them. With
one argument, the other side is the current project's shared configuration (the
machine-local overlay is left out so both sides see what a teammate sees). Remote
includes and installed skills are not resolved for a revision or the working tree;
compare two catalog.json files to include them.

Exit codes: 0 unless the command could not run (1); with --exit-code, 2 when the
catalogs differ.`,
	Example: `  ai-rulez catalog diff main
  ai-rulez catalog diff v5.0.0 HEAD --format json
  ai-rulez catalog diff before.json after.json --exit-code`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		identical, err := runCatalogDiff(cmd.Context(), cmd.OutOrStdout(), args)
		exitOn(err)
		if catalogDiffExitCode && !identical {
			os.Exit(exitCatalogDrift)
		}
	},
}

func init() {
	addFormatFlag(catalogDiffCmd.Flags(), &catalogDiffFormat, "", formatText, formatText, formatJSON)
	addJSONFlagAlias(catalogDiffCmd.Flags())
	catalogDiffCmd.Flags().BoolVar(&catalogDiffExitCode, "exit-code", false, "Exit 2 when the catalogs differ")
	catalogDiffCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	CatalogCmd.AddCommand(catalogDiffCmd)
}

// catalogSide is one resolved input of a diff.
type catalogSide struct {
	doc  *govview.CatalogDocV2
	side govview.DiffSide
	note string
}

// catalogDiffProject finds the project once, for the sides that need it.
type catalogDiffProject struct {
	cfg *config.Config
}

func (p *catalogDiffProject) load(ctx context.Context) (*config.Config, error) {
	if p.cfg != nil {
		return p.cfg, nil
	}
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutRemote(), config.WithoutLocal())
	if err != nil {
		return nil, err
	}
	p.cfg = cfg
	return cfg, nil
}

func runCatalogDiff(ctx context.Context, out io.Writer, args []string) (identical bool, err error) {
	if err := checkFormatFlag(catalogDiffFormat); err != nil {
		return false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	project := &catalogDiffProject{}
	from, err := resolveCatalogSide(ctx, project, args[0])
	if err != nil {
		return false, err
	}
	to := &catalogSide{}
	if len(args) == 2 {
		to, err = resolveCatalogSide(ctx, project, args[1])
	} else {
		to, err = workingTreeSide(ctx, project)
	}
	if err != nil {
		return false, err
	}
	diff, err := govview.DiffCatalogs(from.doc, to.doc, from.side, to.side)
	if err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	for _, s := range []*catalogSide{from, to} {
		if s.note != "" {
			diff.Notes = append(diff.Notes, s.note)
		}
	}
	if catalogDiffFormat == formatJSON {
		return diff.Identical, writeJSON(out, diff)
	}
	writeCatalogDiffText(reportWriter{out}, diff)
	return diff.Identical, nil
}

// resolveCatalogSide reads arg as a catalog JSON file when one exists at that path
// and as a git revision otherwise.
func resolveCatalogSide(ctx context.Context, project *catalogDiffProject, arg string) (*catalogSide, error) {
	if info, err := os.Stat(arg); err == nil && info.Mode().IsRegular() {
		return readCatalogFile(arg)
	}
	if strings.HasSuffix(strings.ToLower(arg), ".json") {
		return nil, oops.With("path", arg).Errorf("catalog file %s was not found", arg)
	}
	return revisionSide(ctx, project, arg)
}

func readCatalogFile(file string) (*catalogSide, error) {
	f, err := os.Open(file) //nolint:gosec // a file the user named on the command line
	if err != nil {
		return nil, oops.With("path", file).Wrapf(err, "open catalog file")
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxCatalogJSONBytes+1))
	if err != nil {
		return nil, oops.With("path", file).Wrapf(err, "read catalog file")
	}
	if len(data) > maxCatalogJSONBytes {
		return nil, oops.With("path", file).Errorf("catalog file %s is larger than %d MiB", filepath.Base(file), maxCatalogJSONBytes>>20)
	}
	doc, err := govview.ParseCatalogV2(data)
	if err != nil {
		return nil, oops.With("path", file).Wrapf(err, "parse catalog file %s", filepath.Base(file))
	}
	return &catalogSide{doc: doc, side: govview.DiffSide{Label: filepath.Base(file)}}, nil
}

func workingTreeSide(ctx context.Context, project *catalogDiffProject) (*catalogSide, error) {
	cfg, err := project.load(ctx)
	if err != nil {
		return nil, err
	}
	doc, err := buildDiffCatalog(cfg)
	if err != nil {
		return nil, err
	}
	return &catalogSide{doc: doc, side: govview.DiffSide{Label: workingTreeLabel}}, nil
}

func revisionSide(ctx context.Context, project *catalogDiffProject, rev string) (*catalogSide, error) {
	cfg, err := project.load(ctx)
	if err != nil {
		return nil, oops.Hint("run catalog diff inside the project, or pass catalog JSON files").Wrapf(err, "locate the project to read revision %q", rev)
	}
	top := gitutil.Git{}.TopLevel(cfg.BaseDir)
	if top == "" {
		return nil, oops.Errorf("%s is not inside a git work tree: cannot read revision %q", cfg.BaseDir, rev)
	}
	base := gitutil.RepoRelative(top, cfg.BaseDir)
	name := filepath.Base(cfg.ConfigDir)
	if base == "" || name == "" || name == "." {
		return nil, oops.Errorf("the configuration directory %s is outside the git work tree", cfg.ConfigDir)
	}
	dest, err := os.MkdirTemp("", "ai-rulez-catalog-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create a snapshot directory")
	}
	defer os.RemoveAll(dest) //nolint:errcheck // best-effort cleanup of a temporary directory
	snap, err := govview.ExtractRevision(ctx, cfg.BaseDir, rev, path.Join(filepath.ToSlash(base), name), dest)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	revCfg, err := loadProjectDir(ctx, filepath.Join(dest, filepath.FromSlash(base)), name, config.WithoutRemote(), config.WithoutLocal())
	if err != nil {
		return nil, oops.With("rev", rev).Wrapf(err, "load the configuration at revision %q", rev)
	}
	if err := revCfg.Validate(); err != nil {
		return nil, oops.With("rev", rev).Wrapf(err, "the configuration at revision %q is invalid", rev)
	}
	doc, err := buildDiffCatalog(revCfg)
	if err != nil {
		return nil, err
	}
	side := &catalogSide{doc: doc, side: govview.DiffSide{Label: rev, Commit: snap.Commit}}
	if len(snap.Symlinks) > 0 {
		side.note = fmt.Sprintf("%d symlink(s) at revision %s were not followed: %s", len(snap.Symlinks), shortCommit(snap.Commit), strings.Join(snap.Symlinks, ", "))
	}
	return side, nil
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// buildDiffCatalog builds the version 2 catalog of cfg for a comparison: no
// excerpts, no eval or usage, linted in process.
func buildDiffCatalog(cfg *config.Config) (*govview.CatalogDocV2, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	counter, err := tokens.New("")
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return buildCatalogV2(cfg, counter, catalogSettings{Excerpt: false})
}

func writeCatalogDiffText(w reportWriter, d *govview.CatalogDiff) {
	w.printf("catalog diff: %s -> %s\n", sideLabel(d.From), sideLabel(d.To))
	if d.Identical {
		w.printf("no differences (%d items)\n", d.To.Items)
		writeDiffNotes(w, d.Notes)
		return
	}
	w.printf("totals: %d -> %d items   listing tokens %d -> %d   body tokens %d -> %d\n", d.From.Items, d.To.Items,
		d.From.ListingTokens, d.To.ListingTokens, d.From.BodyTokens, d.To.BodyTokens)
	writeDiffSection(w, "items", &d.Items)
	writeDiffSection(w, "mcp servers", &d.MCPServers)
	writeDiffSection(w, "roles", &d.Roles)
	writeDiffSection(w, "dependencies", &d.Edges)
	if d.Lint != nil && d.Lint.From != d.Lint.To {
		w.printf("lint: %d errors, %d warnings, %d infos -> %d errors, %d warnings, %d infos\n",
			d.Lint.From.Errors, d.Lint.From.Warnings, d.Lint.From.Infos, d.Lint.To.Errors, d.Lint.To.Warnings, d.Lint.To.Infos)
	}
	writeDiffNotes(w, d.Notes)
}

func sideLabel(s govview.DiffSide) string {
	label := safeText(s.Label)
	if s.Commit != "" {
		label += " (" + shortCommit(s.Commit) + ")"
	}
	return label
}

func writeDiffSection(w reportWriter, title string, s *govview.DiffSection) {
	if s.Empty() {
		return
	}
	w.printf("%s: %d added, %d removed, %d changed\n", title, len(s.Added), len(s.Removed), len(s.Changed))
	for _, e := range s.Added {
		w.printf("  + %s%s\n", safeText(e.Ref), tokenSuffix(e.Tokens))
	}
	for _, e := range s.Removed {
		w.printf("  - %s%s\n", safeText(e.Ref), tokenSuffix(e.Tokens))
	}
	for _, c := range s.Changed {
		parts := make([]string, 0, len(c.Changes))
		for _, ch := range c.Changes {
			parts = append(parts, fmt.Sprintf("%s: %s -> %s", ch.Field, safeText(dashIfEmpty(ch.From)), safeText(dashIfEmpty(ch.To))))
		}
		w.printf("  ~ %s   %s\n", safeText(c.Ref), strings.Join(parts, "; "))
	}
}

func tokenSuffix(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("   (%d tokens)", n)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeDiffNotes(w reportWriter, notes []string) {
	for _, n := range notes {
		w.printf("note: %s\n", safeText(n))
	}
}
