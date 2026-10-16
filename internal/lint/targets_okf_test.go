package lint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectedTargetsExcludeOnlyGeneratedOKFListings(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".claude/rules/index.md":   "# Rules\n\n* [Rule](rule.md) - A rule\n",
		".claude/agents/log.md":    "# Log\n\n## 2026-10-01\n\n* [Agent](agent.md) - changed\n",
		".claude/rules/guide.md":   "# Guide\n\n* [Rule](rule.md) - A rule\n",
		".claude/context/index.md": "# Index\n\nThis is authored context, not a directory listing.\n",
	})
	tree, err := LoadTree(root)
	require.NoError(t, err)
	targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Detect: true, Cwd: root})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".claude/rules/guide.md", ".claude/context/index.md"}, targets.Rels())
	assert.ElementsMatch(t, []string{".claude/rules/index.md", ".claude/agents/log.md"}, targets.GeneratedRels)
	outputs, err := ResolveTargets(context.Background(), tree, TargetSpec{Detect: true, Generated: true, Cwd: root})
	require.NoError(t, err)
	assert.Len(t, outputs.Generated, 2, "--outputs can inspect the generated listings")
}
