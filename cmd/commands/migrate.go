package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var MigrateCmd = &cobra.Command{
	Use:   "migrate [version]",
	Short: "Migrate configuration to a newer format",
	Long: `Migrate your ai-rulez configuration to a newer format version.

Supported targets: v4 (also 4, 4.0). The input is a V3 .ai-rulez/config.yaml;
-C/--config selects another project's config file or directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "v4", "4", "4.0":
			runMigrateV4()
			return nil
		default:
			return oops.Hint("supported targets: v4").Errorf("unsupported migration target %q", args[0])
		}
	},
}

// migrateTarget resolves what to migrate: the project directory, the config
// directory and the path handed to the loader. -C/--config names a config file
// (or the config directory) anywhere; without it the .ai-rulez of the working
// directory is used.
func migrateTarget() (workingDir, configDir, loadPath string) {
	if cfgFile == "" {
		workingDir = "."
		return workingDir, filepath.Join(workingDir, ".ai-rulez"), workingDir
	}
	abs, err := filepath.Abs(cfgFile)
	if err != nil {
		abs = cfgFile
	}
	configDir = filepath.Dir(abs)
	if info, serr := os.Stat(abs); serr == nil && info.IsDir() {
		configDir = abs
	}
	return filepath.Dir(configDir), configDir, abs
}

func runMigrateV4() {
	workingDir, configDir, loadPath := migrateTarget()

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		logger.Error("No config directory found", "path", configDir)
		fmt.Println("Run 'ai-rulez init' to create a new configuration")
		os.Exit(1)
	}

	migrateLocalOverlay(configDir)

	tomlPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		logger.Info("Already using config.toml — nothing to migrate")
		return
	}

	var cfg *config.Config
	var err error
	if cfgFile != "" {
		cfg, err = config.LoadConfigFromFile(context.Background(), loadPath, config.WithoutLocal())
	} else {
		cfg, err = config.LoadConfig(context.Background(), workingDir, config.WithoutLocal())
	}
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
	fmt.Println("   Config:", filepath.ToSlash(tomlPath))
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
