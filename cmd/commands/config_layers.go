package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// newConfigCmd groups read-only commands that show how the configuration is composed.
func newConfigCmd() *cobra.Command {
	var asJSON bool
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show how the configuration is composed (read-only)",
		Long: `Inspect how the configuration is composed from config.toml, the config.<name>.toml layers and the
machine-local overlay.

  ai-rulez config layers   list the layers and which are active`,
		Example: `  ai-rulez config layers`,
	}
	layersCmd := &cobra.Command{
		Use:   "layers",
		Short: "List the config.<name>.toml layers and which are active",
		Long: `Layers split the configuration into config.<name>.toml files beside config.toml.
	The active layers are merged over config.toml in the order they are named, so a later
	layer wins; config.local.toml is applied last. Layers named in default_layers of
	config.toml are always active (origin "default", a missing file is an error); AI_RULEZ_LAYERS
	(a comma-separated list), or AI_RULEZ_PROFILE when AI_RULEZ_LAYERS is not set, adds more
	(origin "env", a missing file is skipped).

	Tables merge per key, named lists (mcp_servers, installed_skills, plugins, ...) merge entry
	by entry, and any other value is replaced. A layer with no file is skipped.`,
		Example: `  ai-rulez config layers
  ai-rulez config layers --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigForCommand(watchParentContext(cmd), config.WithoutRemote())
			if err != nil {
				return fail(err)
			}
			return fail(writeConfigLayers(cmd.OutOrStdout(), cfg.ConfigDir, cfg.ActiveLayers, cfg.LayerOrigins, asJSON))
		},
	}
	configCmd.AddCommand(layersCmd)
	addJSONFormat(layersCmd.Flags(), &asJSON, "")
	specNoLocal.Bool(configCmd.PersistentFlags(), &noLocal, "Ignore the machine-local config.local.* overlay")
	return configCmd
}

type configLayerRow struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Active bool   `json:"active"`
	Order  int    `json:"order,omitempty"`
	// Origin is how an active layer was activated: default, env or option.
	Origin string `json:"origin,omitempty"`
}

func writeConfigLayers(w io.Writer, configDir string, active []string, origins map[string]string, asJSON bool) error {
	var rows []configLayerRow
	for _, name := range config.LayerFiles(configDir) {
		row := configLayerRow{Name: name, File: "config." + name + ".toml"}
		if i := slices.Index(active, name); i >= 0 {
			row.Active, row.Order, row.Origin = true, i+1, origins[name]
		}
		rows = append(rows, row)
	}
	if asJSON {
		data, err := json.MarshalIndent(map[string]any{"layers": rows, "active": active}, "", "  ")
		if err != nil {
			return err //nolint:wrapcheck // marshaling a fixed shape
		}
		_, err = fmt.Fprintln(w, string(data))
		return err //nolint:wrapcheck // stdout write
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No layers: add config.<name>.toml files beside config.toml.")
		return err //nolint:wrapcheck // stdout write
	}
	for _, row := range rows {
		state := "inactive"
		if row.Active {
			state = fmt.Sprintf("active (%d, %s)", row.Order, row.Origin)
		}
		if _, err := fmt.Fprintf(w, "%-20s %-28s %s\n", row.Name, row.File, state); err != nil {
			return err //nolint:wrapcheck // stdout write
		}
	}
	return nil
}
