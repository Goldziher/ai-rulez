package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const (
	// VersionDir is the config version returned when an .ai-rulez/ directory is detected.
	VersionDir = "dir"

	aiRulezDirName = ".ai-rulez"
	// altConfigDirName is the project-level .config/ convention
	// (https://github.com/pi0/config-dir). It is consulted as a fallback after
	// .ai-rulez/ when no tool-specific directory exists at a given level.
	altConfigDirName   = ".config/ai-rulez"
	configTOMLFilename = "config.toml"
	configYAMLFilename = "config.yaml"
	configYMLFilename  = "config.yml"
	configJSONFilename = "config.json"
	// configFilenameYAMLV2 is the legacy V2 flat-file YAML config name.
	configFilenameYAMLV2 = "ai-rulez.yaml"
	// configFilenameYMLV2 is the legacy V2 flat-file YAML config name with .yml extension.
	configFilenameYMLV2 = "ai-rulez.yml"
	rulesDir            = "rules"
	contextDir          = "context"
	skillsDir           = "skills"
	agentsDir           = "agents"
	commandsDir         = "commands"
	checksDir           = "checks"
	domainsDir          = "domains"
	skillMarkerFile     = "SKILL.md"
	commandMarkerFile   = "COMMAND.md"
	// localDir holds machine-local override content under the config dir
	// (.ai-rulez/local/rules, .ai-rulez/local/context). It is scanned into a
	// separate tree and never merged into committed output.
	localDir = "local"
)

// configDirCandidates lists the supported directory-based config locations,
// relative to a project base directory, in precedence order: the tool-specific
// .ai-rulez/ first (backward compatible), then the project-level
// .config/ai-rulez/ convention.
var configDirCandidates = []string{aiRulezDirName, altConfigDirName}

// ResolveConfigDirName returns the first config directory candidate that exists
// under baseDir and contains a config file, or "" when none is present. The
// returned name is slash-separated and relative to baseDir.
func ResolveConfigDirName(baseDir string) string {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return ""
	}
	for _, dirName := range configDirCandidates {
		if hasConfigFile(filepath.Join(absDir, filepath.FromSlash(dirName))) {
			return dirName
		}
	}
	return ""
}

// projectBaseDir returns the project directory that owns configDir: the
// directory containing the config directory. For the nested .config/ai-rulez/
// layout it skips past the generic .config/ wrapper so generated outputs are
// rooted in the project, not in .config/.
func projectBaseDir(configDir string) string {
	parent := filepath.Dir(configDir)
	if filepath.Base(parent) == ".config" {
		return filepath.Dir(parent)
	}
	return parent
}

// relConfigDirName names configDir relative to baseDir using forward slashes,
// falling back to the base name when configDir is not under baseDir (e.g. a
// machine-local override outside the project).
func relConfigDirName(baseDir, configDir string) string {
	rel, err := filepath.Rel(baseDir, configDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Base(configDir)
	}
	return filepath.ToSlash(rel)
}

// DetectConfigVersion detects whether a directory contains V2 or directory-based configuration
// Returns "v2" if ai-rulez.yaml/yml exists, "dir" if .ai-rulez/ (or
// .config/ai-rulez/) exists, "" otherwise
func DetectConfigVersion(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", oops.
			With("path", dir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	// Check for directory-based config (.ai-rulez/ directory, then the
	// project-level .config/ai-rulez/ convention).
	for _, dirName := range configDirCandidates {
		configDir := filepath.Join(absDir, filepath.FromSlash(dirName))
		if info, err := os.Stat(configDir); err == nil && info.IsDir() {
			return VersionDir, nil
		}
	}

	// Check for V2 (ai-rulez.yaml or ai-rulez.yml)
	v2Files := []string{configFilenameYAMLV2, configFilenameYMLV2}
	for _, filename := range v2Files {
		v2Path := filepath.Join(absDir, filename)
		if _, err := os.Stat(v2Path); err == nil {
			return "v2", nil
		}
	}

	return "", nil
}

// ResolveIncludesCallback is a callback function type that resolves includes
// This avoids circular import issues between config and includes packages
type ResolveIncludesCallback func(ctx context.Context, cfg *Config) (*ContentTree, error)

// resolveIncludesFunc is set by the includes package during init
var resolveIncludesFunc ResolveIncludesCallback

// SetResolveIncludesCallback sets the callback for resolving includes
// This is called by the includes package to avoid circular imports
func SetResolveIncludesCallback(fn ResolveIncludesCallback) {
	resolveIncludesFunc = fn
}

// ResolveInstalledSkillsCallback is a callback function type that resolves installed skills
type ResolveInstalledSkillsCallback func(ctx context.Context, cfg *Config) ([]ContentFile, error)

// resolveInstalledSkillsFunc is set by the includes package during init
var resolveInstalledSkillsFunc ResolveInstalledSkillsCallback

// SetResolveInstalledSkillsCallback sets the callback for resolving installed skills
func SetResolveInstalledSkillsCallback(fn ResolveInstalledSkillsCallback) {
	resolveInstalledSkillsFunc = fn
}

// LoadConfig loads a configuration from the specified base directory.
// The baseDir should contain an .ai-rulez/ subdirectory (or, as a fallback,
// .config/ai-rulez/) with config.toml, config.yaml, or config.json.
func LoadConfig(ctx context.Context, baseDir string, opts ...LoadOption) (*Config, error) {
	dirName := ResolveConfigDirName(baseDir)
	if dirName == "" {
		dirName = aiRulezDirName
	}
	return LoadConfigFromDir(ctx, baseDir, dirName, opts...)
}

// LoadConfigFromDir loads configuration from configDirName below baseDir.
func LoadConfigFromDir(ctx context.Context, baseDir, configDirName string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, oops.
			With("path", baseDir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	if configDirName == "" {
		configDirName = aiRulezDirName
	}
	configDir := filepath.Join(absDir, filepath.FromSlash(configDirName))

	if info, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			return nil, oops.
				With("path", configDir).
				With("base_dir", baseDir).
				Hint(fmt.Sprintf("Create %s/ directory in %s\nRun 'ai-rulez init' to initialize configuration", configDirName, baseDir)).
				Errorf("%s directory not found", configDirName)
		}
		return nil, oops.
			With("path", configDir).
			Wrapf(err, "stat config directory")
	} else if !info.IsDir() {
		return nil, oops.
			With("path", configDir).
			Hint("Remove the file and create a directory instead").
			Errorf("%s exists but is not a directory", configDirName)
	}

	config, err := loadConfigFile(configDir, lo)
	if err != nil {
		return nil, err
	}

	return finishLoadConfig(ctx, config, absDir, configDir, lo)
}

// LoadConfigFromFile loads a configuration from an exact config file path or
// from a config directory path. For a file path, the project base directory is
// the parent of the config directory (skipping a generic .config/ wrapper).
func LoadConfigFromFile(ctx context.Context, path string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check if the config path is valid and accessible").
			Wrapf(err, "resolve absolute config path")
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, oops.
			With("path", absPath).
			Wrapf(err, "stat config path")
	}

	if info.IsDir() {
		if hasConfigFile(absPath) {
			cfg, loadErr := loadConfigFile(absPath, lo)
			if loadErr != nil {
				return nil, loadErr
			}
			return finishLoadConfig(ctx, cfg, projectBaseDir(absPath), absPath, lo)
		}
		return LoadConfig(ctx, absPath, opts...)
	}

	if isLocalConfigFilename(filepath.Base(absPath)) {
		return nil, oops.
			With("path", absPath).
			Hint("Pass the main config; the overlay is applied automatically").
			Errorf("%s is a local overlay, not a main config", filepath.Base(absPath))
	}

	cfg, err := loadConfigFilePath(absPath, lo)
	if err != nil {
		return nil, err
	}
	configDir := filepath.Dir(absPath)
	if looksLikeProjectRoot(configDir) {
		return nil, oops.
			With("path", absPath).
			Hint("Place config files inside a configuration directory such as .ai-rulez/config.toml, or pass a config directory path. This keeps generated outputs rooted in the project instead of the parent directory.").
			Errorf("directory layout required for root-level config file")
	}
	return finishLoadConfig(ctx, cfg, projectBaseDir(configDir), configDir, lo)
}

func looksLikeProjectRoot(dir string) bool {
	for _, marker := range []string{gitDirName, "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "Taskfile.yml", "Taskfile.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func hasConfigFile(dir string) bool {
	for _, name := range []string{configTOMLFilename, configYAMLFilename, configYMLFilename, configJSONFilename} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func finishLoadConfig(ctx context.Context, config *Config, baseDir, configDir string, lo loadOptions) (*Config, error) {
	config.BaseDir = baseDir
	config.ConfigDir = configDir
	config.ConfigDirName = relConfigDirName(baseDir, configDir)

	// Convert inline MCP servers to map
	config.MCPServers = serversToMap(config.MCPServersRaw)

	// Backward compat: load legacy separate mcp.yaml/mcp.toml if present
	if legacyServers := loadLegacyMCPFile(configDir); len(legacyServers) > 0 {
		logger.Warn("Separate mcp.yaml/mcp.toml is deprecated — add mcp_servers to your config file instead")
		for name, server := range legacyServers {
			if _, exists := config.MCPServers[name]; !exists {
				config.MCPServers[name] = server
			}
		}
	}

	// Scan content directories
	scanner := newProjectScanner(baseDir)
	contentTree, err := scanContentTree(scanner, configDir, config.BundleExclude)
	if err != nil {
		return nil, err
	}
	config.Content = contentTree

	// Scan machine-local override content into a SEPARATE tree. This never
	// enters config.Content, so it cannot leak into committed output; it is
	// emitted only to the per-preset ".local" root variants.
	if !lo.withoutLocal {
		localTree, err := scanLocalContentTree(scanner, configDir, config.BundleExclude)
		if err != nil {
			return nil, err
		}
		config.LocalContent = localTree
	}
	config.ContentProblems = scanner.problems

	// Load builtins (lowest priority — loaded first so includes and local override them).
	// The root `builtins` field is global; `builtin:<name>` references in profiles
	// load packs scoped to the profiles that name them.
	loadBuiltins(config)

	if lo.includeMemo != nil {
		config.IncludeMemo = lo.includeMemo
	}

	if lo.withoutRemote {
		return config, nil
	}

	if err := resolveIncludesIfNeeded(ctx, configDir, config); err != nil {
		return nil, err
	}

	if err := resolveInstalledSkillsIfNeeded(ctx, config); err != nil {
		return nil, err
	}

	return config, nil
}

func resolveIncludesIfNeeded(ctx context.Context, configDir string, config *Config) error {
	if len(config.Includes) == 0 {
		return nil
	}

	if resolveIncludesFunc == nil {
		return oops.
			With("config_dir", configDir).
			Hint("Includes are configured but the includes resolver is not registered.\nImport github.com/Goldziher/ai-rulez/v5/internal/includes (blank import) before loading configs, or ensure the CLI/mcp entrypoint is used.").
			Errorf("includes configured but includes resolver is unavailable")
	}

	logger.Debug("Resolving includes", "count", len(config.Includes))

	mergedContent, err := resolveIncludesFunc(ctx, config)
	if err != nil {
		if errors.Is(err, ErrLockViolation) || errors.Is(err, ErrIncludeOutsideProject) {
			return err
		}
		logger.Warn("Failed to resolve includes", "error", err)
		// Continue with local content only (non-fatal)
		return nil
	}

	// Replace content with merged version
	config.Content = mergedContent
	logger.Debug("Successfully resolved includes",
		"rules", len(mergedContent.Rules),
		"context", len(mergedContent.Context),
		"skills", len(mergedContent.Skills),
		"agents", len(mergedContent.Agents))

	return nil
}

func resolveInstalledSkillsIfNeeded(ctx context.Context, config *Config) error {
	if len(config.InstalledSkills) == 0 {
		return nil
	}

	if resolveInstalledSkillsFunc == nil {
		return oops.
			Hint("Installed skills are configured but the resolver is not registered.\nImport github.com/Goldziher/ai-rulez/v5/internal/includes (blank import) before loading configs.").
			Errorf("installed skills configured but resolver is unavailable")
	}

	logger.Debug("Resolving installed skills", "count", len(config.InstalledSkills))

	skills, err := resolveInstalledSkillsFunc(ctx, config)
	if err != nil {
		if errors.Is(err, ErrLockViolation) {
			return err
		}
		logger.Warn("Failed to resolve installed skills", "error", err)
		return nil
	}

	if config.Content == nil {
		config.Content = &ContentTree{
			Domains: make(map[string]*Domain),
		}
	}

	// Build set of existing local skill names
	existingNames := make(map[string]bool)
	for _, s := range config.Content.Skills {
		existingNames[s.Name] = true
	}

	// Map each installed skill's profile scope so generation can drop it from
	// profiles it was not declared for.
	profilesByName := make(map[string][]string, len(config.InstalledSkills))
	for i := range config.InstalledSkills {
		profilesByName[config.InstalledSkills[i].Name] = config.InstalledSkills[i].Profiles
	}

	// Merge: local skills win over installed skills
	for _, s := range skills {
		if existingNames[s.Name] {
			logger.Warn("Installed skill name conflicts with local skill, skipping", "name", s.Name)
			continue
		}
		s.Profiles = profilesByName[s.Name]
		config.Content.Skills = append(config.Content.Skills, s)
		existingNames[s.Name] = true
	}

	logger.Debug("Successfully resolved installed skills", "count", len(skills))
	return nil
}

// loadConfigFile loads config.toml, config.yaml, or config.json from a config directory.
func loadConfigFile(configDir string, lo loadOptions) (*Config, error) {
	// Try TOML first (V4 preferred format)
	tomlPath := filepath.Join(configDir, configTOMLFilename)
	if _, err := os.Stat(tomlPath); err == nil {
		cfg, err := loadConfigTOML(tomlPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configTOMLFilename
		return withLocalOverlay(cfg, tomlPath, configDir, lo)
	}

	// Try YAML (deprecated in V4)
	yamlPath := filepath.Join(configDir, configYAMLFilename)
	if _, err := os.Stat(yamlPath); err == nil {
		logger.Warn("YAML config is deprecated; run 'ai-rulez migrate v4' to convert to TOML")
		cfg, err := loadConfigYAML(yamlPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configYAMLFilename
		return withLocalOverlay(cfg, yamlPath, configDir, lo)
	}

	// Try JSON
	jsonPath := filepath.Join(configDir, configJSONFilename)
	if _, err := os.Stat(jsonPath); err == nil {
		cfg, err := loadConfigJSON(jsonPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configJSONFilename
		return withLocalOverlay(cfg, jsonPath, configDir, lo)
	}

	return nil, oops.
		With("config_dir", configDir).
		With("toml_path", tomlPath).
		With("yaml_path", yamlPath).
		With("json_path", jsonPath).
		Hint(fmt.Sprintf("Create %s, %s, or %s in %s\nRun 'ai-rulez init' to initialize configuration", configTOMLFilename, configYAMLFilename, configJSONFilename, configDir)).
		Errorf("no config file found (tried %s, %s, and %s)", configTOMLFilename, configYAMLFilename, configJSONFilename)
}

func loadConfigFilePath(path string, lo loadOptions) (*Config, error) {
	cfg, err := loadConfigFilePathMain(path)
	if err != nil {
		return nil, err
	}
	return withLocalOverlay(cfg, path, filepath.Dir(path), lo)
}

func loadConfigFilePathMain(path string) (*Config, error) {
	switch filepath.Base(path) {
	case configTOMLFilename:
		cfg, err := loadConfigTOML(path)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = filepath.Base(path)
		return cfg, nil
	case configYAMLFilename, configYMLFilename:
		if filepath.Base(path) == configYAMLFilename {
			logger.Warn("YAML config is deprecated; run 'ai-rulez migrate v4' to convert to TOML")
		}
		cfg, err := loadConfigYAML(path)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = filepath.Base(path)
		return cfg, nil
	case configJSONFilename:
		cfg, err := loadConfigJSON(path)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = filepath.Base(path)
		return cfg, nil
	default:
		return nil, oops.
			With("path", path).
			Hint("Use config.toml, config.yaml, config.yml, or config.json inside a config directory").
			Errorf("unsupported config filename: %s", filepath.Base(path))
	}
}

// loadConfigYAML loads a config from YAML
func loadConfigYAML(path string) (*Config, error) {
	data, err := readCapped(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}
	return decodeConfigYAML(data, path)
}

// decodeConfigYAML decodes YAML bytes; path is used for error context only.
func decodeConfigYAML(data []byte, path string) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the YAML syntax - ensure proper indentation\nValidate your YAML at: https://www.yamllint.com/\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse YAML config")
	}

	return &config, nil
}

// loadConfigJSON loads a config from JSON
func loadConfigJSON(path string) (*Config, error) {
	data, err := readCapped(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}
	return decodeConfigJSON(data, path)
}

// decodeConfigJSON decodes JSON bytes; path is used for error context only.
func decodeConfigJSON(data []byte, path string) (*Config, error) {
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the JSON syntax - ensure proper formatting\nValidate your JSON at: https://jsonlint.com/\nCommon issues: trailing commas, unquoted keys, missing brackets").
			Wrapf(err, "parse JSON config")
	}

	return &config, nil
}

// loadConfigTOML loads a config from TOML
func loadConfigTOML(path string) (*Config, error) {
	data, err := readCapped(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}
	return decodeConfigTOML(data, path)
}

// decodeConfigTOML decodes TOML bytes; path is used for error context only.
func decodeConfigTOML(data []byte, path string) (*Config, error) {
	// TOML presets are plain strings; we unmarshal into an intermediate
	// struct then convert to []Preset. This avoids custom unmarshaler
	// issues with the TOML library.
	type tomlConfig struct {
		Schema          string                 `toml:"schema"`
		Version         string                 `toml:"version"`
		Name            string                 `toml:"name"`
		Description     string                 `toml:"description"`
		Presets         []any                  `toml:"presets"`
		Default         string                 `toml:"default"`
		Profiles        map[string][]string    `toml:"profiles"`
		Gitignore       *bool                  `toml:"gitignore"`
		Compact         *bool                  `toml:"compact"`
		AgentsMD        bool                   `toml:"agents_md"`
		BundleExclude   []string               `toml:"bundle_exclude"`
		CodexSkillsDir  string                 `toml:"codex_skills_dir"`
		Includes        []IncludeConfig        `toml:"includes"`
		InstalledSkills []InstalledSkillConfig `toml:"installed_skills"`
		MCPServers      []MCPServer            `toml:"mcp_servers"`
		Header          *HeaderConfig          `toml:"header"`
		Builtins        interface{}            `toml:"builtins"`
		Plugins         []PluginConfig         `toml:"plugins"`
		Marketplaces    []MarketplaceConfig    `toml:"marketplaces"`
		Scopes          []ScopeConfig          `toml:"scopes"`
		MCP             *MCPConfig             `toml:"mcp"`
		Defaults        *DefaultsConfig        `toml:"defaults"`
		Rules           *RulesConfig           `toml:"rules"`
		Lint            *LintConfig            `toml:"lint"`
		Verifiers       []VerifierConfig       `toml:"verifiers"`
		Usage           *UsageConfig           `toml:"usage"`
		Skills          *SkillsConfig          `toml:"skills"`
		DomainSettings  DomainConfigs          `toml:"domains"`
		SkillSources    []SkillSourceConfig    `toml:"skill_sources"`
		Roles           []RoleConfig           `toml:"roles"`
		RoleManifest    *RoleManifestConfig    `toml:"role_manifest"`
		Lock            *LockConfig            `toml:"lock"`
		LLM             *llm.Config            `toml:"llm"`
		Telemetry       *TelemetryConfig       `toml:"telemetry"`
		Plugin          *PluginAuthoring       `toml:"plugin"`
		Marketplace     *MarketplaceAuthoring  `toml:"marketplace"`
		Placement       *PlacementConfig       `toml:"placement"`
		Claude          *ClaudeConfig          `toml:"claude"`
		Codex           *CodexConfig           `toml:"codex"`
		Hooks           []HookGroup            `toml:"hooks"`
		Guard           *GuardConfig           `toml:"guard"`
		Permissions     *Permissions           `toml:"permissions"`
		OKF             *OKFConfig             `toml:"okf"`
	}

	var raw tomlConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		if swapped := swappedLintTablesTOML(path, data); swapped != nil {
			return nil, swapped
		}
		return nil, oops.
			With("path", path).
			Hint("Check the TOML syntax - ensure proper formatting\nCommon issues: missing quotes around strings, incorrect table syntax").
			Wrapf(err, "parse TOML config")
	}

	warnDeprecatedLintBudget(path, raw.Lint)

	presets, err := presetsFromTOML(raw.Presets)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse presets")
	}

	// Convert builtins from interface{} to BuiltinsConfig
	var builtinsCfg *BuiltinsConfig
	if raw.Builtins != nil {
		builtinsCfg = &BuiltinsConfig{}
		switch v := raw.Builtins.(type) {
		case bool:
			builtinsCfg.All = &v
		case []interface{}:
			names := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					names = append(names, s)
				}
			}
			builtinsCfg.Names = names
		}
	}

	cfg := &Config{
		Schema:          raw.Schema,
		Version:         raw.Version,
		Name:            raw.Name,
		Description:     raw.Description,
		Presets:         presets,
		Default:         raw.Default,
		Profiles:        raw.Profiles,
		Gitignore:       raw.Gitignore,
		Compact:         raw.Compact,
		AgentsMD:        raw.AgentsMD,
		BundleExclude:   raw.BundleExclude,
		CodexSkillsDir:  raw.CodexSkillsDir,
		Includes:        raw.Includes,
		InstalledSkills: raw.InstalledSkills,
		MCPServersRaw:   raw.MCPServers,
		Header:          raw.Header,
		Builtins:        builtinsCfg,
		Plugins:         raw.Plugins,
		Marketplaces:    raw.Marketplaces,
		Scopes:          raw.Scopes,
		MCP:             raw.MCP,
		Defaults:        raw.Defaults,
		Rules:           raw.Rules,
		Lint:            raw.Lint,
		Verifiers:       raw.Verifiers,
		Usage:           raw.Usage,
		Skills:          raw.Skills,
		DomainSettings:  raw.DomainSettings,
		SkillSources:    raw.SkillSources,
		Roles:           raw.Roles,
		RoleManifest:    raw.RoleManifest,
		Lock:            raw.Lock,
		LLM:             raw.LLM,
		Telemetry:       raw.Telemetry,
		Plugin:          raw.Plugin,
		Marketplace:     raw.Marketplace,
		Placement:       raw.Placement,
		Claude:          raw.Claude,
		Codex:           raw.Codex,
		Hooks:           raw.Hooks,
		Guard:           raw.Guard,
		Permissions:     raw.Permissions,
		OKF:             raw.OKF,
	}

	return cfg, nil
}

// presetsFromTOML converts the TOML `presets` array — a mix of built-in name
// strings and custom/provider inline tables (TOML 1.0 allows mixed arrays) —
// into typed Presets. Inline tables are re-encoded as JSON and routed through
// Preset.UnmarshalJSON so the YAML/JSON/TOML object forms share one code path.
func presetsFromTOML(items []any) ([]Preset, error) {
	presets := make([]Preset, 0, len(items))
	for i, item := range items {
		switch v := item.(type) {
		case string:
			presets = append(presets, Preset{BuiltIn: v})
		case map[string]interface{}:
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, oops.With("index", i).Wrapf(err, "encode preset entry")
			}
			var preset Preset
			if err := preset.UnmarshalJSON(encoded); err != nil {
				return nil, oops.With("index", i).Wrapf(err, "decode preset entry")
			}
			presets = append(presets, preset)
		default:
			return nil, oops.
				With("index", i).
				Hint("Each preset must be a built-in name string or a custom preset table").
				Errorf("invalid preset entry of type %T", item)
		}
	}
	return presets, nil
}

// ScanContentTree scans all content directories and returns a populated ContentTree.
// Root content goes into the top-level slices; domain content goes only into the Domains map.
// This keeps the two layers separate so callers (e.g. include sources) can merge without duplication.
func ScanContentTree(configDir string) (*ContentTree, error) {
	return ScanContentTreeWith(configDir, nil)
}

// ScanContentTreeWith is ScanContentTree with extra bundle_exclude patterns
// applied to the resources of every skill and command.
func ScanContentTreeWith(configDir string, bundleExclude []string) (*ContentTree, error) {
	return scanContentTree(&contentScanner{}, configDir, bundleExclude)
}

// scanContentTree scans configDir under the symlink policy held by s.
func scanContentTree(s *contentScanner, configDir string, bundleExclude []string) (*ContentTree, error) {
	if !s.admitTreeRoot(configDir) {
		return &ContentTree{Domains: make(map[string]*Domain)}, nil
	}
	tree := &ContentTree{
		Domains: make(map[string]*Domain),
	}

	// Scan root rules/
	rulesPath := filepath.Join(configDir, rulesDir)
	var rules []ContentFile
	var err error
	if rules, err = s.markdownFiles(rulesPath); err != nil {
		return nil, oops.
			With("path", rulesPath).
			Wrapf(err, "scan rules directory")
	}
	tree.Rules = rules

	// Scan root context/
	contextPath := filepath.Join(configDir, contextDir)
	var contextFiles []ContentFile
	if contextFiles, err = s.markdownFiles(contextPath); err != nil {
		return nil, oops.
			With("path", contextPath).
			Wrapf(err, "scan context directory")
	}
	tree.Context = contextFiles

	// Scan root skills/
	skillsPath := filepath.Join(configDir, skillsDir)
	var skills []ContentFile
	if skills, err = s.skills(skillsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", skillsPath).
			Wrapf(err, "scan skills directory")
	}
	tree.Skills = skills

	// Scan root agents/
	agentsPath := filepath.Join(configDir, agentsDir)
	var agents []ContentFile
	if agents, err = s.agents(agentsPath); err != nil {
		return nil, oops.
			With("path", agentsPath).
			Wrapf(err, "scan agents directory")
	}
	tree.Agents = agents
	logger.Debug("Scanned agents directory", "path", agentsPath, "count", len(agents))

	// Scan root commands/
	commandsPath := filepath.Join(configDir, commandsDir)
	var commands []ContentFile
	if commands, err = s.commands(commandsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", commandsPath).
			Wrapf(err, "scan commands directory")
	}
	tree.Commands = commands
	logger.Debug("Scanned commands directory", "path", commandsPath, "count", len(commands))

	// Scan root checks/
	checksPath := filepath.Join(configDir, checksDir)
	var checks []ContentFile
	if checks, err = s.markdownFiles(checksPath); err != nil {
		return nil, oops.
			With("path", checksPath).
			Wrapf(err, "scan checks directory")
	}
	tree.Checks = checks

	// Scan domains/
	domainsPath := filepath.Join(configDir, domainsDir)
	var domains map[string]*Domain
	if domains, err = s.domains(domainsPath, bundleExclude); err != nil {
		return nil, oops.
			With("path", domainsPath).
			Wrapf(err, "scan domains directory")
	}
	tree.Domains = domains

	return tree, nil
}

// ScanLocalContentTree scans machine-local override content from
// <configDir>/local/, which mirrors the shared layout: rules, context, skills,
// agents, commands and domains. The tree is kept separate from the committed
// content tree so local content is only ever written to gitignored outputs.
func ScanLocalContentTree(configDir string) (*ContentTree, error) {
	return ScanLocalContentTreeWith(configDir, nil)
}

// ScanLocalContentTreeWith is ScanLocalContentTree with extra bundle_exclude patterns.
func ScanLocalContentTreeWith(configDir string, bundleExclude []string) (*ContentTree, error) {
	return scanLocalContentTree(&contentScanner{}, configDir, bundleExclude)
}

func scanLocalContentTree(s *contentScanner, configDir string, bundleExclude []string) (*ContentTree, error) {
	localBase := filepath.Join(configDir, localDir)
	tree, err := scanContentTree(s, localBase, bundleExclude)
	if err != nil {
		return nil, oops.With("path", localBase).Wrapf(err, "scan local content")
	}
	return tree, nil
}

// scanMarkdownFiles scans a directory for .md files and returns ContentFile entries
func (s *contentScanner) markdownFiles(dir string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var files []ContentFile

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if isDir, ok := s.entryInfo(filePath, entry); !ok || isDir {
			continue
		}
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		files = append(files, contentFile)
	}

	return files, nil
}

// scanSkills scans the skills/ directory for SKILL.md files in subdirectories
func (s *contentScanner) skills(skillsDir string, bundleExclude []string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(skillsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var skills []ContentFile

	for _, entry := range entries {
		skillRoot := filepath.Join(skillsDir, entry.Name())
		if isDir, ok := s.entryInfo(skillRoot, entry); !ok || !isDir {
			continue
		}

		skillPath := filepath.Join(skillRoot, skillMarkerFile)
		if _, err := os.Lstat(skillPath); os.IsNotExist(err) {
			// No SKILL.md file, skip this directory
			continue
		}

		contentFile, err := s.loadFile(skillPath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		// Override the name with the directory name instead of filename
		contentFile.Name = entry.Name()

		// Load skill supporting files (references/, scripts/, assets/) so
		// presets can preserve the canonical Agent Skills layout instead of
		// concatenating everything into SKILL.md.
		resources, resErr := s.loadResources(skillRoot, ItemKindSkill, bundleExclude)
		if resErr != nil {
			logger.Warn("Failed to load skill resources", "skill", entry.Name(), "error", resErr)
		}
		contentFile.Resources = resources

		skills = append(skills, contentFile)
	}

	return skills, nil
}

// scanCommands scans the commands/ directory for .md files (flat form) and
// COMMAND.md files in subdirectories (directory form with optional resources/).
// Mirrors the structure of scanSkills to support bundled reference material.
func scanCommands(commandsDir string) ([]ContentFile, error) {
	return (&contentScanner{}).commands(commandsDir, nil)
}

func (s *contentScanner) commands(commandsDir string, bundleExclude []string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(commandsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var commands []ContentFile

	for _, entry := range entries {
		entryPath := filepath.Join(commandsDir, entry.Name())
		isDir, ok := s.entryInfo(entryPath, entry)
		if !ok {
			continue
		}
		if isDir {
			// Directory structure: commands/name/COMMAND.md
			commandRoot := entryPath
			commandPath := filepath.Join(commandRoot, commandMarkerFile)
			if _, err := os.Lstat(commandPath); os.IsNotExist(err) {
				// No COMMAND.md file, skip this directory
				continue
			}

			contentFile, err := s.loadFile(commandPath)
			if err != nil {
				// Non-fatal: one unreadable command must not fail the whole
				// load, but dropping it without a diagnostic makes an
				// unreadable COMMAND.md indistinguishable from a missing one.
				logger.Warn("failed to load command file", "path", commandPath, "error", err)
				continue
			}

			// Override the name with the directory name instead of filename
			contentFile.Name = entry.Name()

			// Load command supporting files (references/, scripts/, assets/) so
			// presets can preserve the canonical layout instead of concatenating
			// everything into COMMAND.md.
			resources, resErr := s.loadResources(commandRoot, ItemKindCommand, bundleExclude)
			if resErr != nil {
				logger.Warn("Failed to load command resources", "command", entry.Name(), "error", resErr)
			}
			contentFile.Resources = resources

			commands = append(commands, contentFile)
			continue
		}

		// Flat file structure: commands/name.md
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := entryPath
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			logger.Warn("failed to load command file", "path", filePath, "error", err)
			continue
		}

		commands = append(commands, contentFile)
	}

	return commands, nil
}

// scanAgents scans the agents/ directory for .md files
func (s *contentScanner) agents(agentsPath string) ([]ContentFile, error) {
	entries, ok, err := s.dirEntries(agentsPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContentFile{}, nil
	}

	var agents []ContentFile

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(agentsPath, entry.Name())
		if isDir, ok := s.entryInfo(filePath, entry); !ok || isDir {
			continue
		}
		contentFile, err := s.loadFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		agents = append(agents, contentFile)
	}

	return agents, nil
}

// scanDomains scans the domains/ directory and returns a map of domain name to Domain
func (s *contentScanner) domains(domainsDir string, bundleExclude []string) (map[string]*Domain, error) {
	entries, ok, err := s.dirEntries(domainsDir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return make(map[string]*Domain), nil
	}

	domains := make(map[string]*Domain)

	for _, entry := range entries {
		domainName := entry.Name()
		domainPath := filepath.Join(domainsDir, domainName)
		if isDir, ok := s.entryInfo(domainPath, entry); !ok || !isDir {
			continue
		}

		domain := &Domain{
			Name: domainName,
		}

		// Scan domain/rules/
		rulesPath := filepath.Join(domainPath, rulesDir)
		var rules []ContentFile
		var err error
		if rules, err = s.markdownFiles(rulesPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", rulesPath).
				Wrapf(err, "scan domain rules")
		}
		domain.Rules = rules

		// Scan domain/context/
		contextPath := filepath.Join(domainPath, contextDir)
		var contextFiles []ContentFile
		if contextFiles, err = s.markdownFiles(contextPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", contextPath).
				Wrapf(err, "scan domain context")
		}
		domain.Context = contextFiles

		// Scan domain/skills/
		skillsPath := filepath.Join(domainPath, skillsDir)
		var skills []ContentFile
		if skills, err = s.skills(skillsPath, bundleExclude); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", skillsPath).
				Wrapf(err, "scan domain skills")
		}
		domain.Skills = skills

		// Scan domain/agents/
		agentsPath := filepath.Join(domainPath, agentsDir)
		var agentsContent []ContentFile
		if agentsContent, err = s.agents(agentsPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", agentsPath).
				Wrapf(err, "scan domain agents")
		}
		domain.Agents = agentsContent

		// Scan domain/commands/
		domainCommandsPath := filepath.Join(domainPath, commandsDir)
		var domainCommands []ContentFile
		if domainCommands, err = s.commands(domainCommandsPath, bundleExclude); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", domainCommandsPath).
				Wrapf(err, "scan domain commands")
		}
		domain.Commands = domainCommands
		logger.Debug("Scanned domain commands directory", "domain", domainName, "path", domainCommandsPath, "count", len(domainCommands))

		// Scan domain/checks/
		domainChecksPath := filepath.Join(domainPath, checksDir)
		var domainChecks []ContentFile
		if domainChecks, err = s.markdownFiles(domainChecksPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", domainChecksPath).
				Wrapf(err, "scan domain checks")
		}
		domain.Checks = domainChecks

		domains[domainName] = domain
	}

	return domains, nil
}

// scanAgents scans one agents directory without following symlinks.
func scanAgents(agentsPath string) ([]ContentFile, error) {
	return (&contentScanner{}).agents(agentsPath)
}

// ParseFrontmatterPublic is the exported version of parseFrontmatter for use by other packages
func ParseFrontmatterPublic(content string) (metadata *Metadata, body string) {
	metadata, body, _ = parseFrontmatter(content)
	return metadata, body
}

// parseFrontmatter parses optional YAML frontmatter from content.
// Returns metadata (nil if none), the actual content (without frontmatter),
// and a malformed flag set to true when a delimited frontmatter block was
// present but its YAML was unparseable (e.g. unquoted values containing ": ").
func parseFrontmatter(content string) (metadata *Metadata, body string, malformed bool) {
	// Check if content starts with ---
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, content, false
	}

	// Find the closing ---
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return nil, content, false
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		// No closing ---, treat as regular content
		return nil, content, false
	}

	// Extract frontmatter YAML
	frontmatterLines := lines[1:endIdx]
	frontmatterYAML := strings.Join(frontmatterLines, "\n")

	// Extract actual content (after the closing ---). Computed up front so a
	// parse failure still strips the delimited block from the body.
	body = strings.Join(lines[endIdx+1:], "\n")
	body = strings.TrimPrefix(body, "\n")

	// First try direct unmarshal into Metadata (works for simple key-value frontmatter)
	var parsedMetadata Metadata
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &parsedMetadata); err != nil {
		// Direct unmarshal failed (e.g., nested YAML objects in Extra fields).
		// Fall back to raw map parsing: extract known fields and stringify the rest.
		result, ok := parseFrontmatterFromRawMap(frontmatterYAML)
		if !ok {
			// A delimited frontmatter block is present but its YAML is
			// unparseable (e.g. an unquoted value containing ": "). Do NOT
			// return the content unstripped — that would re-emit the raw block
			// after the generated frontmatter (#156). Warn loudly and strip it.
			// Mark it malformed so Config.Validate can fail fast (#175).
			logger.Warn("Ignoring malformed YAML frontmatter — check for unquoted values containing ': '",
				"frontmatter", frontmatterYAML)
			return nil, body, true
		}
		parsedMetadata = result
	}

	parsedMetadata.extraNodes = extraNodes(frontmatterYAML)
	metadata = &parsedMetadata
	return metadata, body, false
}

// parseFrontmatterFromRawMap parses frontmatter YAML into Metadata via a raw map.
// Used as fallback when direct unmarshal fails (e.g., nested YAML objects).
func parseFrontmatterFromRawMap(frontmatterYAML string) (Metadata, bool) {
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &rawMap); err != nil {
		return Metadata{}, false
	}

	m := Metadata{Extra: make(map[string]string)}
	nodes := extraNodes(frontmatterYAML)
	for k, v := range rawMap {
		if dst := m.scalarField(k); dst != nil {
			*dst = fmt.Sprintf("%v", v)
			continue
		}
		switch k {
		case "priority":
			m.Priority = fmt.Sprintf("%v", v)
		case "targets":
			m.Targets = stringSliceFromAny(v)
		case "aliases":
			m.Aliases = stringSliceFromAny(v)
		case "tools":
			m.Tools = stringSliceFromAny(v)
		case "skills":
			m.Skills = stringSliceFromAny(v)
		case "keywords":
			m.Keywords = stringSliceFromAny(v)
		case "globs":
			m.Globs = NormalizeGlobs(stringSliceFromAny(v))
		case "paths":
			m.Paths = NormalizeGlobs(stringSliceFromAny(v))
		default:
			m.Extra[k] = extraText(nodes[k], v)
		}
	}

	return m, true
}

// extraText is the string form of an extra frontmatter value. A scalar keeps its
// source text (a date stays 2026-10-01, not the Go time rendering); a collection
// becomes flow YAML.
func extraText(n *yaml.Node, v any) string {
	if n != nil {
		if n.Kind == yaml.ScalarNode {
			return n.Value
		}
		if text := flowText(n); text != "" {
			return text
		}
	}
	return fmt.Sprintf("%v", v)
}

// scalarField returns the destination for a plain string frontmatter key, or
// nil when the key is not one of them.
func (m *Metadata) scalarField(key string) *string {
	switch key {
	case "usage":
		return &m.Usage
	case "shortcut":
		return &m.Shortcut
	case "category":
		return &m.Category
	case "effort":
		return &m.Effort
	case "activation":
		return &m.Activation
	}
	return nil
}

// stringSliceFromAny coerces a YAML value into a []string. Accepts a sequence
// (most common case for tools/targets/etc) or a scalar (treated as a single-element
// list). Returns nil for unsupported types so callers can detect the absence.
func stringSliceFromAny(v interface{}) []string {
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case []string:
		return append([]string(nil), t...)
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

// loadContentFile loads a content file and parses optional frontmatter
//
// A symlinked file is refused and its target is never read: an include (or a
// cloned repository) could otherwise point a content file at any local file and
// have it rendered into the outputs, and the content lock does not pin it.
func loadContentFile(path string) (ContentFile, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		logger.Warn("Skipping symlinked content file; symlinks are not followed", "path", path)
		return ContentFile{}, oops.
			With("path", path).
			Errorf("content file %s is a symlink; symlinks are not followed", path)
	}
	return readContentFile(path)
}

// readContentFile reads and parses a content file the caller has already
// cleared under the symlink policy.
func readContentFile(path string) (ContentFile, error) {
	data, err := readCapped(path)
	if err != nil {
		return ContentFile{}, oops.
			With("path", path).
			Wrapf(err, "read content file")
	}

	content := string(data)
	filename := filepath.Base(path)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Parse frontmatter (if present) - inlined to avoid import cycle
	metadata, actualContent, malformed := parseFrontmatter(content)

	return ContentFile{
		Name:                 name,
		Path:                 path,
		Content:              actualContent,
		Metadata:             metadata,
		MalformedFrontmatter: malformed,
	}, nil
}

// serversToMap converts a slice of MCPServer to a map keyed by server name. The
// map holds copies: the generator mutates its servers while resolving
// placeholders, and the as-written slice must not change with them.
func serversToMap(servers []MCPServer) map[string]*MCPServer {
	result := make(map[string]*MCPServer)
	for i := range servers {
		clone := servers[i].Clone()
		result[servers[i].Name] = &clone
	}
	return result
}

// legacyMCPFileNames are the deprecated separate MCP files, in load order. Only
// the first one that exists is loaded.
var legacyMCPFileNames = []string{"mcp.toml", "mcp.yaml", "mcp.json"}

// loadLegacyMCPFile loads MCP servers from a deprecated separate mcp.toml/mcp.yaml/mcp.json file.
// Returns nil if no legacy file exists.
func loadLegacyMCPFile(configDir string) map[string]*MCPServer {
	for _, filename := range legacyMCPFileNames {
		path := filepath.Join(configDir, filename)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		servers, err := DecodeLegacyMCPFile(path)
		if err != nil {
			logger.Warn("Failed to load legacy MCP file", "path", path, "error", err)
			return nil
		}
		return serversToMap(servers)
	}
	return nil
}

// DecodeLegacyMCPFile reads the servers of one deprecated mcp.toml, mcp.yaml or
// mcp.json file, as written.
func DecodeLegacyMCPFile(path string) ([]MCPServer, error) {
	data, err := readCapped(path)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read legacy MCP file")
	}
	var cfg struct {
		Servers []MCPServer `yaml:"mcp_servers" json:"mcp_servers" toml:"mcp_servers"`
	}
	switch filepath.Ext(path) {
	case extTOML:
		err = toml.Unmarshal(data, &cfg)
	case extYAML, extYML:
		err = yaml.Unmarshal(data, &cfg)
	case extJSON:
		err = json.Unmarshal(data, &cfg)
	}
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse legacy MCP file")
	}
	return cfg.Servers, nil
}

// SaveConfig writes a configuration back to its original on-disk format.
// The target format is taken from cfg.ConfigFile when it names a file present in
// configDir (populated by the loader), falling back to probing config.toml,
// config.yaml, then config.json. TOML is written with MarshalTOML (comments are
// not preserved); YAML uses yaml.Node round-tripping to preserve comments,
// field ordering, and formatting from the original file.
func SaveConfig(cfg *Config, configDir string) error {
	if cfg == nil {
		return oops.
			With("config_dir", configDir).
			Hint("Provide a valid Config struct").
			Errorf("config is nil")
	}

	if cfg.LocalOverlay != nil {
		return errMergedConfigWrite()
	}

	targetPath := selectConfigWritePath(cfg, configDir)

	switch filepath.Base(targetPath) {
	case configTOMLFilename:
		data, err := MarshalTOML(cfg)
		if err != nil {
			return oops.With("path", targetPath).Wrapf(err, "marshal config to TOML")
		}
		return writeConfigAtomically(targetPath, data)
	case configJSONFilename:
		data, err := json.Marshal(cfg)
		if err != nil {
			return oops.With("path", targetPath).Wrapf(err, "marshal config to JSON")
		}
		return writeConfigAtomically(targetPath, data)
	default:
		data, err := marshalYAMLPreserving(cfg, targetPath)
		if err != nil {
			return oops.With("path", targetPath).Wrapf(err, "marshal config to YAML")
		}
		return writeConfigAtomically(targetPath, data)
	}
}

// selectConfigWritePath chooses the file SaveConfig rewrites. An explicit
// cfg.ConfigFile that exists wins; otherwise the same extension preference the
// loader uses (TOML, then YAML, then JSON). A TOML-only project must never be
// handed a freshly created config.yaml, which the loader would then shadow.
func selectConfigWritePath(cfg *Config, configDir string) string {
	if cfg != nil && cfg.ConfigFile != "" {
		candidate := filepath.Join(configDir, cfg.ConfigFile)
		if fileExists(candidate) {
			return candidate
		}
	}
	for _, name := range []string{configTOMLFilename, configYAMLFilename, configYMLFilename, configJSONFilename} {
		candidate := filepath.Join(configDir, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return filepath.Join(configDir, configTOMLFilename)
}

// writeConfigAtomically writes data to path via a temp file + rename. An
// existing file keeps its permission bits; a new one is created 0644.
func writeConfigAtomically(targetPath string, data []byte) error {
	perm := os.FileMode(0o644)
	if info, err := os.Stat(targetPath); err == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(targetPath, data, perm)
}

// writeFileAtomic writes data to path with the given permissions: into an
// exclusively created temp file beside it (so a stale or attacker-placed file can
// never be reused), then synced and renamed over the target.
func writeFileAtomic(targetPath string, data []byte, perm os.FileMode) error {
	return gitutil.WriteFileAtomic(targetPath, data, perm) //nolint:wrapcheck // already contextual
}

// marshalYAMLPreserving marshals a Config to YAML while preserving
// the original file's field ordering, comments, and formatting.
// If the original file doesn't exist, falls back to plain marshal.
func marshalYAMLPreserving(cfg *Config, existingPath string) ([]byte, error) {
	// Read existing file
	existingData, readErr := os.ReadFile(existingPath)
	if readErr != nil {
		// File doesn't exist yet — plain marshal
		return yaml.Marshal(cfg)
	}

	// Parse existing file into a yaml.Node document tree
	var origDoc yaml.Node
	if err := yaml.Unmarshal(existingData, &origDoc); err != nil {
		// Can't parse original — fall back to plain marshal
		return yaml.Marshal(cfg)
	}

	// Marshal the new config into a fresh yaml.Node document tree
	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var newDoc yaml.Node
	if err := yaml.Unmarshal(newData, &newDoc); err != nil {
		return nil, err
	}

	// Both documents should have Kind==DocumentNode with one MappingNode child
	origMapping := getDocumentMapping(&origDoc)
	newMapping := getDocumentMapping(&newDoc)
	if origMapping == nil || newMapping == nil {
		// Unexpected structure — fall back to plain marshal
		return yaml.Marshal(cfg)
	}

	// Merge new values into original tree (preserving order/comments)
	mergeYAMLMappings(origMapping, newMapping)

	// Encode the preserved document back to YAML
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&origDoc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	return []byte(buf.String()), nil
}

// getDocumentMapping returns the top-level MappingNode from a DocumentNode.
func getDocumentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 && doc.Content[0].Kind == yaml.MappingNode {
		return doc.Content[0]
	}
	if doc.Kind == yaml.MappingNode {
		return doc
	}
	return nil
}

// mergeYAMLMappings merges new mapping values into orig, preserving orig's
// key order and comments. Keys present in new but not orig are appended.
// Keys present in orig but not new are removed.
func mergeYAMLMappings(orig, updated *yaml.Node) {
	// Build index of updated keys → value nodes
	updatedKeys := make(map[string]*yaml.Node)
	for i := 0; i+1 < len(updated.Content); i += 2 {
		updatedKeys[updated.Content[i].Value] = updated.Content[i+1]
	}

	// Update existing keys in orig (preserve key node with its comments)
	// and track which orig keys still exist in updated
	kept := make([]*yaml.Node, 0, len(orig.Content))
	for i := 0; i+1 < len(orig.Content); i += 2 {
		keyNode := orig.Content[i]
		origValNode := orig.Content[i+1]

		if updatedValNode, exists := updatedKeys[keyNode.Value]; exists {
			// Key exists in both: update value, keep original key node (comments preserved)
			kept = append(kept, keyNode, updatedValNode)
			// If both values are mappings, recurse to preserve nested comments
			if origValNode.Kind == yaml.MappingNode && updatedValNode.Kind == yaml.MappingNode {
				mergeYAMLMappings(origValNode, updatedValNode)
				kept[len(kept)-1] = origValNode // use the recursively-merged original
			}
			delete(updatedKeys, keyNode.Value)
		}
		// else: key removed from updated config — drop it
	}

	// Append keys that are new (not in orig)
	for i := 0; i+1 < len(updated.Content); i += 2 {
		keyNode := updated.Content[i]
		if valNode, stillNew := updatedKeys[keyNode.Value]; stillNew {
			kept = append(kept, keyNode, valNode)
		}
	}

	orig.Content = kept
}

// fileExists checks if a file exists (utility function for SaveConfig)
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// warnUnknownBuiltinExclusions logs a warning for every "!name" entry in the
// builtins list that excludes something that does not exist. Such an entry is
// otherwise inert: resolution simply never matches it, so the builtin the author
// meant to drop stays in every generated file with nothing reporting it.
//
// Deliberately a warning, not an error: builtins are renamed and retired upstream,
// and a project excluding a rule that has since moved into a skill must keep
// generating. This runs during config load, so `validate` and `generate` — and
// every other command that loads a config — report it alike.
func warnUnknownBuiltinExclusions(names []string) {
	for _, unknown := range builtins.UnknownExclusions(names) {
		args := []any{"exclusion", "!" + unknown.Spec}
		if unknown.Suggestion != "" {
			args = append(args, "did_you_mean", "!"+unknown.Suggestion)
		}
		logger.Warn("Unknown builtin exclusion ignored; nothing was suppressed", args...)
	}
}

// loadBuiltins resolves and loads builtin domains into the config content tree.
// Builtins have the lowest priority: they are injected into domains that don't already exist.
// If a domain already exists (from local content or includes), the builtin is skipped.
//
// Two sources are loaded. The root `builtins` field is global: it governs the pack
// set visible to every profile, including the auto-includes. A "builtin:<name>"
// reference in a profile's domain list is scoped: the pack is loaded only if it is
// not already global, and profile resolution then confines it to the profiles that
// name it. Because a profile reference is an explicit opt-in, it is honored even
// when the root field is absent or set to false.
func loadBuiltins(config *Config) {
	if config.Content == nil {
		config.Content = &ContentTree{
			Domains: make(map[string]*Domain),
		}
	}

	if config.Builtins.IsEnabled() && !config.Builtins.IsNone() {
		warnUnknownBuiltinExclusions(config.Builtins.GetNames())

		var resolved []string
		if config.Builtins.IsAll() {
			resolved = builtins.ResolveAll()
		} else {
			resolved = builtins.ResolveBuiltins(config.Builtins.GetNames())
		}
		if len(resolved) > 0 {
			logger.Debug("Loading builtins", "count", len(resolved), "names", resolved)
			loadBuiltinDomains(config, resolved, builtins.ExcludedRules(config.Builtins.GetNames()), false)
		}
	}

	for _, name := range config.ProfileBuiltinRefs() {
		if _, exists := config.Content.Domains[name]; exists {
			continue
		}
		loadBuiltinDomains(config, []string{name}, nil, true)
	}
}

// loadBuiltinDomains loads each named builtin pack into config.Content.Domains,
// skipping names a local or include domain already owns. When scoped is true the
// domain is tagged BuiltinScoped so profile resolution confines it to the
// profiles that reference it.
func loadBuiltinDomains(config *Config, names []string, ruleExclusions map[string]bool, scoped bool) {
	for _, name := range names {
		// Skip if domain already exists (local content has higher priority)
		if _, exists := config.Content.Domains[name]; exists {
			logger.Debug("Skipping builtin (local domain exists)", "name", name)
			continue
		}

		entries, err := builtins.LoadDomainContent(name)
		if err != nil {
			logger.Warn("Failed to load builtin", "name", name, "error", err)
			continue
		}
		if len(entries) == 0 {
			continue
		}

		domain := &Domain{
			Name:          name,
			Builtin:       true,
			BuiltinScoped: scoped,
		}

		for _, entry := range entries {
			// Skip individual builtin content files excluded via "!domain/name".
			if ruleExclusions[name+"/"+entry.Name] {
				logger.Debug("Excluding builtin content", "domain", name, "name", entry.Name)
				continue
			}

			// Parse frontmatter from embedded content
			metadata, body, malformed := parseFrontmatter(entry.Content)
			cf := ContentFile{
				Name:                 entry.Name,
				Path:                 "builtin://" + entry.Path,
				Content:              body,
				Metadata:             metadata,
				MalformedFrontmatter: malformed,
			}

			switch entry.Type {
			case "rules":
				domain.Rules = append(domain.Rules, cf)
			case "context":
				domain.Context = append(domain.Context, cf)
			case "skills":
				domain.Skills = append(domain.Skills, cf)
			case "agents":
				domain.Agents = append(domain.Agents, cf)
			case "commands":
				domain.Commands = append(domain.Commands, cf)
			}
		}

		config.Content.Domains[name] = domain
		logger.Debug("Loaded builtin domain",
			"name", name,
			"scoped", scoped,
			"rules", len(domain.Rules),
			"context", len(domain.Context),
			"skills", len(domain.Skills),
			"agents", len(domain.Agents),
			"commands", len(domain.Commands),
		)
	}
}
