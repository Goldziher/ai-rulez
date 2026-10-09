package commands_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCRUDCommandShorthands pins the shorthand diet: -y is the only one the CRUD
// commands keep (flag_taxonomy_test.go walks the whole tree).
func TestCRUDCommandShorthands(t *testing.T) {
	tests := []struct {
		cmd   *cobra.Command
		path  []string
		flags map[string]string
	}{
		{commands.AddCmd, []string{"rule"}, map[string]string{"domain": "", "priority": "", "targets": "", "content": ""}},
		{commands.AddCmd, []string{"context"}, map[string]string{"domain": "", "priority": "", "content": ""}},
		{commands.AddCmd, []string{"skill"}, map[string]string{"domain": "", "description": "", "content": ""}},
		{commands.RemoveCmd, []string{"rule"}, map[string]string{"domain": "", "yes": "y"}},
		{commands.ListCmd, []string{"rules"}, map[string]string{"domain": "", "format": ""}},
		{commands.DomainCmd, []string{"add"}, map[string]string{"description": ""}},
		{commands.DomainCmd, []string{"remove"}, map[string]string{"yes": "y"}},
		{commands.DomainCmd, []string{"list"}, map[string]string{"format": ""}},
		{commands.IncludeCmd, []string{"add"}, map[string]string{"path": "", "ref": "", "include": "", "merge-strategy": "", "install-to": ""}},
		{commands.IncludeCmd, []string{"remove"}, map[string]string{"yes": "y"}},
		{commands.IncludeCmd, []string{"list"}, map[string]string{"format": ""}},
		{commands.ProfileCmd, []string{"add"}, map[string]string{"set-default": ""}},
		{commands.ProfileCmd, []string{"remove"}, map[string]string{"yes": "y"}},
		{commands.ProfileCmd, []string{"list"}, map[string]string{"format": ""}},
		{commands.SkillCmd, []string{"install"}, map[string]string{"source": "", "path": "", "ref": ""}},
		{commands.SkillCmd, []string{"remove"}, map[string]string{"yes": "y"}},
		{commands.SkillCmd, []string{"list"}, map[string]string{"format": ""}},
		{commands.BuiltinsCmd, []string{"list"}, map[string]string{"format": ""}},
		{commands.BuiltinsCmd, []string{"show"}, map[string]string{"format": ""}},
		// tokens is not CRUD, but its shorthands have to stay consistent with the
		// rest of the tree: --format for JSON, -p for profile, -n for the config dir.
		{commands.TokensCmd, nil, map[string]string{"format": "", "budget": "", "profile": "", "config-dir": ""}},
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
