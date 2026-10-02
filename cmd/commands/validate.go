package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/Goldziher/ai-rulez/schema"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var validateRecursive bool

var ValidateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration file",
	Long: `Validate an AI rules configuration file for syntax errors,
schema compliance, and structural issues.`,
	Aliases: []string{"val", "v", "check"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		progress.SetQuiet(viper.GetBool("quiet"))

		if validateRecursive {
			if len(args) > 0 {
				fmtError(oops.Errorf("validate --recursive does not take a config file argument"))
				os.Exit(1)
			}
			if code := runRecursiveValidate(); code != 0 {
				os.Exit(code)
			}
			return
		}

		cfg, err := loadConfigForCommand(ctx, args)
		if err != nil {
			logger.Error("Failed to load config")
			fmtError(err)
			os.Exit(1)
		}

		// Validate the raw file against the JSON schema first so a key the
		// struct would silently drop (and a value outside an enum) is reported.
		// V3 configs keep the looser Go validation only: the schema is V4-shaped.
		if !cfg.IsV3() && cfg.ConfigFile != "" {
			configPath := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
			if err := schema.ValidateFile(configPath); err != nil {
				logger.Error("Configuration failed schema validation", "path", configPath)
				fmtError(err)
				os.Exit(1)
			}
		}

		if err := validateLocalOverlay(cfg); err != nil {
			logger.Error("Local overlay failed schema validation", "path", cfg.LocalOverlay.Path)
			fmtError(err)
			os.Exit(1)
		}
		if cfg.LocalOverlay != nil {
			progress.PrintIfNotQuiet("local overlay: %s\n", cfg.LocalOverlay.Path)
		}

		if err := cfg.Validate(); err != nil {
			logger.Error("Configuration validation failed", "path", cfg.ConfigDir)
			fmtError(err)
			os.Exit(1)
		}

		logger.Success("Configuration is valid", "path", cfg.ConfigDir)
		presets.WarnDuplicateContent(cfg.Content)
		displayConfigurationSummary(cfg)
	},
}

func init() {
	ValidateCmd.Flags().BoolVarP(&validateRecursive, "recursive", "r", false, "Validate every configuration file found recursively")
	ValidateCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	ValidateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// runRecursiveValidate validates every discovered config, reports all failures,
// and returns 1 if any config is invalid (0 otherwise, including when none exist).
func runRecursiveValidate() int {
	configFiles := findConfigFilesRecursively()
	if len(configFiles) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}

	var failed []string
	for _, configPath := range configFiles {
		if err := validateConfigFile(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %s\n", configPath)
			fmtError(err)
			failed = append(failed, configPath)
			continue
		}
		progress.PrintIfNotQuiet("✅ %s\n", configPath)
	}

	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ %d of %d config(s) invalid\n", len(failed), len(configFiles))
		return 1
	}
	progress.PrintIfNotQuiet("\nAll %d config(s) are valid\n", len(configFiles))
	return 0
}

// validateConfigFile applies the same checks as single-root validate (schema for
// V4 configs, then structural validation) to one config file.
func validateConfigFile(configPath string) error {
	cfg, err := config.LoadConfigFromFile(context.Background(), configPath, pluginLoadOptions(false)...)
	if err != nil {
		return err
	}
	if !cfg.IsV3() {
		if err := schema.ValidateFile(configPath); err != nil {
			return err
		}
	}
	if err := validateLocalOverlay(cfg); err != nil {
		return err
	}
	return cfg.Validate()
}

// validateLocalOverlay checks the config.local.* overlay, when one was merged,
// against the local overlay schema. The returned error names the overlay file.
func validateLocalOverlay(cfg *config.Config) error {
	if cfg.LocalOverlay == nil {
		return nil
	}
	if err := schema.ValidateLocalFile(cfg.LocalOverlay.Path); err != nil {
		return oops.With("path", cfg.LocalOverlay.Path).Wrapf(err, "local overlay %s", cfg.LocalOverlay.Path)
	}
	return nil
}

func displayConfigurationSummary(cfg *config.Config) {
	logger.Info("\nConfiguration summary:")

	if cfg.Content != nil {
		logger.Info("  - Rules:", "count", len(cfg.Content.Rules))
		logger.Info("  - Context files:", "count", len(cfg.Content.Context))
		logger.Info("  - Skills:", "count", len(cfg.Content.Skills))
		logger.Info("  - Domains:", "count", len(cfg.Content.Domains))

		var totalDomainRules, totalDomainContext, totalDomainSkills int
		for _, domain := range cfg.Content.Domains {
			totalDomainRules += len(domain.Rules)
			totalDomainContext += len(domain.Context)
			totalDomainSkills += len(domain.Skills)
		}
		if totalDomainRules > 0 {
			logger.Info("    - Domain rules:", "count", totalDomainRules)
		}
		if totalDomainContext > 0 {
			logger.Info("    - Domain context:", "count", totalDomainContext)
		}
		if totalDomainSkills > 0 {
			logger.Info("    - Domain skills:", "count", totalDomainSkills)
		}
	}

	logger.Info("  - Presets:", "count", len(cfg.Presets))
}
