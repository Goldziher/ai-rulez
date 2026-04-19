package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/builtins"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const (
	aiRulezDirName     = ".ai-rulez"
	configYAMLFilename = "config.yaml"
	configJSONFilename = "config.json"
	rulesDir           = "rules"
	contextDir         = "context"
	skillsDir          = "skills"
	agentsDir          = "agents"
	commandsDir        = "commands"
	domainsDir         = "domains"
	skillMarkerFile    = "SKILL.md"
	mcpYAMLFilename    = "mcp.yaml"
	mcpJSONFilename    = "mcp.json"
)

// DetectConfigVersion detects whether a directory contains V2 or V3 configuration
// Returns "v2" if ai-rulez.yaml/yml exists, "v3" if .ai-rulez/ exists, "" otherwise
func DetectConfigVersion(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", oops.
			With("path", dir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	// Check for V3 (.ai-rulez/ directory)
	v3Dir := filepath.Join(absDir, aiRulezDirName)
	if info, err := os.Stat(v3Dir); err == nil && info.IsDir() {
		return "v3", nil
	}

	// Check for V2 (ai-rulez.yaml or ai-rulez.yml)
	v2Files := []string{"ai-rulez.yaml", "ai-rulez.yml"}
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
type ResolveIncludesCallback func(ctx context.Context, cfg *ConfigV3) (*ContentTreeV3, error)

// resolveIncludesFunc is set by the includes package during init
var resolveIncludesFunc ResolveIncludesCallback

// SetResolveIncludesCallback sets the callback for resolving includes
// This is called by the includes package to avoid circular imports
func SetResolveIncludesCallback(fn ResolveIncludesCallback) {
	resolveIncludesFunc = fn
}

// ResolveInstalledSkillsCallback is a callback function type that resolves installed skills
type ResolveInstalledSkillsCallback func(ctx context.Context, cfg *ConfigV3) ([]ContentFile, error)

// resolveInstalledSkillsFunc is set by the includes package during init
var resolveInstalledSkillsFunc ResolveInstalledSkillsCallback

// SetResolveInstalledSkillsCallback sets the callback for resolving installed skills
func SetResolveInstalledSkillsCallback(fn ResolveInstalledSkillsCallback) {
	resolveInstalledSkillsFunc = fn
}

// LoadConfigV3 loads a V3 configuration from the specified base directory
// The baseDir should contain a .ai-rulez/ subdirectory with config.yaml or config.json
func LoadConfigV3(ctx context.Context, baseDir string) (*ConfigV3, error) {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, oops.
			With("path", baseDir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	configDir := filepath.Join(absDir, aiRulezDirName)

	// Check if .ai-rulez/ directory exists
	if info, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			return nil, oops.
				With("path", configDir).
				With("base_dir", baseDir).
				Hint(fmt.Sprintf("Create .ai-rulez/ directory in %s\nRun 'ai-rulez init' to initialize V3 configuration", baseDir)).
				Errorf(".ai-rulez directory not found")
		}
		return nil, oops.
			With("path", configDir).
			Wrapf(err, "stat .ai-rulez directory")
	} else if !info.IsDir() {
		return nil, oops.
			With("path", configDir).
			Hint("Remove the .ai-rulez file and create a directory instead").
			Errorf(".ai-rulez exists but is not a directory")
	}

	// Load config file (try YAML first, then JSON)
	config, err := loadConfigFile(configDir)
	if err != nil {
		return nil, err
	}

	// Set runtime fields
	config.BaseDir = absDir

	warnDeprecatedCompression(config)

	// Load root MCP servers (optional - don't fail if missing)
	rootMCPServers, err := loadMCPServers(configDir)
	if err != nil {
		logger.Warn("Failed to load root MCP servers", "error", err)
		rootMCPServers = make(map[string]*MCPServerV3)
	}
	config.MCPServers = rootMCPServers

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

	// Load domain-specific MCP servers
	for domainName, domain := range config.Content.Domains {
		domainMCPServers, err := loadDomainMCPServers(
			filepath.Join(configDir, domainsDir, domainName),
		)
		if err != nil {
			logger.Warn("Failed to load MCP servers", "domain", domainName, "error", err)
			domainMCPServers = make(map[string]*MCPServerV3)
		}
		domain.MCPServers = domainMCPServers
	}

	return config, nil
}

func resolveIncludesIfNeeded(ctx context.Context, configDir string, config *ConfigV3) error {
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

func resolveInstalledSkillsIfNeeded(ctx context.Context, config *ConfigV3) error {
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
		config.Content = &ContentTreeV3{
			Domains: make(map[string]*DomainV3),
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

func warnDeprecatedCompression(config *ConfigV3) {
	if config.Compression != nil && config.Compression.IsDeprecatedUsage() {
		logger.Warn("\"compression\" config is deprecated and will be removed in a future version. It is now a no-op. Condense content at the source level instead.")
	}
}

// loadConfigFile loads config.yaml or config.json from the .ai-rulez/ directory
func loadConfigFile(configDir string) (*ConfigV3, error) {
	// Try YAML first
	yamlPath := filepath.Join(configDir, configYAMLFilename)
	if _, err := os.Stat(yamlPath); err == nil {
		return loadConfigYAML(yamlPath)
	}

	// Try JSON
	jsonPath := filepath.Join(configDir, configJSONFilename)
	if _, err := os.Stat(jsonPath); err == nil {
		return loadConfigJSON(jsonPath)
	}

	return nil, oops.
		With("config_dir", configDir).
		With("yaml_path", yamlPath).
		With("json_path", jsonPath).
		Hint(fmt.Sprintf("Create %s or %s in %s\nRun 'ai-rulez init' to initialize configuration", configYAMLFilename, configJSONFilename, configDir)).
		Errorf("no config file found (tried %s and %s)", configYAMLFilename, configJSONFilename)
}

// loadConfigYAML loads a V3 config from YAML
func loadConfigYAML(path string) (*ConfigV3, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}

	var config ConfigV3
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the YAML syntax - ensure proper indentation\nValidate your YAML at: https://www.yamllint.com/\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse YAML config")
	}

	return &config, nil
}

// loadConfigJSON loads a V3 config from JSON
func loadConfigJSON(path string) (*ConfigV3, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}

	var config ConfigV3
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the JSON syntax - ensure proper formatting\nValidate your JSON at: https://jsonlint.com/\nCommon issues: trailing commas, unquoted keys, missing brackets").
			Wrapf(err, "parse JSON config")
	}

	return &config, nil
}

// ScanContentTree scans all content directories and returns a populated ContentTreeV3.
// Root content goes into the top-level slices; domain content goes only into the Domains map.
// This keeps the two layers separate so callers (e.g. include sources) can merge without duplication.
func ScanContentTree(configDir string) (*ContentTreeV3, error) {
	tree := &ContentTreeV3{
		Domains: make(map[string]*DomainV3),
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
	logger.Info("Scanned commands directory", "path", commandsPath, "count", len(commands))

	// Scan domains/
	domainsPath := filepath.Join(configDir, domainsDir)
	var domains map[string]*DomainV3
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

		skillPath := filepath.Join(skillsDir, entry.Name(), skillMarkerFile)
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

// scanDomains scans the domains/ directory and returns a map of domain name to DomainV3
func scanDomains(domainsDir string) (map[string]*DomainV3, error) {
	// Check if directory exists
	if _, err := os.Stat(domainsDir); os.IsNotExist(err) {
		// Directory doesn't exist, return empty map (not an error)
		return make(map[string]*DomainV3), nil
	}

	domains := make(map[string]*DomainV3)

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

		domain := &DomainV3{
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
		logger.Info("Scanned domain commands directory", "domain", domainName, "path", domainCommandsPath, "count", len(domainCommands))

		domains[domainName] = domain
	}

	return domains, nil
}

// ParseFrontmatterPublic is the exported version of parseFrontmatter for use by other packages
func ParseFrontmatterPublic(content string) (metadata *MetadataV3, body string) {
	return parseFrontmatter(content)
}

// parseFrontmatter parses optional YAML frontmatter from content
// Returns metadata (nil if none) and the actual content (without frontmatter)
// Inlined here to avoid import cycle with parser package
func parseFrontmatter(content string) (metadata *MetadataV3, body string) {
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

	// First try direct unmarshal into MetadataV3 (works for simple key-value frontmatter)
	var parsedMetadata MetadataV3
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

// parseFrontmatterFromRawMap parses frontmatter YAML into MetadataV3 via a raw map.
// Used as fallback when direct unmarshal fails (e.g., nested YAML objects).
func parseFrontmatterFromRawMap(frontmatterYAML string) (MetadataV3, bool) {
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &rawMap); err != nil {
		return MetadataV3{}, false
	}

	m := MetadataV3{Extra: make(map[string]string)}
	for k, v := range rawMap {
		switch k {
		case "priority":
			m.Priority = fmt.Sprintf("%v", v)
		case "targets":
			if targets, ok := v.([]interface{}); ok {
				for _, t := range targets {
					m.Targets = append(m.Targets, fmt.Sprintf("%v", t))
				}
			}
		case "aliases":
			if aliases, ok := v.([]interface{}); ok {
				for _, a := range aliases {
					m.Aliases = append(m.Aliases, fmt.Sprintf("%v", a))
				}
			}
		case "usage":
			m.Usage = fmt.Sprintf("%v", v)
		case "shortcut":
			m.Shortcut = fmt.Sprintf("%v", v)
		case "category":
			m.Category = fmt.Sprintf("%v", v)
		default:
			m.Extra[k] = fmt.Sprintf("%v", v)
		}
	}

	return m, true
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

// loadMCPConfig loads MCP configuration from a YAML or JSON file
func loadMCPConfig(path string) (*MCPConfigV3, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Wrapf(err, "read MCP config file")
	}

	var config MCPConfigV3

	// Determine if it's JSON or YAML based on file extension
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, oops.
				With("path", path).
				Hint("Check the JSON syntax - ensure proper formatting").
				Wrapf(err, "parse JSON MCP config")
		}
	} else {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, oops.
				With("path", path).
				Hint("Check the YAML syntax - ensure proper indentation").
				Wrapf(err, "parse YAML MCP config")
		}
	}

	return &config, nil
}

// loadMCPServers loads root MCP servers from .ai-rulez/mcp.yaml or mcp.json
func loadMCPServers(configDir string) (map[string]*MCPServerV3, error) {
	yamlPath := filepath.Join(configDir, mcpYAMLFilename)
	jsonPath := filepath.Join(configDir, mcpJSONFilename)

	// Try YAML first
	if _, err := os.Stat(yamlPath); err == nil {
		cfg, err := loadMCPConfig(yamlPath)
		if err != nil {
			return nil, err
		}
		return serversToMap(cfg.Servers), nil
	}

	// Try JSON
	if _, err := os.Stat(jsonPath); err == nil {
		cfg, err := loadMCPConfig(jsonPath)
		if err != nil {
			return nil, err
		}
		return serversToMap(cfg.Servers), nil
	}

	// No MCP file found - return empty map (not an error)
	return make(map[string]*MCPServerV3), nil
}

// loadDomainMCPServers loads domain-specific MCP servers from domain mcp.yaml or mcp.json
func loadDomainMCPServers(domainDir string) (map[string]*MCPServerV3, error) {
	yamlPath := filepath.Join(domainDir, mcpYAMLFilename)
	jsonPath := filepath.Join(domainDir, mcpJSONFilename)

	// Try YAML first
	if _, err := os.Stat(yamlPath); err == nil {
		cfg, err := loadMCPConfig(yamlPath)
		if err != nil {
			return nil, err
		}
		return serversToMap(cfg.Servers), nil
	}

	// Try JSON
	if _, err := os.Stat(jsonPath); err == nil {
		cfg, err := loadMCPConfig(jsonPath)
		if err != nil {
			return nil, err
		}
		return serversToMap(cfg.Servers), nil
	}

	// No MCP file found - return empty map (not an error)
	return make(map[string]*MCPServerV3), nil
}

// serversToMap converts a slice of MCPServerV3 to a map keyed by server name
func serversToMap(servers []MCPServerV3) map[string]*MCPServerV3 {
	result := make(map[string]*MCPServerV3)
	for i := range servers {
		result[servers[i].Name] = &servers[i]
	}
	return result
}

// mergeMCPServersV3 merges domain MCP servers into root servers
// Domain servers with the same name override root servers
func mergeMCPServersV3(root, domain map[string]*MCPServerV3) map[string]*MCPServerV3 {
	result := make(map[string]*MCPServerV3)

	// Copy root servers
	for name, server := range root {
		result[name] = server
	}

	// Override/add domain servers
	for name, server := range domain {
		result[name] = server
	}

	return result
}

// LoadV3AsV2 loads a V3 config and converts it to V2 Config struct
// This enables backward compatibility with existing V2 generator code
func LoadV3AsV2(ctx context.Context, baseDir string) (*Config, error) {
	// 1. Load V3 config
	v3Config, err := LoadConfigV3(ctx, baseDir)
	if err != nil {
		return nil, err
	}

	// 2. Convert to V2 Config struct
	v2Config := convertV3ToV2(v3Config)

	return v2Config, nil
}

// convertV3ToV2 converts a V3 ConfigV3 to a V2 Config struct
func convertV3ToV2(v3 *ConfigV3) *Config {
	v2 := &Config{
		Metadata: Metadata{
			Name:        v3.Name,
			Description: v3.Description,
		},
		Gitignore: v3.Gitignore,
	}

	// Convert root content
	if v3.Content != nil {
		v2.Rules = convertContentToRules(v3.Content.Rules)
		v2.Sections = convertContentToSections(v3.Content.Context)
		v2.Agents = convertContentToAgents(v3.Content.Agents)
		v2.Commands = convertContentToCommands(v3.Content.Commands)

		// Collect all names for collision detection
		seenRuleNames := make(map[string]bool)
		seenSectionNames := make(map[string]bool)
		seenAgentNames := make(map[string]bool)
		seenCommandNames := make(map[string]bool)

		// Track root names
		for i := range v2.Rules {
			seenRuleNames[v2.Rules[i].Name] = true
		}
		for i := range v2.Sections {
			seenSectionNames[v2.Sections[i].Name] = true
		}
		for i := range v2.Agents {
			seenAgentNames[v2.Agents[i].Name] = true
		}
		for i := range v2.Commands {
			seenCommandNames[v2.Commands[i].Name] = true
		}

		// Add domain content to rules/sections/agents/commands with domain prefixing
		for domainName, domain := range v3.Content.Domains {
			domainRules := convertContentToRulesWithDomain(domain.Rules, domainName, seenRuleNames)
			domainSections := convertContentToSectionsWithDomain(domain.Context, domainName, seenSectionNames)
			domainAgents := convertContentToAgentsWithDomain(domain.Skills, domainName, seenAgentNames)
			domainCommands := convertContentToCommandsWithDomain(domain.Commands, domainName, seenCommandNames)

			v2.Rules = append(v2.Rules, domainRules...)
			v2.Sections = append(v2.Sections, domainSections...)
			v2.Agents = append(v2.Agents, domainAgents...)
			v2.Commands = append(v2.Commands, domainCommands...)

			// Update seen names
			for i := range domainRules {
				seenRuleNames[domainRules[i].Name] = true
			}
			for i := range domainSections {
				seenSectionNames[domainSections[i].Name] = true
			}
			for i := range domainAgents {
				seenAgentNames[domainAgents[i].Name] = true
			}
			for i := range domainCommands {
				seenCommandNames[domainCommands[i].Name] = true
			}
		}
	}

	// Convert presets to outputs
	v2.Outputs = convertPresetsToOutputs(v3.Presets)

	// Store profile information in metadata version field if profiles exist
	if len(v3.Profiles) > 0 || v3.Default != "" {
		v2.Metadata.Version = encodeProfileMetadata(v3.Profiles, v3.Default)
	}

	return v2
}

// convertContentToRules converts ContentFile entries to Rule entries
func convertContentToRules(files []ContentFile) []Rule {
	var rules []Rule
	for _, file := range files {
		rule := Rule{
			ID:       sanitizeNameToID(file.Name),
			Name:     file.Name,
			Content:  file.Content,
			Priority: getPriorityFromMetadata(file.Metadata),
			Targets:  getTargetsFromMetadata(file.Metadata),
		}
		rules = append(rules, rule)
	}
	return rules
}

// convertContentToRulesWithDomain converts ContentFile entries to Rule entries with domain context
// It prefixes names with domain to preserve domain membership and prevent collisions
func convertContentToRulesWithDomain(files []ContentFile, domainName string, seenNames map[string]bool) []Rule {
	var rules []Rule
	for _, file := range files {
		// Prefix name with domain to preserve domain membership
		prefixedName := domainName + ": " + file.Name
		uniqueName := prefixedName

		// Handle collisions with numeric suffix
		counter := 1
		for seenNames[uniqueName] {
			uniqueName = fmt.Sprintf("%s (%d)", prefixedName, counter)
			counter++
		}

		rule := Rule{
			ID:       sanitizeNameToID(uniqueName),
			Name:     uniqueName,
			Content:  file.Content,
			Priority: getPriorityFromMetadata(file.Metadata),
			Targets:  getTargetsFromMetadata(file.Metadata),
		}
		rules = append(rules, rule)
		seenNames[uniqueName] = true
	}
	return rules
}

// convertContentToSections converts ContentFile entries to Section entries
func convertContentToSections(files []ContentFile) []Section {
	var sections []Section
	for _, file := range files {
		section := Section{
			ID:       sanitizeNameToID(file.Name),
			Name:     file.Name,
			Content:  file.Content,
			Priority: getPriorityFromMetadata(file.Metadata),
			Targets:  getTargetsFromMetadata(file.Metadata),
		}
		sections = append(sections, section)
	}
	return sections
}

// convertContentToSectionsWithDomain converts ContentFile entries to Section entries with domain context
// It prefixes names with domain to preserve domain membership and prevent collisions
func convertContentToSectionsWithDomain(files []ContentFile, domainName string, seenNames map[string]bool) []Section {
	var sections []Section
	for _, file := range files {
		// Prefix name with domain to preserve domain membership
		prefixedName := domainName + ": " + file.Name
		uniqueName := prefixedName

		// Handle collisions with numeric suffix
		counter := 1
		for seenNames[uniqueName] {
			uniqueName = fmt.Sprintf("%s (%d)", prefixedName, counter)
			counter++
		}

		section := Section{
			ID:       sanitizeNameToID(uniqueName),
			Name:     uniqueName,
			Content:  file.Content,
			Priority: getPriorityFromMetadata(file.Metadata),
			Targets:  getTargetsFromMetadata(file.Metadata),
		}
		sections = append(sections, section)
		seenNames[uniqueName] = true
	}
	return sections
}

// convertContentToAgents converts ContentFile entries to Agent entries
func convertContentToAgents(files []ContentFile) []Agent {
	var agents []Agent
	for _, file := range files {
		agent := Agent{
			ID:           sanitizeNameToID(file.Name),
			Name:         file.Name,
			Description:  getAgentDescription(file.Metadata),
			SystemPrompt: file.Content,
			Priority:     getPriorityFromMetadata(file.Metadata),
		}
		agents = append(agents, agent)
	}
	return agents
}

// convertContentToAgentsWithDomain converts ContentFile entries to Agent entries with domain context
// It prefixes names with domain to preserve domain membership and prevent collisions
func convertContentToAgentsWithDomain(files []ContentFile, domainName string, seenNames map[string]bool) []Agent {
	var agents []Agent
	for _, file := range files {
		// Prefix name with domain to preserve domain membership
		prefixedName := domainName + ": " + file.Name
		uniqueName := prefixedName

		// Handle collisions with numeric suffix
		counter := 1
		for seenNames[uniqueName] {
			uniqueName = fmt.Sprintf("%s (%d)", prefixedName, counter)
			counter++
		}

		agent := Agent{
			ID:           sanitizeNameToID(uniqueName),
			Name:         uniqueName,
			Description:  getAgentDescription(file.Metadata),
			SystemPrompt: file.Content,
			Priority:     getPriorityFromMetadata(file.Metadata),
		}
		agents = append(agents, agent)
		seenNames[uniqueName] = true
	}
	return agents
}

// convertContentToCommands converts ContentFile entries to Command entries
func convertContentToCommands(files []ContentFile) []Command {
	var commands []Command
	for _, file := range files {
		command := Command{
			ID:           sanitizeNameToID(file.Name),
			Name:         file.Name,
			Description:  getCommandDescription(file.Metadata),
			SystemPrompt: file.Content,
			Aliases:      getCommandAliases(file.Metadata),
			Usage:        getCommandUsage(file.Metadata),
			Shortcut:     getCommandShortcut(file.Metadata),
			Targets:      getTargetsFromMetadata(file.Metadata),
		}
		commands = append(commands, command)
	}
	return commands
}

// convertContentToCommandsWithDomain converts ContentFile entries to Command entries with domain context
// It prefixes names with domain to preserve domain membership and prevent collisions
func convertContentToCommandsWithDomain(files []ContentFile, domainName string, seenNames map[string]bool) []Command {
	var commands []Command
	for _, file := range files {
		// Prefix name with domain to preserve domain membership
		prefixedName := domainName + ": " + file.Name
		uniqueName := prefixedName

		// Handle collisions with numeric suffix
		counter := 1
		for seenNames[uniqueName] {
			uniqueName = fmt.Sprintf("%s (%d)", prefixedName, counter)
			counter++
		}

		command := Command{
			ID:           sanitizeNameToID(uniqueName),
			Name:         uniqueName,
			Description:  getCommandDescription(file.Metadata),
			SystemPrompt: file.Content,
			Aliases:      getCommandAliases(file.Metadata),
			Usage:        getCommandUsage(file.Metadata),
			Shortcut:     getCommandShortcut(file.Metadata),
			Targets:      getTargetsFromMetadata(file.Metadata),
		}
		commands = append(commands, command)
		seenNames[uniqueName] = true
	}
	return commands
}

// convertPresetsToOutputs converts V3 presets to V2 Output entries
func convertPresetsToOutputs(presets []PresetV3) []Output {
	var outputs []Output

	for _, preset := range presets {
		if preset.BuiltIn != "" {
			// Use preset registry to get outputs
			presetOutputs, exists := PresetRegistry[preset.BuiltIn]
			if exists {
				outputs = append(outputs, presetOutputs...)
			}
		} else {
			// Custom preset
			output := Output{
				Path:     preset.Path,
				Type:     string(preset.Type),
				Template: Template{Type: TemplateBuiltin, Value: preset.Template},
			}
			outputs = append(outputs, output)
		}
	}

	return outputs
}

// getPriorityFromMetadata extracts priority from metadata, defaulting to medium
func getPriorityFromMetadata(meta *MetadataV3) Priority {
	if meta == nil {
		return PriorityMedium
	}
	return meta.GetPriority()
}

// getTargetsFromMetadata extracts targets from metadata
func getTargetsFromMetadata(meta *MetadataV3) []string {
	if meta == nil || !meta.HasTargets() {
		return nil
	}
	return meta.Targets
}

// getAgentDescription extracts agent description from metadata
func getAgentDescription(meta *MetadataV3) string {
	if meta == nil || meta.Extra == nil {
		return ""
	}
	return meta.Extra["description"]
}

// getCommandDescription extracts command description from metadata
func getCommandDescription(meta *MetadataV3) string {
	if meta == nil || meta.Extra == nil {
		return ""
	}
	return meta.Extra["description"]
}

// getCommandAliases extracts command aliases from metadata
func getCommandAliases(meta *MetadataV3) []string {
	if meta == nil {
		return nil
	}
	return meta.Aliases
}

// getCommandUsage extracts command usage from metadata
func getCommandUsage(meta *MetadataV3) string {
	if meta == nil {
		return ""
	}
	return meta.Usage
}

// getCommandShortcut extracts command shortcut from metadata
func getCommandShortcut(meta *MetadataV3) string {
	if meta == nil {
		return ""
	}
	return meta.Shortcut
}

// sanitizeNameToID converts a human-readable name to a valid ID
// It replaces spaces and special characters with hyphens, and converts to lowercase
func sanitizeNameToID(name string) string {
	// Convert to lowercase
	id := strings.ToLower(name)

	// Replace spaces with hyphens
	id = strings.ReplaceAll(id, " ", "-")

	// Replace colons with hyphens (for domain prefixed names)
	id = strings.ReplaceAll(id, ":", "-")

	// Remove or replace other special characters
	re := regexp.MustCompile(`[^a-z0-9\-_]`)
	id = re.ReplaceAllString(id, "-")

	// Remove leading/trailing hyphens
	id = strings.Trim(id, "-")

	// Replace multiple consecutive hyphens with single hyphen
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}

	// Ensure ID is not empty and doesn't start with digit or hyphen
	if id == "" || id[0] == '-' || (id[0] >= '0' && id[0] <= '9') {
		id = "id-" + id
	}

	return id
}

// SaveConfigV3 saves a V3 configuration back to YAML or JSON format.
// For YAML files, it uses yaml.Node round-tripping to preserve comments,
// field ordering, and formatting from the original file.
func SaveConfigV3(cfg *ConfigV3, configDir string) error {
	if cfg == nil {
		return oops.
			With("config_dir", configDir).
			Hint("Provide a valid ConfigV3 struct").
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

// marshalYAMLPreserving marshals a ConfigV3 to YAML while preserving
// the original file's field ordering, comments, and formatting.
// If the original file doesn't exist, falls back to plain marshal.
func marshalYAMLPreserving(cfg *ConfigV3, existingPath string) ([]byte, error) {
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

// fileExists checks if a file exists (utility function for SaveConfigV3)
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// encodeProfileMetadata encodes profile information into a version string
// Format: "profiles:<profile1>:<domain1>,<domain2>;profile2:<domain3>"
func encodeProfileMetadata(profiles map[string][]string, defaultProfile string) string {
	if len(profiles) == 0 && defaultProfile == "" {
		return ""
	}

	var parts []string
	for profileName, domains := range profiles {
		profileEntry := profileName
		if len(domains) > 0 {
			profileEntry += ":" + strings.Join(domains, ",")
		}
		parts = append(parts, profileEntry)
	}

	encoded := "profiles:" + strings.Join(parts, ";")

	if defaultProfile != "" {
		encoded += ";default:" + defaultProfile
	}

	return encoded
}

// loadBuiltins resolves and loads builtin domains into the config content tree.
// Builtins have the lowest priority: they are injected into domains that don't already exist.
// If a domain already exists (from local content or includes), the builtin is skipped.
func loadBuiltins(config *ConfigV3) {
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
		config.Content = &ContentTreeV3{
			Domains: make(map[string]*DomainV3),
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

		domain := &DomainV3{
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

// decodeProfileMetadata is unused and kept for potential future use
// nolint:unused // Keeping for potential future use
// func decodeProfileMetadata(versionStr string) (map[string][]string, string) {
// 	if !strings.HasPrefix(versionStr, "profiles:") {
// 		return nil, ""
// 	}
//
// 	profiles := make(map[string][]string)
// 	var defaultProfile string
//
// 	// Remove "profiles:" prefix
// 	content := strings.TrimPrefix(versionStr, "profiles:")
//
// 	// Split by semicolon to get profile entries and default
// 	parts := strings.Split(content, ";")
//
// 	for _, part := range parts {
// 		if strings.HasPrefix(part, "default:") {
// 			defaultProfile = strings.TrimPrefix(part, "default:")
// 		} else if part != "" {
// 			// Split profile name and domains
// 			profileParts := strings.SplitN(part, ":", 2)
// 			profileName := profileParts[0]
//
// 			var domains []string
// 			if len(profileParts) > 1 && profileParts[1] != "" {
// 				domains = strings.Split(profileParts[1], ",")
// 			}
//
// 			profiles[profileName] = domains
// 		}
// 	}
//
// 	return profiles, defaultProfile
// }
