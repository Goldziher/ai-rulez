package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestMCPCommand_DynamicSkillLoadingFlags(t *testing.T) {
	flags := commands.MCPCmd.Flags()
	for _, name := range []string{"source", "role", "frozen", "offline", "include-static", "budget-bytes", "usage-log", "usage-sink", "no-watch", "reload-interval"} {
		f := flags.Lookup(name)
		if assert.NotNil(t, f, "mcp is missing --%s", name) {
			assert.Contains(t, f.Usage, "--serve-skills", "--%s must say it needs --serve-skills", name)
		}
	}
	assert.Equal(t, "stringArray", flags.Lookup("source").Value.Type(), "--source repeats and keeps '#' and ',' intact")
}
