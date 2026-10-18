// Excerpt of https://github.com/esengine/DeepSeek-Reasonix/blob/v2.43.0/internal/ext/skill/builtins.go
package skill

const builtinInitBody = `Name: "not-a-skill",`

func builtinSkills() []Skill {
	base := []Skill{
		{
			Name:  "init",
			Scope: ScopeBuiltin,
		},
		// {
		//	Name:        "gone",
		// },
		{
			Name:  "explore",
			Scope: ScopeBuiltin,
		},
	}
	return append(base, loadEmbeddedBuiltins()...)
}
