// Package roles builds the roles.json manifest: every role with the items it
// resolves to, their byte sizes and token estimates. It is the stable, versioned
// machine interface an identity tool or a web UI reads to show and assign roles.
// ai-rulez itself never decides who holds a role.
package roles

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/tokens"
)

// FileName is the manifest's file name inside the configuration directory.
const FileName = "roles.json"

// SchemaVersion is bumped on any incompatible change to the manifest shape. The
// JSON schema is schema/roles-manifest.schema.json.
const SchemaVersion = 1

// Item is one item a role keeps (or, in a catalog, any item of the project).
type Item struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Domain string `json:"domain,omitempty"`
	// Path is the source, relative to the configuration directory.
	Path string `json:"path,omitempty"`
	// Mode is the skill_mode the role sets for a skill; empty when it sets none.
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Version string `json:"version,omitempty"`
	// Bytes is the size of the item's source files on disk (a skill counts its
	// resources).
	Bytes int `json:"bytes"`
	// Tokens estimates the primary file (SKILL.md, the rule body, ...) with the
	// manifest's tokenizer.
	Tokens int `json:"tokens"`
}

// Totals sums a role's items.
type Totals struct {
	Items  int            `json:"items"`
	Bytes  int            `json:"bytes"`
	Tokens int            `json:"tokens"`
	ByKind map[string]int `json:"by_kind"`
}

// Role is one role of the manifest.
type Role struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Extends     string            `json:"extends,omitempty"`
	Match       *config.RoleMatch `json:"match,omitempty"`
	// Domains are the domains the role selects, inherited ones included.
	Domains []string `json:"domains"`
	// SkillModes maps a skill id to the skillOverrides state the role sets.
	SkillModes map[string]string `json:"skill_modes,omitempty"`
	Items      []Item            `json:"items"`
	Totals     Totals            `json:"totals"`
}

// Manifest is the content of roles.json.
type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Tokenizer     string `json:"tokenizer"`
	Roles         []Role `json:"roles"`
}

// Build resolves every role of cfg. Roles are sorted by name, items by kind,
// domain and id, so the output is byte-stable.
func Build(cfg *config.Config, counter tokens.Counter) *Manifest {
	m := &Manifest{SchemaVersion: SchemaVersion, Tokenizer: counter.Name(), Roles: []Role{}}
	for _, name := range cfg.RoleNames() {
		role, err := BuildRole(cfg, name, counter)
		if err != nil {
			// A role with broken inheritance is reported by `validate --strict`
			// (AR972); it must not stop the others from being published.
			logger.Warn("Left a role out of the manifest", "role", name, "error", err)
			continue
		}
		m.Roles = append(m.Roles, *role)
	}
	return m
}

// BuildRole resolves one role.
func BuildRole(cfg *config.Config, name string, counter tokens.Counter) (*Role, error) {
	res, err := cfg.ResolveRole(name)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	role := &Role{
		Name: res.Name, Description: res.Description, Extends: res.Extends, Match: res.Match,
		Domains: append([]string{}, res.Domains...), SkillModes: res.SkillOverrides,
		Items:  []Item{},
		Totals: Totals{ByKind: map[string]int{}},
	}
	for i := range res.Items {
		item := ItemOf(&res.Items[i], counter)
		role.Items = append(role.Items, item)
		role.Totals.Items++
		role.Totals.Bytes += item.Bytes
		role.Totals.Tokens += item.Tokens
		role.Totals.ByKind[item.Kind]++
	}
	return role, nil
}

// ItemOf measures one item.
func ItemOf(it *config.RoleItem, counter tokens.Counter) Item {
	out := Item{Kind: it.Kind, ID: it.ID, Domain: it.Domain, Path: it.Path, Mode: it.Mode}
	cf := it.File
	if cf == nil {
		return out
	}
	if cf.Metadata != nil {
		out.Owner = strings.TrimSpace(cf.Metadata.Extra["owner"])
		out.Version = strings.TrimSpace(cf.Metadata.Extra["version"])
	}
	text := cf.Content
	if data, err := os.ReadFile(cf.Path); err == nil {
		text = string(data)
		out.Bytes = len(data)
	} else {
		out.Bytes = len(cf.Content)
	}
	for _, res := range cf.Resources {
		out.Bytes += len(res.Content)
	}
	out.Tokens = counter.Count(text)
	return out
}

// Marshal renders the manifest as indented JSON with a trailing newline.
func (m *Manifest) Marshal() ([]byte, error) {
	sorted := *m
	sorted.Roles = append([]Role(nil), m.Roles...)
	sort.SliceStable(sorted.Roles, func(i, j int) bool { return sorted.Roles[i].Name < sorted.Roles[j].Name })
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode roles manifest")
	}
	return append(data, '\n'), nil
}
