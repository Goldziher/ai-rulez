package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitExample splits an example line into words, keeping a quoted phrase whole.
func splitExample(line string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote = r
		case quote == 0 && (r == ' ' || r == '\t'):
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// initialFlagValues holds every flag's value before any test parses one (taken
// in TestMain). A flag's DefValue is not always its initial value: addFormatFlag
// shows "text" while some commands start empty, so resetFlags restores from here.
var initialFlagValues = map[*pflag.Flag][]string{}

// snapshotFlagValues records the current value of every flag in the tree.
func snapshotFlagValues(root *cobra.Command) {
	record := func(f *pflag.Flag) {
		if _, seen := initialFlagValues[f]; seen {
			return
		}
		if sv, isSlice := f.Value.(pflag.SliceValue); isSlice {
			initialFlagValues[f] = append([]string{}, sv.GetSlice()...)
			return
		}
		initialFlagValues[f] = []string{f.Value.String()}
	}
	walkCommands(root, func(c *cobra.Command) {
		c.Flags().VisitAll(record)
		c.PersistentFlags().VisitAll(record)
	})
}

// resetFlags puts every flag a parse touched back to its initial value, so
// parsing an example leaves no state for the tests that follow.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if !f.Changed {
			return
		}
		initial, known := initialFlagValues[f]
		switch sv, isSlice := f.Value.(pflag.SliceValue); {
		case isSlice:
			_ = sv.Replace(initial)
		case known:
			_ = f.Value.Set(initial[0])
		default:
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.InheritedFlags().VisitAll(reset)
}

// exampleCommands splits an example line into the commands it runs: a pipeline
// contributes each stage that starts with ai-rulez, a redirection is dropped. Comment lines contribute none.
func exampleCommands(line string) [][]string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	var cmds [][]string
	for _, stage := range strings.Split(line, " | ") {
		stage, _, _ = strings.Cut(stage, " > ") // a redirection is not part of the command
		if i := strings.Index(stage, "<(ai-rulez "); i >= 0 {
			stage = strings.TrimSuffix(stage[i+2:], ")") // source <(ai-rulez completion bash)
		}
		if words := splitExample(strings.TrimSpace(stage)); len(words) > 0 && words[0] == "ai-rulez" {
			cmds = append(cmds, words)
		}
	}
	return cmds
}

// documentedCommands lists the commands help shows: everything but hidden
// commands and cobra's generated help.
func documentedCommands() []*cobra.Command {
	var all []*cobra.Command
	walkCommands(RootCmd, func(c *cobra.Command) {
		if !c.Hidden && c.Name() != "help" {
			all = append(all, c)
		}
	})
	return all
}

func TestHelpExamplesNameRealCommandsAndFlags(t *testing.T) {
	for _, cmd := range documentedCommands() {
		for _, line := range strings.Split(cmd.Example, "\n") {
			for _, words := range exampleCommands(line) {
				found, rest, err := RootCmd.Find(words[1:])
				require.NoError(t, err, "%s: %s", cmd.CommandPath(), line)
				require.NoError(t, found.ParseFlags(rest), "%s: %s", cmd.CommandPath(), line)
				require.NoError(t, found.ValidateArgs(found.Flags().Args()), "%s: %s", cmd.CommandPath(), line)
				assert.True(t, strings.HasPrefix(found.CommandPath(), cmd.CommandPath()),
					"%s shows an example for %s: %s", cmd.CommandPath(), found.CommandPath(), line)
				resetFlags(found)
			}
		}
	}
}

// Every command in the tree, groups and leaves, carries an Example, so `--help`
// always shows how to run it. A new command without one fails here.
func TestEveryCommandHasAnExample(t *testing.T) {
	for _, cmd := range documentedCommands() {
		assert.NotEmpty(t, strings.TrimSpace(cmd.Example), "%s has no Example (add it to helpExamples)", cmd.CommandPath())
	}
}

func TestEveryExampleLineIsIndentedAndRunsTheCommand(t *testing.T) {
	for _, cmd := range documentedCommands() {
		for _, line := range strings.Split(cmd.Example, "\n") {
			assert.True(t, strings.HasPrefix(line, "  "), "%s: example line %q is not indented by two spaces", cmd.CommandPath(), line)
			assert.Contains(t, line, "ai-rulez", "%s: example line %q does not run ai-rulez", cmd.CommandPath(), line)
		}
	}
}

func TestHelpExampleTableNamesRealCommands(t *testing.T) {
	for path, text := range helpExamples() {
		cmd, _, err := RootCmd.Find(strings.Fields(path))
		require.NoError(t, err, path)
		assert.Equal(t, strings.TrimSpace("ai-rulez "+path), cmd.CommandPath(), "helpExamples key %q names no command", path)
		assert.Equal(t, text, cmd.Example, "%s keeps the example of the table (a command with its own Example must not be in it)", path)
	}
}

// A Short is one sentence fragment of the command list: it starts with a capital
// letter or a parenthesis, does not end with a period and fits one line.
func TestShortDescriptionsReadAsOneLine(t *testing.T) {
	for _, cmd := range documentedCommands() {
		short := cmd.Short
		require.NotEmpty(t, short, "%s has no Short", cmd.CommandPath())
		assert.LessOrEqual(t, len(short), 100, "%s Short is too long", cmd.CommandPath())
		assert.False(t, strings.HasSuffix(short, "."), "%s Short ends with a period", cmd.CommandPath())
		first := short[:1]
		assert.True(t, first == "(" || first == strings.ToUpper(first), "%s Short %q does not start with a capital", cmd.CommandPath(), short)
		for _, bad := range []string{" a agent", " a a ", "Add new "} {
			assert.NotContains(t, short, bad, "%s Short %q has a grammar slip", cmd.CommandPath(), short)
		}
	}
}

// Every group command says what its subcommands are for in Long, so the help of
// `ai-rulez <group>` is more than the list of names.
func TestEveryGroupCommandHasALongDescription(t *testing.T) {
	for _, cmd := range documentedCommands() {
		if cmd != RootCmd && cmd.HasAvailableSubCommands() && cmd.Name() != "completion" {
			assert.NotEmpty(t, strings.TrimSpace(cmd.Long), "%s is a group without a Long", cmd.CommandPath())
		}
	}
}
