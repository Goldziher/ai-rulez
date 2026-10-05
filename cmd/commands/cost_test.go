package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/cost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetCostFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		costFormat, costTarget, costTop, costBudget, costOnDemandBudget = "text", "", 10, 0, 0
		profile = ""
	})
}

func runCostOn(t *testing.T, root string) (string, bool, error) {
	t.Helper()
	var out bytes.Buffer
	CostCmd.SetOut(&out)
	exceeded, err := runCost(CostCmd, []string{filepath.Join(root, ".ai-rulez", "config.toml")})
	return out.String(), exceeded, err
}

func TestCostCommand(t *testing.T) {
	resetCostFlags(t)
	root, _ := strictProject(t, "", map[string]string{
		".ai-rulez/rules/a.md":                "---\ndescription: a rule\n---\n" + "Always be careful. Always be careful. Always be careful.\n",
		".ai-rulez/skills/big-skill/SKILL.md": "---\nname: big-skill\ndescription: Use when you need the big skill for tests.\n---\n" + "Long body. Long body. Long body. Long body.\n",
	})

	costFormat = "json"
	out, exceeded, err := runCostOn(t, root)
	require.NoError(t, err)
	assert.False(t, exceeded)
	var rep cost.Report
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	assert.Equal(t, "claude", rep.Target)
	assert.NotEmpty(t, rep.TopAlways)

	costFormat, costBudget = "markdown", 1
	out, exceeded, err = runCostOn(t, root)
	require.NoError(t, err)
	assert.True(t, exceeded, "over budget exits 2 via the caller")
	assert.Contains(t, out, "OVER BUDGET")

	costFormat, costBudget, costTarget = "text", 0, "nope"
	_, _, err = runCostOn(t, root)
	assert.Error(t, err)
	costTarget, costFormat = "", "xml"
	_, _, err = runCostOn(t, root)
	assert.Error(t, err)

	for _, name := range []string{"format", "target", "top", "budget", "on-demand-budget", "profile", "no-local", "config-dir"} {
		assert.NotNil(t, CostCmd.Flags().Lookup(name), name)
	}
}
