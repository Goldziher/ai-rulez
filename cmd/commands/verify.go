package commands

import (
	"context"
	"errors"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var verifyPlugin bool
var verifyIfConfigured bool
var verifyRecursive bool
var verifyIfGenerated bool

// VerifyCmd verifies generated artifacts without modifying them.
var VerifyCmd = &cobra.Command{
	Use:   "verify [config-file]",
	Short: "Verify generated artifacts",
	Long: `Verify generated files without modifying them.

Without --plugin, every file in the generated manifest must exist and still match
the Content-Hash in its own header, which catches hand edits and deleted files
offline. Use "generate --check" to also catch sources that changed since the last
generate. With --plugin, generated plugin bundles are checked against their
provenance hashes.

Exit codes: 0 verified, 1 the check could not run, 2 generated files differ.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if !verifyPlugin {
			if verifyIfConfigured || verifyIfGenerated {
				fmtError(oops.Errorf("--if-configured and --if-generated only apply to --plugin"))
				os.Exit(1)
			}
			if code := runDriftCheck(args, verifyRecursive, driftManifest); code != 0 {
				os.Exit(code)
			}
			return
		}
		if verifyRecursive {
			runRecursivePluginVerify()
			return
		}
		cfg, err := loadConfigForCommand(context.Background(), args, config.WithoutLocal())
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := cfg.Validate(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if verifyIfConfigured && !cfg.HasPluginAuthoring() {
			logger.Info("Skipping plugin verification: no plugin authoring configuration")
			return
		}
		if err := generator.NewGenerator(cfg).VerifyPlugin(profile); err != nil {
			if verifyIfGenerated && errors.Is(err, generator.ErrPluginNotGenerated) {
				logger.Info("Skipping plugin verification: the plugin bundle has not been generated")
				return
			}
			fmtError(err)
			os.Exit(1)
		}
		logger.Success("Generated plugin artifacts are valid", "path", cfg.BaseDir)
	},
}

func init() {
	VerifyCmd.Flags().BoolVar(&verifyPlugin, "plugin", false, "Verify generated plugin bundles using provenance hashes")
	VerifyCmd.Flags().BoolVar(&verifyIfConfigured, "if-configured", false, "Skip plugin verification when no plugin authoring configuration is present")
	VerifyCmd.Flags().BoolVar(&verifyIfGenerated, "if-generated", false, "Skip plugin verification when the plugin bundle has not been generated yet")
	VerifyCmd.Flags().BoolVarP(&verifyRecursive, "recursive", "r", false, "Verify plugin outputs for configurations recursively")
	VerifyCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	VerifyCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
