package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/catalogsite"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

var (
	catalogFormat        string
	catalogSchemaFlag    int
	catalogHTMLDir       string
	catalogRole          string
	catalogExcerpt       bool
	catalogIndexable     bool
	catalogClean         bool
	catalogCheck         bool
	catalogTitle         string
	catalogAllowFindings []string
	// catalogWithEval and catalogWithUsage hold the file --with-eval and
	// --with-usage name, or catalogDefaultInput for the project's own.
	catalogWithEval, catalogWithUsage string
	// catalogExcerptSet records that --include-excerpt was given, so the
	// default (on, except for --indexable) is not mistaken for a choice. The other
	// *Set variables do the same for the flags [catalog] can also set.
	catalogExcerptSet, catalogIndexableSet, catalogPageSizeSet, catalogMarkdownSet bool
	catalogNoOwners, catalogMarkdown                                               bool
	catalogPageSize                                                                int
)

// catalogSchemaVersion versions the default JSON of `ai-rulez catalog --format json`.
const catalogSchemaVersion = govview.CatalogSchemaVersion

// CatalogCmd prints every authored item with its owner, version, size, digest,
// the roles that keep it and the lock status, for a UI or an audit script, or
// renders the same data as a static website.
var CatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List every item with owner, version, tokens, roles and lock status, or a static site",
	Long: `Print the project's catalog: every rule, skill, agent, command and context file
with its id, domain, source, owner and version, size, sha256 digest (the one
ai-rulez.lock pins), the roles that keep it, plus a summary of every role and the
lock status. Nothing is rendered into harness outputs.

--format json is versioned by schema_version. It prints version 1 by default
(schema/catalog.v1.schema.json); --schema-version 2 adds the description, source,
load cost (listing, body and resource tokens), lint result and body excerpt of
every item (schema/catalog.schema.json). Version 1 stays the default for one minor
release.

--html <dir> writes a static, self-contained website of the version 2 catalog: an
overview with search, one page per item and role, the lock and the lint findings.
It opens from file://, loads nothing from the network and gives the same bytes for
the same input. The directory must be new, empty or one a previous run filled
(marked by .ai-rulez-catalog); --clean also removes files an earlier run wrote
that the site no longer has. --role keeps one role's items. Excerpts are on by
default and off with --indexable; pass --include-excerpt=false to leave them out.
--check writes nothing: it exits 2 when the directory differs from the site that
would be generated (a changed, missing or unexpected file), 0 when it matches.

--with-eval and --with-usage add each skill's recorded eval result and use count
(aggregates only) from the project's eval-results.json and usage log, or from the
file they name.`,
	Example: `  ai-rulez catalog --format json --schema-version 2
  ai-rulez catalog --html site/
  ai-rulez catalog --html site/ --role backend --clean
  ai-rulez catalog --html site/ --check
  ai-rulez catalog --html site/ --with-eval --with-usage`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		catalogExcerptSet = cmd.Flags().Changed("include-excerpt")
		catalogIndexableSet = cmd.Flags().Changed("indexable")
		catalogPageSizeSet = cmd.Flags().Changed("max-items-per-page")
		catalogMarkdownSet = cmd.Flags().Changed("render-markdown")
		err := runCatalog(cmd.OutOrStdout())
		if errors.Is(err, errCatalogDrift) {
			os.Exit(exitCatalogDrift)
		}
		exitOn(err)
	},
}

func init() {
	addFormatFlag(CatalogCmd.Flags(), &catalogFormat, "", formatText, formatText, formatJSON)
	addJSONFlagAlias(CatalogCmd.Flags())
	CatalogCmd.Flags().IntVar(&catalogSchemaFlag, "schema-version", govview.CatalogSchemaVersion, "JSON schema version: 1 or 2")
	CatalogCmd.Flags().StringVar(&catalogHTMLDir, "html", "", "Write a static website of the catalog into this directory")
	CatalogCmd.Flags().StringVar(&catalogRole, "role", "", "With --html: keep only the items this role keeps")
	CatalogCmd.Flags().BoolVar(&catalogExcerpt, "include-excerpt", true, "Include a body excerpt of each item (version 2 JSON and --html); --indexable turns it off unless this flag is set")
	CatalogCmd.Flags().BoolVar(&catalogIndexable, "indexable", false, "With --html: let search engines index the site (no robots.txt, no noindex)")
	CatalogCmd.Flags().IntVar(&catalogPageSize, "max-items-per-page", 0, "With --html: overview rows per page (default: [catalog] max_items_per_page, else 200)")
	CatalogCmd.Flags().BoolVar(&catalogMarkdown, "render-markdown", false, "With --html: render item excerpts as sanitized Markdown (no raw HTML; links shown as text)")
	CatalogCmd.Flags().BoolVar(&catalogNoOwners, "no-owners", false, "Leave owner names out of the version 2 JSON and the site")
	CatalogCmd.Flags().BoolVar(&catalogClean, "clean", false, "With --html: remove files a previous run wrote that the site no longer has")
	CatalogCmd.Flags().BoolVar(&catalogCheck, "check", false, "With --html: write nothing, exit 2 when the directory differs from the site that would be generated")
	CatalogCmd.Flags().StringVar(&catalogTitle, "base-title", "", "With --html: site title (default: AI-Rulez catalog)")
	CatalogCmd.Flags().StringSliceVar(&catalogAllowFindings, "allow-findings", nil, "With --html: publish despite findings of these codes (AR001: a secret in an item)")
	CatalogCmd.Flags().StringVar(&catalogWithEval, "with-eval", "", "Add each skill's recorded eval result (default file: <config dir>/eval-results.json)")
	CatalogCmd.Flags().StringVar(&catalogWithUsage, "with-usage", "", "Add each skill's use count from a usage log (default file: <config dir>/local/usage.jsonl)")
	CatalogCmd.Flags().Lookup("with-eval").NoOptDefVal = catalogDefaultInput
	CatalogCmd.Flags().Lookup("with-usage").NoOptDefVal = catalogDefaultInput
	CatalogCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	CatalogCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// The catalog documents are built by internal/govview, which the MCP tools share.
type (
	catalogItem = govview.CatalogItem
	catalogDoc  = govview.CatalogDoc
	catalogLock = govview.CatalogLock
)

func checkCatalogFlags() error {
	if err := checkFormatFlag(catalogFormat); err != nil {
		return err
	}
	if catalogSchemaFlag != govview.CatalogSchemaVersion && catalogSchemaFlag != govview.CatalogSchemaVersionV2 {
		return oops.Errorf("unknown --schema-version %d (use 1 or 2)", catalogSchemaFlag)
	}
	if catalogHTMLDir != "" {
		return checkCatalogHTMLFlags()
	}
	return checkCatalogListFlags()
}

// checkCatalogHTMLFlags validates the flags of a --html run.
func checkCatalogHTMLFlags() error {
	switch {
	case catalogFormat != "":
		return oops.Errorf("--html writes a website; drop --format")
	case catalogCheck && catalogClean:
		return oops.Errorf("--check writes nothing: drop --clean")
	case catalogPageSizeSet && catalogPageSize < 0:
		return oops.Errorf("--max-items-per-page must not be negative")
	}
	return nil
}

// checkCatalogListFlags validates the flags of a text or JSON run, which take no
// --html-only option.
func checkCatalogListFlags() error {
	json2 := catalogFormat == formatJSON && catalogSchemaFlag == govview.CatalogSchemaVersionV2
	switch {
	case catalogCheck:
		return oops.Errorf("--check applies to --html only")
	case catalogRole != "" || catalogClean || catalogIndexable || catalogTitle != "" || len(catalogAllowFindings) > 0 || catalogPageSizeSet:
		return oops.Errorf("--role, --clean, --indexable, --base-title, --max-items-per-page and --allow-findings apply to --html only")
	case catalogMarkdownSet:
		return oops.Errorf("--render-markdown applies to --html only")
	case catalogNoOwners && !json2:
		return oops.Errorf("--no-owners applies to --html and to --format json --schema-version 2")
	case catalogFormat != formatJSON && catalogSchemaFlag != govview.CatalogSchemaVersion:
		return oops.Errorf("--schema-version applies to --format json")
	case (catalogWithEval != "" || catalogWithUsage != "") && !json2:
		return oops.Errorf("--with-eval and --with-usage apply to --html and to --format json --schema-version 2")
	}
	return nil
}

func runCatalog(out io.Writer) error {
	if err := checkCatalogFlags(); err != nil {
		return err
	}
	cfg, err := loadConfigForCommand(cmdContext(), nil)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	counter, err := tokens.New("")
	if err != nil {
		return oops.Wrap(err)
	}
	settings := resolveCatalogSettings(cfg)
	if catalogHTMLDir != "" {
		return runCatalogHTML(out, cfg, counter, settings)
	}
	if catalogFormat == formatJSON && catalogSchemaFlag == govview.CatalogSchemaVersionV2 {
		doc, err := buildCatalogV2(cfg, counter, settings)
		if err != nil {
			return err
		}
		return writeJSON(out, doc)
	}
	doc, err := buildCatalog(cfg, counter)
	if err != nil {
		return err
	}
	if catalogFormat == formatJSON {
		return writeJSON(out, doc)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	tp := reportWriter{tw}
	tp.printf("KIND\tDOMAIN\tID\tOWNER\tVERSION\tTOKENS\tROLES\n")
	for i := range doc.Items {
		it := &doc.Items[i]
		tp.printf("%s\t%s\t%s\t%s\t%s\t%d\t%d\n", it.Kind, dash(it.Domain), it.ID, dash(it.Owner), dash(it.Version), it.Tokens, len(it.Roles))
	}
	if err := tw.Flush(); err != nil {
		return oops.Wrap(err)
	}
	reportWriter{out}.printf("\n%d item(s), %d role(s); lock: %s\n", len(doc.Items), len(doc.Roles), lockSummary(doc.Lock))
	return nil
}

// catalogSettings are the effective catalog options: a flag that was given wins
// over the [catalog] table, which wins over the default.
type catalogSettings struct {
	Title                                  string
	Excerpt, Indexable, NoOwners, Markdown bool
	PageSize                               int
}

func resolveCatalogSettings(cfg *config.Config) catalogSettings {
	cat := cfg.Catalog
	if cat == nil {
		cat = &config.CatalogConfig{}
	}
	st := catalogSettings{Title: catalogTitle, Indexable: catalogIndexable, NoOwners: catalogNoOwners || cat.ExcludeOwners, PageSize: cat.MaxItemsPerPage}
	if st.Title == "" {
		st.Title = cat.Title
	}
	if !catalogIndexableSet && !catalogIndexable {
		st.Indexable = cat.Indexable
	}
	switch {
	case catalogExcerptSet || !catalogExcerpt: // a false value can only come from the flag
		st.Excerpt = catalogExcerpt
	case cat.IncludeExcerpt != nil:
		st.Excerpt = *cat.IncludeExcerpt
	default:
		st.Excerpt = !st.Indexable
	}
	if catalogPageSizeSet {
		st.PageSize = catalogPageSize
	}
	st.Markdown = cat.RenderMarkdown
	if catalogMarkdownSet {
		st.Markdown = catalogMarkdown
	}
	return st
}

// buildCatalogV2 builds the version 2 catalog, linting the project in process
// (the same engine as `validate --strict`). A lint that cannot run leaves the
// lint fields out and says so; it never fails the catalog.
func buildCatalogV2(cfg *config.Config, counter tokens.Counter, st catalogSettings) (*govview.CatalogDocV2, error) {
	opts := govview.CatalogOptions{NoExcerpt: !st.Excerpt}
	report, err := strictLint(cfg)
	if err != nil {
		logger.Warn("Catalog lint did not run", "error", err)
		opts.LintReason = "the lint engine could not run"
	} else {
		opts.Lint = report
	}
	if err := addCatalogSignals(cfg, &opts); err != nil {
		return nil, err
	}
	doc, err := govview.BuildCatalogV2(cfg, counter, Version, opts)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if st.NoOwners {
		doc = govview.OmitOwners(doc)
	}
	return doc, nil
}

func runCatalogHTML(out io.Writer, cfg *config.Config, counter tokens.Counter, st catalogSettings) error {
	doc, err := buildCatalogV2(cfg, counter, st)
	if err != nil {
		return err
	}
	if err := refuseSecrets(doc, catalogAllowFindings); err != nil {
		return err
	}
	if doc, err = govview.ViewCatalogV2(doc, catalogRole); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	site, err := catalogsite.Render(doc, catalogsite.Options{Title: st.Title, Indexable: st.Indexable, PageSize: st.PageSize, Markdown: st.Markdown})
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if catalogCheck {
		return checkCatalogSite(out, doc, site)
	}
	res, err := catalogsite.Write(nil, catalogHTMLDir, site, catalogClean)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	w := reportWriter{out}
	w.printf("wrote %d files to %s (%d items, %d roles, %d MCP servers)   catalog digest %s\n", res.Written, catalogHTMLDir, len(doc.Items), len(doc.Roles), len(doc.MCPServers), site.Digest)
	if len(res.Removed) > 0 {
		w.printf("removed %d stale file(s)\n", len(res.Removed))
	}
	if doc.Lint.Available {
		w.printf("lint: %d errors, %d warnings   lock: %s\n", doc.Lint.Summary.Errors, doc.Lint.Summary.Warnings, lockSummary(doc.Lock))
	} else {
		w.printf("lint: unavailable   lock: %s\n", lockSummary(doc.Lock))
	}
	return nil
}

// exitCatalogDrift is the exit code of `catalog --html --check` when the
// directory differs from the site that would be generated.
const exitCatalogDrift = 2

// errCatalogDrift says the checked directory differs from the generated site; the
// differences were already printed.
var errCatalogDrift = errors.New("catalog site is out of date")

// checkCatalogSite compares the directory with the site that would be written and
// prints what differs.
func checkCatalogSite(out io.Writer, doc *govview.CatalogDocV2, site *catalogsite.Site) error {
	res, err := catalogsite.Check(catalogHTMLDir, site)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	w := reportWriter{out}
	if !res.Drift() {
		w.printf("%s is up to date (%d items, %d roles, %d MCP servers)   catalog digest %s\n", catalogHTMLDir, len(doc.Items), len(doc.Roles), len(doc.MCPServers), site.Digest)
		return nil
	}
	w.printf("%s differs from the catalog site that would be generated:\n", catalogHTMLDir)
	for _, group := range []struct {
		label string
		paths []string
	}{{"missing", res.Missing}, {"changed", res.Changed}, {"unexpected", res.Extra}} {
		for _, p := range group.paths {
			w.printf("  %s  %s\n", group.label, p)
		}
	}
	w.printf("run `ai-rulez catalog --html %s` to regenerate it\n", catalogHTMLDir)
	return errCatalogDrift
}

// refuseSecrets stops a site that would publish an item the secret scanner
// flagged (AR001), unless the code is allowed.
func refuseSecrets(doc *govview.CatalogDocV2, allow []string) error {
	if slices.Contains(allow, lint.CodeSecretDetected) {
		return nil
	}
	var flagged []string
	for i := range doc.Items {
		it := &doc.Items[i]
		if itemLintFlagged(it) || publishedTextHasSecret(it) {
			flagged = append(flagged, it.Ref)
		}
	}
	if len(flagged) == 0 {
		return nil
	}
	return oops.Hint("remove the secret, or pass --allow-findings AR001 to publish anyway (discouraged)").
		Errorf("refusing to publish: the secret scanner (%s) flagged %s", lint.CodeSecretDetected, strings.Join(flagged, ", "))
}

// itemLintFlagged says the item's lint findings include the secret code.
func itemLintFlagged(it *govview.CatalogItemV2) bool {
	if it.Lint == nil {
		return false
	}
	for _, f := range it.Lint.Findings {
		if f.Code == lint.CodeSecretDetected {
			return true
		}
	}
	return false
}

// publishedTextHasSecret scans the text of the item the site would publish (its
// description and excerpt) directly. The lint findings alone are not enough: the
// lint may not have run, or the finding may be baselined or suppressed, and the
// text would still go out. Inline ignore comments are not honoured.
func publishedTextHasSecret(it *govview.CatalogItemV2) bool {
	text := it.Description
	if it.Excerpt != nil {
		text += "\n" + it.Excerpt.Text
	}
	for _, f := range lint.ScanText(it.Ref, text) {
		if f.Code == lint.CodeSecretDetected {
			return true
		}
	}
	return false
}

func lockSummary(l catalogLock) string {
	switch {
	case !l.Present:
		return "none"
	case !l.HasContentPins:
		return "no content pins"
	case l.SourcesInSync != nil && *l.SourcesInSync:
		return "sources match"
	default:
		return fmt.Sprintf("%d source(s) differ", l.Changed)
	}
}

func buildCatalog(cfg *config.Config, counter tokens.Counter) (*catalogDoc, error) {
	return govview.BuildCatalog(cfg, counter, Version)
}
