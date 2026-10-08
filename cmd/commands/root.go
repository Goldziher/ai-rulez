package commands

import (
	"log/slog"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// envPrefix is the prefix of every environment variable the CLI reads for its
// own global settings.
const envPrefix = "AI_RULEZ"

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
		applyLogLevel()
		return normalizeFormatFlags(cmd)
	},
}

// Execute runs the CLI and returns the error the command ended with, unrendered.
// Main is the entry point of the binary: it renders the error once and returns
// the exit code.
func Execute() error {
	_, err := execute()
	return err
}

// execute runs the CLI and returns the command that ran, which tells the error
// renderer which --format the user asked for.
func execute() (*cobra.Command, error) {
	// main sets Version from the build after init has run.
	RootCmd.Version = Version
	RootCmd.SetVersionTemplate("ai-rulez version {{.Version}}\n")
	requireKnownSubcommands(RootCmd)
	explainArgErrors(RootCmd)
	return RootCmd.ExecuteContextC(cmdContext())
}

// applyLogLevel sets the logger from --debug and --quiet (or AI_RULEZ_DEBUG and
// AI_RULEZ_QUIET). -q drops progress, information and success lines; warnings,
// errors, hints and every command result stay.
func applyLogLevel() {
	switch {
	case viper.GetBool("debug"):
		logger.SetLevel(slog.LevelDebug)
	case viper.GetBool("quiet"):
		logger.SetLevel(slog.LevelWarn)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "C", "", "config file (default is to auto-discover)")
	RootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "Configuration directory name (default: .ai-rulez, then .config/ai-rulez)")
	RootCmd.PersistentFlags().BoolP("debug", "D", false, "enable debug output (or AI_RULEZ_DEBUG=1)")
	RootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress progress and informational output on stderr; results, warnings and errors stay (or AI_RULEZ_QUIET=1)")
	RootCmd.PersistentFlags().StringVarP(&gitToken, "token", "T", "", "Git access token for private repositories (or use AI_RULEZ_GIT_TOKEN env var)")

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
	RootCmd.AddCommand(MigrateCmd)
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

	applyHelpExamples()
}

func initConfig() {
	// Only AI_RULEZ_* variables are read (AI_RULEZ_DEBUG, AI_RULEZ_QUIET, ...):
	// a bare DEBUG or QUIET in the environment of a CI image must not change
	// what the tool prints. No configuration file is read for the CLI's own
	// settings.
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	// Bind git token from environment variable
	if err := viper.BindEnv("git_token", "AI_RULEZ_GIT_TOKEN"); err != nil {
		logger.Debug("Failed to bind git token env var", "error", err)
	}
	if err := viper.BindPFlag("git_token", RootCmd.PersistentFlags().Lookup("token")); err != nil {
		logger.Debug("Failed to bind git token flag", "error", err)
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
