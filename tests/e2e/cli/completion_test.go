package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The built binary prints a script for every shell and answers the shell's
// completion request with enum values and the names found in the project.
func TestCompletionE2E(t *testing.T) {
	t.Run("the completion command prints a script for every shell", func(t *testing.T) {
		env := newIsoEnv(t)
		root := minimalProject(t, "")

		for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
			res := env.run(root, "completion", shell)

			assert.Equal(t, 0, res.ExitCode, shell+": "+res.Stderr)
			assert.Contains(t, res.Stdout, "ai-rulez", shell)
			assert.Greater(t, len(res.Stdout), 500, shell)
		}
	})

	t.Run("enum values and project names are offered", func(t *testing.T) {
		env := newIsoEnv(t)
		root := minimalProject(t, "")

		format := env.run(root, "__complete", "validate", "--format", "")
		rules := env.run(root, "__complete", "show", "rule", "")
		skills := env.run(root, "__complete", "remove", "skill", "d")

		require.Equal(t, 0, format.ExitCode, format.Stderr)
		assert.Contains(t, strings.Fields(format.Stdout), "sarif")
		assert.Contains(t, strings.Fields(rules.Stdout), "local")
		assert.Contains(t, strings.Fields(skills.Stdout), "deploy")
	})
}
