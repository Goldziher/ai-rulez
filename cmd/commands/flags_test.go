package commands_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/cmd/commands"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRUDCommandShorthands(t *testing.T) {
	tests := []struct {
		cmd   *cobra.Command
		path  []string
		flags map[string]string
	}{
		{commands.AddCmd, []string{"rule"}, map[string]string{"domain": "d", "priority": "p", "targets": "t", "content": "c"}},
		{commands.AddCmd, []string{"context"}, map[string]string{"domain": "d", "priority": "p", "content": "c"}},
		{commands.AddCmd, []string{"skill"}, map[string]string{"domain": "d", "description": "s", "content": "c"}},
		{commands.RemoveCmd, []string{"rule"}, map[string]string{"domain": "d", "force": "f"}},
		{commands.ListCmd, []string{"rules"}, map[string]string{"domain": "d", "json": "j"}},
		{commands.DomainCmd, []string{"add"}, map[string]string{"description": "s"}},
		{commands.DomainCmd, []string{"remove"}, map[string]string{"force": "f"}},
		{commands.DomainCmd, []string{"list"}, map[string]string{"json": "j"}},
		{commands.IncludeCmd, []string{"add"}, map[string]string{"path": "p", "ref": "r", "include": "i", "merge-strategy": "m", "install-to": "t"}},
		{commands.IncludeCmd, []string{"remove"}, map[string]string{"force": "f"}},
		{commands.IncludeCmd, []string{"list"}, map[string]string{"json": "j"}},
		{commands.ProfileCmd, []string{"add"}, map[string]string{"set-default": "s"}},
		{commands.ProfileCmd, []string{"remove"}, map[string]string{"force": "f"}},
		{commands.ProfileCmd, []string{"list"}, map[string]string{"json": "j"}},
		{commands.SkillCmd, []string{"install"}, map[string]string{"source": "s", "path": "p", "ref": "r"}},
		{commands.SkillCmd, []string{"remove"}, map[string]string{"force": "f"}},
		{commands.SkillCmd, []string{"list"}, map[string]string{"json": "j"}},
		{commands.BuiltinsCmd, []string{"list"}, map[string]string{"json": "j"}},
		{commands.BuiltinsCmd, []string{"show"}, map[string]string{"json": "j"}},
	}

	for _, tt := range tests {
		cmd := findCommand(t, tt.cmd, tt.path...)
		for name, shorthand := range tt.flags {
			flag := cmd.Flags().Lookup(name)
			require.NotNil(t, flag, "%s missing --%s", cmd.CommandPath(), name)
			assert.Equal(t, shorthand, flag.Shorthand, "%s --%s", cmd.CommandPath(), name)
		}
	}
}

func findCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cmd := root
	for _, segment := range path {
		var next *cobra.Command
		for _, candidate := range cmd.Commands() {
			use := strings.Fields(candidate.Use)
			if len(use) > 0 && use[0] == segment {
				next = candidate
				break
			}
		}
		require.NotNil(t, next, "missing command %s under %s", segment, cmd.CommandPath())
		cmd = next
	}
	return cmd
}
