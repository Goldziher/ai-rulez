package exposure

import "github.com/Goldziher/ai-rulez/v5/internal/areas"

// AreaSkill is the skill whose owner the area map resolves. A repository skill
// is owned by its scope and classified by its listing text. A plugin skill is
// owned by its plugin directory and classified by its qualified name
// (`plugin:skill`), so its description never places it: the plugin directory
// or an explicit skill_areas entry does.
func (a *Artifact) AreaSkill() areas.Skill {
	if a.Source == SourcePlugin {
		return areas.Skill{Name: a.Name, Plugin: a.Plugin, Scope: AreaScope(a), Text: a.QualifiedName}
	}
	return areas.Skill{Name: a.Name, Scope: AreaScope(a), Text: a.ListingText}
}

// Classify sets Area and AreaSource on every artifact: a skill gets its owner
// (Map.Owner of AreaSkill), any other artifact the classification of its
// directory and text (Map.ClassifyWithSource of AreaScope and Text). The
// directory is read as ClaimScope gives it, relative to the base of the map's
// path claims. Call it before WithoutText. A nil map leaves every artifact
// Unclassified.
func (inv *Inventory) Classify(m *areas.Map) {
	for i := range inv.Artifacts {
		a := &inv.Artifacts[i]
		scope := inv.ClaimScope(a)
		if a.Kind == KindSkill {
			skill := a.AreaSkill()
			skill.Scope = scope
			owner := m.Owner(skill)
			a.Area, a.AreaSource = owner.Area, owner.Source
			continue
		}
		a.Area, a.AreaSource = m.ClassifyWithSource(scope, a.Text)
	}
}

// SchemaAreas is the schema_version of an AreasReport
// (schema/exposure-areas.v1.schema.json).
const SchemaAreas = 1

// AreasReport is the area map as loaded and the area of every artifact of an
// inventory, with how it was decided: what `ai-rulez exposure areas` prints,
// for a consumer to diff against its own classifier.
type AreasReport struct {
	SchemaVersion int    `json:"schema_version"`
	Root          string `json:"root"`
	// AreasFile is the areas file the map was read from, "" for the inline
	// [exposure] form.
	AreasFile string `json:"areas_file"`
	// Plugins is the plugin mode of the inventory.
	Plugins    PluginMode           `json:"plugins"`
	Areas      []AreaInfo           `json:"areas"`
	SkillAreas map[string]string    `json:"skill_areas"`
	Artifacts  []ClassifiedArtifact `json:"artifacts"`
	// Counts is the number of artifacts per area: every area of the map and
	// Unclassified, 0 when no artifact is in it.
	Counts map[string]int `json:"counts"`
	// Problems are the inventory's: the files it could not read or decode. An
	// unread artifact is classified on an empty text.
	Problems []Problem `json:"problems"`
}

// AreaInfo is one area of the map, normalized as the map holds it.
type AreaInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Owner       string   `json:"owner"`
	Paths       []string `json:"paths"`
	Terms       []string `json:"terms"`
	Ambiguous   []string `json:"ambiguous"`
	Shared      bool     `json:"shared"`
	Declare     string   `json:"declare"`
}

// ClassifiedArtifact identifies an artifact and gives its area. AreaScope is
// the directory whose path claims were tried (see Inventory.ClaimScope).
type ClassifiedArtifact struct {
	Kind          Kind         `json:"kind"`
	Source        Source       `json:"source"`
	Name          string       `json:"name"`
	QualifiedName string       `json:"qualified_name"`
	Plugin        string       `json:"plugin,omitempty"`
	Path          string       `json:"path"`
	Scope         string       `json:"scope"`
	AreaScope     string       `json:"area_scope"`
	Phase         Phase        `json:"phase"`
	Area          string       `json:"area"`
	AreaSource    areas.Source `json:"area_source"`
}

// NewAreasReport classifies a copy of the inventory with the map; areasFile
// names where the map came from ("" for the inline form).
func NewAreasReport(inv *Inventory, m *areas.Map, areasFile string) *AreasReport {
	classified := *inv
	classified.Artifacts = append([]Artifact(nil), inv.Artifacts...)
	classified.Classify(m)
	rep := &AreasReport{
		SchemaVersion: SchemaAreas,
		Root:          inv.Root,
		AreasFile:     areasFile,
		Plugins:       inv.Plugins.Mode,
		Areas:         []AreaInfo{},
		SkillAreas:    map[string]string{},
		Artifacts:     make([]ClassifiedArtifact, 0, len(classified.Artifacts)),
		Counts:        map[string]int{areas.Unclassified: 0},
		Problems:      append([]Problem{}, inv.Problems...),
	}
	all := m.Areas()
	for i := range all {
		a := &all[i]
		rep.Areas = append(rep.Areas, AreaInfo{
			Name: a.Name, Description: a.Description, Owner: a.Owner, Paths: nonNil(a.Paths), Terms: nonNil(a.Terms),
			Ambiguous: nonNil(a.Ambiguous), Shared: a.Shared, Declare: a.Declare,
		})
		rep.Counts[a.Name] = 0
	}
	for k, v := range m.SkillAreas() {
		rep.SkillAreas[k] = v
	}
	for i := range classified.Artifacts {
		a := &classified.Artifacts[i]
		rep.Artifacts = append(rep.Artifacts, ClassifiedArtifact{
			Kind: a.Kind, Source: a.Source, Name: a.Name, QualifiedName: a.QualifiedName, Plugin: a.Plugin, Path: a.Path,
			Scope: a.Scope, AreaScope: classified.ClaimScope(a), Phase: a.Phase, Area: a.Area, AreaSource: a.AreaSource,
		})
		rep.Counts[a.Area]++
	}
	return rep
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
