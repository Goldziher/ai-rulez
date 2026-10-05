package providers

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
)

func skillFile(name, path string, extra map[string]string, targets ...string) config.ContentFile {
	return config.ContentFile{
		Name: name, Path: path,
		Metadata: &config.Metadata{Targets: targets, Extra: extra},
	}
}

func TestResolvePlacement(t *testing.T) {
	rootSkill := skillFile("root-s", "/p/.ai-rulez/skills/root-s/SKILL.md", nil)
	domainSkill := skillFile("py", "/p/.ai-rulez/domains/backend/skills/py/SKILL.md", nil)
	override := skillFile("py2", "/p/.ai-rulez/domains/backend/skills/py2/SKILL.md", map[string]string{"placement": "Core"})
	badValue := skillFile("x", "/p/.ai-rulez/skills/x/SKILL.md", map[string]string{"placement": "nowhere"})
	builtinSkill := skillFile("docs", "builtin://documentation/docs", nil)
	tree := &config.ContentTree{
		Skills: []config.ContentFile{rootSkill, badValue},
		Domains: map[string]*config.Domain{
			"backend":       {Name: "backend", Skills: []config.ContentFile{domainSkill, override}},
			"documentation": {Name: "documentation", Builtin: true, Skills: []config.ContentFile{builtinSkill}},
		},
	}
	tests := []struct {
		name  string
		place *config.PlacementConfig
		item  config.ContentFile
		want  string
	}{
		{name: "no block is core", item: domainSkill, want: config.PlacementCore},
		{name: "empty block is core", place: &config.PlacementConfig{}, item: domainSkill, want: config.PlacementCore},
		{name: "default plugin", place: &config.PlacementConfig{Default: "plugin"}, item: rootSkill, want: config.PlacementPlugin},
		{name: "domains glob is plugin", place: &config.PlacementConfig{Plugin: []string{"domains/*"}}, item: domainSkill, want: config.PlacementPlugin},
		{name: "domains glob leaves root alone", place: &config.PlacementConfig{Plugin: []string{"domains/*"}}, item: rootSkill, want: config.PlacementCore},
		{name: "name pattern", place: &config.PlacementConfig{Plugin: []string{"py"}}, item: domainSkill, want: config.PlacementPlugin},
		{name: "per-domain glob", place: &config.PlacementConfig{Plugin: []string{"domains/backend/*"}}, item: domainSkill, want: config.PlacementPlugin},
		{name: "core beats plugin", place: &config.PlacementConfig{Plugin: []string{"domains/*"}, Core: []string{"domains/backend/py"}}, item: domainSkill, want: config.PlacementCore},
		{name: "frontmatter beats block", place: &config.PlacementConfig{Plugin: []string{"domains/*"}}, item: override, want: config.PlacementCore},
		{name: "builtin domain content is not domains/*", place: &config.PlacementConfig{Plugin: []string{"domains/*"}}, item: builtinSkill, want: config.PlacementCore},
		{name: "generated catalog skill is always core", place: &config.PlacementConfig{Default: "plugin"}, item: skillFile("plugin-catalog", "generated://plugin-catalog/SKILL.md", nil), want: config.PlacementCore},
		{name: "unknown frontmatter is ignored", place: &config.PlacementConfig{Default: "plugin"}, item: badValue, want: config.PlacementPlugin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{Placement: tt.place}

			// Act
			got := ResolvePlacement(cfg, OutputTypeSkills, tt.item, tree)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlacementAllows_Targets(t *testing.T) {
	gen := &Generator{Spec: &ProviderSpec{Name: "claude"}}
	cursorOnly := skillFile("s", "/p/.ai-rulez/skills/s/SKILL.md", nil, "cursor")
	tests := []struct {
		name string
		typ  string
		cfg  *config.Config
		want bool
	}{
		{name: "skill targets ignored by default", typ: OutputTypeSkills, cfg: &config.Config{}, want: true},
		{name: "skill targets honored on request", typ: OutputTypeSkills, cfg: &config.Config{Placement: &config.PlacementConfig{HonorTargets: true}}, want: false},
		{name: "command targets always honored", typ: OutputTypeCommands, cfg: &config.Config{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := gen.placementAllows(tt.typ, cursorOnly, &config.ContentTree{}, tt.cfg)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
