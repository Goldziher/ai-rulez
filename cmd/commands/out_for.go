package commands

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

// outFor is the stdout/stderr pair of cmd. -q (or AI_RULEZ_QUIET) removes only
// the informational lines written through Info; results always print.
func outFor(cmd *cobra.Command) render.Out {
	return render.New(cmd.OutOrStdout(), cmd.ErrOrStderr(), viper.GetBool("quiet")).WithFormat(commandFormat(cmd))
}

// defaultOut is outFor for a helper that has no command: the process streams,
// read at call time.
func defaultOut() render.Out {
	return render.New(os.Stdout, os.Stderr, viper.GetBool("quiet")).WithFormat(RootCmd.Annotations[activeFormatKey])
}

// reportFailure renders err for a helper that returns an exit code instead of
// an error: the text on stderr and, under --format json, the error document on
// stdout, exactly as the root renderer does for a returned error.
func reportFailure(format string, err error) {
	ReportError(os.Stdout, os.Stderr, format, err)
}
