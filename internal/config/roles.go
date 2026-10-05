package config

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Roles map a person's job to the slice of the shared content they need: which
// domains, which skills, rules, agents and commands, and how Claude Code should
// surface each skill. ai-rulez never decides who holds a role. It has no notion
// of identity, network or authentication: an external tool (an identity-provider
// integration, a web UI) picks a role name and asks ai-rulez to generate it. The
// `match` hints are inert strings that tool may read from `roles.json`.

// Item kinds roles select on, singular, as used in resolved output and the lock.
const (
	RoleKindRule    = "rule"
	RoleKindSkill   = "skill"
	RoleKindAgent   = "agent"
	RoleKindCommand = "command"
	RoleKindCheck   = "check"
)

// RoleKinds lists the kinds a role selects, in output order.
var RoleKinds = []string{RoleKindRule, RoleKindSkill, RoleKindAgent, RoleKindCommand, RoleKindCheck}

// Problem kinds reported by RoleProblems; the lint package maps them to AR codes.
const (
	RoleProblemReference   = "reference-unknown"
	RoleProblemExtends     = "extends-invalid"
	RoleProblemUnreachable = "unreachable-dependency"
)

var roleNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// RoleSelector include/exclude lists for one item kind. Entries are item ids
// (the rule, skill, agent or command name) or path.Match globs over the id. An
// entry containing a slash is matched against "<domain>/<id>". An empty Include
// means every item the role's domains provide.
type RoleSelector struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
}

// RoleMatch holds hints for an external tool that maps people to roles. ai-rulez
// stores and republishes them and never interprets them.
type RoleMatch struct {
	Groups []string `yaml:"groups,omitempty" json:"groups,omitempty" toml:"groups,omitempty"`
}

// RoleConfig is one [[roles]] entry.
type RoleConfig struct {
	Name        string        `yaml:"name" json:"name" toml:"name"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Extends     string        `yaml:"extends,omitempty" json:"extends,omitempty" toml:"extends,omitempty"`
	Domains     []string      `yaml:"domains,omitempty" json:"domains,omitempty" toml:"domains,omitempty"`
	Skills      *RoleSelector `yaml:"skills,omitempty" json:"skills,omitempty" toml:"skills,omitempty"`
	Rules       *RoleSelector `yaml:"rules,omitempty" json:"rules,omitempty" toml:"rules,omitempty"`
	Agents      *RoleSelector `yaml:"agents,omitempty" json:"agents,omitempty" toml:"agents,omitempty"`
	Commands    *RoleSelector `yaml:"commands,omitempty" json:"commands,omitempty" toml:"commands,omitempty"`
	Checks      *RoleSelector `yaml:"checks,omitempty" json:"checks,omitempty" toml:"checks,omitempty"`
	// SkillMode maps a skill id or glob to on, name-only, user-invocable-only or
	// off, Claude Code's skillOverrides values.
	SkillMode map[string]string `yaml:"skill_mode,omitempty" json:"skill_mode,omitempty" toml:"skill_mode,omitempty"` //nolint:tagliatelle
	// Delivery maps a skill id or glob ("domain/id" for one domain) to static,
	// served or both: how a skill reaches this role's agent. It sits between a
	// skill's own `delivery` frontmatter and the domain and global defaults (see
	// EffectiveDelivery) and is inherited through extends like SkillMode.
	Delivery map[string]string `yaml:"delivery,omitempty" json:"delivery,omitempty" toml:"delivery,omitempty"`
	Match    *RoleMatch        `yaml:"match,omitempty" json:"match,omitempty" toml:"match,omitempty"`
}

// RoleManifestConfig is the [role_manifest] table. The role list itself is the
// [[roles]] array, so the manifest switch cannot live in a [roles] table.
type RoleManifestConfig struct {
	// Enabled writes <config dir>/roles.json on generate.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
}

// RoleManifestEnabled reports whether generate writes roles.json.
func (c *Config) RoleManifestEnabled() bool {
	return c != nil && c.RoleManifest != nil && c.RoleManifest.Enabled && len(c.Roles) > 0
}

func (s *RoleSelector) lists() (include, exclude []string) {
	if s == nil {
		return nil, nil
	}
	return s.Include, s.Exclude
}

func (r *RoleConfig) selector(kind string) *RoleSelector {
	switch kind {
	case RoleKindRule:
		return r.Rules
	case RoleKindSkill:
		return r.Skills
	case RoleKindAgent:
		return r.Agents
	case RoleKindCommand:
		return r.Commands
	case RoleKindCheck:
		return r.Checks
	}
	return nil
}

// FindRole returns the role declared under name.
func (c *Config) FindRole(name string) (*RoleConfig, bool) {
	for i := range c.Roles {
		if c.Roles[i].Name == name {
			return &c.Roles[i], true
		}
	}
	return nil, false
}

// RoleNames returns the declared role names, sorted.
func (c *Config) RoleNames() []string {
	names := make([]string, 0, len(c.Roles))
	for i := range c.Roles {
		names = append(names, c.Roles[i].Name)
	}
	sort.Strings(names)
	return names
}

// validateRoles checks what can be judged without the content tree: names,
// duplicates and skill_mode values. Problems that need the content (unknown
// references, unreachable dependencies) and inheritance problems are reported by
// RoleProblems, which `validate --strict` surfaces as AR971 to AR973.
func (c *Config) validateRoles() error {
	seen := map[string]bool{}
	modes := SkillOverrideValues()
	for i := range c.Roles {
		r := &c.Roles[i]
		if !roleNamePattern.MatchString(r.Name) {
			return oops.With("field", "roles").With("role", r.Name).
				Hint("Use lowercase letters, digits, '-' and '_', starting with a letter or digit").
				Errorf("invalid role name %q", r.Name)
		}
		if seen[r.Name] {
			return oops.With("field", "roles").With("role", r.Name).
				Hint("Role names are what --role selects; rename or merge one of the entries").
				Errorf("duplicate role name %q", r.Name)
		}
		seen[r.Name] = true
		for pattern, mode := range r.SkillMode {
			if !containsString(modes, mode) {
				return oops.With("field", "roles").With("role", r.Name).With("skill", pattern).
					Hint("Valid skill_mode values: "+strings.Join(modes, ", ")).
					Errorf("role %q skill_mode %q is %q, not a Claude Code skillOverrides value", r.Name, pattern, mode)
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return oops.With("field", "roles").With("role", r.Name).
					Errorf("role %q skill_mode key %q is not a valid glob: %v", r.Name, pattern, err)
			}
		}
		if err := validateRoleDelivery(r); err != nil {
			return err
		}
		if err := validateRoleGlobs(r); err != nil {
			return err
		}
	}
	for _, p := range c.roleExtendsProblems() {
		logger.Warn("Role inheritance problem; `ai-rulez validate --strict` reports it as AR972", "role", p.Role, "problem", p.Message)
	}
	return nil
}

func validateRoleDelivery(r *RoleConfig) error {
	for pattern, value := range r.Delivery {
		if _, ok := ParseDelivery(value); !ok {
			return oops.With("field", "roles").With("role", r.Name).With("skill", pattern).
				Hint("Valid delivery values: static, served, both").
				Errorf("role %q delivery %q is %q, not static, served or both", r.Name, pattern, value)
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return oops.With("field", "roles").With("role", r.Name).
				Errorf("role %q delivery key %q is not a valid glob: %v", r.Name, pattern, err)
		}
	}
	return nil
}

func validateRoleGlobs(r *RoleConfig) error {
	for _, kind := range RoleKinds {
		include, exclude := r.selector(kind).lists()
		for _, pattern := range append(append([]string(nil), include...), exclude...) {
			if strings.TrimSpace(pattern) == "" {
				return oops.With("field", "roles").With("role", r.Name).
					Errorf("role %q has an empty %s selector entry", r.Name, kind)
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return oops.With("field", "roles").With("role", r.Name).
					Errorf("role %q %s selector %q is not a valid glob: %v", r.Name, kind, pattern, err)
			}
		}
	}
	return nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// RoleProblem is a role defect found against the loaded configuration.
type RoleProblem struct {
	// Kind is one of the RoleProblem* constants.
	Kind string `json:"kind"`
	Role string `json:"role"`
	// Message is a complete sentence naming the role.
	Message string `json:"message"`
}

// FlatRole is a role with its parent's settings merged in.
type FlatRole struct {
	RoleConfig
	// Chain is the role followed by its parent, when it has one.
	Chain []string
}

// roleParent walks one role's extends chain. It reports a cycle, an unknown
// parent, or a chain longer than one level as an error.
func (c *Config) roleParent(r *RoleConfig) (*RoleConfig, error) {
	if r.Extends == "" {
		return nil, nil //nolint:nilnil // no parent is not an error
	}
	if r.Extends == r.Name {
		return nil, fmt.Errorf("role %q extends itself", r.Name)
	}
	parent, ok := c.FindRole(r.Extends)
	if !ok {
		return nil, fmt.Errorf("role %q extends %q, which is not defined", r.Name, r.Extends)
	}
	visited := map[string]bool{r.Name: true}
	for cur := parent; cur != nil; {
		if visited[cur.Name] {
			return nil, fmt.Errorf("role %q is part of an extends cycle (%s)", r.Name, cycleText(c, r))
		}
		visited[cur.Name] = true
		if cur.Extends == "" {
			break
		}
		next, found := c.FindRole(cur.Extends)
		if !found {
			return nil, fmt.Errorf("role %q extends %q, which is not defined", cur.Name, cur.Extends)
		}
		cur = next
	}
	if parent.Extends != "" {
		return nil, fmt.Errorf("role %q extends %q which itself extends %q; inheritance is one level deep", r.Name, parent.Name, parent.Extends)
	}
	return parent, nil
}

func cycleText(c *Config, start *RoleConfig) string {
	parts := []string{start.Name}
	seen := map[string]bool{start.Name: true}
	for cur := start; cur.Extends != ""; {
		next, ok := c.FindRole(cur.Extends)
		if !ok {
			break
		}
		parts = append(parts, next.Name)
		if seen[next.Name] {
			break
		}
		seen[next.Name] = true
		cur = next
	}
	return strings.Join(parts, " -> ")
}

func (c *Config) roleExtendsProblems() []RoleProblem {
	var out []RoleProblem
	for i := range c.Roles {
		if _, err := c.roleParent(&c.Roles[i]); err != nil {
			out = append(out, RoleProblem{Kind: RoleProblemExtends, Role: c.Roles[i].Name, Message: err.Error()})
		}
	}
	return out
}

// FlattenRole merges a role with its parent. Merge rules:
//
//   - domains and every exclude list are the union, parent first;
//   - include lists are the union when both sides set one, otherwise whichever
//     side sets it (an empty include means "everything the domains provide");
//   - skill_mode and delivery entries are merged and the child wins per key;
//   - description falls back to the parent's;
//   - match hints are never inherited: they identify who holds this role.
func (c *Config) FlattenRole(name string) (*FlatRole, error) {
	role, ok := c.FindRole(name)
	if !ok {
		return nil, oops.With("role", name).With("available", c.RoleNames()).
			Hint("List the declared roles with `ai-rulez roles list`").
			Errorf("role %q is not defined", name)
	}
	parent, err := c.roleParent(role)
	if err != nil {
		return nil, oops.With("role", name).Wrap(err)
	}
	flat := &FlatRole{RoleConfig: *role, Chain: []string{role.Name}}
	flat.Extends = role.Extends
	if parent == nil {
		return flat, nil
	}
	flat.Chain = append(flat.Chain, parent.Name)
	flat.Domains = unionStrings(parent.Domains, role.Domains)
	if flat.Description == "" {
		flat.Description = parent.Description
	}
	for _, kind := range RoleKinds {
		pi, pe := parent.selector(kind).lists()
		ci, ce := role.selector(kind).lists()
		include := unionStrings(pi, ci)
		exclude := unionStrings(pe, ce)
		var sel *RoleSelector
		if len(include)+len(exclude) > 0 {
			sel = &RoleSelector{Include: include, Exclude: exclude}
		}
		flat.setSelector(kind, sel)
	}
	if len(parent.SkillMode)+len(role.SkillMode) > 0 {
		flat.SkillMode = map[string]string{}
		for k, v := range parent.SkillMode {
			flat.SkillMode[k] = v
		}
		for k, v := range role.SkillMode {
			flat.SkillMode[k] = v
		}
	}
	if len(parent.Delivery)+len(role.Delivery) > 0 {
		flat.Delivery = map[string]string{}
		for k, v := range parent.Delivery {
			flat.Delivery[k] = v
		}
		for k, v := range role.Delivery {
			flat.Delivery[k] = v
		}
	}
	return flat, nil
}

func (r *RoleConfig) setSelector(kind string, s *RoleSelector) {
	switch kind {
	case RoleKindRule:
		r.Rules = s
	case RoleKindSkill:
		r.Skills = s
	case RoleKindAgent:
		r.Agents = s
	case RoleKindCommand:
		r.Commands = s
	case RoleKindCheck:
		r.Checks = s
	}
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// matchRoleItem reports whether pattern selects the item. A pattern with a slash
// is compared with "<domain>/<id>"; others with the id alone.
func matchRoleItem(pattern, domain, id string) bool {
	target := id
	if strings.Contains(pattern, "/") {
		if domain == "" {
			return false
		}
		target = domain + "/" + id
	}
	ok, err := path.Match(pattern, target)
	return err == nil && ok
}

func matchesAny(patterns []string, domain, id string) bool {
	for _, p := range patterns {
		if matchRoleItem(p, domain, id) {
			return true
		}
	}
	return false
}

// selected reports whether a flat role keeps the item.
func (r *RoleConfig) selected(kind, domain, id string) bool {
	include, exclude := r.selector(kind).lists()
	if len(include) > 0 && !matchesAny(include, domain, id) {
		return false
	}
	return !matchesAny(exclude, domain, id)
}

// SkillModeFor returns the explicit skill_mode of a skill: an exact id wins over
// a glob, a longer glob over a shorter one, and ties go to the lexically first
// pattern, so the answer never depends on map order. The bool is false when the
// role does not mention the skill.
func (r *RoleConfig) SkillModeFor(domain, id string) (string, bool) {
	return bestMatch(r.SkillMode, domain, id)
}

// bestMatch picks the entry of a selector map that applies to an item.
func bestMatch(entries map[string]string, domain, id string) (string, bool) {
	best, bestRank := "", -1
	patterns := make([]string, 0, len(entries))
	for p := range entries {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	for _, p := range patterns {
		if !matchRoleItem(p, domain, id) {
			continue
		}
		rank := len(p)
		if p == id || p == domain+"/"+id {
			rank += 1 << 20
		}
		if rank > bestRank {
			best, bestRank = p, rank
		}
	}
	if bestRank < 0 {
		return "", false
	}
	return entries[best], true
}

// Keeps reports whether a flat role keeps the item of the given kind. domain is
// empty for root content.
func (r *RoleConfig) Keeps(kind, domain, id string) bool { return r.selected(kind, domain, id) }

// DeliveryFor returns the delivery the role sets for a skill, with the same
// specificity rule as SkillModeFor. The bool is false when it sets none.
func (r *RoleConfig) DeliveryFor(domain, id string) (Delivery, bool) {
	mode, ok := bestMatch(r.Delivery, domain, id)
	if !ok {
		return "", false
	}
	d, valid := ParseDelivery(mode)
	return d, valid
}

// RoleItem is one item a role keeps.
type RoleItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Domain string `json:"domain,omitempty"`
	// Mode is the skill_mode of a skill the role sets explicitly.
	Mode string `json:"mode,omitempty"`
	// Delivery is how a kept skill reaches the role's agent (static, served or
	// both), after the role's delivery, the skill's own and the defaults.
	Delivery string `json:"delivery,omitempty"`
	// Path is the source file, relative to the configuration directory when it lies below it.
	Path string `json:"path,omitempty"`

	File *ContentFile `json:"-"`
}

// ResolvedRole is the effective item set of a role.
type ResolvedRole struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Extends     string     `json:"extends,omitempty"`
	Match       *RoleMatch `json:"match,omitempty"`
	// Domains are the domain ids the role selects (own and inherited), in declaration order.
	Domains []string `json:"domains"`
	// Items are the kept items, sorted by kind, then domain, then id.
	Items []RoleItem `json:"items"`
	// Context lists the context files that stay, as they are not role-selectable.
	SkillOverrides map[string]string `json:"skill_overrides,omitempty"`
	// DeliveryOverride is the per-skill delivery the role sets (see RoleDeliveryOverride).
	DeliveryOverride map[string]string `json:"-"`

	flat FlatRole
}

// SkillOverrideMap returns skillOverrides.<skill> entries for the explicit
// skill_mode values of kept skills, keyed by skill id.
func (r *ResolvedRole) SkillOverrideMap() map[string]string { return r.SkillOverrides }

// Flat returns the merged role configuration.
func (r *ResolvedRole) Flat() *RoleConfig { return &r.flat.RoleConfig }

// ItemsOf returns the kept items of one kind.
func (r *ResolvedRole) ItemsOf(kind string) []RoleItem {
	var out []RoleItem
	for i := range r.Items {
		if r.Items[i].Kind == kind {
			out = append(out, r.Items[i])
		}
	}
	return out
}

// SelectContentForDomains applies a domain list to a content tree the way a
// profile does: root content, globally active builtins, every included domain and
// the listed domains.
func (c *Config) SelectContentForDomains(content *ContentTree, domains []string) (*ContentTree, error) {
	return c.selectContent(content, domains, "")
}

// FilterTreeForRole returns the tree a role keeps: the role's domains plus the
// always-active ones, with each kind narrowed by the role's selectors. Context
// files are not role-selectable and are kept as they are.
func (c *Config) FilterTreeForRole(content *ContentTree, flat *RoleConfig) (*ContentTree, error) {
	selected, err := c.SelectContentForDomains(content, flat.Domains)
	if err != nil {
		return nil, err
	}
	out := &ContentTree{
		Rules:    filterRoleFiles(flat, RoleKindRule, "", selected.Rules),
		Context:  selected.Context,
		Skills:   filterRoleFiles(flat, RoleKindSkill, "", selected.Skills),
		Agents:   filterRoleFiles(flat, RoleKindAgent, "", selected.Agents),
		Commands: filterRoleFiles(flat, RoleKindCommand, "", selected.Commands),
		Checks:   filterRoleFiles(flat, RoleKindCheck, "", selected.Checks),
		Domains:  make(map[string]*Domain, len(selected.Domains)),
	}
	for name, d := range selected.Domains {
		nd := *d
		nd.Rules = filterRoleFiles(flat, RoleKindRule, name, d.Rules)
		nd.Skills = filterRoleFiles(flat, RoleKindSkill, name, d.Skills)
		nd.Agents = filterRoleFiles(flat, RoleKindAgent, name, d.Agents)
		nd.Commands = filterRoleFiles(flat, RoleKindCommand, name, d.Commands)
		nd.Checks = filterRoleFiles(flat, RoleKindCheck, name, d.Checks)
		out.Domains[name] = &nd
	}
	return out, nil
}

func filterRoleFiles(flat *RoleConfig, kind, domain string, files []ContentFile) []ContentFile {
	if len(files) == 0 {
		return files
	}
	out := make([]ContentFile, 0, len(files))
	for i := range files {
		if flat.selected(kind, domain, roleItemID(kind, files[i])) {
			out = append(out, files[i])
		}
	}
	return out
}

// ResolveRole computes the effective item set of a role over the loaded content.
func (c *Config) ResolveRole(name string) (*ResolvedRole, error) {
	flat, err := c.FlattenRole(name)
	if err != nil {
		return nil, err
	}
	if c.Content == nil {
		return nil, ErrNoContent
	}
	tree, err := c.FilterTreeForRole(c.Content, &flat.RoleConfig)
	if err != nil {
		return nil, err
	}
	res := &ResolvedRole{
		Name: flat.Name, Description: flat.Description, Extends: flat.Extends, Match: role(c, name).Match,
		Domains: append([]string{}, flat.Domains...), flat: *flat,
	}
	res.DeliveryOverride = orEmpty(c.RoleDeliveryOverride(&flat.RoleConfig))
	add := func(kind, domain string, files []ContentFile) {
		for i := range files {
			item := RoleItem{Kind: kind, ID: roleItemID(kind, files[i]), Domain: domain, Path: c.relToConfigDir(files[i].Path), File: &files[i]}
			if kind == RoleKindSkill {
				item.Delivery = string(c.EffectiveDelivery(files[i], domain, res.DeliveryOverride))
				if mode, ok := flat.SkillModeFor(domain, roleItemID(kind, files[i])); ok {
					item.Mode = mode
					if res.SkillOverrides == nil {
						res.SkillOverrides = map[string]string{}
					}
					res.SkillOverrides[item.ID] = mode
				}
			}
			res.Items = append(res.Items, item)
		}
	}
	for _, kind := range RoleKinds {
		add(kind, "", treeFiles(tree, kind))
		for _, dn := range sortedKeys(tree.Domains) {
			add(kind, dn, treeFiles(&ContentTree{Rules: tree.Domains[dn].Rules, Skills: tree.Domains[dn].Skills,
				Agents: tree.Domains[dn].Agents, Commands: tree.Domains[dn].Commands, Checks: tree.Domains[dn].Checks}, kind))
		}
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		a, b := res.Items[i], res.Items[j]
		if a.Kind != b.Kind {
			return kindOrder(a.Kind) < kindOrder(b.Kind)
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		return a.ID < b.ID
	})
	return res, nil
}

func role(c *Config, name string) *RoleConfig {
	r, _ := c.FindRole(name)
	if r == nil {
		return &RoleConfig{}
	}
	return r
}

func kindOrder(kind string) int {
	for i, k := range RoleKinds {
		if k == kind {
			return i
		}
	}
	return len(RoleKinds)
}

func treeFiles(t *ContentTree, kind string) []ContentFile {
	switch kind {
	case RoleKindRule:
		return t.Rules
	case RoleKindSkill:
		return t.Skills
	case RoleKindAgent:
		return t.Agents
	case RoleKindCommand:
		return t.Commands
	case RoleKindCheck:
		return t.Checks
	}
	return nil
}

func (c *Config) relToConfigDir(p string) string {
	if p == "" || c.ConfigDir == "" {
		return filepath.ToSlash(p)
	}
	if rel, err := filepath.Rel(c.ConfigDir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

// AllItems lists every rule, skill, agent, command and check of the full content tree
// (all domains), sorted by kind, domain and id.
func (c *Config) AllItems() []RoleItem {
	if c.Content == nil {
		return nil
	}
	var out []RoleItem
	add := func(kind, domain string, files []ContentFile) {
		for i := range files {
			out = append(out, RoleItem{Kind: kind, ID: roleItemID(kind, files[i]), Domain: domain, Path: c.relToConfigDir(files[i].Path), File: &files[i]})
		}
	}
	for _, kind := range RoleKinds {
		add(kind, "", treeFiles(c.Content, kind))
		for _, dn := range sortedKeys(c.Content.Domains) {
			d := c.Content.Domains[dn]
			add(kind, dn, treeFiles(&ContentTree{Rules: d.Rules, Skills: d.Skills, Agents: d.Agents, Commands: d.Commands, Checks: d.Checks}, kind))
		}
	}
	return out
}

// RoleProblems checks every role against the loaded content: references that
// name nothing, broken inheritance, and skills a kept item depends on that the
// role leaves out.
func (c *Config) RoleProblems() []RoleProblem {
	problems := c.roleExtendsProblems()
	if c.Content == nil {
		return sortProblems(problems)
	}
	all := c.AllItems()
	for i := range c.Roles {
		name := c.Roles[i].Name
		flat, err := c.FlattenRole(name)
		if err != nil {
			continue // reported above
		}
		problems = append(problems, c.roleReferenceProblems(flat, all)...)
		res, rerr := c.ResolveRole(name)
		if rerr != nil {
			continue
		}
		problems = append(problems, c.roleReachabilityProblems(res, all)...)
		problems = append(problems, roleSkillModeCollisions(res)...)
	}
	return sortProblems(problems)
}

// roleSkillModeCollisions reports kept skills that share an id across domains but
// get different skill_mode values. Claude Code's skillOverrides is one flat map
// keyed by skill id, so only one of them could win; the role must make the modes
// agree (or exclude one of the skills).
func roleSkillModeCollisions(res *ResolvedRole) []RoleProblem {
	type seenMode struct {
		mode    string
		domains []string
	}
	byID := map[string][]seenMode{}
	for _, item := range res.ItemsOf(RoleKindSkill) {
		label := item.Domain
		if label == "" {
			label = "(root)"
		}
		found := false
		for i := range byID[item.ID] {
			if byID[item.ID][i].mode == item.Mode {
				byID[item.ID][i].domains = append(byID[item.ID][i].domains, label)
				found = true
			}
		}
		if !found {
			byID[item.ID] = append(byID[item.ID], seenMode{mode: item.Mode, domains: []string{label}})
		}
	}
	var out []RoleProblem
	for _, id := range sortedKeys(byID) {
		modes := byID[id]
		if len(modes) < 2 {
			continue
		}
		var parts []string
		for _, m := range modes {
			mode := m.mode
			if mode == "" {
				mode = "(no skill_mode)"
			}
			parts = append(parts, fmt.Sprintf("%s in %s", mode, strings.Join(m.domains, ", ")))
		}
		out = append(out, RoleProblem{Kind: RoleProblemReference, Role: res.Name, Message: fmt.Sprintf(
			"role %q keeps several skills with id %q that resolve to different skill_mode values (%s); skillOverrides is keyed by skill id, so only one would apply. Use one mode for all of them or exclude one of the skills",
			res.Name, id, strings.Join(parts, "; "))})
	}
	return out
}

func sortProblems(p []RoleProblem) []RoleProblem {
	sort.SliceStable(p, func(i, j int) bool {
		if p[i].Role != p[j].Role {
			return p[i].Role < p[j].Role
		}
		if p[i].Kind != p[j].Kind {
			return p[i].Kind < p[j].Kind
		}
		return p[i].Message < p[j].Message
	})
	return p
}

func (c *Config) roleReferenceProblems(flat *FlatRole, all []RoleItem) []RoleProblem {
	var out []RoleProblem
	add := func(format string, args ...any) {
		out = append(out, RoleProblem{Kind: RoleProblemReference, Role: flat.Name, Message: fmt.Sprintf(format, args...)})
	}
	selectedDomains := map[string]bool{}
	for _, d := range flat.Domains {
		if !c.domainExists(d) {
			add("role %q lists domain %q, which does not exist", flat.Name, d)
		}
		selectedDomains[trimBuiltinRef(d)] = true
	}
	for _, kind := range RoleKinds {
		include, exclude := flat.selector(kind).lists()
		for _, p := range include {
			if msg := c.unmatchedEntry(flat.Name, kind+"s include", kind, p, selectedDomains, all); msg != "" {
				add("%s", msg)
			}
		}
		for _, p := range exclude {
			if msg := c.unmatchedEntry(flat.Name, kind+"s exclude", kind, p, selectedDomains, all); msg != "" {
				add("%s", msg)
			}
		}
	}
	for _, p := range sortedKeys(flat.SkillMode) {
		if msg := c.unmatchedEntry(flat.Name, "skill_mode", RoleKindSkill, p, selectedDomains, all); msg != "" {
			add("%s", msg)
		}
	}
	for _, p := range sortedKeys(flat.Delivery) {
		if msg := c.unmatchedEntry(flat.Name, "delivery", RoleKindSkill, p, selectedDomains, all); msg != "" {
			add("%s", msg)
		}
	}
	return out
}

// unmatchedEntry explains why a selector entry matches nothing the role can see;
// it returns "" when the entry matches an item in root content, an always-active
// domain or a domain the role selects.
func (c *Config) unmatchedEntry(role, field, kind, pattern string, selectedDomains map[string]bool, all []RoleItem) string {
	var elsewhere []string
	for i := range all {
		if all[i].Kind != kind || !matchRoleItem(pattern, all[i].Domain, all[i].ID) {
			continue
		}
		if all[i].Domain == "" || selectedDomains[all[i].Domain] || c.alwaysActiveDomain(all[i].Domain) {
			return ""
		}
		elsewhere = append(elsewhere, all[i].Domain)
	}
	if len(elsewhere) == 0 {
		if kind == RoleKindSkill && len(c.SkillSources) > 0 {
			return "" // the entry may name a skill of a [[skill_sources]] entry, which is only known once the source is fetched
		}
		return fmt.Sprintf("role %q %s entry %q matches no %s", role, field, pattern, kind)
	}
	sort.Strings(elsewhere)
	return fmt.Sprintf("role %q %s entry %q exists only in domain %q, which the role does not select", role, field, pattern, elsewhere[0])
}

func (c *Config) domainExists(ref string) bool {
	if c.Content == nil {
		return true
	}
	_, ok := c.Content.Domains[trimBuiltinRef(ref)]
	return ok
}

func (c *Config) alwaysActiveDomain(name string) bool {
	d, ok := c.Content.Domains[name]
	return ok && (d.FromInclude || (d.Builtin && !d.BuiltinScoped))
}

func trimBuiltinRef(ref string) string { return strings.TrimPrefix(ref, "builtin:") }

// roleReachabilityProblems reports skills that a kept rule, skill or agent names
// in its `skills:` frontmatter but that the role leaves out or turns off.
func (c *Config) roleReachabilityProblems(res *ResolvedRole, all []RoleItem) []RoleProblem {
	kept := map[string]*RoleItem{}
	for i := range res.Items {
		if res.Items[i].Kind == RoleKindSkill {
			kept[res.Items[i].ID] = &res.Items[i]
		}
	}
	exists := map[string]bool{}
	for i := range all {
		if all[i].Kind == RoleKindSkill {
			exists[all[i].ID] = true
		}
	}
	var out []RoleProblem
	for i := range res.Items {
		item := &res.Items[i]
		if item.File == nil || item.File.Metadata == nil || item.Kind == RoleKindCommand {
			continue
		}
		for _, dep := range item.File.Metadata.Skills {
			if !exists[dep] || (dep == item.ID && item.Kind == RoleKindSkill) {
				continue
			}
			if msg := unreachableMessage(res.Name, item, dep, kept[dep]); msg != "" {
				out = append(out, RoleProblem{Kind: RoleProblemUnreachable, Role: res.Name, Message: msg})
			}
		}
	}
	return out
}

// unreachableMessage describes a skill dependency the role cannot satisfy, or "".
func unreachableMessage(role string, item *RoleItem, dep string, kept *RoleItem) string {
	switch {
	case kept == nil:
		return fmt.Sprintf("role %q keeps %s %q, which uses skill %q, but the role does not include that skill", role, item.Kind, item.ID, dep)
	case kept.Mode == SkillModeOff || kept.Mode == SkillModeUserInvocableOnly:
		return fmt.Sprintf("role %q keeps %s %q, which uses skill %q, but skill_mode %q hides it from the model", role, item.Kind, item.ID, dep, kept.Mode)
	}
	return ""
}

func roleItemID(kind string, f ContentFile) string {
	if kind == RoleKindSkill {
		return SkillID(f)
	}
	return f.Name
}

// orEmpty returns a non-nil map, so EffectiveDelivery does not fall back to the
// delivery of the role being rendered when the role sets none.
func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
