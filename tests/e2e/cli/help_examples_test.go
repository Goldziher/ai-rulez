package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

var mostUsedCommands = []string{"init", "generate", "validate", "lock", "approve", "verify", "publish", "import", "convert", "migrate", "add", "remove", "roles", "search", "telemetry"}

// exampleLines returns the invocations under "Examples:" of a help text.
func exampleLines(help string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(help, "\n") {
		switch {
		case strings.HasPrefix(line, "Examples:"):
			in = true
		case in && strings.HasPrefix(line, "  ai-rulez "):
			out = append(out, strings.TrimSpace(line))
		case in && strings.TrimSpace(line) == "":
			in = false
		}
	}
	return out
}

func TestCLI_MostUsedCommandsShowExamplesInHelp(t *testing.T) {
	dir := errorsProject(t, "")
	for _, name := range mostUsedCommands {
		res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), name, "--help")

		require.Equal(t, 0, res.ExitCode, name)
		assert.Contains(t, res.Stdout, "Examples:", name)
		assert.NotEmpty(t, exampleLines(res.Stdout), name)
	}
}

// The examples that only read (or write inside a scratch project) are run as
// written, so the help cannot describe an invocation that fails.
func TestCLI_ReadOnlyHelpExamplesRun(t *testing.T) {
	dir := errorsProject(t, "")
	env := isolatedEnv(t)
	runnable := map[string]bool{
		"ai-rulez validate":                       true,
		"ai-rulez validate --explain AR001":       true,
		"ai-rulez generate --dry-run":             true,
		"ai-rulez generate":                       true,
		"ai-rulez generate --check":               true,
		"ai-rulez lock":                           true,
		"ai-rulez lock --check":                   true,
		"ai-rulez lock --diff":                    true,
		"ai-rulez lock --content-only":            true,
		"ai-rulez approve --list":                 true,
		"ai-rulez convert --list":                 true,
		"ai-rulez roles list":                     true,
		"ai-rulez roles list --format json":       true,
		"ai-rulez telemetry status":               true,
		"ai-rulez telemetry doctor":               true,
		"ai-rulez add rule code-quality":          true,
		"ai-rulez remove rule code-quality --yes": true,
	}
	ran := 0
	for _, name := range mostUsedCommands {
		help := testutil.RunCLIWithEnv(t, dir, env, name, "--help")
		for _, line := range exampleLines(help.Stdout) {
			if !runnable[line] {
				continue
			}
			res := testutil.RunCLIWithEnv(t, dir, env, strings.Fields(line)[1:]...)
			assert.Equal(t, 0, res.ExitCode, "%s: %s%s", line, res.Stdout, res.Stderr)
			ran++
		}
	}
	assert.GreaterOrEqual(t, ran, 12, "most examples are runnable here")
}
