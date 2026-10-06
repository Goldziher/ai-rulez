package commands

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

var catalogFormat string

// catalogSchemaVersion versions the JSON of `ai-rulez catalog --format json`.
const catalogSchemaVersion = govview.CatalogSchemaVersion

// CatalogCmd prints every authored item with its owner, version, size, digest,
// the roles that keep it and the lock status, for a UI or an audit script.
var CatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List every item with owner, version, tokens, roles and lock status",
	Long: `Print the project's catalog: every rule, skill, agent, command and context file
with its id, domain, source, owner and version, size, sha256 digest (the one
ai-rulez.lock pins), the roles that keep it, plus a summary of every role and the
lock status. Nothing is written and nothing is rendered.

The JSON (--format json) is versioned by schema_version and is meant for a web UI
or an audit script; see docs/roles.md ("Integrating an identity tool or UI").`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		exitOn(runCatalog(cmd.OutOrStdout()))
	},
}

func init() {
	CatalogCmd.Flags().StringVar(&catalogFormat, "format", "", "Output format: text (default) or json")
	CatalogCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	CatalogCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// The catalog documents are built by internal/govview, which the MCP tools share.
type (
	catalogItem = govview.CatalogItem
	catalogDoc  = govview.CatalogDoc
	catalogLock = govview.CatalogLock
)

func runCatalog(out io.Writer) error {
	if catalogFormat != "" && catalogFormat != formatText && catalogFormat != formatJSON {
		return oops.Errorf("unknown --format %q (use text or json)", catalogFormat)
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
