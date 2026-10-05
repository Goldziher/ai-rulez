package generator

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/roles"
	"github.com/Goldziher/ai-rulez/internal/tokens"
)

// SetRole makes the Generator render the named role instead of a profile. The
// content tree is narrowed by the role's domains and selectors, and the role's
// skill_mode entries are rendered as Claude Code skillOverrides through the same
// merge-by-key ownership as [claude.settings.managed] (only the listed skills are
// owned, every other key of .claude/settings.json is left alone). It is an error
// to combine a role with a profile; the caller checks that.
func (g *Generator) SetRole(name string) error {
	resolved, err := g.config.ResolveRole(name)
	if err != nil {
		return oops.Wrap(err)
	}
	flat := resolved.Flat()
	g.role = flat
	g.applyRoleSkillOverrides(resolved)
	g.warnRoleSkillModeHarnesses(name, flat)
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
// shared Claude config is cloned, never mutated.
func (g *Generator) applyRoleSkillOverrides(res *config.ResolvedRole) {
	if len(res.SkillOverrides) == 0 {
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
	merged := make(map[string]string, len(managed.SkillOverrides)+len(res.SkillOverrides))
	for k, v := range managed.SkillOverrides {
		merged[k] = v
	}
	for k, v := range res.SkillOverrides {
		merged[k] = v
	}
	managed.SkillOverrides = merged
	settings.Managed = &managed
	claude.Settings = &settings
	// Work on a copy of the config so one Config can serve several roles in turn
	// (tokens --by-role) without the overrides of one leaking into the next.
	cp := *g.config
	cp.Claude = &claude
	g.config = &cp
}

// warnRoleSkillModeHarnesses names the configured presets that cannot express a
// per-skill invocation mode. Only Claude Code documents one (skillOverrides);
// nothing is approximated for the others.
func (g *Generator) warnRoleSkillModeHarnesses(name string, flat *config.RoleConfig) {
	if len(flat.SkillMode) == 0 {
		return
	}
	var others []string
	for i := range g.config.Presets {
		p := g.config.Presets[i].GetName()
		if p != "claude" && p != "mcp" {
			others = append(others, p)
		}
	}
	if len(others) == 0 {
		return
	}
	sort.Strings(others)
	logger.Warn("skill_mode of role "+name+" is applied to Claude Code only (skillOverrides); these presets have no documented per-skill invocation setting, so it is not applied",
		"presets", strings.Join(others, ", "))
}

// rolesManifestOutput builds <config dir>/roles.json when [role_manifest]
// enabled is set. The manifest always describes every role over the whole content
// tree, so it is the same whichever role or profile is being generated, and it
// carries no timestamp. User scope writes none.
func (g *Generator) rolesManifestOutput() (config.OutputFile, bool, error) {
	if g.userMode || !g.config.RoleManifestEnabled() {
		return config.OutputFile{}, false, nil
	}
	counter, err := tokens.New("")
	if err != nil {
		return config.OutputFile{}, false, oops.Wrapf(err, "token counter for the roles manifest")
	}
	data, err := roles.Build(g.config, counter).Marshal()
	if err != nil {
		return config.OutputFile{}, false, err //nolint:wrapcheck // already contextual
	}
	return config.OutputFile{Path: filepath.Join(g.config.ConfigDir, roles.FileName), RawContent: data}, true, nil
}
