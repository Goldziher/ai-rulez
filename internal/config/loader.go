package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/builtins"
	"github.com/Goldziher/ai-rulez/internal/logger"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const (
	// VersionDir is the config version returned when an .ai-rulez/ directory is detected.
	VersionDir = "dir"

	aiRulezDirName     = ".ai-rulez"
	configTOMLFilename = "config.toml"
	configYAMLFilename = "config.yaml"
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
	domainsDir          = "domains"
	skillMarkerFile     = "SKILL.md"
)

// DetectConfigVersion detects whether a directory contains V2 or directory-based configuration
// Returns "v2" if ai-rulez.yaml/yml exists, "dir" if .ai-rulez/ exists, "" otherwise
func DetectConfigVersion(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", oops.
			With("path", dir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	// Check for directory-based config (.ai-rulez/ directory)
	configDir := filepath.Join(absDir, aiRulezDirName)
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		return VersionDir, nil
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
// The baseDir should contain a .ai-rulez/ subdirectory with config.toml,
// config.yaml, or config.json.
func LoadConfig(ctx context.Context, baseDir string) (*Config, error) {
	return LoadConfigFromDir(ctx, baseDir, aiRulezDirName)
}

// LoadConfigFromDir loads configuration from configDirName below baseDir.
func LoadConfigFromDir(ctx context.Context, baseDir, configDirName string) (*Config, error) {
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
	configDir := filepath.Join(absDir, configDirName)

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

	config, err := loadConfigFile(configDir)
	if err != nil {
		return nil, err
	}

	return finishLoadConfig(ctx, config, absDir, configDir)
}

// LoadConfigFromFile loads a configuration from an exact config file path or
// from a config directory path. For a file path, the project base directory is
// the parent of the config directory.
func LoadConfigFromFile(ctx context.Context, path string) (*Config, error) {
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
			cfg, loadErr := loadConfigFile(absPath)
			if loadErr != nil {
				return nil, loadErr
			}
			return finishLoadConfig(ctx, cfg, filepath.Dir(absPath), absPath)
		}
		return LoadConfig(ctx, absPath)
	}

	cfg, err := loadConfigFilePath(absPath)
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
	return finishLoadConfig(ctx, cfg, filepath.Dir(configDir), configDir)
}

func looksLikeProjectRoot(dir string) bool {
	for _, marker := range []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "Taskfile.yml", "Taskfile.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func hasConfigFile(dir string) bool {
	for _, name := range []string{configTOMLFilename, configYAMLFilename, "config.yml", configJSONFilename} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func finishLoadConfig(ctx context.Context, config *Config, baseDir, configDir string) (*Config, error) {
	config.BaseDir = baseDir
	config.ConfigDir = configDir
	config.ConfigDirName = filepath.Base(configDir)

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
	contentTree, err := ScanContentTree(configDir)
	if err != nil {
		return nil, err
	}
	config.Content = contentTree

	// Load builtins (lowest priority — loaded first so includes and local override them)
	// Only load when the builtins field is explicitly configured in the config file
	if config.Builtins.IsEnabled() && !config.Builtins.IsNone() {
		loadBuiltins(config)
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
			Hint("Includes are configured but the includes resolver is not registered.\nImport github.com/Goldziher/ai-rulez/internal/includes (blank import) before loading configs, or ensure the CLI/mcp entrypoint is used.").
			Errorf("includes configured but includes resolver is unavailable")
	}

	logger.Debug("Resolving includes", "count", len(config.Includes))

	mergedContent, err := resolveIncludesFunc(ctx, config)
	if err != nil {
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
			Hint("Installed skills are configured but the resolver is not registered.\nImport github.com/Goldziher/ai-rulez/internal/includes (blank import) before loading configs.").
			Errorf("installed skills configured but resolver is unavailable")
	}

	logger.Debug("Resolving installed skills", "count", len(config.InstalledSkills))

	skills, err := resolveInstalledSkillsFunc(ctx, config)
	if err != nil {
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

	// Merge: local skills win over installed skills
	for _, s := range skills {
		if existingNames[s.Name] {
			logger.Warn("Installed skill name conflicts with local skill, skipping", "name", s.Name)
			continue
		}
		config.Content.Skills = append(config.Content.Skills, s)
		existingNames[s.Name] = true
	}

	logger.Debug("Successfully resolved installed skills", "count", len(skills))
	return nil
}

// loadConfigFile loads config.toml, config.yaml, or config.json from a config directory.
func loadConfigFile(configDir string) (*Config, error) {
	// Try TOML first (V4 preferred format)
	tomlPath := filepath.Join(configDir, configTOMLFilename)
	if _, err := os.Stat(tomlPath); err == nil {
		cfg, err := loadConfigTOML(tomlPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configTOMLFilename
		return cfg, nil
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
		return cfg, nil
	}

	// Try JSON
	jsonPath := filepath.Join(configDir, configJSONFilename)
	if _, err := os.Stat(jsonPath); err == nil {
		cfg, err := loadConfigJSON(jsonPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configJSONFilename
		return cfg, nil
	}

	return nil, oops.
		With("config_dir", configDir).
		With("toml_path", tomlPath).
		With("yaml_path", yamlPath).
		With("json_path", jsonPath).
		Hint(fmt.Sprintf("Create %s, %s, or %s in %s\nRun 'ai-rulez init' to initialize configuration", configTOMLFilename, configYAMLFilename, configJSONFilename, configDir)).
		Errorf("no config file found (tried %s, %s, and %s)", configTOMLFilename, configYAMLFilename, configJSONFilename)
}

func loadConfigFilePath(path string) (*Config, error) {
	switch filepath.Base(path) {
	case configTOMLFilename:
		cfg, err := loadConfigTOML(path)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = filepath.Base(path)
		return cfg, nil
	case configYAMLFilename, "config.yml":
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}

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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}

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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}

	// TOML presets are plain strings; we unmarshal into an intermediate
	// struct then convert to []Preset. This avoids custom unmarshaler
	// issues with the TOML library.
	type tomlConfig struct {
		Schema          string                 `toml:"schema"`
		Version         string                 `toml:"version"`
		Name            string                 `toml:"name"`
		Description     string                 `toml:"description"`
		Presets         []string               `toml:"presets"`
		Default         string                 `toml:"default"`
		Profiles        map[string][]string    `toml:"profiles"`
		Gitignore       *bool                  `toml:"gitignore"`
		Includes        []IncludeConfig        `toml:"includes"`
		InstalledSkills []InstalledSkillConfig `toml:"installed_skills"`
		MCPServers      []MCPServer            `toml:"mcp_servers"`
		Header          *HeaderConfig          `toml:"header"`
		Builtins        interface{}            `toml:"builtins"`
		Plugins         []PluginConfig         `toml:"plugins"`
		Marketplaces    []MarketplaceConfig    `toml:"marketplaces"`
		Scopes          []ScopeConfig          `toml:"scopes"`
	}

	var raw tomlConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the TOML syntax - ensure proper formatting\nCommon issues: missing quotes around strings, incorrect table syntax").
			Wrapf(err, "parse TOML config")
	}

	presets := make([]Preset, 0, len(raw.Presets))
	for _, name := range raw.Presets {
		presets = append(presets, Preset{BuiltIn: name})
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
		Includes:        raw.Includes,
		InstalledSkills: raw.InstalledSkills,
		MCPServersRaw:   raw.MCPServers,
		Header:          raw.Header,
		Builtins:        builtinsCfg,
		Plugins:         raw.Plugins,
		Marketplaces:    raw.Marketplaces,
		Scopes:          raw.Scopes,
	}

	return cfg, nil
}

// ScanContentTree scans all content directories and returns a populated ContentTree.
// Root content goes into the top-level slices; domain content goes only into the Domains map.
// This keeps the two layers separate so callers (e.g. include sources) can merge without duplication.
func ScanContentTree(configDir string) (*ContentTree, error) {
	tree := &ContentTree{
		Domains: make(map[string]*Domain),
	}

	// Scan root rules/
	rulesPath := filepath.Join(configDir, rulesDir)
	var rules []ContentFile
	var err error
	if rules, err = scanMarkdownFiles(rulesPath); err != nil {
		return nil, oops.
			With("path", rulesPath).
			Wrapf(err, "scan rules directory")
	}
	tree.Rules = rules

	// Scan root context/
	contextPath := filepath.Join(configDir, contextDir)
	var contextFiles []ContentFile
	if contextFiles, err = scanMarkdownFiles(contextPath); err != nil {
		return nil, oops.
			With("path", contextPath).
			Wrapf(err, "scan context directory")
	}
	tree.Context = contextFiles

	// Scan root skills/
	skillsPath := filepath.Join(configDir, skillsDir)
	var skills []ContentFile
	if skills, err = scanSkills(skillsPath); err != nil {
		return nil, oops.
			With("path", skillsPath).
			Wrapf(err, "scan skills directory")
	}
	tree.Skills = skills

	// Scan root agents/
	agentsPath := filepath.Join(configDir, agentsDir)
	var agents []ContentFile
	if agents, err = scanAgents(agentsPath); err != nil {
		return nil, oops.
			With("path", agentsPath).
			Wrapf(err, "scan agents directory")
	}
	tree.Agents = agents
	logger.Debug("Scanned agents directory", "path", agentsPath, "count", len(agents))

	// Scan root commands/
	commandsPath := filepath.Join(configDir, commandsDir)
	var commands []ContentFile
	if commands, err = scanMarkdownFiles(commandsPath); err != nil {
		return nil, oops.
			With("path", commandsPath).
			Wrapf(err, "scan commands directory")
	}
	tree.Commands = commands
	logger.Debug("Scanned commands directory", "path", commandsPath, "count", len(commands))

	// Scan domains/
	domainsPath := filepath.Join(configDir, domainsDir)
	var domains map[string]*Domain
	if domains, err = scanDomains(domainsPath); err != nil {
		return nil, oops.
			With("path", domainsPath).
			Wrapf(err, "scan domains directory")
	}
	tree.Domains = domains

	return tree, nil
}

// scanMarkdownFiles scans a directory for .md files and returns ContentFile entries
func scanMarkdownFiles(dir string) ([]ContentFile, error) {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Directory doesn't exist, return empty slice (not an error)
		return []ContentFile{}, nil
	}

	var files []ContentFile

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, oops.
			With("path", dir).
			Wrapf(err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		contentFile, err := loadContentFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		files = append(files, contentFile)
	}

	return files, nil
}

// scanSkills scans the skills/ directory for SKILL.md files in subdirectories
func scanSkills(skillsDir string) ([]ContentFile, error) {
	// Check if directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		// Directory doesn't exist, return empty slice (not an error)
		return []ContentFile{}, nil
	}

	var skills []ContentFile

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, oops.
			With("path", skillsDir).
			Wrapf(err, "read skills directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillRoot := filepath.Join(skillsDir, entry.Name())
		skillPath := filepath.Join(skillRoot, skillMarkerFile)
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			// No SKILL.md file, skip this directory
			continue
		}

		contentFile, err := loadContentFile(skillPath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		// Override the name with the directory name instead of filename
		contentFile.Name = entry.Name()

		// Load skill supporting files (references/, scripts/, assets/) so
		// presets can preserve the canonical Agent Skills layout instead of
		// concatenating everything into SKILL.md.
		resources, resErr := LoadSkillResources(skillRoot)
		if resErr != nil {
			logger.Warn("Failed to load skill resources", "skill", entry.Name(), "error", resErr)
		}
		contentFile.Resources = resources

		skills = append(skills, contentFile)
	}

	return skills, nil
}

// scanAgents scans the agents/ directory for .md files
func scanAgents(agentsPath string) ([]ContentFile, error) {
	// Check if directory exists
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		// Directory doesn't exist, return empty slice (not an error)
		return []ContentFile{}, nil
	}

	var agents []ContentFile

	entries, err := os.ReadDir(agentsPath)
	if err != nil {
		return nil, oops.
			With("path", agentsPath).
			Wrapf(err, "read agents directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(agentsPath, entry.Name())
		contentFile, err := loadContentFile(filePath)
		if err != nil {
			// Log warning but continue (non-fatal)
			continue
		}

		agents = append(agents, contentFile)
	}

	return agents, nil
}

// scanDomains scans the domains/ directory and returns a map of domain name to Domain
func scanDomains(domainsDir string) (map[string]*Domain, error) {
	// Check if directory exists
	if _, err := os.Stat(domainsDir); os.IsNotExist(err) {
		// Directory doesn't exist, return empty map (not an error)
		return make(map[string]*Domain), nil
	}

	domains := make(map[string]*Domain)

	entries, err := os.ReadDir(domainsDir)
	if err != nil {
		return nil, oops.
			With("path", domainsDir).
			Wrapf(err, "read domains directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		domainName := entry.Name()
		domainPath := filepath.Join(domainsDir, domainName)

		domain := &Domain{
			Name: domainName,
		}

		// Scan domain/rules/
		rulesPath := filepath.Join(domainPath, rulesDir)
		var rules []ContentFile
		var err error
		if rules, err = scanMarkdownFiles(rulesPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", rulesPath).
				Wrapf(err, "scan domain rules")
		}
		domain.Rules = rules

		// Scan domain/context/
		contextPath := filepath.Join(domainPath, contextDir)
		var contextFiles []ContentFile
		if contextFiles, err = scanMarkdownFiles(contextPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", contextPath).
				Wrapf(err, "scan domain context")
		}
		domain.Context = contextFiles

		// Scan domain/skills/
		skillsPath := filepath.Join(domainPath, skillsDir)
		var skills []ContentFile
		if skills, err = scanSkills(skillsPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", skillsPath).
				Wrapf(err, "scan domain skills")
		}
		domain.Skills = skills

		// Scan domain/agents/
		agentsPath := filepath.Join(domainPath, agentsDir)
		var agentsContent []ContentFile
		if agentsContent, err = scanAgents(agentsPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", agentsPath).
				Wrapf(err, "scan domain agents")
		}
		domain.Agents = agentsContent

		// Scan domain/commands/
		domainCommandsPath := filepath.Join(domainPath, commandsDir)
		var domainCommands []ContentFile
		if domainCommands, err = scanMarkdownFiles(domainCommandsPath); err != nil {
			return nil, oops.
				With("domain", domainName).
				With("path", domainCommandsPath).
				Wrapf(err, "scan domain commands")
		}
		domain.Commands = domainCommands
		logger.Debug("Scanned domain commands directory", "domain", domainName, "path", domainCommandsPath, "count", len(domainCommands))

		domains[domainName] = domain
	}

	return domains, nil
}

// ParseFrontmatterPublic is the exported version of parseFrontmatter for use by other packages
func ParseFrontmatterPublic(content string) (metadata *Metadata, body string) {
	return parseFrontmatter(content)
}

// parseFrontmatter parses optional YAML frontmatter from content
// Returns metadata (nil if none) and the actual content (without frontmatter)
// Inlined here to avoid import cycle with parser package
func parseFrontmatter(content string) (metadata *Metadata, body string) {
	// Check if content starts with ---
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, content
	}

	// Find the closing ---
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return nil, content
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
		return nil, content
	}

	// Extract frontmatter YAML
	frontmatterLines := lines[1:endIdx]
	frontmatterYAML := strings.Join(frontmatterLines, "\n")

	// First try direct unmarshal into Metadata (works for simple key-value frontmatter)
	var parsedMetadata Metadata
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &parsedMetadata); err != nil {
		// Direct unmarshal failed (e.g., nested YAML objects in Extra fields).
		// Fall back to raw map parsing: extract known fields and stringify the rest.
		result, ok := parseFrontmatterFromRawMap(frontmatterYAML)
		if !ok {
			return nil, content
		}
		parsedMetadata = result
	}

	// Extract actual content (after frontmatter)
	body = strings.Join(lines[endIdx+1:], "\n")
	body = strings.TrimPrefix(body, "\n")

	metadata = &parsedMetadata
	return metadata, body
}

// parseFrontmatterFromRawMap parses frontmatter YAML into Metadata via a raw map.
// Used as fallback when direct unmarshal fails (e.g., nested YAML objects).
func parseFrontmatterFromRawMap(frontmatterYAML string) (Metadata, bool) {
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &rawMap); err != nil {
		return Metadata{}, false
	}

	m := Metadata{Extra: make(map[string]string)}
	for k, v := range rawMap {
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
		case "usage":
			m.Usage = fmt.Sprintf("%v", v)
		case "shortcut":
			m.Shortcut = fmt.Sprintf("%v", v)
		case "category":
			m.Category = fmt.Sprintf("%v", v)
		case "effort":
			m.Effort = fmt.Sprintf("%v", v)
		default:
			m.Extra[k] = fmt.Sprintf("%v", v)
		}
	}

	return m, true
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
func loadContentFile(path string) (ContentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContentFile{}, oops.
			With("path", path).
			Wrapf(err, "read content file")
	}

	content := string(data)
	filename := filepath.Base(path)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Parse frontmatter (if present) - inlined to avoid import cycle
	metadata, actualContent := parseFrontmatter(content)

	return ContentFile{
		Name:     name,
		Path:     path,
		Content:  actualContent,
		Metadata: metadata,
	}, nil
}

// serversToMap converts a slice of MCPServer to a map keyed by server name
func serversToMap(servers []MCPServer) map[string]*MCPServer {
	result := make(map[string]*MCPServer)
	for i := range servers {
		result[servers[i].Name] = &servers[i]
	}
	return result
}

// loadLegacyMCPFile loads MCP servers from a deprecated separate mcp.toml/mcp.yaml/mcp.json file.
// Returns nil if no legacy file exists.
func loadLegacyMCPFile(configDir string) map[string]*MCPServer {
	for _, filename := range []string{"mcp.toml", "mcp.yaml", "mcp.json"} {
		path := filepath.Join(configDir, filename)
		if _, err := os.Stat(path); err != nil {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("Failed to read legacy MCP file", "path", path, "error", err)
			return nil
		}

		// Parse the legacy MCP config structure
		type legacyMCPConfig struct {
			Servers []MCPServer `yaml:"mcp_servers" json:"mcp_servers" toml:"mcp_servers"`
		}

		var cfg legacyMCPConfig
		ext := filepath.Ext(filename)
		switch ext {
		case ".toml":
			if err := toml.Unmarshal(data, &cfg); err != nil {
				logger.Warn("Failed to parse legacy MCP TOML", "path", path, "error", err)
				return nil
			}
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				logger.Warn("Failed to parse legacy MCP YAML", "path", path, "error", err)
				return nil
			}
		case ".json":
			if err := json.Unmarshal(data, &cfg); err != nil {
				logger.Warn("Failed to parse legacy MCP JSON", "path", path, "error", err)
				return nil
			}
		}

		return serversToMap(cfg.Servers)
	}
	return nil
}

// SaveConfig saves a configuration back to YAML or JSON format.
// For YAML files, it uses yaml.Node round-tripping to preserve comments,
// field ordering, and formatting from the original file.
func SaveConfig(cfg *Config, configDir string) error {
	if cfg == nil {
		return oops.
			With("config_dir", configDir).
			Hint("Provide a valid Config struct").
			Errorf("config is nil")
	}

	// Check which format exists (prefer YAML if both exist)
	yamlPath := filepath.Join(configDir, configYAMLFilename)
	jsonPath := filepath.Join(configDir, configJSONFilename)

	yamlExists := fileExists(yamlPath)
	jsonExists := fileExists(jsonPath)

	// Determine target format
	var targetPath string
	var isYAML bool

	switch {
	case yamlExists:
		targetPath = yamlPath
		isYAML = true
	case jsonExists:
		targetPath = jsonPath
		isYAML = false
	default:
		// Default to YAML if neither exists
		targetPath = yamlPath
		isYAML = true
	}

	var data []byte
	var err error

	if isYAML {
		data, err = marshalYAMLPreserving(cfg, targetPath)
		if err != nil {
			return oops.With("path", targetPath).Wrapf(err, "marshal config to YAML")
		}
	} else {
		data, err = json.Marshal(cfg)
		if err != nil {
			return oops.With("path", targetPath).Wrapf(err, "marshal config to JSON")
		}
	}

	// Write to temporary file first (atomic write)
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return oops.With("path", tmpPath).Wrapf(err, "write temporary config file")
	}

	// Rename temporary file to actual config file (atomic)
	if err := os.Rename(tmpPath, targetPath); err != nil {
		//nolint:gosec,errcheck // temp file cleanup is best-effort
		_ = os.Remove(tmpPath)
		return oops.With("src", tmpPath).With("dst", targetPath).Wrapf(err, "rename config file")
	}

	return nil
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

// loadBuiltins resolves and loads builtin domains into the config content tree.
// Builtins have the lowest priority: they are injected into domains that don't already exist.
// If a domain already exists (from local content or includes), the builtin is skipped.
func loadBuiltins(config *Config) {
	var resolved []string
	if config.Builtins.IsAll() {
		resolved = builtins.ResolveAll()
	} else {
		resolved = builtins.ResolveBuiltins(config.Builtins.GetNames())
	}
	if len(resolved) == 0 {
		return
	}

	logger.Debug("Loading builtins", "count", len(resolved), "names", resolved)

	if config.Content == nil {
		config.Content = &ContentTree{
			Domains: make(map[string]*Domain),
		}
	}

	for _, name := range resolved {
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
			Name:    name,
			Builtin: true,
		}

		for _, entry := range entries {
			// Parse frontmatter from embedded content
			metadata, body := parseFrontmatter(entry.Content)
			cf := ContentFile{
				Name:     entry.Name,
				Path:     "builtin://" + entry.Path,
				Content:  body,
				Metadata: metadata,
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
			"rules", len(domain.Rules),
			"context", len(domain.Context),
			"skills", len(domain.Skills),
			"agents", len(domain.Agents),
			"commands", len(domain.Commands),
		)
	}
}
