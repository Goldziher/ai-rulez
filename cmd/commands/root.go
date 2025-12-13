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
	Version = "2.4.2"
)

var RootCmd = &cobra.Command{
	Use:          "ai-rulez",
	Short:        "Lightning-fast CLI tool for managing AI assistant rules",
	Long:         `ai-rulez is a lightning-fast CLI tool for managing AI assistant rules \nacross multiple platforms including Claude, Cursor, Windsurf, GitHub Copilot, \nand more. It provides a unified configuration format with support for remote \nincludes, dynamic generation, MCP server integration, and AI-powered rule enforcement.`,
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
	RootCmd.AddCommand(EnforceCmd)
	RootCmd.AddCommand(AddCmd)
	RootCmd.AddCommand(UpdateCmd)
	RootCmd.AddCommand(DeleteCmd)
	RootCmd.AddCommand(GetCmd)
	RootCmd.AddCommand(ListCmd)
	RootCmd.AddCommand(MCPCmd)
	RootCmd.AddCommand(SetCmd)

	AddCmd.AddCommand(crud.AddRuleCmd)
	AddCmd.AddCommand(crud.AddSectionCmd)
	AddCmd.AddCommand(crud.AddOutputCmd)
	AddCmd.AddCommand(crud.AddAgentCmd)
	AddCmd.AddCommand(crud.AddMCPServerCmd)
	AddCmd.AddCommand(crud.AddCommandCmd)
	AddCmd.AddCommand(crud.AddIncludeCmd)

	UpdateCmd.AddCommand(crud.UpdateRuleCmd)
	UpdateCmd.AddCommand(crud.UpdateSectionCmd)
	UpdateCmd.AddCommand(crud.UpdateOutputCmd)
	UpdateCmd.AddCommand(crud.UpdateAgentCmd)
	UpdateCmd.AddCommand(crud.UpdateMCPServerCmd)
	UpdateCmd.AddCommand(crud.UpdateCommandCmd)

	DeleteCmd.AddCommand(crud.DeleteRuleCmd)
	DeleteCmd.AddCommand(crud.DeleteSectionCmd)
	DeleteCmd.AddCommand(crud.DeleteOutputCmd)
	DeleteCmd.AddCommand(crud.DeleteAgentCmd)
	DeleteCmd.AddCommand(crud.DeleteMCPServerCmd)
	DeleteCmd.AddCommand(crud.DeleteCommandCmd)
	DeleteCmd.AddCommand(crud.DeleteExtendsCmd)
	DeleteCmd.AddCommand(crud.DeleteIncludeCmd)

	GetCmd.AddCommand(crud.GetRuleCmd)
	GetCmd.AddCommand(crud.GetSectionCmd)
	GetCmd.AddCommand(crud.GetOutputCmd)
	GetCmd.AddCommand(crud.GetAgentCmd)
	GetCmd.AddCommand(crud.GetMCPServerCmd)
	GetCmd.AddCommand(crud.GetCommandCmd)
	GetCmd.AddCommand(crud.GetMetadataCmd)
	GetCmd.AddCommand(crud.GetExtendsCmd)

	SetCmd.AddCommand(crud.SetMetadataCmd)
	SetCmd.AddCommand(crud.SetExtendsCmd)

	ListCmd.AddCommand(crud.ListRulesCmd)
	ListCmd.AddCommand(crud.ListSectionsCmd)
	ListCmd.AddCommand(crud.ListOutputsCmd)
	ListCmd.AddCommand(crud.ListAgentsCmd)
	ListCmd.AddCommand(crud.ListMCPServersCmd)
	ListCmd.AddCommand(crud.ListCommandsCmd)
	ListCmd.AddCommand(crud.ListIncludesCmd)
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

	if err := viper.ReadInConfig(); err == nil && viper.GetBool("verbose") {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
