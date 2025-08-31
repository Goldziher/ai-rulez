package commands

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/cmd/commands/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	Version = "dev" // Set during build
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "ai-rulez",
	Short: "Lightning-fast CLI tool for managing AI assistant rules",
	Long: `ai-rulez is a lightning-fast CLI tool for managing AI assistant rules 
across multiple platforms including Claude, Cursor, Windsurf, GitHub Copilot, 
and more. It provides a unified configuration format with support for remote 
includes, dynamic generation, and MCP server integration.`,
	SilenceUsage: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is to auto-discover)")
	RootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug output")
	RootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress progress bars and non-essential output")

	// Bind flags to viper
	if err := viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		logger.Debug("Failed to bind verbose flag", "error", err)
	}
	if err := viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug")); err != nil {
		logger.Debug("Failed to bind debug flag", "error", err)
	}
	if err := viper.BindPFlag("quiet", RootCmd.PersistentFlags().Lookup("quiet")); err != nil {
		logger.Debug("Failed to bind quiet flag", "error", err)
	}

	// Add all commands
	RootCmd.AddCommand(GenerateCmd)
	RootCmd.AddCommand(ValidateCmd)
	RootCmd.AddCommand(VersionCmd)
	RootCmd.AddCommand(InitCmd)
	RootCmd.AddCommand(AddCmd)
	RootCmd.AddCommand(UpdateCmd)
	RootCmd.AddCommand(DeleteCmd)
	RootCmd.AddCommand(ListCmd)
	RootCmd.AddCommand(MCPCmd)

	// Add subcommands to add, update, delete
	AddCmd.AddCommand(crud.AddRuleCmd)
	AddCmd.AddCommand(crud.AddSectionCmd)
	AddCmd.AddCommand(crud.AddOutputCmd)
	AddCmd.AddCommand(crud.AddAgentCmd)

	UpdateCmd.AddCommand(crud.UpdateRuleCmd)
	UpdateCmd.AddCommand(crud.UpdateSectionCmd)
	UpdateCmd.AddCommand(crud.UpdateOutputCmd)
	UpdateCmd.AddCommand(crud.UpdateAgentCmd)

	DeleteCmd.AddCommand(crud.DeleteRuleCmd)
	DeleteCmd.AddCommand(crud.DeleteSectionCmd)
	DeleteCmd.AddCommand(crud.DeleteOutputCmd)
	DeleteCmd.AddCommand(crud.DeleteAgentCmd)

	ListCmd.AddCommand(crud.ListRulesCmd)
	ListCmd.AddCommand(crud.ListSectionsCmd)
	ListCmd.AddCommand(crud.ListOutputsCmd)
	ListCmd.AddCommand(crud.ListAgentsCmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config file in home directory
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}

		// Search config in current directory
		viper.AddConfigPath(".")
		viper.SetConfigName(".ai-rulez")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil && viper.GetBool("verbose") {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
