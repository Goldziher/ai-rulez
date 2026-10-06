package config

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// DynamicSkillsName is the name of the single generated stub skill that tells
// the agent to call find_skill / load_skill when any skill is served.
const DynamicSkillsName = "dynamic-skills"

// keyFrontmatterDescription is the frontmatter key of a description.
const keyFrontmatterDescription = "description"

const dynamicSkillsPath = "generated://" + DynamicSkillsName + "/SKILL.md"

const dynamicSkillsDescription = "Find and load more skills on demand from the ai-rulez MCP server. " +
	"Use when a task needs domain conventions or a workflow that no listed skill covers."

const dynamicSkillsBody = `More skills are available on demand and are not listed in this session. The ` + "`{{server}}`" + ` MCP server serves them.

1. Call ` + "`find_skill`" + ` with a short description of the task. It returns ranked matches.
2. Call ` + "`load_skill`" + ` with the chosen ` + "`name`" + ` and follow the skill it returns.
3. ` + "`list_skill_resources`" + ` lists a skill's reference files; pass one as ` + "`path`" + ` to ` + "`load_skill`" + ` to read it.

Search before starting work in an unfamiliar area, not only when stuck.
`

func normalizeKey(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// mcpHarnesses are the built-in presets whose harness can call MCP tools. A
// skill that is served needs one of these to be reachable; every other preset
// keeps serving skills as static files. A `cmd/providers` test keeps this in
// step with the embedded provider specs.
var mcpHarnesses = map[string]bool{
	"aiassistant": true, "amp": true, "antigravity": true, "augment": true, "bob": true, "claude": true,
	"codebuddy": true, "codebuff": true, "codewhale": true, "codex": true, "commandcode": true,
	"copilot": true, "copilot-cli": true, "crush": true, "cursor": true, "deepagents": true,
	"devin": true, "factory": true, "gemini": true, "gitlab-duo": true, "goose": true, "grok": true,
	"junie": true, "kilo": true, "kimi": true, "kiro": true, "mimocode": true, "omp": true,
	"opencode": true, "pi": true, "poolside": true, "qoder": true, "qwen": true, "reasonix": true,
	"trae": true, "vibe": true, "warp": true, "zcode": true, "zed": true, "zoocode": true,
}

// HarnessSupportsMCP reports whether the named preset's harness can call MCP
// tools, which is what serving a skill needs. Custom and unknown presets report
// false: they are not known to, so served skills stay static for them.
func HarnessSupportsMCP(preset string) bool { return mcpHarnesses[preset] }

// MCPHarnessNames lists the presets HarnessSupportsMCP accepts, sorted.
func MCPHarnessNames() []string {
	names := make([]string, 0, len(mcpHarnesses))
	for n := range mcpHarnesses {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SkillDeliveryValue returns the raw `delivery` frontmatter value of a skill, or "".
func SkillDeliveryValue(skill ContentFile) string {
	if skill.Metadata == nil {
		return ""
	}
	return skill.Metadata.Extra["delivery"]
}

// EffectiveDelivery resolves how one skill reaches the agent. First match wins:
//
//  1. the skill's own `delivery` frontmatter key;
//  2. roleOverride, keyed by "<domain>/<skill>" for a skill of a domain, then by
//     skill id and name, then by domain name (per-role delivery; nil means the
//     role being rendered, see SetRoleDelivery, and none when no role applies);
//  3. [domains.<domain>] delivery, for a skill owned by a domain;
//  4. [skills] delivery, the global default;
//  5. static.
//
// Values that are not static/served/both are skipped, so one typo cannot change
// delivery silently; validation reports them (AR994).
func (c *Config) EffectiveDelivery(skill ContentFile, domain string, roleOverride map[string]string) Delivery {
	if d, ok := ParseDelivery(SkillDeliveryValue(skill)); ok {
		return d
	}
	if roleOverride == nil && c != nil {
		roleOverride = c.roleDelivery
	}
	if len(roleOverride) > 0 {
		keys := []string{SkillID(skill), skill.Name, domain}
		if domain != "" {
			keys = append([]string{domain + "/" + SkillID(skill)}, keys...)
		}
		for _, key := range keys {
			if key == "" {
				continue
			}
			if d, ok := ParseDelivery(roleOverride[key]); ok {
				return d
			}
		}
	}
	if c != nil {
		if dc, ok := c.DomainSettings[domain]; ok && domain != "" {
			if d, ok := ParseDelivery(dc.Delivery); ok {
				return d
			}
		}
		if c.Skills != nil {
			if d, ok := ParseDelivery(c.Skills.Delivery); ok {
				return d
			}
		}
	}
	return DeliveryStatic
}

// PlannedSkill is one skill with its effective delivery.
type PlannedSkill struct {
	// ID is the skill directory name.
	ID string
	// Domain owns the skill; empty for root content.
	Domain string
	// Delivery is the effective delivery.
	Delivery Delivery
}

// SkillDeliveries lists every skill of tree with its effective delivery, root
// skills first then domains in name order. roleOverride is passed to
// EffectiveDelivery.
func (c *Config) SkillDeliveries(tree *ContentTree, roleOverride map[string]string) []PlannedSkill {
	if tree == nil {
		return nil
	}
	var out []PlannedSkill
	for _, s := range tree.Skills {
		out = append(out, PlannedSkill{ID: SkillID(s), Delivery: c.EffectiveDelivery(s, "", roleOverride)})
	}
	names := make([]string, 0, len(tree.Domains))
	for n := range tree.Domains {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, s := range tree.Domains[n].Skills {
			out = append(out, PlannedSkill{ID: SkillID(s), Domain: n, Delivery: c.EffectiveDelivery(s, n, roleOverride)})
		}
	}
	return out
}

// DeliveryConfigured reports whether anything in tree or the configuration opts
// into dynamic delivery (served or both), at any level.
func (c *Config) DeliveryConfigured(tree *ContentTree) bool {
	for _, p := range c.SkillDeliveries(tree, nil) {
		if p.Delivery != DeliveryStatic {
			return true
		}
	}
	return false
}

// ServesSkills reports whether a skills server has anything to serve: a skill
// of tree whose delivery is served or both, or a [[skill_sources]] entry (whose
// skills are served, never written). The dynamic-skills stub, and the checks
// that an agent is told to call find_skill, apply exactly then.
func (c *Config) ServesSkills(tree *ContentTree) bool {
	return len(c.SkillSources) > 0 || c.DeliveryConfigured(tree)
}

// hasServedSkills reports whether anything is served (see ServesSkills).
func (c *Config) hasServedSkills(tree *ContentTree) bool { return c.ServesSkills(tree) }

// DeliveryFallback describes served skills a preset keeps static because its
// harness has no MCP support.
type DeliveryFallback struct {
	Preset string
	Skills []string
}

// DeliveryFallbacks lists, per configured preset without MCP support, the skills
// that are served but written statically instead. Sorted by preset.
func (c *Config) DeliveryFallbacks(tree *ContentTree) []DeliveryFallback {
	var served []string
	for _, p := range c.SkillDeliveries(tree, nil) {
		if p.Delivery == DeliveryServed {
			served = append(served, p.ID)
		}
	}
	if len(served) == 0 {
		return nil
	}
	sort.Strings(served)
	var out []DeliveryFallback
	seen := map[string]bool{}
	for i := range c.Presets {
		name := c.Presets[i].GetName()
		if seen[name] || HarnessSupportsMCP(name) || name == "mcp" {
			continue
		}
		seen[name] = true
		out = append(out, DeliveryFallback{Preset: name, Skills: served})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Preset < out[j].Preset })
	return out
}

// DefaultSkillsServerName is the server name docs register for `mcp --serve-skills`;
// the stub names it when no [[mcp_servers]] entry runs that command.
const DefaultSkillsServerName = "ai-rulez-skills"

// skillsServerName is the name of the configured MCP server that runs
// `mcp --serve-skills`, so the stub names a server the harness actually has.
func (c *Config) skillsServerName() string {
	runs := func(s *MCPServer) bool {
		for _, a := range s.Args {
			if a == "--serve-skills" {
				return true
			}
		}
		return false
	}
	for i := range c.MCPServersRaw {
		if runs(&c.MCPServersRaw[i]) && c.MCPServersRaw[i].Name != "" {
			return c.MCPServersRaw[i].Name
		}
	}
	names := make([]string, 0, len(c.MCPServers))
	for name, s := range c.MCPServers {
		if s != nil && runs(s) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		return names[0]
	}
	return DefaultSkillsServerName
}

// DynamicSkillsStub builds the generated stub skill; server names the MCP
// server that serves the skills.
func DynamicSkillsStub(server string) ContentFile {
	return ContentFile{
		Name:     DynamicSkillsName,
		Path:     dynamicSkillsPath,
		Content:  strings.ReplaceAll(dynamicSkillsBody, "{{server}}", server),
		Metadata: &Metadata{Extra: map[string]string{keyFrontmatterDescription: dynamicSkillsDescription}},
	}
}

// ContentForPreset returns the content tree one preset renders. Without served
// skills, or while a serving session renders (ServeMode), it is c.Content.
//
// Otherwise, for a harness that can call MCP, skills whose delivery is served
// are left out of the tree (so no static tree lists them), and the single
// dynamic-skills stub is added once to the root skills unless a skill of that
// name already exists. A harness without MCP support gets every skill as
// before: a served skill is never dropped for it.
func (c *Config) ContentForPreset(preset string) *ContentTree {
	tree := c.Content
	if tree == nil || c.ServeMode || !c.hasServedSkills(tree) {
		return tree
	}
	if !HarnessSupportsMCP(preset) {
		return tree
	}
	out := *tree
	out.Skills = c.keepStatic(tree.Skills, "")
	out.Domains = make(map[string]*Domain, len(tree.Domains))
	for name, d := range tree.Domains {
		kept := *d
		kept.Skills = c.keepStatic(d.Skills, name)
		out.Domains[name] = &kept
	}
	switch authored := c.authoredStubCount(&out); {
	case authored == 0:
		out.Skills = append(out.Skills, DynamicSkillsStub(c.skillsServerName()))
	case authored > 1:
		c.warnDuplicateStub(authored)
	}
	return &out
}

// authoredStubCount counts the skills named dynamic-skills that tree renders,
// in the root and in every domain. Any one of them stands in for the generated stub.
func (c *Config) authoredStubCount(tree *ContentTree) int {
	isStub := func(s ContentFile) bool { return SkillID(s) == DynamicSkillsName }
	n := 0
	for _, s := range tree.Skills {
		if isStub(s) {
			n++
		}
	}
	for _, d := range tree.Domains {
		for _, s := range d.Skills {
			if isStub(s) {
				n++
			}
		}
	}
	return n
}

// warnDuplicateStub says, once per project, that several authored skills are
// named dynamic-skills, so the harness gets whichever its tree keeps last.
func (c *Config) warnDuplicateStub(n int) {
	key := c.BaseDir + "\x00duplicate-stub"
	fallbackWarnMu.Lock()
	done := fallbackWarned[key]
	fallbackWarned[key] = true
	fallbackWarnMu.Unlock()
	if !done {
		logger.Warn(fmt.Sprintf("%d authored skills are named %q; they replace the generated stub and collide in a harness skill tree: rename all but one", n, DynamicSkillsName))
	}
}

func (c *Config) keepStatic(skills []ContentFile, domain string) []ContentFile {
	kept := make([]ContentFile, 0, len(skills))
	for _, s := range skills {
		if c.EffectiveDelivery(s, domain, nil) != DeliveryServed {
			kept = append(kept, s)
		}
	}
	return kept
}

var (
	fallbackWarnMu sync.Mutex
	fallbackWarned = map[string]bool{}
)

// SourceSkillBlindPresets lists the configured presets without MCP support when
// the project has [[skill_sources]]. Source skills are served only, never written
// to a harness's skill tree, so these harnesses cannot see them. Sorted.
func (c *Config) SourceSkillBlindPresets() []string {
	if len(c.SkillSources) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for i := range c.Presets {
		name := c.Presets[i].GetName()
		if seen[name] || HarnessSupportsMCP(name) || name == "mcp" {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// WarnDeliveryFallbacks logs, once per project and preset, that a preset without
// MCP support keeps served skills as static files. Nothing is dropped silently.
func (c *Config) WarnDeliveryFallbacks() {
	if c.ServeMode {
		return
	}
	for _, preset := range c.SourceSkillBlindPresets() {
		key := c.BaseDir + "\x00" + preset + "\x00skill_sources"
		fallbackWarnMu.Lock()
		done := fallbackWarned[key]
		fallbackWarned[key] = true
		fallbackWarnMu.Unlock()
		if !done {
			logger.Warn(fmt.Sprintf("Preset %q has no MCP support: skills from [[skill_sources]] are served only and never reach it (AR992)", preset))
		}
	}
	for _, fb := range c.DeliveryFallbacks(c.Content) {
		key := c.BaseDir + "\x00" + fb.Preset + "\x00" + strings.Join(fb.Skills, ",")
		fallbackWarnMu.Lock()
		done := fallbackWarned[key]
		fallbackWarned[key] = true
		fallbackWarnMu.Unlock()
		if done {
			continue
		}
		logger.Warn(fmt.Sprintf("Preset %q has no MCP support: %d served skill(s) are written statically for it instead (AR992)", fb.Preset, len(fb.Skills)),
			"skills", strings.Join(fb.Skills, ", "))
	}
}

// ExtraList returns a list-valued frontmatter key that has no typed field (such
// as `triggers`): a YAML sequence, or a single comma-separated scalar.
func (m *Metadata) ExtraList(key string) []string {
	if m == nil {
		return nil
	}
	raw := strings.TrimSpace(m.Extra[key])
	if raw == "" {
		return nil
	}
	var list []string
	if strings.HasPrefix(raw, "[") {
		if err := yaml.Unmarshal([]byte(raw), &list); err == nil {
			return cleanList(list)
		}
	}
	return cleanList(strings.Split(raw, ","))
}

func cleanList(in []string) []string {
	out := in[:0:0]
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SetRoleDelivery makes the per-skill delivery of a role the one every later
// EffectiveDelivery call without an explicit override uses, so a role render, a
// token report and the skills server all see the same split. It changes this
// Config: call it on a copy when the Config is shared. A nil override still
// marks a role as active (it sets no delivery of its own).
func (c *Config) SetRoleDelivery(override map[string]string) { c.roleDelivery = orEmpty(override) }

// RoleActive reports whether this Config is rendering a role (SetRoleDelivery
// was called). Outputs that belong to the whole project, such as the committed
// OKF bundle, are left alone while a role renders.
func (c *Config) RoleActive() bool { return c.roleDelivery != nil }

// RoleDeliveryOverride resolves a (flattened) role's delivery selectors against
// the content tree and returns the override EffectiveDelivery takes: one entry
// per skill the role sets a delivery for, keyed by "<domain>/<id>" for a skill
// of a domain and by id for a root skill.
func (c *Config) RoleDeliveryOverride(role *RoleConfig) map[string]string {
	if role == nil || len(role.Delivery) == 0 || c.Content == nil {
		return nil
	}
	out := map[string]string{}
	add := func(domain string, skills []ContentFile) {
		for i := range skills {
			id := SkillID(skills[i])
			d, ok := role.DeliveryFor(domain, id)
			if !ok {
				continue
			}
			key := id
			if domain != "" {
				key = domain + "/" + id
			}
			out[key] = string(d)
		}
	}
	add("", c.Content.Skills)
	for name, d := range c.Content.Domains {
		add(name, d.Skills)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RolesServeSkills reports whether some role sets a delivery other than static,
// so the project can serve skills even when no skill, domain or global default
// does.
func (c *Config) RolesServeSkills() bool {
	for i := range c.Roles {
		for _, v := range c.Roles[i].Delivery {
			if d, ok := ParseDelivery(v); ok && d != DeliveryStatic {
				return true
			}
		}
	}
	return false
}
