package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exposureAreasConfig is an inline area map: the plugin lives under the shared tools area, and its skill's
// description names a billing term, so the owner shows whether the plugin directory decided.
const exposureAreasConfig = `
[exposure]
plugins = "all"

[[exposure.areas]]
name = "billing"
paths = ["services/billing"]
terms = ["invoice"]

[[exposure.areas]]
name = "tooling"
paths = ["tools"]
shared = true

[exposure.skill_areas]
"kit:solo" = "billing"
`

// exposureProject is a generated project with native Claude Code files, a repository marketplace and two
// plugins, one under a claimed directory.
func exposureProject(t *testing.T, env *isoEnv, extraConfig string) string {
	t.Helper()
	root := minimalProject(t, extraConfig)
	res := env.run(root, "generate", "--yes")
	require.Equal(t, 0, res.ExitCode, res.Stderr)
	writeTree(t, root, map[string]string{
		".claude-plugin/marketplace.json": `{"name": "mkt", "plugins": [
  {"name": "ops", "source": "./tools/plugins/ops"},
  {"name": "kit", "source": "./kit"}
]}`,
		"tools/plugins/ops/skills/probe/SKILL.md":         "---\nname: probe\ndescription: Probes every invoice.\n---\nbody\n",
		"kit/skills/solo/SKILL.md":                        "---\nname: solo\ndescription: Does one thing.\n---\nbody\n",
		"services/billing/.claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Charges a card.\n---\nbody\n",
		"areas.toml": "[[area]]\nname = \"search\"\nterms = [\"probes\"]\n",
	})
	return root
}

type exposureArtifact struct {
	Kind          string  `json:"kind"`
	QualifiedName string  `json:"qualified_name"`
	Path          string  `json:"path"`
	Phase         string  `json:"phase"`
	Area          string  `json:"area"`
	AreaSource    string  `json:"area_source"`
	AreaScope     string  `json:"area_scope"`
	Text          *string `json:"text"`
}

func exposureArtifacts(t *testing.T, stdout string) map[string]exposureArtifact {
	t.Helper()
	var doc struct {
		Artifacts []exposureArtifact `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	out := map[string]exposureArtifact{}
	for _, a := range doc.Artifacts {
		out[a.QualifiedName] = a
	}
	return out
}

func TestExposureInventory(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, exposureAreasConfig)

	t.Run("json classifies every artifact and leaves the text out", func(t *testing.T) {
		res := env.run(root, "exposure", "inventory", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-inventory.v1.schema.json", res.Stdout)
		arts := exposureArtifacts(t, res.Stdout)
		require.Contains(t, arts, "ops:probe", "[exposure] plugins = \"all\" lists the plugin skills")
		assert.Equal(t, "tooling", arts["ops:probe"].Area)
		assert.Equal(t, "path", arts["ops:probe"].AreaSource)
		assert.Equal(t, "skill_areas", arts["kit:solo"].AreaSource)
		assert.Equal(t, "billing", arts["charge"].Area)
		assert.Equal(t, "on_demand", arts["charge"].Phase)
		assert.Nil(t, arts["ops:probe"].Text)
	})

	t.Run("--with-text includes the texts", func(t *testing.T) {
		res := env.run(root, "exposure", "inventory", "--format", "json", "--with-text")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		probe := exposureArtifacts(t, res.Stdout)["ops:probe"]
		require.NotNil(t, probe.Text)
		assert.Equal(t, "ops:probe: Probes every invoice.", *probe.Text)
	})

	t.Run("--plugins overrides the configured mode", func(t *testing.T) {
		res := env.run(root, "exposure", "inventory", "--format", "json", "--plugins", "none")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		assert.NotContains(t, exposureArtifacts(t, res.Stdout), "ops:probe")
	})

	t.Run("text lists phases, areas and a summary", func(t *testing.T) {
		res := env.run(root, "exposure", "inventory")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		for _, want := range []string{"PHASE", "AREA", "ops:probe", "tooling (path)", "start ("} {
			assert.Contains(t, res.Stdout, want)
		}
	})

	t.Run("without an area map nothing is classified", func(t *testing.T) {
		bare := exposureProject(t, env, "")
		res := env.run(bare, "exposure", "inventory", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-inventory.v1.schema.json", res.Stdout)
		assert.NotContains(t, res.Stdout, `"area"`)
		assert.NotContains(t, exposureArtifacts(t, res.Stdout), "ops:probe", "plugins default to the enabled ones")
	})

	t.Run("--root inspects a directory without a configuration", func(t *testing.T) {
		other := t.TempDir()
		writeTree(t, other, map[string]string{".claude/skills/lone/SKILL.md": "---\nname: lone\ndescription: Alone.\n---\n"})
		res := env.run(root, "exposure", "inventory", "--format", "json", "--root", other)
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		arts := exposureArtifacts(t, res.Stdout)
		assert.Contains(t, arts, "lone")
		assert.NotContains(t, arts, "charge")
	})
}

func TestExposureAreas(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, exposureAreasConfig)

	t.Run("json shows the map and the area and source of every artifact", func(t *testing.T) {
		res := env.run(root, "exposure", "areas", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-areas.v1.schema.json", res.Stdout)
		var doc struct {
			AreasFile  string                  `json:"areas_file"`
			Areas      []struct{ Name string } `json:"areas"`
			SkillAreas map[string]string       `json:"skill_areas"`
			Counts     map[string]int          `json:"counts"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc))
		assert.Empty(t, doc.AreasFile)
		require.Len(t, doc.Areas, 2)
		assert.Equal(t, map[string]string{"kit:solo": "billing"}, doc.SkillAreas)
		probe := exposureArtifacts(t, res.Stdout)["ops:probe"]
		assert.Equal(t, exposureArtifact{Kind: "skill", QualifiedName: "ops:probe", Path: "tools/plugins/ops/skills/probe/SKILL.md", Phase: "start", Area: "tooling", AreaSource: "path", AreaScope: "tools/plugins/ops"}, probe)
		assert.Positive(t, doc.Counts["unclassified"])
	})

	t.Run("--areas replaces the configured map", func(t *testing.T) {
		res := env.run(root, "exposure", "areas", "--format", "json", "--areas", filepath.Join(root, "areas.toml"))
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		arts := exposureArtifacts(t, res.Stdout)
		assert.Equal(t, "unclassified", arts["ops:probe"].Area, "a plugin skill is never placed by its description")
		assert.Equal(t, "unclassified", arts["charge"].Area)
		assert.Contains(t, res.Stdout, `"areas_file": "`+filepath.ToSlash(filepath.Join(root, "areas.toml")))
	})

	t.Run("text shows the map and the classification", func(t *testing.T) {
		res := env.run(root, "exposure", "areas")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		for _, want := range []string{"the [exposure] table", "AREA", "tooling", "unclassified", "kit:solo", "SOURCE"} {
			assert.Contains(t, res.Stdout, want)
		}
	})
}

func TestExposureFailures(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, "")
	writeTree(t, root, map[string]string{"bad.toml": "[[area]]\nname = \"a\"\n[[area]]\nname = \"a\"\n"})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"areas without a map", []string{"exposure", "areas"}, "no area map"},
		{"an invalid map names its line", []string{"exposure", "inventory", "--areas", "bad.toml"}, `bad.toml:4: duplicate area name "a"`},
		{"an unknown plugin mode", []string{"exposure", "inventory", "--plugins", "some"}, `unknown --plugins "some"`},
		{"a missing root", []string{"exposure", "inventory", "--root", "nope"}, "is not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := env.run(root, tt.args...)
			require.Equal(t, 1, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			assert.Contains(t, res.Stderr, tt.want)
		})
	}
}
