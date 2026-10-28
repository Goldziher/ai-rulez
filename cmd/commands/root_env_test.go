package commands

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func TestBareEnvironmentVariablesDoNotChangeBehaviour(t *testing.T) {
	// Arrange
	clearSwitchOverrides(t)
	t.Setenv("QUIET", "1")
	t.Setenv("DEBUG", "1")
	t.Setenv("VERBOSE", "1")
	initConfig()

	// Act / Assert
	assert.False(t, viper.GetBool("quiet"), "bare QUIET must not silence the CLI")
	assert.False(t, viper.GetBool("debug"), "bare DEBUG must not turn on debug logging")
}

// clearSwitchOverrides drops a value an earlier test set with viper.Set, which
// would otherwise win over the environment these tests read.
func clearSwitchOverrides(t *testing.T) {
	t.Helper()
	for _, key := range []string{"quiet", "debug"} {
		viper.Set(key, nil)
	}
}

func TestPrefixedEnvironmentVariablesSetTheGlobalSwitches(t *testing.T) {
	// Arrange
	clearSwitchOverrides(t)
	t.Setenv("AI_RULEZ_QUIET", "1")
	t.Setenv("AI_RULEZ_DEBUG", "true")
	initConfig()
	t.Cleanup(func() { logger.SetLevel(slog.LevelInfo) })

	// Act / Assert
	assert.True(t, viper.GetBool("quiet"))
	assert.True(t, viper.GetBool("debug"))
}

func TestHomeConfigFileIsNotRead(t *testing.T) {
	// Arrange: a dotfile that used to be picked up by the root command
	home := t.TempDir()
	t.Setenv("HOME", home)
	assert.NoError(t, os.WriteFile(filepath.Join(home, ".ai-rulez.toml"), []byte("quiet = true\n"), 0o600))
	initConfig()

	// Act / Assert
	assert.False(t, viper.GetBool("quiet"))
	assert.Empty(t, viper.ConfigFileUsed())
}

func TestVerboseFlagIsGone(t *testing.T) {
	assert.Nil(t, RootCmd.PersistentFlags().Lookup("verbose"))
	assert.NotNil(t, RootCmd.PersistentFlags().Lookup("config-dir"))
}

func TestApplySelectionEnv(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		role     string
		args     []string
		wantProf string
		wantRole string
	}{
		{name: "empty environment changes nothing"},
		{name: "profile from the environment", profile: "backend", wantProf: "backend"},
		{name: "role from the environment", role: "dev,oncall", wantRole: "dev,oncall"},
		{name: "the flag beats the environment", profile: "backend", args: []string{"--profile", "frontend"}, wantProf: "frontend"},
		{name: "a blank environment value is ignored", profile: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envProfile, tt.profile)
			t.Setenv(envRole, tt.role)
			var gotProf, gotRole string
			cmd := &cobra.Command{RunE: func(*cobra.Command, []string) error { return nil }}
			cmd.Flags().StringVar(&gotProf, "profile", "", "")
			cmd.Flags().StringVar(&gotRole, "role", "", "")
			require.NoError(t, cmd.ParseFlags(tt.args))

			require.NoError(t, applySelectionEnv(cmd))

			assert.Equal(t, tt.wantProf, gotProf)
			assert.Equal(t, tt.wantRole, gotRole)
		})
	}
}

func TestApplySelectionEnvFillsServeRole(t *testing.T) {
	// Arrange: mcp --serve-skills takes its role as --serve-role
	t.Setenv(envRole, "dev")
	var got string
	cmd := &cobra.Command{RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringVar(&got, "serve-role", "", "")

	// Act
	require.NoError(t, applySelectionEnv(cmd))

	// Assert
	assert.Equal(t, "dev", got)
}

func TestGenerateSelectsTheRoleFromTheEnvironment(t *testing.T) {
	// Arrange: a project whose base role keeps the backend skills and excludes deploy
	rolesCmdProject(t)
	t.Setenv(envRole, "base")
	// Executing the root command leaves the process-wide lock policy as generate
	// set it (generate.go); restore it so later tests start from the zero policy.
	prevPolicy := cliLockPolicy
	t.Cleanup(func() { cliLockPolicy, generateRole, profile = prevPolicy, "", "" })

	// Act: generate with no --role at all
	cmd := CommandTree()
	cmd.SetArgs([]string{"generate", "--quiet"})
	require.NoError(t, cmd.Execute())
	t.Cleanup(func() { RootCmd.SetArgs(nil) })

	// Assert: the role the environment named was the one generated
	assert.FileExists(t, filepath.Join(".claude", "skills", "migrate", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(".claude", "skills", "deploy", "SKILL.md"), "excluded by the env-selected role")
}
