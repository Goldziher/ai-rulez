// Package govview builds the read-only governance documents that both the CLI
// (catalog, roles, lock --check) and the MCP tools print, so the two cannot drift.
package govview

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// CatalogSchemaVersion versions the JSON of `ai-rulez catalog --format json`.
const CatalogSchemaVersion = 1

type CatalogItem struct {
	roles.Item
	Digest string   `json:"digest,omitempty"`
	Roles  []string `json:"roles"`
	// RoleDelivery is, for a skill, the delivery each role that keeps it gives it
	// when that differs from the skill's own.
	RoleDelivery map[string]string `json:"role_delivery,omitempty"`
}

type CatalogRole struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Extends     string            `json:"extends,omitempty"`
	Match       *config.RoleMatch `json:"match,omitempty"`
	Domains     []string          `json:"domains"`
	Delivery    map[string]string `json:"delivery,omitempty"`
	Totals      roles.Totals      `json:"totals"`
}

type CatalogLock struct {
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

type CatalogDoc struct {
	SchemaVersion int           `json:"schema_version"`
	Tokenizer     string        `json:"tokenizer"`
	Items         []CatalogItem `json:"items"`
	Roles         []CatalogRole `json:"roles"`
	Lock          CatalogLock   `json:"lock"`
}

// addCatalogRoles appends every resolvable role to doc and returns, per item,
// the roles that keep it and the delivery each gives it (skills only).
func addCatalogRoles(doc *CatalogDoc, cfg *config.Config, counter tokens.Counter) (membership map[string][]string, roleDelivery map[string]map[string]string) {
	membership, roleDelivery = map[string][]string{}, map[string]map[string]string{}
	for _, name := range cfg.RoleNames() {
		role, err := roles.BuildRole(cfg, name, counter)
		if err != nil {
			continue // broken inheritance is reported by `validate --strict` (AR972)
		}
		doc.Roles = append(doc.Roles, CatalogRole{Name: role.Name, Description: role.Description, Extends: role.Extends,
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
	return membership, roleDelivery
}

func BuildCatalog(cfg *config.Config, counter tokens.Counter, toolVersion string) (*CatalogDoc, error) {
	doc := &CatalogDoc{SchemaVersion: CatalogSchemaVersion, Tokenizer: counter.Name(), Items: []CatalogItem{}, Roles: []CatalogRole{}}

	membership, roleDelivery := addCatalogRoles(doc, cfg, counter)

	snap, err := Snapshot(cfg, "", true, toolVersion)
	if err != nil {
		return nil, err
	}
	digests := NewDigestIndex(snap.Items)

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
		doc.Items = append(doc.Items, CatalogItem{Item: measured, Digest: digests.Lookup(item.Kind, item.Domain, item.ID, item.Path), Roles: members, RoleDelivery: differing})
	}

	lockStatus, err := catalogLockStatus(cfg, toolVersion)
	if err != nil {
		return nil, err
	}
	doc.Lock = lockStatus
	return doc, nil
}

// DigestIndex finds the lock digest of a catalog item. The lock disambiguates a
// second item with the same kind, domain and id as "<id>#2", so an item is looked
// up by its source path first (unique per file) and by its key second.
type DigestIndex struct{ byPath, byKey map[string]string }

func NewDigestIndex(items []lockfile.Item) DigestIndex {
	d := DigestIndex{byPath: map[string]string{}, byKey: map[string]string{}}
	for _, it := range items {
		d.byKey[it.Key()] = it.Digest
		if it.Path != "" {
			d.byPath[it.Kind+"\x00"+it.Domain+"\x00"+it.Path] = it.Digest
		}
	}
	return d
}

func (d DigestIndex) Lookup(kind, domain, id, path string) string {
	if path != "" {
		if digest, ok := d.byPath[kind+"\x00"+domain+"\x00"+path]; ok {
			return digest
		}
	}
	return d.byKey[kind+"\x00"+domain+"\x00"+id]
}

// catalogLockStatus reports whether a lock exists, whether it pins content and
// how many authored sources differ from it.
func catalogLockStatus(cfg *config.Config, toolVersion string) (CatalogLock, error) {
	status := CatalogLock{Enforce: cfg.LockEnforced()}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return status, err //nolint:wrapcheck // already contextual
	}
	if lock == nil {
		return status, nil
	}
	status.Present, status.Version = true, lock.Version
	status.HasContentPins, status.Tree = lock.HasContentPins(), lock.Tree
	if lock.HasContentPins() {
		sourcesOnly, err := Snapshot(cfg, lock.Profile, true, toolVersion)
		if err != nil {
			return status, err
		}
		diff := contentlock.Compare(lock, sourcesOnly)
		inSync := diff.InSync
		status.SourcesInSync, status.Changed = &inSync, len(diff.Changes)
	}
	return status, nil
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

// Bounds of the MCP views; the CLI documents are not limited.
const (
	DefaultLimit = 200
	MaxLimit     = 1000
)

// ClampLimit applies the default and the maximum to a requested limit.
func ClampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultLimit
	case limit > MaxLimit:
		return MaxLimit
	}
	return limit
}

// CatalogView is a catalog narrowed by kind and role and capped at a limit. With
// no filter and fewer items than the limit it serialises exactly as CatalogDoc.
type CatalogView struct {
	*CatalogDoc
	// TotalItems is the number of matching items before the cap; set only when
	// the list was cut.
	TotalItems int  `json:"total_items,omitempty"`
	Truncated  bool `json:"truncated,omitempty"`
}

// ViewCatalog narrows doc to the items of kind and kept by role, and keeps at
// most limit of them (see ClampLimit). The roles list shrinks to role.
func ViewCatalog(doc *CatalogDoc, kind, role string, limit int) (*CatalogView, error) {
	kinds := append(append([]string(nil), config.RoleKinds...), contentlock.KindContext)
	if kind != "" && !slices.Contains(kinds, kind) {
		return nil, oops.Errorf("unknown kind %q (use %s)", kind, strings.Join(kinds, ", "))
	}
	out := *doc
	if role != "" {
		idx := slices.IndexFunc(doc.Roles, func(r CatalogRole) bool { return r.Name == role })
		if idx < 0 {
			return nil, oops.Errorf("role %q is not defined", role)
		}
		out.Roles = []CatalogRole{doc.Roles[idx]}
	}
	if kind != "" || role != "" {
		out.Items = []CatalogItem{}
		for i := range doc.Items {
			it := &doc.Items[i]
			if (kind == "" || it.Kind == kind) && (role == "" || slices.Contains(it.Roles, role)) {
				out.Items = append(out.Items, *it)
			}
		}
	}
	view := &CatalogView{CatalogDoc: &out}
	if limit = ClampLimit(limit); len(out.Items) > limit {
		view.TotalItems, view.Truncated = len(out.Items), true
		out.Items = out.Items[:limit]
	}
	return view, nil
}
