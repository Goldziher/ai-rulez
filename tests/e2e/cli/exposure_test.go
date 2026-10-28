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

	t.Run("an empty directory prints an empty artifact list", func(t *testing.T) {
		for _, args := range [][]string{nil, {"--with-text"}} {
			res := env.run(root, append([]string{"exposure", "inventory", "--format", "json", "--root", t.TempDir()}, args...)...)
			require.Equal(t, 0, res.ExitCode, res.Stderr)
			validateAgainstSchema(t, "exposure-inventory.v1.schema.json", res.Stdout)
			assert.Contains(t, res.Stdout, `"artifacts": []`)
		}
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
		assert.Contains(t, res.Stdout, `"areas_file": "areas.toml"`, "relative to the root, whatever the flag says")
	})

	t.Run("--root below the project uses the project's map", func(t *testing.T) {
		res := env.run(root, "exposure", "areas", "--format", "json", "--root", filepath.Join("services", "billing"))
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-areas.v1.schema.json", res.Stdout)
		arts := exposureArtifacts(t, res.Stdout)
		require.Contains(t, arts, "charge")
		assert.NotContains(t, arts, "ops:probe", "the inventory is of --root")
		assert.Equal(t, exposureArtifact{
			Kind: "skill", QualifiedName: "charge", Path: ".claude/skills/charge/SKILL.md", Phase: "start",
			Area: "billing", AreaSource: "path", AreaScope: "services/billing",
		}, arts["charge"], "the claim services/billing of the project's map owns what lies at --root")
	})

	t.Run("-C names the map's configuration for another --root", func(t *testing.T) {
		other := t.TempDir()
		res := env.run(other, "-C", filepath.Join(root, ".ai-rulez"), "exposure", "areas", "--format", "json", "--root", other)
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-areas.v1.schema.json", res.Stdout)
		assert.Contains(t, res.Stdout, `"name": "billing"`)
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
	writeTree(t, root, map[string]string{
		"bad.toml":           "[[area]]\nname = \"a\"\n[[area]]\nname = \"a\"\n",
		"badcfg/config.toml": "version = \"5.0\"\nname = \"bad\"\n\n[exposure]\nplugins = \"some\"\n",
		"empty/.gitkeep":     "",
	})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"areas without a map", []string{"exposure", "areas"}, "no area map"},
		{"an invalid map names its line", []string{"exposure", "inventory", "--areas", "bad.toml"}, `bad.toml:4: duplicate area name "a"`},
		{"an unknown plugin mode", []string{"exposure", "inventory", "--plugins", "some"}, `unknown --plugins "some"`},
		{"a missing root", []string{"exposure", "inventory", "--root", "nope"}, "is not a directory"},
		{"an invalid -C with --root", []string{"-C", "badcfg/config.toml", "exposure", "inventory", "--root", "empty"}, "AR9W5"},
		{"a missing -C with --root", []string{"-C", "nope.toml", "exposure", "inventory", "--root", "empty"}, "does not exist"},
		{"--with-text without json", []string{"exposure", "inventory", "--with-text"}, "--with-text applies to --format json"},
		{"an unknown harness", []string{"exposure", "--harness", "codex"}, `unknown --harness "codex"`},
		{"a negative context window", []string{"exposure", "--context-window", "-5"}, "--context-window must not be negative"},
		{"an unknown subcommand", []string{"exposure", "inventroy"}, `unknown command "inventroy"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := env.run(root, tt.args...)
			require.Equal(t, 1, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			assert.Contains(t, res.Stderr, tt.want)
		})
	}
}

type exposureReportDoc struct {
	Plugins struct {
		Loaded []string `json:"loaded"`
	} `json:"plugins"`
	StartBudget int `json:"start_budget"`
	Listing     struct {
		ContextWindow   int `json:"context_window"`
		Skills          int `json:"skills"`
		Chars           int `json:"chars"`
		BudgetChars     int `json:"budget_chars"`
		OverBudgetChars int `json:"over_budget_chars"`
	} `json:"listing"`
	StartTokens          int                       `json:"start_tokens"`
	StartTokensByKind    map[string]int            `json:"start_tokens_by_kind"`
	CountsByPhaseAndKind map[string]map[string]int `json:"counts_by_phase_and_kind"`
	Items                []struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		Phase string `json:"phase"`
		Area  string `json:"area"`
	} `json:"items"`
}

func exposureReport(t *testing.T, stdout string) exposureReportDoc {
	t.Helper()
	var doc exposureReportDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	return doc
}

func TestExposureReport(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, exposureAreasConfig)

	t.Run("json reports the start of the session and the listing budget", func(t *testing.T) {
		res := env.run(root, "exposure", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-report.v1.schema.json", res.Stdout)
		doc := exposureReport(t, res.Stdout)
		assert.Equal(t, []string{"ops", "kit"}, doc.Plugins.Loaded)
		assert.Equal(t, 200000, doc.Listing.ContextWindow)
		assert.Equal(t, 8000, doc.Listing.BudgetChars)
		assert.Equal(t, doc.CountsByPhaseAndKind["start"]["skill"], doc.Listing.Skills)
		assert.GreaterOrEqual(t, doc.Listing.Skills, 3, "the generated deploy skill and both plugin skills")
		assert.Positive(t, doc.Listing.Chars)
		assert.Zero(t, doc.StartBudget)
		sum := 0
		for _, n := range doc.StartTokensByKind {
			sum += n
		}
		assert.Equal(t, doc.StartTokens, sum)
		areas := map[string]string{}
		for _, it := range doc.Items {
			areas[it.Name] = it.Area
		}
		assert.Equal(t, "tooling", areas["ops:probe"], "items carry the area of the configured map")
	})

	t.Run("--context-window scales the listing budget", func(t *testing.T) {
		res := env.run(root, "exposure", "--format", "json", "--context-window", "1000")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		doc := exposureReport(t, res.Stdout)
		assert.Equal(t, 40, doc.Listing.BudgetChars)
		assert.Equal(t, doc.Listing.Chars-40, doc.Listing.OverBudgetChars)
	})

	t.Run("--plugins none leaves the plugin skills out", func(t *testing.T) {
		res := env.run(root, "exposure", "--format", "json", "--plugins", "none")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		for _, it := range exposureReport(t, res.Stdout).Items {
			assert.NotEqual(t, "ops:probe", it.Name)
		}
	})

	t.Run("text names the totals and the heaviest artifacts", func(t *testing.T) {
		res := env.run(root, "exposure")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		for _, want := range []string{"start of session:", "skill listing:", "of 8000 budget (200000 token window)", "start tokens by kind:", "artifacts by phase and kind:", "heaviest at start:"} {
			assert.Contains(t, res.Stdout, want)
		}
	})

	t.Run("the subcommands still run", func(t *testing.T) {
		for _, sub := range []string{"inventory", "areas"} {
			res := env.run(root, "exposure", sub, "--format", "json")
			require.Equal(t, 0, res.ExitCode, "%s: %s", sub, res.Stderr)
			assert.NotContains(t, res.Stdout, `"start_tokens"`, sub)
		}
	})
}

func TestExposureStartBudget(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, "\n[exposure]\nstart_budget = 1\n")

	t.Run("the report exits 2 over the budget", func(t *testing.T) {
		res := env.run(root, "exposure", "--format", "json")
		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		validateAgainstSchema(t, "exposure-report.v1.schema.json", res.Stdout)
		doc := exposureReport(t, res.Stdout)
		assert.Equal(t, 1, doc.StartBudget)
		assert.Greater(t, doc.StartTokens, 1)
	})

	t.Run("text says by how much", func(t *testing.T) {
		res := env.run(root, "exposure")
		require.Equal(t, 2, res.ExitCode, res.Stderr)
		assert.Contains(t, res.Stdout, "start budget: OVER by")
	})

	t.Run("validate reports AR9W3", func(t *testing.T) {
		res := env.run(root, "validate")
		assert.Equal(t, 2, res.ExitCode, res.Stdout+res.Stderr)
		assert.Contains(t, res.Stdout+res.Stderr, "AR9W3")
	})

	t.Run("within the budget exits 0 and validate is quiet", func(t *testing.T) {
		roomy := exposureProject(t, env, "\n[exposure]\nstart_budget = 1000000\n")
		res := env.run(roomy, "exposure")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		assert.Contains(t, res.Stdout, "start budget: within")
		assert.NotContains(t, env.run(roomy, "validate").Stdout, "AR9W3")
	})
}
