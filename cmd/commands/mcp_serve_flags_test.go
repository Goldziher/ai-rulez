package commands

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeSetupFromFlags_MaxCloneBytes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int64
	}{
		{"unset leaves the environment variable and the default in charge", nil, 0},
		{"the flag sets the limit", []string{"--max-clone-bytes", "1048576"}, 1048576},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			require.NoError(t, MCPCmd.Flags().Parse(tt.args))
			t.Cleanup(func() { _ = MCPCmd.Flags().Set(flagServeMaxClone, "0") })

			// Act
			setup, err := serveSetupFromFlags(MCPCmd)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, setup.MaxCloneBytes)
		})
	}
}

func TestServeSetupFromFlags_DiscoverSkills(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unset leaves the configuration in charge", nil, ""},
		{"the flag without a value means repo", []string{"--discover-skills"}, "repo"},
		{"repo", []string{"--discover-skills=repo"}, "repo"},
		{"all", []string{"--discover-skills=all"}, "all"},
		{"off", []string{"--discover-skills=off"}, "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			require.NoError(t, MCPCmd.Flags().Parse(tt.args))
			t.Cleanup(func() {
				f := MCPCmd.Flags().Lookup(flagServeDiscover)
				_ = f.Value.Set("") //nolint:errcheck // resets the flag for the next case
				f.Changed = false
			})

			// Act
			setup, err := serveSetupFromFlags(MCPCmd)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, setup.DiscoverSkills)
		})
	}
}

func TestMCPCmd_DiscoverSkillsOnlyMakesSenseWhenServing(t *testing.T) {
	assert.Contains(t, dynamicServeFlagNames, flagServeDiscover)
	flag := MCPCmd.Flags().Lookup(flagServeDiscover)
	require.NotNil(t, flag)
	assert.Equal(t, "repo", flag.NoOptDefVal)
}

func TestMCPCmd_ServeOnlyFlagsIncludeMaxCloneBytes(t *testing.T) {
	assert.Contains(t, dynamicServeFlagNames, flagServeMaxClone)
	assert.NotNil(t, MCPCmd.Flags().Lookup(flagServeMaxClone))
}

// `mcp --serve-skills` now registers the authoring surface too, so the setup
// carries the authoring options; the read-only setting narrows them.
func TestServeSetupFor_MergesTheAuthoringSurface(t *testing.T) {
	// Arrange
	require.NoError(t, MCPCmd.Flags().Parse(nil))

	// Act
	setup, err := serveSetupFor(MCPCmd, mcpSettings{})

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, setup.Authoring, "mcp --serve-skills registers the authoring tools too")
	plain := len(setup.Authoring)

	// Act: read-only adds the narrowing option.
	setup, err = serveSetupFor(MCPCmd, mcpSettings{readOnly: true})

	// Assert
	require.NoError(t, err)
	assert.Len(t, setup.Authoring, plain+1, "read-only adds WithReadOnly")

	// Act: a tool allow-list adds its option too.
	setup, err = serveSetupFor(MCPCmd, mcpSettings{tools: []string{"find_skill"}})

	// Assert
	require.NoError(t, err)
	assert.Len(t, setup.Authoring, plain+1, "tools add WithTools")
}

func TestServeSetupFor_AppliesTheResolvedSettings(t *testing.T) {
	// Arrange
	require.NoError(t, MCPCmd.Flags().Parse(nil))
	settings := mcpSettings{daemon: true, noWatch: true, allow: []string{"a-*"}, deny: []string{"b"}}

	// Act
	setup, err := serveSetupFor(MCPCmd, settings)

	// Assert
	require.NoError(t, err)
	assert.True(t, setup.NoWatch)
	assert.True(t, setup.Daemon)
	assert.Equal(t, daemonIdentity(), setup.DaemonIdentity, "a daemon turned on by the configuration gets the same identity as one turned on by the flag")
	assert.Equal(t, []string{"a-*"}, setup.Filter.Allow)
	assert.Equal(t, []string{"b"}, setup.Filter.Deny)
}

func TestServeSetupFromFlags_ExcludeIsRepeatable(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"unset", nil, nil},
		{"one glob", []string{"--exclude", "vendor/**"}, []string{"vendor/**"}},
		{"repeated", []string{"--exclude", "vendor/**", "--exclude=**/fixtures/"}, []string{"vendor/**", "**/fixtures/"}},
		{"a glob keeps its commas", []string{"--exclude", "a/{b,c}/**"}, []string{"a/{b,c}/**"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			require.NoError(t, MCPCmd.Flags().Parse(tt.args))
			t.Cleanup(func() {
				f := MCPCmd.Flags().Lookup(flagServeExclude)
				require.NoError(t, f.Value.(pflag.SliceValue).Replace(nil))
				f.Changed = false
			})

			// Act
			setup, err := serveSetupFromFlags(MCPCmd)

			// Assert
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, setup.Exclude)
		})
	}
}

func TestMCPCmd_ExcludeOnlyMakesSenseWhenServing(t *testing.T) {
	assert.Contains(t, dynamicServeFlagNames, flagServeExclude)
	assert.NotNil(t, MCPCmd.Flags().Lookup(flagServeExclude))
}
