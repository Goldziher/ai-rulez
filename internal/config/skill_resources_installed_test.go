package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shadcnLayout is the skill layout from #419: documents beside SKILL.md, a rules/
// folder, canonical assets/, and evals/ and agents/ that are not for the agent.
func shadcnLayout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"SKILL.md":                    "---\nname: shadcn\ndescription: d\n---\n# s\n",
		"cli.md":                      "# cli\n",
		"customization.md":            "# custom\n",
		"rules/forms.md":              "# forms\n",
		"rules/styling.md":            "# styling\n",
		"assets/logo.png":             "png",
		"evals/evals.json":            "{}",
		"agents/openai.yml":           "x: 1\n",
		".gitignore":                  "x\n",
		"scripts/setup.sh":            "#!/bin/sh\n",
		"templates/component/tmpl.ts": "export {}\n",
	}
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return dir
}

func resourcePaths(resources []SkillResource) []string {
	out := make([]string, len(resources))
	for i := range resources {
		out[i] = resources[i].RelPath
	}
	return out
}

func TestLoadSkillResourcesContext_InstalledSkillIsBundledWhole(t *testing.T) {
	dir := shadcnLayout(t)

	resources, err := LoadSkillResourcesContext(context.Background(), dir)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"cli.md", "customization.md", "rules/forms.md", "rules/styling.md", // documents are references
		"scripts/setup.sh", "templates/component/tmpl.ts", // scripts
		"assets/logo.png", // assets
	}, resourcePaths(resources), "everything but SKILL.md, evals/, agents/ and hidden files")
	kinds := map[string]string{}
	for i := range resources {
		kinds[resources[i].RelPath] = resources[i].Kind
	}
	assert.Equal(t, SkillKindReferences, kinds["cli.md"])
	assert.Equal(t, SkillKindScripts, kinds["templates/component/tmpl.ts"])
	assert.Equal(t, SkillKindAssets, kinds["assets/logo.png"])
}

func TestLoadSkillResourcesContext_ResourcesReplaceTheDefault(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"only a folder", []string{"rules"}, []string{"rules/forms.md", "rules/styling.md", "assets/logo.png", "scripts/setup.sh"}},
		{"everything but a folder", []string{"*", "!rules", "!templates", "!evals", "!agents", "!.*"}, []string{"cli.md", "customization.md", "assets/logo.png", "scripts/setup.sh"}},
		{"evals opted in", []string{"*", "!agents", "!.*"}, []string{"cli.md", "customization.md", "evals/evals.json", "rules/forms.md", "rules/styling.md", "scripts/setup.sh", "templates/component/tmpl.ts", "assets/logo.png"}},
		{"nothing extra", []string{"none"}, []string{"scripts/setup.sh", "assets/logo.png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithSkillResources(context.Background(), tt.patterns)
			resources, err := LoadSkillResourcesContext(ctx, shadcnLayout(t))
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, resourcePaths(resources))
		})
	}
}

func TestLoadSkillResources_LocalSkillsKeepTheCanonicalLayout(t *testing.T) {
	resources, err := LoadSkillResources(shadcnLayout(t))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"scripts/setup.sh", "assets/logo.png"}, resourcePaths(resources),
		"a project's own skill is not bundled whole")
}

func TestValidateInstalledSkillFields_ResourcePatterns(t *testing.T) {
	tests := []struct {
		pattern string
		wantErr string
	}{
		{"rules", ""},
		{"!evals", ""},
		{"docs/*.md", ""},
		{"", "is empty"},
		{"!", "is empty"},
		{"/etc", "relative"},
		{"../x", "'..'"},
		{"a[", "valid glob"},
	}
	for _, tt := range tests {
		err := ValidateInstalledSkillFields(&InstalledSkillConfig{Name: "s", Source: "https://github.com/o/r", Resources: []string{tt.pattern}})
		if tt.wantErr == "" {
			assert.NoError(t, err, tt.pattern)
			continue
		}
		require.Error(t, err, tt.pattern)
		assert.Contains(t, err.Error(), tt.wantErr, tt.pattern)
	}
}
