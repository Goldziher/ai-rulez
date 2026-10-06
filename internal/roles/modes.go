package roles

import (
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Skill modes a role can set (Claude Code's skillOverrides states).
const (
	ModeOn              = "on"
	ModeNameOnly        = "name-only"
	ModeUserInvocable   = "user-invocable-only"
	ModeOff             = "off"
	capabilitiesChecked = "2026-10-06"
)

// Capability says which skill modes one harness can honour, from its vendor
// documentation. A mode is listed only where the vendor documents a setting for
// it; nothing is approximated, and a harness not listed here is "not documented".
//
// Modes holds what ai-rulez renders. Documented holds a documented setting that
// ai-rulez does not render (so the mode still counts as not honoured), with the
// reason, so a later change has the evidence at hand.
type Capability struct {
	Preset     string
	Modes      map[string]string
	Documented map[string]string
	// Source is the vendor documentation page the rows come from.
	Source string
	// Verified is the date the page was read.
	Verified string
}

// Honours returns the mechanism that implements mode on the harness.
func (c Capability) Honours(mode string) (string, bool) {
	if mode == ModeOn {
		return "default", true
	}
	how, ok := c.Modes[mode]
	return how, ok
}

const (
	viaSettings    = "skillOverrides in .claude/settings.json"
	viaFrontmatter = "disable-model-invocation: true in SKILL.md"
	viaBothKeys    = "disable-model-invocation: true and user-invocable: false in SKILL.md"
	viaCodexPolicy = "policy.allow_implicit_invocation: false in agents/openai.yaml"
)

var capabilities = []Capability{
	{
		Preset: "claude", Source: "https://code.claude.com/docs/en/skills", Verified: capabilitiesChecked,
		Modes: map[string]string{ModeNameOnly: viaSettings, ModeUserInvocable: viaSettings, ModeOff: viaSettings},
	},
	{
		Preset: "cursor", Source: "https://cursor.com/docs/context/skills", Verified: capabilitiesChecked,
		Modes: map[string]string{ModeUserInvocable: viaFrontmatter},
	},
	{
		Preset: "codex", Source: "https://developers.openai.com/codex/skills", Verified: capabilitiesChecked,
		Modes:      map[string]string{ModeUserInvocable: viaCodexPolicy},
		Documented: map[string]string{ModeOff: "[[skills.config]] enabled = false in ~/.codex/config.toml takes an absolute skill path, which is machine specific"},
	},
	{
		Preset: "copilot", Source: "https://code.visualstudio.com/docs/copilot/customization/agent-skills", Verified: capabilitiesChecked,
		Modes: map[string]string{ModeUserInvocable: viaFrontmatter, ModeOff: viaBothKeys},
	},
	{
		Preset: "opencode", Source: "https://opencode.ai/docs/skills/", Verified: capabilitiesChecked,
		Documented: map[string]string{ModeOff: `permission.skill."<name>" = "deny" in opencode.json is documented; ai-rulez does not render it`},
	},
	{
		Preset: "gemini", Source: "https://geminicli.com/docs/cli/skills/", Verified: capabilitiesChecked,
		Documented: map[string]string{ModeOff: "only the /skills disable <name> command is documented, not a settings key"},
	},
}

// Capabilities returns the per-harness table, sorted by preset.
func Capabilities() []Capability {
	out := append([]Capability(nil), capabilities...)
	sort.Slice(out, func(i, j int) bool { return out[i].Preset < out[j].Preset })
	return out
}

// CapabilityOf returns the row of a preset; false when its vendor documents no
// per-skill invocation setting that was checked.
func CapabilityOf(preset string) (Capability, bool) {
	for _, c := range capabilities {
		if c.Preset == preset {
			return c, true
		}
	}
	return Capability{Preset: preset}, false
}

// Actions of a SkillOutcome.
const (
	// ActionFrontmatter writes the invocation keys into the generated SKILL.md.
	ActionFrontmatter = "frontmatter"
	// ActionDrop leaves the skill out of the role's render.
	ActionDrop = "drop"
	// ActionServe delivers the skill through the skills server only.
	ActionServe = "serve"
)

// SkillOutcome is what one skill_mode of a role comes to on the configured
// harnesses.
type SkillOutcome struct {
	Domain string `json:"domain,omitempty"`
	ID     string `json:"id"`
	Mode   string `json:"mode"`
	// Honoured maps each harness that implements the mode to its mechanism.
	Honoured map[string]string `json:"honoured,omitempty"`
	// Degraded lists the harnesses that cannot implement it.
	Degraded []string `json:"degraded,omitempty"`
	// Action is what generate does beyond Claude's settings: ActionFrontmatter,
	// or the configured fallback (ActionDrop, ActionServe) for an "off" skill a
	// harness cannot hide. Empty when nothing more is done.
	Action string `json:"action,omitempty"`
	// Keys are the SKILL.md keys ActionFrontmatter writes.
	Keys map[string]bool `json:"-"`
}

// Key is "<domain>/<id>" or the id of a root skill.
func (o SkillOutcome) Key() string {
	if o.Domain == "" {
		return o.ID
	}
	return o.Domain + "/" + o.ID
}

// skillPresets are the configured presets that can carry a skill.
func skillPresets(cfg *config.Config) []string {
	var out []string
	for i := range cfg.Presets {
		name := cfg.Presets[i].GetName()
		if name == "mcp" || name == "okf" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PlanSkillModes decides, for every skill the role gives a mode other than "on",
// which configured harnesses honour it and what generate does for the rest.
//
//   - name-only has no equivalent outside Claude Code: the skill stays listed there.
//   - user-invocable-only is written as disable-model-invocation: true where a
//     harness documents it (and as Codex's agents/openai.yaml policy).
//   - off is written where every harness documents a way to hide the skill; when
//     one does not, [role_manifest] skill_mode_fallback decides: "drop" leaves the
//     skill out of the render, "serve" delivers it through the skills server.
//
// One content tree is rendered for all harnesses, so the fallback applies to the
// skill on every harness of the run.
func PlanSkillModes(cfg *config.Config, res *config.ResolvedRole) []SkillOutcome {
	presets := skillPresets(cfg)
	var out []SkillOutcome
	for i := range res.Items {
		item := &res.Items[i]
		if item.Kind != config.RoleKindSkill || item.Mode == "" || item.Mode == ModeOn {
			continue
		}
		o := SkillOutcome{Domain: item.Domain, ID: item.ID, Mode: item.Mode, Honoured: map[string]string{}}
		viaFile := false
		for _, p := range presets {
			capability, _ := CapabilityOf(p)
			how, ok := capability.Honours(item.Mode)
			if !ok {
				o.Degraded = append(o.Degraded, p)
				continue
			}
			o.Honoured[p] = how
			if how != viaSettings {
				viaFile = true
			}
		}
		switch {
		case item.Mode == ModeOff && len(o.Degraded) > 0 && item.Delivery != string(config.DeliveryServed):
			o.Action = ActionDrop
			if cfg.RoleSkillModeFallback() == config.SkillModeFallbackServe {
				o.Action = ActionServe
			}
		case viaFile && item.Mode == ModeUserInvocable:
			o.Action, o.Keys = ActionFrontmatter, map[string]bool{"disable-model-invocation": true}
		case viaFile && item.Mode == ModeOff:
			o.Action, o.Keys = ActionFrontmatter, map[string]bool{"disable-model-invocation": true, "user-invocable": false}
		}
		if len(o.Honoured) == 0 {
			o.Honoured = nil
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}
