package commands

import (
	"context"
	"fmt"
	"io"
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
	catalogTitle         string
	catalogAllowFindings []string
	// catalogExcerptSet records that --include-excerpt was given, so the
	// default (on, except for --indexable) is not mistaken for a choice.
	catalogExcerptSet bool
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
default and off with --indexable; pass --include-excerpt=false to leave them out.`,
	Example: `  ai-rulez catalog --format json --schema-version 2
  ai-rulez catalog --html site/
  ai-rulez catalog --html site/ --role backend --clean`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		catalogExcerptSet = cmd.Flags().Changed("include-excerpt")
		exitOn(runCatalog(cmd.OutOrStdout()))
	},
}

func init() {
	addFormatFlag(CatalogCmd.Flags(), &catalogFormat, "", formatText, formatText, formatJSON)
	addJSONFlagAlias(CatalogCmd.Flags())
	CatalogCmd.Flags().IntVar(&catalogSchemaFlag, "schema-version", govview.CatalogSchemaVersion, "JSON schema version: 1 (default) or 2")
	CatalogCmd.Flags().StringVar(&catalogHTMLDir, "html", "", "Write a static website of the catalog into this directory")
	CatalogCmd.Flags().StringVar(&catalogRole, "role", "", "With --html: keep only the items this role keeps")
	CatalogCmd.Flags().BoolVar(&catalogExcerpt, "include-excerpt", true, "Include a body excerpt of each item (version 2 JSON and --html; off by default with --indexable)")
	CatalogCmd.Flags().BoolVar(&catalogIndexable, "indexable", false, "With --html: let search engines index the site (no robots.txt, no noindex)")
	CatalogCmd.Flags().BoolVar(&catalogClean, "clean", false, "With --html: remove files a previous run wrote that the site no longer has")
	CatalogCmd.Flags().StringVar(&catalogTitle, "base-title", "", "With --html: site title (default: AI-Rulez catalog)")
	CatalogCmd.Flags().StringSliceVar(&catalogAllowFindings, "allow-findings", nil, "With --html: publish despite findings of these codes (AR001: a secret in an item)")
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
	switch {
	case catalogHTMLDir != "" && catalogFormat != "":
		return oops.Errorf("--html writes a website; drop --format")
	case catalogHTMLDir == "" && (catalogRole != "" || catalogClean || catalogIndexable || catalogTitle != "" || len(catalogAllowFindings) > 0):
		return oops.Errorf("--role, --clean, --indexable, --base-title and --allow-findings apply to --html only")
	case catalogHTMLDir == "" && catalogFormat != formatJSON && catalogSchemaFlag != govview.CatalogSchemaVersion:
		return oops.Errorf("--schema-version applies to --format json")
	}
	return nil
}

func runCatalog(out io.Writer) error {
	if err := checkCatalogFlags(); err != nil {
		return err
	}
	cfg, err := loadConfigForCommand(context.Background(), nil)
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
	if catalogHTMLDir != "" {
		return runCatalogHTML(out, cfg, counter)
	}
	if catalogFormat == formatJSON && catalogSchemaFlag == govview.CatalogSchemaVersionV2 {
		doc, err := buildCatalogV2(cfg, counter, catalogExcerpt)
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

// buildCatalogV2 builds the version 2 catalog, linting the project in process
// (the same engine as `validate --strict`). A lint that cannot run leaves the
// lint fields out and says so; it never fails the catalog.
func buildCatalogV2(cfg *config.Config, counter tokens.Counter, excerpt bool) (*govview.CatalogDocV2, error) {
	opts := govview.CatalogOptions{NoExcerpt: !excerpt}
	report, err := strictLint(cfg)
	if err != nil {
		logger.Warn("Catalog lint did not run", "error", err)
		opts.LintReason = "the lint engine could not run"
	} else {
		opts.Lint = report
	}
	return govview.BuildCatalogV2(cfg, counter, Version, opts)
}

func runCatalogHTML(out io.Writer, cfg *config.Config, counter tokens.Counter) error {
	excerpt := catalogExcerpt
	if !catalogExcerptSet {
		excerpt = !catalogIndexable
	}
	doc, err := buildCatalogV2(cfg, counter, excerpt)
	if err != nil {
		return err
	}
	if err := refuseSecrets(doc, catalogAllowFindings); err != nil {
		return err
	}
	if doc, err = govview.ViewCatalogV2(doc, catalogRole); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	site, err := catalogsite.Render(doc, catalogsite.Options{Title: catalogTitle, Indexable: catalogIndexable})
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	res, err := catalogsite.Write(catalogHTMLDir, site, catalogClean)
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

// refuseSecrets stops a site that would publish an item the secret scanner
// flagged (AR001), unless the code is allowed.
func refuseSecrets(doc *govview.CatalogDocV2, allow []string) error {
	if slices.Contains(allow, lint.CodeSecretDetected) {
		return nil
	}
	var flagged []string
	for i := range doc.Items {
		it := &doc.Items[i]
		if it.Lint == nil {
			continue
		}
		for _, f := range it.Lint.Findings {
			if f.Code == lint.CodeSecretDetected {
				flagged = append(flagged, it.Ref)
				break
			}
		}
	}
	if len(flagged) == 0 {
		return nil
	}
	return oops.Hint("remove the secret, or pass --allow-findings AR001 to publish anyway (discouraged)").
		Errorf("refusing to publish: the secret scanner (%s) flagged %s", lint.CodeSecretDetected, strings.Join(flagged, ", "))
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
