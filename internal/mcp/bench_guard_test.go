package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

// TestBenchSkillServerLoadsPastTheDefaultBudget keeps BenchmarkSkillServer from
// failing once it has loaded more than the default session budget: the
// benchmark repeats load_skill thousands of times on one session.
func TestBenchSkillServerLoadsPastTheDefaultBudget(t *testing.T) {
	big := generator.ServedSkillFile{RelPath: "references/big.md", Content: []byte(strings.Repeat("x", 100_000))}
	cat, err := BuildCatalog("p", "claude", []generator.ServedSkill{servedSkill("big", "", "A skill with a big resource", nil, big)}, SkillFilter{})
	require.NoError(t, err)
	session := connect(t, benchSkillServer(cat))

	for range 5 {
		res := call(t, session, "load_skill", map[string]any{"name": "big", "path": "references/big.md"})
		require.False(t, res.IsError, firstText(res))
	}

	res := call(t, connect(t, NewSkillServer("bench", cat)), "load_skill", map[string]any{"name": "big", "path": "references/big.md"})
	assert.False(t, res.IsError, "a first load fits the default budget: %s", firstText(res))
}
