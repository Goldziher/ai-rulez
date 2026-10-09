package commands

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func TestBareEnvironmentVariablesDoNotChangeBehaviour(t *testing.T) {
	// Arrange
	t.Setenv("QUIET", "1")
	t.Setenv("DEBUG", "1")
	t.Setenv("VERBOSE", "1")
	initConfig()

	// Act / Assert
	assert.False(t, viper.GetBool("quiet"), "bare QUIET must not silence the CLI")
	assert.False(t, viper.GetBool("debug"), "bare DEBUG must not turn on debug logging")
}

func TestPrefixedEnvironmentVariablesSetTheGlobalSwitches(t *testing.T) {
	// Arrange
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
