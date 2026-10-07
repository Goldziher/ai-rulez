package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	gitToken string
	Version  = "5.0.0"
)

var RootCmd = &cobra.Command{
	Use:   "ai-rulez",
	Short: "Lightning-fast CLI tool for managing AI assistant rules",
	Long: `ai-rulez is a lightning-fast CLI tool for managing AI assistant rules
across multiple platforms including Claude, Cursor, Devin, GitHub Copilot,
and more. It provides a unified configuration format with support for remote
includes, dynamic generation, and MCP server integration.`,
	SilenceUsage: true,
	// main prints the returned error once; cobra's own "Error:" line would repeat it.
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		switch {
		case viper.GetBool("debug"):
			logger.SetLevel(slog.LevelDebug)
		case viper.GetBool("quiet"):
			logger.SetLevel(slog.LevelError)
		}
		return normalizeFormatFlags(cmd)
	},
}

func Execute() error {
	// main sets Version from the build after init has run.
	RootCmd.Version = Version
	RootCmd.SetVersionTemplate("ai-rulez version {{.Version}}\n")
	requireKnownSubcommands(RootCmd)
	return RootCmd.ExecuteContext(config.WithPolicyContext(context.Background(), activePolicy))
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "C", "", "config file (default is to auto-discover)")
	RootCmd.PersistentFlags().BoolP("verbose", "V", false, "enable verbose output")
	RootCmd.PersistentFlags().BoolP("debug", "D", false, "enable debug output")
	RootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress progress bars and non-essential output")
	RootCmd.PersistentFlags().StringVarP(&gitToken, "token", "T", "", "Git access token for private repositories (or use AI_RULEZ_GIT_TOKEN env var)")

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
	RootCmd.AddCommand(CleanCmd)
	RootCmd.AddCommand(ValidateCmd)
	RootCmd.AddCommand(ScanCmd)
	RootCmd.AddCommand(VerifyCmd)
	RootCmd.AddCommand(DoctorCmd)
	RootCmd.AddCommand(GuardCmd)
	RootCmd.AddCommand(VerifiersCmd)
	RootCmd.AddCommand(ScannersCmd)
	RootCmd.AddCommand(VersionCmd)
	RootCmd.AddCommand(InitCmd)
	RootCmd.AddCommand(ConvertCmd)
	RootCmd.AddCommand(MCPCmd)
	RootCmd.AddCommand(DomainCmd)
	RootCmd.AddCommand(AddCmd)
	RootCmd.AddCommand(RemoveCmd)
	RootCmd.AddCommand(ListCmd)
	RootCmd.AddCommand(IncludeCmd)
	RootCmd.AddCommand(ProfileCmd)
	RootCmd.AddCommand(BuiltinsCmd)
	RootCmd.AddCommand(SkillCmd)
	RootCmd.AddCommand(LockCmd)
	RootCmd.AddCommand(ApproveCmd)
	RootCmd.AddCommand(SignCmd, TrustCmd)
	RootCmd.AddCommand(RolesCmd)
	RootCmd.AddCommand(CatalogCmd)
	RootCmd.AddCommand(SBOMCmd)
	RootCmd.AddCommand(TokensCmd)
	RootCmd.AddCommand(CostCmd)
	RootCmd.AddCommand(UsageCmd)
	RootCmd.AddCommand(ReportCmd)
	RootCmd.AddCommand(EvalCmd)
	RootCmd.AddCommand(ImproveCmd)
	RootCmd.AddCommand(LocalCmd)
	RootCmd.AddCommand(OKFCmd)
	RootCmd.AddCommand(ExportCmd)
	RootCmd.AddCommand(ImportCmd)
	RootCmd.AddCommand(LLMCmd)
	RootCmd.AddCommand(SearchCmd)
	RootCmd.AddCommand(PublishCmd)
	RootCmd.AddCommand(ReviewCmd, RubricCmd)
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
