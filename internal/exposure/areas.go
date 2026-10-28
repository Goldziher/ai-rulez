package exposure

import "github.com/Goldziher/ai-rulez/v5/internal/areas"

// AreaSkill is the skill whose owner the area map resolves, filled as the
// reference implementation does. A repository skill is owned by its scope and
// classified by its listing text. A plugin skill is owned by its plugin
// directory and classified by its qualified name (`plugin:skill`), so its
// description never places it: the plugin directory or an explicit skill_areas
// entry does.
func (a *Artifact) AreaSkill() areas.Skill {
	if a.Source == SourcePlugin {
		return areas.Skill{Name: a.Name, Plugin: a.Plugin, Scope: a.PluginPath, Text: a.QualifiedName}
	}
	return areas.Skill{Name: a.Name, Scope: a.Scope, Text: a.ListingText}
}

// Classify sets Area and AreaSource on every artifact: a skill gets its owner
// (Map.Owner of AreaSkill), any other artifact the classification of its
// directory and text (Map.ClassifyWithSource of AreaScope and Text). Call it
// before WithoutText. A nil map leaves every artifact Unclassified.
func (inv *Inventory) Classify(m *areas.Map) {
	for i := range inv.Artifacts {
		a := &inv.Artifacts[i]
		if a.Kind == KindSkill {
			owner := m.Owner(a.AreaSkill())
			a.Area, a.AreaSource = owner.Area, owner.Source
			continue
		}
		a.Area, a.AreaSource = m.ClassifyWithSource(AreaScope(a), a.Text)
	}
}
