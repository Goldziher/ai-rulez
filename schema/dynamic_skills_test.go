package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestSchema_DynamicSkillLoading(t *testing.T) {
	head := "version: \"4.0\"\nname: p\npresets: [claude]\n"

	valid := head + `
skills:
  delivery: served
domains:
  billing:
    delivery: both
lock:
  enforce: true
skill_sources:
  - name: team
    url: git+https://example.com/org/skills
    ref: v1.2.0
    path: skills
    include: ["pdf-*"]
    exclude: ["*-wip"]
    name_prefix: team-
    trust: warn
`
	require.NoError(t, schema.ValidateWithSchema([]byte(valid)))

	for name, bad := range map[string]string{
		"global delivery":  head + "skills:\n  delivery: lazy\n",
		"domain delivery":  head + "domains:\n  billing:\n    delivery: sometimes\n",
		"unknown skills":   head + "skills:\n  mode: served\n",
		"source no url":    head + "skill_sources:\n  - name: team\n",
		"source bad trust": head + "skill_sources:\n  - name: team\n    url: x\n    trust: off\n",
		"source bad name":  head + "skill_sources:\n  - name: Team Skills\n    url: x\n",
		"source extra key": head + "skill_sources:\n  - name: t\n    url: x\n    branch: main\n",
		"lock extra key":   head + "lock:\n  enforce: true\n  mode: strict\n",
	} {
		assert.Error(t, schema.ValidateWithSchema([]byte(bad)), name)
	}
}
