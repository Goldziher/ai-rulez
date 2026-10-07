package generator

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// SetRole makes the Generator render the named role instead of a profile. The
// content tree is narrowed by the role's domains and selectors, and the role's
// skill_mode entries are rendered as Claude Code skillOverrides through the same
// merge-by-key ownership as [claude.settings.managed] (only the listed skills are
// owned, every other key of .claude/settings.json is left alone). For the other
// harnesses a mode is rendered where the vendor documents an equivalent, and an
// "off" skill a harness cannot hide follows [role_manifest] skill_mode_fallback
// (see roles.PlanSkillModes). It is an error to combine a role with a profile;
// the caller checks that.
func (g *Generator) SetRole(name string) error {
	resolved, err := g.config.ResolveRole(name)
	if err != nil {
		return oops.Wrap(err)
	}
	flat := resolved.Flat()
	outcomes := roles.PlanSkillModes(g.config, resolved)
	flat = applyModeFallback(flat, outcomes)
	g.role = flat
	// Work on a copy of the config so one Config can serve several roles in turn
	// (tokens --by-role) without the delivery or overrides of one leaking into
	// the next. Skills the role serves are left out of the static trees and
	// reach the agent through the skills server instead.
	cp := *g.config
	cp.SetRoleDelivery(withServedFallback(resolved.DeliveryOverride, outcomes))
	cp.Content = withSkillKeys(cp.Content, outcomes)
	g.config = &cp
	g.applyRoleSkillOverrides(resolved, outcomes)
	g.warnRoleSkillModeHarnesses(name, outcomes)
	return nil
}

// Role returns the name of the role being rendered, or "".
func (g *Generator) Role() string {
	if g.role == nil {
		return ""
	}
	return g.role.Name
}

// applyRoleSkillOverrides merges the role's skillOverrides over the explicit
// [claude.settings.managed] ones (a role is the more specific statement). The
// shared Claude config is cloned, never mutated. A skill the fallback drops is
// not rendered, so it gets no override.
func (g *Generator) applyRoleSkillOverrides(res *config.ResolvedRole, outcomes []roles.SkillOutcome) {
	dropped := map[string]bool{}
	for i := range outcomes {
		o := outcomes[i]
		if o.Action == roles.ActionDrop {
			dropped[o.ID] = true
		}
	}
	overrides := map[string]string{}
	for skill, mode := range res.SkillOverrides {
		if !dropped[skill] {
			overrides[skill] = mode
		}
	}
	if len(overrides) == 0 {
		return
	}
	claude := config.ClaudeConfig{}
	if g.config.Claude != nil {
		claude = *g.config.Claude
	}
	settings := config.ClaudeSettings{}
	if claude.Settings != nil {
		settings = *claude.Settings
	}
	managed := config.ManagedSettings{}
	if settings.Managed != nil {
		managed = *settings.Managed
	}
	merged := make(map[string]string, len(managed.SkillOverrides)+len(overrides))
	for k, v := range managed.SkillOverrides {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	managed.SkillOverrides = merged
	settings.Managed = &managed
	claude.Settings = &settings
	cp := *g.config
	cp.Claude = &claude
	g.config = &cp
}

// applyModeFallback returns the role with the skills the "drop" fallback removes
// added to its skills exclude list (a copy; the resolved role is not touched).
func applyModeFallback(flat *config.RoleConfig, outcomes []roles.SkillOutcome) *config.RoleConfig {
	var drop []string
	for i := range outcomes {
		o := outcomes[i]
		if o.Action == roles.ActionDrop {
			drop = append(drop, o.Key())
		}
	}
	if len(drop) == 0 {
		return flat
	}
	cp := *flat
	sel := config.RoleSelector{}
	if cp.Skills != nil {
		sel = *cp.Skills
	}
	sel.Exclude = append(append([]string(nil), sel.Exclude...), drop...)
	cp.Skills = &sel
	return &cp
}

// withServedFallback adds the skills the "serve" fallback moves to served
// delivery to the role's delivery override.
func withServedFallback(override map[string]string, outcomes []roles.SkillOutcome) map[string]string {
	out := map[string]string{}
	for k, v := range override {
		out[k] = v
	}
	for i := range outcomes {
		o := outcomes[i]
		if o.Action == roles.ActionServe {
			out[o.Key()] = string(config.DeliveryServed)
		}
	}
	return out
}

// withSkillKeys returns the content tree with the invocation keys a role's
// skill_mode needs written into the frontmatter of its skills (a copy; the loaded
// tree is shared). A key the skill's author set is never overwritten.
func withSkillKeys(tree *config.ContentTree, outcomes []roles.SkillOutcome) *config.ContentTree {
	if tree == nil {
		return nil
	}
	keys := map[string]map[string]bool{}
	for i := range outcomes {
		o := outcomes[i]
		if o.Action == roles.ActionFrontmatter {
			keys[o.Key()] = o.Keys
		}
	}
	if len(keys) == 0 {
		return tree
	}
	cp := *tree
	cp.Skills = skillsWithKeys(tree.Skills, "", keys)
	cp.Domains = make(map[string]*config.Domain, len(tree.Domains))
	for name, d := range tree.Domains {
		dc := *d
		dc.Skills = skillsWithKeys(d.Skills, name, keys)
		cp.Domains[name] = &dc
	}
	return &cp
}

func skillsWithKeys(skills []config.ContentFile, domain string, keys map[string]map[string]bool) []config.ContentFile {
	out := make([]config.ContentFile, len(skills))
	copy(out, skills)
	for i := range out {
		id := config.SkillID(out[i])
		key := id
		if domain != "" {
			key = domain + "/" + id
		}
		want, ok := keys[key]
		if !ok {
			continue
		}
		meta := config.Metadata{}
		if out[i].Metadata != nil {
			meta = *out[i].Metadata
		}
		extra := make(map[string]string, len(meta.Extra)+len(want))
		for k, v := range meta.Extra {
			extra[k] = v
		}
		for k, v := range want {
			if _, set := meta.ExtraBool(k); !set {
				extra[k] = strconv.FormatBool(v)
			}
		}
		meta.Extra = extra
		out[i].Metadata = &meta
	}
	return out
}

// warnRoleSkillModeHarnesses names, per skill, the configured presets that cannot
// express the mode the role sets, so the person knows what is not applied. Nothing
// is approximated for them.
func (g *Generator) warnRoleSkillModeHarnesses(name string, outcomes []roles.SkillOutcome) {
	for i := range outcomes {
		o := outcomes[i]
		if len(o.Overridden) > 0 {
			g.log().Warn("skill_mode "+o.Mode+" of skill "+o.Key()+" in role "+name+" is not applied on "+strings.Join(o.Overridden, ", ")+
				": the skill's own frontmatter sets disable-model-invocation or user-invocable to another value, and an author's key is never overwritten",
				"role", name, "skill", o.Key())
		}
		if len(o.Degraded) == 0 {
			continue
		}
		switch o.Action {
		case roles.ActionDrop:
			g.log().Warn("skill_mode off of skill "+o.Key()+" in role "+name+" has no documented setting on "+strings.Join(o.Degraded, ", ")+
				"; the skill is left out ([role_manifest] skill_mode_fallback = \"drop\")", "role", name, "skill", o.Key())
		case roles.ActionServe:
			g.log().Warn("skill_mode off of skill "+o.Key()+" in role "+name+" has no documented setting on "+strings.Join(o.Degraded, ", ")+
				"; the skill is served over MCP instead ([role_manifest] skill_mode_fallback = \"serve\")", "role", name, "skill", o.Key())
		default:
			g.log().Warn("skill_mode "+o.Mode+" of skill "+o.Key()+" in role "+name+" is not applied on "+strings.Join(o.Degraded, ", ")+
				": no documented per-skill setting; the skill stays listed there", "role", name, "skill", o.Key())
		}
	}
}

// rolesManifestOutput builds <config dir>/roles.json when [role_manifest]
// enabled is set. The manifest always describes every role over the whole content
// tree, so it is the same whichever role or profile is being generated, and it
// carries no timestamp. User scope writes none.
//
// roles.json is committed, so it is built from the shared sources only: roles
// declared in config.local.* and items under local/ never appear in it, exactly
// as the okf export leaves local content out. Local roles stay usable with
// generate --role and roles list.
func (g *Generator) rolesManifestOutput() (config.OutputFile, bool, error) {
	if g.userMode || !g.config.RoleManifestEnabled() {
		return config.OutputFile{}, false, nil
	}
	source, err := g.sharedConfig()
	if err != nil {
		return config.OutputFile{}, false, err
	}
	if !source.RoleManifestEnabled() {
		return config.OutputFile{}, false, nil
	}
	counter, err := tokens.New("")
	if err != nil {
		return config.OutputFile{}, false, oops.Wrapf(err, "token counter for the roles manifest")
	}
	data, err := roles.Build(source, counter).Marshal()
	if err != nil {
		return config.OutputFile{}, false, err //nolint:wrapcheck // already contextual
	}
	return config.OutputFile{Path: filepath.Join(g.config.ConfigDir, roles.FileName), RawContent: data}, true, nil
}

// sharedConfig returns the configuration without the machine-local overlay and
// local/ content: the loaded one when it has no local input, else a fresh load of
// the same file with local inputs skipped (the view a teammate sees).
func (g *Generator) sharedConfig() (*config.Config, error) {
	if !g.config.HasLocalInputs() {
		return g.config, nil
	}
	if g.config.ConfigDir == "" || g.config.ConfigFile == "" {
		return nil, oops.Errorf("the config file location is unknown, so roles.json cannot be built from the shared sources")
	}
	path := filepath.Join(g.config.ConfigDir, g.config.ConfigFile)
	shared, err := config.LoadConfigFromFile(g.context(), path,
		config.WithoutLocal(), config.WithIncludeMemo(g.config.IncludeMemo), config.WithResolvers(g.config.Resolve),
		config.WithPolicy(g.config.Policy()), config.WithLockPolicy(g.config.LockPolicy))
	if err != nil {
		return nil, oops.Wrapf(err, "load the shared configuration for roles.json")
	}
	if err := g.nestedPolicy(shared, false); err != nil {
		return nil, oops.Wrapf(err, "the shared configuration for roles.json")
	}
	shared.Diag = g.config.Diag
	return shared, nil
}

// nestedPolicy holds a configuration the generator loaded itself (a monorepo
// member, the shared view of the project) to the organization policy: a
// violation fails the run, except under --policy-mode warn, where the warnings
// are logged when warn is set. The shared views reload the root's own files, which
// the command line already reported, so they pass warn false.
func (g *Generator) nestedPolicy(cfg *config.Config, warn bool) error {
	if warn {
		for _, line := range config.PolicyWarnings(cfg) {
			g.log().Warn("Organization policy violation (--policy-mode warn): " + line)
		}
	}
	return config.CheckPolicy(cfg) //nolint:wrapcheck // already contextual
}

// presetClaude is the preset whose settings carry skillOverrides.
const presetClaude = "claude"
