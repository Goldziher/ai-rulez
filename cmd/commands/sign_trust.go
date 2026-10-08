package commands

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

// TrustCmd groups the commands that manage the Sigstore trust anchor.
var TrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Manage the Sigstore trusted root used to verify keyless attestations",
}

// TrustUpdateCmd fetches the trusted root; it is the only network step of verification.
var TrustUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Fetch the public-good Sigstore trusted root and cache it for offline verification",
	Long: `Fetch the public-good Sigstore trusted root over TUF and cache it under
~/.cache/ai-rulez/sigstore/trusted_root.json. "verify --attestation" reads this
file when neither --trusted-root nor [signing] trusted_root names another, and
never touches the network itself. Key-signed attestations need no trusted root.

This is the only command that contacts a Sigstore service for verification. For
a private Sigstore deployment, pass its trusted root file with --trusted-root.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		path, err := sigstore.UpdateTrustedRoot(nil)
		if err != nil {
			return fail(err)
		}
		logger.Success("Cached the Sigstore trusted root", "path", path)
		reportWriter{os.Stdout}.printf("%s\n", path)
		return nil
	},
}

func init() {
	TrustCmd.AddCommand(TrustUpdateCmd)
}
