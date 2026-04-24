package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	toml "github.com/pelletier/go-toml/v2"
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

	tomlPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		logger.Info("Already using config.toml — nothing to migrate")
		return
	}

	cfg, err := config.LoadConfig(context.Background(), workingDir)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	cfg.Version = "4.0"

	data, err := marshalConfigTOML(cfg)
	if err != nil {
		logger.Error("Failed to marshal TOML", "error", err)
		os.Exit(1)
	}

	if err := os.WriteFile(tomlPath, data, 0o644); err != nil {
		logger.Error("Failed to write config.toml", "error", err)
		os.Exit(1)
	}

	logger.Success("Created config.toml")
	removeOldConfigFiles(configDir)

	fmt.Println("\n✅ Migration complete!")
	fmt.Println("   Config: .ai-rulez/config.toml")
	fmt.Println("   Version: 4.0")
}

type tomlOutput struct {
	Version         string                        `toml:"version"`
	Name            string                        `toml:"name"`
	Description     string                        `toml:"description,omitempty"`
	Gitignore       *bool                         `toml:"gitignore,omitempty"`
	Default         string                        `toml:"default,omitempty"`
	Presets         []string                      `toml:"presets,omitempty"`
	Header          *config.HeaderConfig          `toml:"header,omitempty"`
	Profiles        map[string][]string           `toml:"profiles,omitempty"`
	Builtins        interface{}                   `toml:"builtins,omitempty"`
	Includes        []config.IncludeConfig        `toml:"includes,omitempty"`
	InstalledSkills []config.InstalledSkillConfig `toml:"installed_skills,omitempty"`
	Plugins         []config.PluginConfig         `toml:"plugins,omitempty"`
	Marketplaces    []config.MarketplaceConfig    `toml:"marketplaces,omitempty"`
	MCPServers      []config.MCPServer            `toml:"mcp_servers,omitempty"`
}

func marshalConfigTOML(cfg *config.Config) ([]byte, error) {
	presetNames := make([]string, len(cfg.Presets))
	for i, p := range cfg.Presets {
		presetNames[i] = p.GetName()
	}

	var builtinsVal interface{}
	if cfg.Builtins != nil {
		if cfg.Builtins.All != nil {
			builtinsVal = *cfg.Builtins.All
		} else if len(cfg.Builtins.Names) > 0 {
			builtinsVal = cfg.Builtins.Names
		}
	}

	// Use inline MCP servers if available, otherwise convert from the map
	// (which includes servers merged from legacy mcp.yaml)
	mcpServers := cfg.MCPServersRaw
	if len(mcpServers) == 0 && len(cfg.MCPServers) > 0 {
		for _, server := range cfg.MCPServers {
			mcpServers = append(mcpServers, *server)
		}
	}

	output := tomlOutput{
		Version:         cfg.Version,
		Name:            cfg.Name,
		Description:     cfg.Description,
		Gitignore:       cfg.Gitignore,
		Default:         cfg.Default,
		Presets:         presetNames,
		Header:          cfg.Header,
		Profiles:        cfg.Profiles,
		Builtins:        builtinsVal,
		Includes:        cfg.Includes,
		InstalledSkills: cfg.InstalledSkills,
		Plugins:         cfg.Plugins,
		Marketplaces:    cfg.Marketplaces,
		MCPServers:      mcpServers,
	}

	data, err := toml.Marshal(output)
	if err != nil {
		return nil, err
	}

	header := "# AI-Rulez Configuration (migrated to V4 TOML format)\n# Documentation: https://github.com/Goldziher/ai-rulez\n\n"
	return []byte(header + string(data)), nil
}

func removeOldConfigFiles(configDir string) {
	for _, old := range []string{"config.yaml", "config.json", "mcp.yaml", "mcp.toml", "mcp.json"} {
		oldPath := filepath.Join(configDir, old)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Remove(oldPath); err != nil {
				logger.Warn("Failed to remove old file", "path", oldPath, "error", err)
			} else {
				logger.Info("Removed", "file", old)
			}
		}
	}
}
