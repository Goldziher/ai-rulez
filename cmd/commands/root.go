package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	gitToken string
	Version  = "3.7.3"
)

var RootCmd = &cobra.Command{
	Use:          "ai-rulez",
	Short:        "Lightning-fast CLI tool for managing AI assistant rules",
	Long:         `ai-rulez is a lightning-fast CLI tool for managing AI assistant rules \nacross multiple platforms including Claude, Cursor, Windsurf, GitHub Copilot, \nand more. It provides a unified configuration format with support for remote \nincludes, dynamic generation, and MCP server integration.`,
	SilenceUsage: true,
}

func Execute() error {
	return RootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is to auto-discover)")
	RootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug output")
	RootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress progress bars and non-essential output")
	RootCmd.PersistentFlags().StringVar(&gitToken, "token", "", "Git access token for private repositories (or use AI_RULEZ_GIT_TOKEN env var)")

	if err := viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		logger.Debug("Failed to bind verbose flag", "error", err)
	}
	if err := viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug")); err != nil {
		logger.Debug("Failed to bind debug flag", "error", err)
	}
	if err := viper.BindPFlag("quiet", RootCmd.PersistentFlags().Lookup("quiet")); err != nil {
		logger.Debug("Failed to bind quiet flag", "error", err)
	}

	RootCmd.AddCommand(GenerateCmd)
	RootCmd.AddCommand(ValidateCmd)
	RootCmd.AddCommand(VersionCmd)
	RootCmd.AddCommand(InitCmd)
	RootCmd.AddCommand(MCPCmd)
	RootCmd.AddCommand(MigrateCmd)
	RootCmd.AddCommand(DomainCmd)
	RootCmd.AddCommand(AddCmd)
	RootCmd.AddCommand(RemoveCmd)
	RootCmd.AddCommand(ListCmd)
	RootCmd.AddCommand(IncludeCmd)
	RootCmd.AddCommand(ProfileCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}

		viper.AddConfigPath(".")
		viper.SetConfigName(".ai-rulez")
	}

	viper.AutomaticEnv()

	// Bind git token from environment variable
	if err := viper.BindEnv("git_token", "AI_RULEZ_GIT_TOKEN"); err != nil {
		logger.Debug("Failed to bind git token env var", "error", err)
	}
	if err := viper.BindPFlag("git_token", RootCmd.PersistentFlags().Lookup("token")); err != nil {
		logger.Debug("Failed to bind git token flag", "error", err)
	}

	if err := viper.ReadInConfig(); err == nil && viper.GetBool("verbose") {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// GetGitToken returns the git token from flag or environment variable
// Priority: CLI flag > Environment variable
func GetGitToken() string {
	// CLI flag takes precedence
	if gitToken != "" {
		return strings.TrimSpace(gitToken)
	}

	// Fall back to viper (environment variable)
	token := viper.GetString("git_token")
	return strings.TrimSpace(token)
}
