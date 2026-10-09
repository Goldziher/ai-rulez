package clidocs

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture() *cobra.Command {
	root := &cobra.Command{Use: "tool"}
	root.PersistentFlags().String("config-dir", "", "global flag")
	group := &cobra.Command{Use: "group", Short: "A group"}
	group.PersistentFlags().String("format", "text", "Output format: text, json")
	leaf := &cobra.Command{
		Use: "leaf <name>", Short: "Do a | thing", Aliases: []string{"l"},
		Example: "  tool group leaf one\n  tool group leaf two --yes",
		Run:     func(*cobra.Command, []string) {},
	}
	leaf.Flags().BoolP("yes", "y", false, "Skip the prompt")
	leaf.Flags().StringSlice("tags", nil, "Tags")
	leaf.Flags().Int("count", 3, "How many")
	hidden := &cobra.Command{Use: "secret", Short: "Hidden", Hidden: true}
	group.AddCommand(leaf, hidden)
	root.AddCommand(group)
	return root
}

func TestIndexListsEveryVisibleCommandAndEscapesPipes(t *testing.T) {
	got := Index(fixture())

	assert.Contains(t, got, "| `tool group` | A group |")
	assert.Contains(t, got, "| `tool group leaf` | Do a \\| thing |")
	assert.NotContains(t, got, "secret")
}

func TestReferenceShowsUsageAliasesExamplesAndFlags(t *testing.T) {
	got := Reference(fixture())

	assert.True(t, strings.HasPrefix(got, "# CLI Command Reference"))
	assert.Contains(t, got, "## `tool group leaf`")
	assert.Contains(t, got, "tool group leaf <name> [flags]")
	assert.Contains(t, got, "Aliases: `l`")
	assert.Contains(t, got, "```bash\ntool group leaf one\ntool group leaf two --yes\n```")
	assert.Contains(t, got, "| `--yes` / `-y` | bool |  | Skip the prompt |")
	assert.Contains(t, got, "| `--tags` | string list |  | Tags |")
	assert.Contains(t, got, "| `--count` | int | `3` | How many |")
	assert.Contains(t, got, "| `--format` | string | `text` | Output format: text, json |", "a group's persistent flag is listed on its subcommands")
	assert.NotContains(t, got, "global flag", "the root's global flags are documented once, in cli.md")
	assert.NotContains(t, got, "secret")
}

func TestReferenceIsDeterministic(t *testing.T) {
	assert.Equal(t, Reference(fixture()), Reference(fixture()))
}

func TestApplyIndexReplacesOnlyTheMarkedBlock(t *testing.T) {
	page := "before\n" + IndexBegin + "\nold\n" + IndexEnd + "\nafter\n"

	got, err := ApplyIndex(page, "NEW\n")
	require.NoError(t, err)

	assert.Equal(t, "before\n"+IndexBegin+"\n\nNEW\n\n"+IndexEnd+"\nafter\n", got)
	again, err := ApplyIndex(got, "NEW\n")
	require.NoError(t, err)
	assert.Equal(t, got, again, "applying twice changes nothing")
}

func TestApplyIndexFailsWithoutMarkers(t *testing.T) {
	_, err := ApplyIndex("no markers here", "x")

	require.Error(t, err)
}
