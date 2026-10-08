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

// resetFlags puts every flag a parse touched back to its default, so parsing an
// example leaves no state for the tests that follow.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if !f.Changed {
			return
		}
		_, isFormat := f.Annotations[formatValuesAnnotation]
		switch sv, isSlice := f.Value.(pflag.SliceValue); {
		case isSlice:
			_ = sv.Replace(nil)
		case isFormat && f.Value.Set("") == nil:
			// addFormatFlag shows "text" as the default, but the variable's real default is empty.
			// Restoring "text" would leave validate --format set, which only --strict accepts.
		default:
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.InheritedFlags().VisitAll(reset)
}

func TestHelpExamplesNameRealCommandsAndFlags(t *testing.T) {
	examples := helpExamples()
	require.Len(t, examples, 13)
	for cmd, text := range examples {
		assert.Equal(t, text, cmd.Example, "%s carries its examples", cmd.CommandPath())
		for _, line := range strings.Split(text, "\n") {
			words := splitExample(strings.TrimSpace(line))
			require.Greater(t, len(words), 1, line)
			require.Equal(t, "ai-rulez", words[0], line)

			found, rest, err := RootCmd.Find(words[1:])
			require.NoError(t, err, line)
			require.NoError(t, found.ParseFlags(rest), line)
			require.NoError(t, found.ValidateArgs(found.Flags().Args()), line)
			assert.Contains(t, found.CommandPath(), cmd.Name(), line)
			resetFlags(found)
		}
	}
}

func TestEveryMostUsedCommandHasExamples(t *testing.T) {
	for _, name := range []string{"init", "generate", "validate", "lock", "approve", "verify", "publish", "import", "convert", "migrate", "add", "remove", "roles", "search", "telemetry"} {
		c, _, err := RootCmd.Find([]string{name})
		require.NoError(t, err, name)
		assert.Contains(t, c.Example, "ai-rulez "+name, name)
	}
}
