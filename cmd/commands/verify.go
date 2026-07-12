package commands

import (
	"context"
	"os"

	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var verifyPlugin bool

// VerifyCmd verifies generated artifacts without modifying them.
var VerifyCmd = &cobra.Command{
	Use:   "verify [config-file]",
	Short: "Verify generated artifacts",
	Args:  cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		cfg, err := loadConfigForCommand(context.Background(), args)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if !verifyPlugin {
			fmtError(oops.Errorf("verify requires --plugin"))
			os.Exit(1)
		}
		if err := cfg.Validate(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := generator.NewGenerator(cfg).VerifyPlugin(profile); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		logger.Success("Generated plugin artifacts are valid", "path", cfg.BaseDir)
	},
}

func init() {
	VerifyCmd.Flags().BoolVar(&verifyPlugin, "plugin", false, "Verify generated plugin bundles using provenance hashes")
	VerifyCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	VerifyCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
