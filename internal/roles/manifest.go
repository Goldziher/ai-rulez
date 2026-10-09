// Package roles builds the roles.json manifest: every role with the items it
// resolves to, their byte sizes and token estimates. It is the stable, versioned
// machine interface an identity tool or a web UI reads to show and assign roles.
// ai-rulez itself never decides who holds a role.
package roles

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
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
	// Path is the source, relative to the configuration directory. An item that
	// comes from an include lives outside it and is reported as
	// included/<path inside that include's .ai-rulez>, so the manifest does not
	// depend on a cache or temporary directory of this machine.
	Path string `json:"path,omitempty"`
	// Mode is the skill_mode the role sets for a skill; empty when it sets none.
	Mode string `json:"mode,omitempty"`
	// Delivery is how a skill reaches the agent: static (written to the skill
	// trees), served (fetched over MCP on demand, so never listed) or both. Set
	// for skills only.
	Delivery string `json:"delivery,omitempty"`
	Owner    string `json:"owner,omitempty"`
	Version  string `json:"version,omitempty"`
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
	// Served counts the skills the role receives through the skills server only:
	// they are not listed in the agent's context (see docs/mcp-server.md).
	Served int `json:"served_skills"`
	// ServedTokens is the share of Tokens held by those skills; they cost nothing
	// until the agent calls load_skill.
	ServedTokens int `json:"served_tokens"`
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
	// Delivery is the role's own delivery selectors (inherited ones merged in).
	Delivery map[string]string `json:"delivery,omitempty"`
	Items    []Item            `json:"items"`
	Totals   Totals            `json:"totals"`
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
			// A role with broken inheritance is reported by `validate`
			// (AR972); it must not stop the others from being published.
			cfg.Log().Warn("Left a role out of the manifest", "role", name, "error", err)
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
		Domains: append([]string{}, res.Domains...), SkillModes: res.SkillOverrides, Delivery: res.Flat().Delivery,
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
		if item.Delivery == string(config.DeliveryServed) {
			role.Totals.Served++
			role.Totals.ServedTokens += item.Tokens
		}
	}
	return role, nil
}

// manifestPath keeps a path relative to the configuration directory as it is and
// turns an absolute one (an item from an include, read from a cache or a
// temporary directory) into a location that is the same on every machine.
func manifestPath(p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	slash := filepath.ToSlash(p)
	const marker = "/.ai-rulez/"
	if i := strings.LastIndex(slash, marker); i >= 0 {
		return "included/" + slash[i+len(marker):]
	}
	return "included/" + path.Base(slash)
}

// ItemOf measures one item.
func ItemOf(it *config.RoleItem, counter tokens.Counter) Item {
	out := Item{Kind: it.Kind, ID: it.ID, Domain: it.Domain, Path: manifestPath(it.Path), Mode: it.Mode, Delivery: it.Delivery}
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
	sorted.Roles = append([]Role{}, m.Roles...)
	sort.SliceStable(sorted.Roles, func(i, j int) bool { return sorted.Roles[i].Name < sorted.Roles[j].Name })
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode roles manifest")
	}
	return append(data, '\n'), nil
}
