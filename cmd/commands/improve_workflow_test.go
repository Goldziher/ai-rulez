package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/improve/adapter"
)

// aiRulezInvocations returns the lines of a shell recipe that start with ai-rulez, with continuation lines joined and
// everything from a redirection, pipe or comment on cut off.
func aiRulezInvocations(text string) [][]string {
	var out [][]string
	joined := strings.ReplaceAll(text, "\\\n", " ")
	for _, line := range strings.Split(joined, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "ai-rulez" {
			continue
		}
		const start = 0
		var args []string
		for _, f := range fields[start+1:] {
			if f == ">" || f == "|" || f == ";" || f == "<" || strings.HasPrefix(f, "#") {
				break
			}
			args = append(args, f)
		}
		out = append(out, args)
	}
	return out
}

// TestRepairWorkflowTemplate_UsesOnlyFlagsThatExist keeps the shipped CI recipe from drifting away from the CLI.
func TestRepairWorkflowTemplate_UsesOnlyFlagsThatExist(t *testing.T) {
	// Arrange
	text, ok := adapter.Template(adapter.RepairWorkflow)
	require.True(t, ok)
	invocations := aiRulezInvocations(text)
	require.GreaterOrEqual(t, len(invocations), 3, "eval run, improve run and improve pr")

	for _, args := range invocations {
		t.Run(strings.Join(args[:min(len(args), 2)], " "), func(t *testing.T) {
			// Act
			sub, rest, err := RootCmd.Find(args)
			require.NoError(t, err)
			require.NotSame(t, RootCmd, sub, "unknown subcommand in %v", args)

			// Assert: every flag the recipe passes is declared by the command it reaches
			for _, a := range rest {
				name, found := strings.CutPrefix(a, "--")
				if !found {
					continue
				}
				name, _, _ = strings.Cut(name, "=")
				assert.NotNil(t, sub.Flags().Lookup(name), "%s has no --%s", sub.CommandPath(), name)
			}
		})
	}
}
