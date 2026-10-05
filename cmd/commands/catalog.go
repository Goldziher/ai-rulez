package commands

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/contentlock"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/Goldziher/ai-rulez/internal/roles"
	"github.com/Goldziher/ai-rulez/internal/tokens"
)

var catalogFormat string

// catalogSchemaVersion versions the JSON of `ai-rulez catalog --format json`.
const catalogSchemaVersion = 1

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

type catalogItem struct {
	roles.Item
	Digest string   `json:"digest,omitempty"`
	Roles  []string `json:"roles"`
	// RoleDelivery is, for a skill, the delivery each role that keeps it gives it
	// when that differs from the skill's own.
	RoleDelivery map[string]string `json:"role_delivery,omitempty"`
}

type catalogRole struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Extends     string            `json:"extends,omitempty"`
	Match       *config.RoleMatch `json:"match,omitempty"`
	Domains     []string          `json:"domains"`
	Delivery    map[string]string `json:"delivery,omitempty"`
	Totals      roles.Totals      `json:"totals"`
}

type catalogLock struct {
	Present        bool   `json:"present"`
	Version        int    `json:"version,omitempty"`
	HasContentPins bool   `json:"has_content_pins"`
	Enforce        bool   `json:"enforce"`
	Tree           string `json:"tree,omitempty"`
	// SourcesInSync is null when the lock has no content pins to compare with.
	SourcesInSync *bool `json:"sources_in_sync"`
	// Changed counts the sources that differ from the lock.
	Changed int `json:"sources_changed"`
}

type catalogDoc struct {
	SchemaVersion int           `json:"schema_version"`
	Tokenizer     string        `json:"tokenizer"`
	Items         []catalogItem `json:"items"`
	Roles         []catalogRole `json:"roles"`
	Lock          catalogLock   `json:"lock"`
}

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
	doc := &catalogDoc{SchemaVersion: catalogSchemaVersion, Tokenizer: counter.Name(), Items: []catalogItem{}, Roles: []catalogRole{}}

	membership := map[string][]string{}
	roleDelivery := map[string]map[string]string{}
	for _, name := range cfg.RoleNames() {
		role, err := roles.BuildRole(cfg, name, counter)
		if err != nil {
			continue // broken inheritance is reported by `validate --strict` (AR972)
		}
		doc.Roles = append(doc.Roles, catalogRole{Name: role.Name, Description: role.Description, Extends: role.Extends,
			Match: role.Match, Domains: role.Domains, Delivery: role.Delivery, Totals: role.Totals})
		for i := range role.Items {
			it := &role.Items[i]
			key := it.Kind + "\x00" + it.Domain + "\x00" + it.ID
			membership[key] = append(membership[key], role.Name)
			if it.Delivery != "" {
				if roleDelivery[key] == nil {
					roleDelivery[key] = map[string]string{}
				}
				roleDelivery[key][role.Name] = it.Delivery
			}
		}
	}

	snap, err := lockSnapshot(cfg, "", true)
	if err != nil {
		return nil, err
	}
	digests := map[string]string{}
	for _, it := range snap.Items {
		digests[it.Key()] = it.Digest
	}

	for _, item := range allCatalogItems(cfg) {
		if item.Kind == config.RoleKindSkill && item.File != nil {
			item.Delivery = string(cfg.EffectiveDelivery(*item.File, item.Domain, map[string]string{}))
		}
		measured := roles.ItemOf(&item, counter)
		key := item.Kind + "\x00" + item.Domain + "\x00" + item.ID
		members := membership[key]
		if members == nil {
			members = []string{}
		}
		differing := map[string]string{}
		for roleName, d := range roleDelivery[key] {
			if d != item.Delivery {
				differing[roleName] = d
			}
		}
		if len(differing) == 0 {
			differing = nil
		}
		doc.Items = append(doc.Items, catalogItem{Item: measured, Digest: digests[key], Roles: members, RoleDelivery: differing})
	}

	doc.Lock = catalogLock{Enforce: cfg.LockEnforced()}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if lock != nil {
		doc.Lock.Present, doc.Lock.Version = true, lock.Version
		doc.Lock.HasContentPins, doc.Lock.Tree = lock.HasContentPins(), lock.Tree
		if lock.HasContentPins() {
			sourcesOnly, cerr := lockSnapshot(cfg, lock.Profile, true)
			if cerr != nil {
				return nil, cerr
			}
			diff := contentlock.Compare(lock, sourcesOnly)
			inSync := diff.InSync
			doc.Lock.SourcesInSync, doc.Lock.Changed = &inSync, len(diff.Changes)
		}
	}
	return doc, nil
}

// allCatalogItems lists the content of every domain, context files included,
// sorted by kind, domain and id.
func allCatalogItems(cfg *config.Config) []config.RoleItem {
	items := cfg.AllItems()
	if cfg.Content != nil {
		add := func(domain string, files []config.ContentFile) {
			for i := range files {
				items = append(items, config.RoleItem{Kind: contentlock.KindContext, ID: files[i].Name, Domain: domain,
					Path: relToConfig(cfg, files[i].Path), File: &files[i]})
			}
		}
		add("", cfg.Content.Context)
		for name, d := range cfg.Content.Domains {
			add(name, d.Context)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		return a.ID < b.ID
	})
	return items
}

func relToConfig(cfg *config.Config, p string) string {
	if rel, err := filepath.Rel(cfg.ConfigDir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}
