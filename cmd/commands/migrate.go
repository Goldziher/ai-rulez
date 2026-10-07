package commands

import (
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// MigrateCmd stands in for the 4.x migrate command, removed in v5, so that a user
// who runs it gets the upgrade path instead of "unknown command". It is hidden:
// v5 reads only .ai-rulez/config.toml, which 4.x writes. main prints only the
// error text of a RunE error, so the upgrade path is part of the message.
var MigrateCmd = &cobra.Command{
	Use:                "migrate",
	Short:              "Removed: migrate a V2/V3 config with ai-rulez 4.x",
	Hidden:             true,
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		return oops.Errorf("`ai-rulez migrate` was removed in ai-rulez 5: run `npx ai-rulez@4 migrate v4` " +
			"to convert a V3 .ai-rulez/config.yaml to config.toml, then use ai-rulez 5 " +
			"(move a flat V2 ai-rulez.yaml to .ai-rulez/config.yaml first)")
	},
}
