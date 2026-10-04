package config

import (
	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// tomlHeader is prepended to every marshaled config document. TOML parsing does
// not preserve comments, so a hand-written config.toml loses its comments when a
// CRUD mutation rewrites it; at minimum the file keeps a pointer back to the docs.
const tomlHeader = "# AI-Rulez Configuration\n" +
	"# Documentation: https://github.com/Goldziher/ai-rulez\n\n"

// tomlPresetDoc is the on-disk shape of a custom preset inside a TOML config.
// Built-in presets are emitted as bare strings; custom/provider presets as
// inline tables. TOML 1.0 permits mixing both element types in one array.
type tomlPresetDoc struct {
	Name     string     `toml:"name,omitempty"`
	Type     PresetType `toml:"type,omitempty"`
	Path     string     `toml:"path,omitempty"`
	Template string     `toml:"template,omitempty"`
	Provider string     `toml:"provider,omitempty"`
}

// tomlOutput is the serializable projection of Config for the TOML format. It
// exists because Config carries runtime-only fields (BaseDir, Content, ...) and
// because Presets must be flattened into strings/inline-tables rather than the
// struct's custom (YAML/JSON-only) marshalers.
type tomlOutput struct {
	Schema          string                 `toml:"schema,omitempty"`
	Version         string                 `toml:"version"`
	Name            string                 `toml:"name"`
	Description     string                 `toml:"description,omitempty"`
	Gitignore       *bool                  `toml:"gitignore,omitempty"`
	Compact         *bool                  `toml:"compact,omitempty"`
	AgentsMD        bool                   `toml:"agents_md,omitempty"`
	BundleExclude   []string               `toml:"bundle_exclude,omitempty"`
	CodexSkillsDir  string                 `toml:"codex_skills_dir,omitempty"`
	Default         string                 `toml:"default,omitempty"`
	Presets         []any                  `toml:"presets,omitempty"`
	Header          *HeaderConfig          `toml:"header,omitempty"`
	Profiles        map[string][]string    `toml:"profiles,omitempty"`
	Builtins        interface{}            `toml:"builtins,omitempty"`
	Includes        []IncludeConfig        `toml:"includes,omitempty"`
	InstalledSkills []InstalledSkillConfig `toml:"installed_skills,omitempty"` //nolint:tagliatelle
	Defaults        *DefaultsConfig        `toml:"defaults,omitempty"`
	Rules           *RulesConfig           `toml:"rules,omitempty"`
	Lint            *LintConfig            `toml:"lint,omitempty"`
	Usage           *UsageConfig           `toml:"usage,omitempty"`
	Scopes          []ScopeConfig          `toml:"scopes,omitempty"`
	Plugins         []PluginConfig         `toml:"plugins,omitempty"`
	Marketplaces    []MarketplaceConfig    `toml:"marketplaces,omitempty"`
	MCPServers      []MCPServer            `toml:"mcp_servers,omitempty"`
	MCP             *MCPConfig             `toml:"mcp,omitempty"`
	Plugin          *PluginAuthoring       `toml:"plugin,omitempty"`
	Marketplace     *MarketplaceAuthoring  `toml:"marketplace,omitempty"`
	Placement       *PlacementConfig       `toml:"placement,omitempty"`
	Claude          *ClaudeConfig          `toml:"claude,omitempty"`
	Codex           *CodexConfig           `toml:"codex,omitempty"`
}

// MarshalTOML serializes a Config to a TOML document with a leading docs header.
// It is the TOML counterpart of the YAML/JSON paths in SaveConfig and is also
// used by the v4 migration command so both stay in sync.
func MarshalTOML(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, oops.Hint("Provide a valid Config struct").Errorf("config is nil")
	}

	if cfg.LocalOverlay != nil {
		return nil, errMergedConfigWrite()
	}

	data, err := toml.Marshal(toTOMLOutput(cfg))
	if err != nil {
		return nil, oops.Wrapf(err, "marshal config to TOML")
	}
	return append([]byte(tomlHeader), data...), nil
}

func toTOMLOutput(cfg *Config) tomlOutput {
	presets := make([]any, 0, len(cfg.Presets))
	for i := range cfg.Presets {
		p := &cfg.Presets[i]
		if p.IsBuiltIn() {
			presets = append(presets, p.BuiltIn)
			continue
		}
		presets = append(presets, tomlPresetDoc{
			Name:     p.Name,
			Type:     p.Type,
			Path:     p.Path,
			Template: p.Template,
			Provider: p.Provider,
		})
	}

	var builtinsVal interface{}
	if cfg.Builtins != nil {
		if cfg.Builtins.All != nil {
			builtinsVal = *cfg.Builtins.All
		} else if len(cfg.Builtins.Names) > 0 {
			builtinsVal = cfg.Builtins.Names
		}
	}

	// Prefer the raw MCP server slice (preserves author order and inline-only
	// fields); fall back to the resolved map merged from legacy mcp.yaml.
	mcpServers := cfg.MCPServersRaw
	if len(mcpServers) == 0 && len(cfg.MCPServers) > 0 {
		for _, server := range cfg.MCPServers {
			mcpServers = append(mcpServers, *server)
		}
	}

	return tomlOutput{
		Schema:          cfg.Schema,
		Version:         cfg.Version,
		Name:            cfg.Name,
		Description:     cfg.Description,
		Gitignore:       cfg.Gitignore,
		Compact:         cfg.Compact,
		AgentsMD:        cfg.AgentsMD,
		BundleExclude:   cfg.BundleExclude,
		CodexSkillsDir:  cfg.CodexSkillsDir,
		Default:         cfg.Default,
		Presets:         presets,
		Header:          cfg.Header,
		Profiles:        cfg.Profiles,
		Builtins:        builtinsVal,
		Includes:        cfg.Includes,
		InstalledSkills: cfg.InstalledSkills,
		Defaults:        cfg.Defaults,
		Rules:           cfg.Rules,
		Lint:            cfg.Lint,
		Usage:           cfg.Usage,
		Scopes:          cfg.Scopes,
		Plugins:         cfg.Plugins,
		Marketplaces:    cfg.Marketplaces,
		MCPServers:      mcpServers,
		MCP:             cfg.MCP,
		Plugin:          cfg.Plugin,
		Marketplace:     cfg.Marketplace,
		Placement:       cfg.Placement,
		Claude:          cfg.Claude,
		Codex:           cfg.Codex,
	}
}
