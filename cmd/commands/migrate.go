package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var MigrateCmd = &cobra.Command{
	Use:   "migrate [version]",
	Short: "Migrate configuration to a newer format",
	Long:  "Migrate your ai-rulez configuration to a newer format version.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetVersion := args[0]
		switch targetVersion {
		case "v4", "4", "4.0":
			runMigrateV4()
		default:
			logger.Error("Unsupported migration target", "version", targetVersion)
			fmt.Println("Supported targets: v4")
		}
	},
}

func runMigrateV4() {
	workingDir := "."
	configDir := filepath.Join(workingDir, ".ai-rulez")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		logger.Error("No .ai-rulez directory found", "path", workingDir)
		fmt.Println("Run 'ai-rulez init' to create a new configuration")
		os.Exit(1)
	}

	migrateLocalOverlay(configDir)

	tomlPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		logger.Info("Already using config.toml — nothing to migrate")
		return
	}

	cfg, err := config.LoadConfig(context.Background(), workingDir, config.WithoutLocal())
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	cfg.Version = "4.0"

	data, err := config.MarshalTOML(cfg)
	if err != nil {
		logger.Error("Failed to marshal TOML", "error", err)
		os.Exit(1)
	}

	if err := os.WriteFile(tomlPath, data, 0o644); err != nil {
		logger.Error("Failed to write config.toml", "error", err)
		os.Exit(1)
	}

	logger.Success("Created config.toml")
	written := map[string]bool{}
	for _, s := range cfg.EffectiveMCPServers() {
		written[s.Name] = true
	}
	removeOldConfigFiles(configDir, written)

	fmt.Println("\n✅ Migration complete!")
	fmt.Println("   Config: .ai-rulez/config.toml")
	fmt.Println("   Version: 4.0")
}

// migrateLocalOverlay converts config.local.yaml|yml|json to config.local.toml.
func migrateLocalOverlay(configDir string) {
	from, to, err := config.MigrateLocalOverlayToTOML(configDir)
	if err != nil {
		// The overlay is optional and machine-local: a conflict in it (for
		// example a second config.local.* file) must not block migrating the
		// shared config.
		logger.Warn("Local config was not converted to TOML", "error", err)
		return
	}
	if from != "" {
		logger.Success("Converted local config", "from", filepath.Base(from), "to", filepath.Base(to))
	}
}

// removeOldConfigFiles deletes the V3 files the migration replaced. A legacy MCP
// file goes only when every server it declares is in config.toml (written holds
// their names): the loader reads just the first of mcp.toml, mcp.yaml and
// mcp.json, so a later one can hold servers that were never carried over.
func removeOldConfigFiles(configDir string, written map[string]bool) {
	for _, old := range []string{configFileYAML, configFileJSON, "mcp.yaml", "mcp.toml", "mcp.json"} {
		oldPath := filepath.Join(configDir, old)
		if _, err := os.Stat(oldPath); err != nil {
			continue
		}
		if strings.HasPrefix(old, "mcp.") {
			if missing := legacyServersNotWritten(oldPath, written); len(missing) > 0 {
				logger.Warn("Kept a legacy MCP file: some of its servers are not in config.toml",
					"file", old, "servers", strings.Join(missing, ", "))
				continue
			}
		}
		if err := os.Remove(oldPath); err != nil {
			logger.Warn("Failed to remove old file", "path", oldPath, "error", err)
		} else {
			logger.Info("Removed", "file", old)
		}
	}
}

// legacyServersNotWritten lists the servers of a legacy MCP file that are not in
// written. A file that cannot be read counts as holding an unknown server.
func legacyServersNotWritten(path string, written map[string]bool) []string {
	servers, err := config.DecodeLegacyMCPFile(path)
	if err != nil {
		return []string{"(unreadable: " + err.Error() + ")"}
	}
	var missing []string
	for _, s := range servers {
		if !written[s.Name] {
			missing = append(missing, s.Name)
		}
	}
	return missing
}
