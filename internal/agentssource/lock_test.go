package agentssource_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

func TestLockPinsANestedAGENTSMD(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{"svc/AGENTS.md": "# Service\n\nText.\n"})

	// Act
	snap, err := contentlock.Compute(load(t, root), contentlock.Options{})
	require.NoError(t, err)

	// Assert
	paths := map[string]string{}
	for _, item := range snap.Items {
		paths[item.Kind+":"+item.ID] = item.Path
	}
	assert.Equal(t, "svc/AGENTS.md", paths["context:agents-svc"])
}

func TestLockPinsAGENTSMDAndTheAgentsTree(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := setup(t, layout.cfgPath, layout.cfgBody)

			// Act
			snap, err := contentlock.Compute(load(t, root), contentlock.Options{})
			require.NoError(t, err)

			// Assert
			paths := map[string]string{}
			for _, item := range snap.Items {
				paths[item.Kind+":"+item.ID] = item.Path
			}
			assert.Equal(t, "AGENTS.md", paths["context:agents"])
			assert.Equal(t, ".agents/skills/review", paths["skill:review"])
			assert.Equal(t, ".agents/rules/go-style.md", paths["rule:go-style"])

			// Act: an edit to a file of the standard moves its pin
			write(t, root, ".agents/skills/review/SKILL.md", handSkill+"\nMore.\n")
			after, err := contentlock.Compute(load(t, root), contentlock.Options{})
			require.NoError(t, err)

			// Assert
			assert.NotEqual(t, snap.Items, after.Items)
		})
	}
}
