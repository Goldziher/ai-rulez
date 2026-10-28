package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collisionsAreas is an inline area map whose billing and search skills share most of their listing words.
const collisionsAreas = `
[[exposure.areas]]
name = "billing"
terms = ["invoice"]

[[exposure.areas]]
name = "search"
terms = ["ranker"]
`

// collisionsProject is a project with that map and three listings at session start, left ungenerated so no other
// skill is listed; exposureKeys are added to its [exposure] table.
func collisionsProject(t *testing.T, exposureKeys string) string {
	t.Helper()
	root := minimalProject(t, "\n[exposure]\nplugins = \"none\"\n"+exposureKeys+collisionsAreas)
	writeTree(t, root, map[string]string{
		".claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Look up customer invoice details by name.\n---\nbody\n",
		".claude/skills/rank/SKILL.md":   "---\nname: rank\ndescription: Look up customer ranker details by name.\n---\nbody\n",
		".claude/agents/finder.md":       "---\nname: finder\ndescription: Finds customer invoice records.\n---\nbody\n",
	})
	return root
}

type collisionsDoc struct {
	MinScore          float64 `json:"min_score"`
	ArtifactsCompared int     `json:"artifacts_compared"`
	Total             int     `json:"total"`
	Collisions        []struct {
		Score      float64  `json:"score"`
		Shared     []string `json:"shared"`
		First      string   `json:"first"`
		FirstArea  string   `json:"first_area"`
		Second     string   `json:"second"`
		SecondArea string   `json:"second_area"`
	} `json:"collisions"`
}

func TestExposureCollisions(t *testing.T) {
	env := newIsoEnv(t)
	root := collisionsProject(t, "")

	t.Run("json ranks the pairs of different areas", func(t *testing.T) {
		res := env.run(root, "exposure", "collisions", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		validateAgainstSchema(t, "exposure-collisions.v1.schema.json", res.Stdout)
		var doc collisionsDoc
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), res.Stdout)
		assert.Equal(t, 3, doc.ArtifactsCompared)
		assert.Equal(t, 2, doc.Total)
		require.Len(t, doc.Collisions, 2)
		top := doc.Collisions[0]
		assert.InDelta(t, 0.5, top.Score, 0)
		assert.Equal(t, []string{"customer", "details", "look", "name"}, top.Shared)
		assert.Equal(t, ".claude/skills/charge/SKILL.md", top.First)
		assert.Equal(t, "billing", top.FirstArea)
		assert.Equal(t, ".claude/skills/rank/SKILL.md", top.Second)
		assert.Equal(t, "search", top.SecondArea)
		assert.InDelta(t, 0.1, doc.Collisions[1].Score, 0)
	})
	t.Run("--min-score and --limit filter", func(t *testing.T) {
		res := env.run(root, "exposure", "collisions", "--format", "json", "--min-score", "0.2", "--limit", "5")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		var doc collisionsDoc
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), res.Stdout)
		assert.Equal(t, 1, doc.Total)
		require.Len(t, doc.Collisions, 1)

		res = env.run(root, "exposure", "collisions", "--format", "json", "--limit", "1")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), res.Stdout)
		assert.Equal(t, 2, doc.Total)
		assert.Len(t, doc.Collisions, 1)
	})
	t.Run("text names both sides and the shared words", func(t *testing.T) {
		res := env.run(root, "exposure", "collisions")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		for _, want := range []string{"2 of 2 pairs among 3 listings", "0.50", "skill charge", "skill rank", "customer, details, look, name"} {
			assert.Contains(t, res.Stdout, want)
		}
	})
	t.Run("the configured threshold is the default", func(t *testing.T) {
		configured := collisionsProject(t, "collision_min_score = 0.3\n")
		res := env.run(configured, "exposure", "collisions", "--format", "json")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
		var doc collisionsDoc
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), res.Stdout)
		assert.InDelta(t, 0.3, doc.MinScore, 0)
		assert.Equal(t, 1, doc.Total)
	})
}

func TestExposureCollisionsFailures(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, "")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"without a map", []string{"exposure", "collisions"}, "no area map"},
		{"a score above one", []string{"exposure", "collisions", "--areas", "areas.toml", "--min-score", "2"}, "--min-score must be between 0 and 1"},
		{"a negative limit", []string{"exposure", "collisions", "--areas", "areas.toml", "--limit", "-1"}, "--limit must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := env.run(root, tt.args...)
			require.Equal(t, 1, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			assert.Contains(t, res.Stderr, tt.want)
		})
	}
}

func TestValidateReportsExposureAreaCollisions(t *testing.T) {
	env := newIsoEnv(t)
	tests := []struct {
		name  string
		extra string
		want  int
	}{
		{"off without collision_min_score", "", 0},
		{"a warning per pair at or over collision_min_score", "collision_min_score = 0.1\n", 2},
		{"a higher threshold keeps fewer pairs", "collision_min_score = 0.3\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := collisionsProject(t, tt.extra)
			res := env.run(root, "validate", "--format", "json")
			var doc struct {
				Findings []struct {
					Code     string `json:"code"`
					Severity string `json:"severity"`
					File     string `json:"file"`
					Message  string `json:"message"`
				} `json:"findings"`
			}
			require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), res.Stdout+res.Stderr)
			got := 0
			for _, f := range doc.Findings {
				if f.Code != "AR9W2" {
					continue
				}
				got++
				assert.Equal(t, "warning", f.Severity)
				assert.True(t, strings.HasSuffix(f.File, ".claude/skills/charge/SKILL.md") || strings.HasSuffix(f.File, ".claude/agents/finder.md"), f.File)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
