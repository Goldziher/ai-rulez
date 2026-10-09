package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

const (
	aiRulezDirName = ".ai-rulez"
	// altConfigDirName is the project-level .config/ convention
	// (https://github.com/pi0/config-dir). It is consulted as a fallback after
	// .ai-rulez/ when no tool-specific directory exists at a given level.
	altConfigDirName   = ".config/ai-rulez"
	configTOMLFilename = "config.toml"
	rulesDir           = "rules"
	contextDir         = "context"
	skillsDir          = "skills"
	agentsDir          = "agents"
	commandsDir        = "commands"
	checksDir          = "checks"
	domainsDir         = "domains"
	skillMarkerFile    = "SKILL.md"
	commandMarkerFile  = "COMMAND.md"
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
	return resolveConfigDirName(osView(absDir), absDir)
}

// resolveConfigDirName is ResolveConfigDirName reading through v; absDir is absolute.
func resolveConfigDirName(v workspace.View, absDir string) string {
	for _, dirName := range configDirCandidates {
		if hasConfigFile(v, filepath.Join(absDir, filepath.FromSlash(dirName))) {
			return dirName
		}
	}
	return ""
}

// ProjectBaseDir returns the project directory that owns configDir: the
// directory containing the config directory. For the nested .config/ai-rulez/
// layout it skips past the generic .config/ wrapper so generated outputs are
// rooted in the project, not in .config/.
func ProjectBaseDir(configDir string) string {
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
	if err != nil || rel == "." || safefs.RelEscapes(rel) {
		return filepath.Base(configDir)
	}
	return filepath.ToSlash(rel)
}

// ResolveIncludesCallback resolves the includes of a loaded configuration into the
// content tree to use. The includes package supplies it (config cannot import it).
type ResolveIncludesCallback func(ctx context.Context, cfg *Config) (*ContentTree, error)

// ResolveInstalledSkillsCallback resolves the installed skills of a loaded configuration.
type ResolveInstalledSkillsCallback func(ctx context.Context, cfg *Config) ([]ContentFile, error)

// Resolvers fetch what a configuration pulls in from outside the project: includes
// and installed skills. They are passed to the load (WithResolvers) rather than
// registered process-wide, so a caller that fetches nothing (an embedding service,
// a test) simply passes none, and one process can serve callers with different
// credentials. A configuration without includes or installed skills never needs them.
type Resolvers struct {
	Includes ResolveIncludesCallback
	Skills   ResolveInstalledSkillsCallback
}

// LoadConfig loads a configuration from the specified base directory.
// The baseDir should contain an .ai-rulez/ subdirectory (or, as a fallback,
// .config/ai-rulez/) with config.toml.
func LoadConfig(ctx context.Context, baseDir string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	v, absDir, err := lo.baseView(ctx, baseDir)
	if err != nil {
		return nil, err
	}
	dirName := resolveConfigDirName(v, absDir)
	if dirName == "" {
		dirName = aiRulezDirName
	}
	return LoadConfigFromDir(ctx, baseDir, dirName, opts...)
}

// LoadConfigFromDir loads configuration from configDirName below baseDir.
func LoadConfigFromDir(ctx context.Context, baseDir, configDirName string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	v, absDir, err := lo.baseView(ctx, baseDir)
	if err != nil {
		return nil, err
	}

	if configDirName == "" {
		configDirName = aiRulezDirName
	}
	configDir := filepath.Join(absDir, filepath.FromSlash(configDirName))

	if info, err := v.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			if legacy := findLegacyConfig(v, absDir); legacy != "" {
				return nil, newLegacyConfigError(legacy)
			}
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

	config, err := loadConfigFile(v, configDir, lo) //nolint:contextcheck // the overlay tracking probe is a bounded local git call with its own timeout
	if err != nil {
		return nil, err
	}

	return finishLoadConfig(ctx, v, config, absDir, configDir, lo)
}

// LoadConfigFromFile loads a configuration from an exact config file path or
// from a config directory path. For a file path, the project base directory is
// the parent of the config directory (skipping a generic .config/ wrapper).
func LoadConfigFromFile(ctx context.Context, path string, opts ...LoadOption) (*Config, error) {
	lo := applyLoadOptions(opts)
	absPath, err := absoluteConfigPath(path, lo)
	if err != nil {
		return nil, err
	}
	v, absPath, err := lo.fileView(ctx, absPath)
	if err != nil {
		return nil, err
	}

	info, err := v.Stat(absPath)
	if err != nil {
		return nil, oops.
			With("path", absPath).
			Wrapf(err, "stat config path")
	}

	if info.IsDir() {
		return loadConfigFromDir(ctx, v, absPath, lo, opts)
	}

	if isLegacyConfigName(filepath.Base(absPath)) {
		return nil, newLegacyConfigError(absPath)
	}
	if isLocalConfigFilename(filepath.Base(absPath)) {
		return nil, oops.
			With("path", absPath).
			Hint("Pass the main config; the overlay is applied automatically").
			Errorf("%s is a local overlay, not a main config", filepath.Base(absPath))
	}

	cfg, err := loadConfigFilePath(v, absPath, lo) //nolint:contextcheck // the overlay tracking probe is a bounded local git call with its own timeout
	if err != nil {
		return nil, err
	}
	configDir := filepath.Dir(absPath)
	if looksLikeProjectRoot(v, configDir) {
		return nil, oops.
			With("path", absPath).
			Hint("Place config files inside a configuration directory such as .ai-rulez/config.toml, or pass a config directory path. This keeps generated outputs rooted in the project instead of the parent directory.").
			Errorf("directory layout required for root-level config file")
	}
	return finishLoadConfig(ctx, v, cfg, ProjectBaseDir(configDir), configDir, lo)
}

// absoluteConfigPath resolves path against the workspace root when one is set,
// otherwise against the working directory.
func absoluteConfigPath(path string, lo loadOptions) (string, error) {
	absPath := filepath.Clean(path)
	switch {
	case filepath.IsAbs(absPath):
	case lo.ws != nil:
		if !rooted(absPath) {
			absPath = filepath.Join(lo.ws.Root(), absPath)
		}
	default:
		var err error
		if absPath, err = filepath.Abs(path); err != nil {
			return "", oops.
				With("path", path).
				Hint("Check if the config path is valid and accessible").
				Wrapf(err, "resolve absolute config path")
		}
	}
	return absPath, nil
}

// loadConfigFromDir loads the configuration of a config directory path: the
// directory's own config.toml when it has one, otherwise a project discovery
// from that path.
func loadConfigFromDir(ctx context.Context, v workspace.View, absPath string, lo loadOptions, opts []LoadOption) (*Config, error) {
	if legacy := legacyConfigIn(v, absPath); legacy != "" && !hasConfigFile(v, absPath) {
		return nil, newLegacyConfigError(legacy)
	}
	if hasConfigFile(v, absPath) {
		cfg, loadErr := loadConfigFile(v, absPath, lo) //nolint:contextcheck // the overlay tracking probe is a bounded local git call with its own timeout
		if loadErr != nil {
			return nil, loadErr
		}
		return finishLoadConfig(ctx, v, cfg, ProjectBaseDir(absPath), absPath, lo)
	}
	return LoadConfig(ctx, absPath, opts...)
}

func looksLikeProjectRoot(v workspace.View, dir string) bool {
	for _, marker := range []string{gitDirName, "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "Taskfile.yml", "Taskfile.yaml"} {
		if v.Exists(filepath.Join(dir, marker)) {
			return true
		}
	}
	return false
}

func hasConfigFile(v workspace.View, dir string) bool {
	return v.IsRegularFile(filepath.Join(dir, configTOMLFilename))
}

// checkLegacyVersion reports the migration error of a config that still carries
// a removed version; a missing version is left to Validate.
func checkLegacyVersion(config *Config, configDir string) error {
	if !IsLegacyVersion(config.Version) {
		return nil
	}
	if err := CheckVersion(config.Version); err != nil {
		return oops.With("path", filepath.Join(configDir, config.ConfigFile)).Wrap(err)
	}
	return nil
}

func finishLoadConfig(ctx context.Context, v workspace.View, config *Config, baseDir, configDir string, lo loadOptions) (*Config, error) {
	if lo.frontmatterErrors {
		lo.host.Log = quietFrontmatterLog{logger.Or(lo.host.Log)}
	}
	// A v3/v4 config must go through `ai-rulez migrate v5` before anything
	// reads it. A missing version is left to Validate, which reports it.
	if err := checkLegacyVersion(config, configDir); err != nil {
		return nil, err
	}
	config.Workspace = v.W
	config.BaseDir = baseDir
	config.ConfigDir = configDir
	config.Host = lo.host
	config.Resolve = lo.resolvers
	config.Registry = lo.registry
	config.LockPolicy = lo.lockPolicy
	config.frontmatterErrors = lo.frontmatterErrors
	config.RulesDirs = &RulesDirSet{}
	config.ConfigDirName = relConfigDirName(baseDir, configDir)
	// Each loaded project owns its warning state: a nil collector stands for the
	// process-wide one, which two projects in one process would share.
	var sink diag.Sink
	if log := lo.host.Log; log != nil {
		sink = func(msg string, args ...any) { log.Warn(msg, args...) }
	}
	if config.Diag = lo.collector; config.Diag == nil {
		config.Diag = diag.New(sink)
	}

	// The organization policy clamps the configuration before anything is fetched.
	config.PolicyDir = lo.policyDir
	if config.enforcer = lo.policy; config.enforcer == nil {
		config.enforcer = policyFromContext(ctx)
	}
	if err := applyPolicy(ctx, config); err != nil {
		return nil, err
	}

	// Convert inline MCP servers to map
	config.MCPServers = serversToMap(config.MCPServersRaw)

	// Scan content directories
	scanner := newProjectScanner(ctx, v)
	scanner.git = gitutil.New(loadHost(lo).Runner)
	scanner.log = lo.host.Log
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
		applyContentPolicy(ctx, config)
		return config, nil
	}

	ctx = diag.WithContext(ambient.WithContext(ctx, loadHost(lo)), config.Diag)
	if lo.lockPolicy.Offline {
		ctx = WithNoFetch(ctx)
	}
	if err := resolveIncludesIfNeeded(ctx, configDir, config, lo.resolvers.Includes); err != nil {
		return nil, err
	}

	if err := resolveInstalledSkillsIfNeeded(ctx, config, lo.resolvers.Skills); err != nil {
		return nil, err
	}

	// What includes and installed skills delivered is bounded now that it is loaded.
	applyContentPolicy(ctx, config)

	return config, nil
}

func resolveIncludesIfNeeded(ctx context.Context, configDir string, config *Config, resolve ResolveIncludesCallback) error {
	log := logger.FromContext(ctx)
	if len(config.Includes) == 0 {
		return nil
	}

	if resolve == nil {
		return oops.
			With("config_dir", configDir).
			Hint("Includes are configured but the load was given no includes resolver; pass config.WithResolvers (the CLI and the MCP server do).").
			Errorf("includes configured but includes resolver is unavailable")
	}

	log.Debug("Resolving includes", "count", len(config.Includes))

	mergedContent, err := resolve(ctx, config)
	if err != nil {
		if errors.Is(err, ErrLockViolation) || errors.Is(err, ErrIncludeOutsideProject) || errors.Is(err, ErrIncludeUnresolved) {
			return err
		}
		log.Warn("Failed to resolve includes", "error", err)
		// Continue with local content only (non-fatal)
		return nil
	}

	// Replace content with merged version
	config.Content = mergedContent
	log.Debug("Successfully resolved includes",
		"rules", len(mergedContent.Rules),
		"context", len(mergedContent.Context),
		"skills", len(mergedContent.Skills),
		"agents", len(mergedContent.Agents))

	return nil
}

func resolveInstalledSkillsIfNeeded(ctx context.Context, config *Config, resolve ResolveInstalledSkillsCallback) error {
	log := logger.FromContext(ctx)
	if len(config.InstalledSkills) == 0 {
		return nil
	}

	if resolve == nil {
		return oops.
			Hint("Installed skills are configured but the load was given no resolver; pass config.WithResolvers (the CLI and the MCP server do).").
			Errorf("installed skills configured but resolver is unavailable")
	}

	log.Debug("Resolving installed skills", "count", len(config.InstalledSkills))

	skills, err := resolve(ctx, config)
	if err != nil {
		if errors.Is(err, ErrLockViolation) || errors.Is(err, ErrSkillUnresolved) {
			return err
		}
		log.Warn("Failed to resolve installed skills", "error", err)
		return nil
	}

	if config.Content == nil {
		config.Content = &ContentTree{
			Domains: make(map[string]*Domain),
		}
	}

	// Build set of existing local skill names
	existingNames := make(map[string]bool)
	for i := range config.Content.Skills {
		existingNames[config.Content.Skills[i].Name] = true
	}

	// Map each installed skill's profile scope so generation can drop it from
	// profiles it was not declared for.
	profilesByName := make(map[string][]string, len(config.InstalledSkills))
	for i := range config.InstalledSkills {
		profilesByName[config.InstalledSkills[i].Name] = config.InstalledSkills[i].Profiles
	}

	// Merge: local skills win over installed skills
	for i := range skills {
		// Copy before setting Profiles: skills belongs to the resolver.
		s := skills[i]
		if existingNames[s.Name] {
			log.Warn("Installed skill name conflicts with local skill, skipping", "name", s.Name)
			continue
		}
		s.Profiles = profilesByName[s.Name]
		config.Content.ImportedVerifiers = append(config.Content.ImportedVerifiers, s.Verifiers...)
		config.Content.Skills = append(config.Content.Skills, s)
		existingNames[s.Name] = true
	}

	log.Debug("Successfully resolved installed skills", "count", len(skills))
	return nil
}

// loadConfigFile loads config.toml from a config directory. A directory that
// holds only a V2/V3 file is reported as such rather than as a missing config.
func loadConfigFile(v workspace.View, configDir string, lo loadOptions) (*Config, error) {
	tomlPath := filepath.Join(configDir, configTOMLFilename)
	if v.Exists(tomlPath) {
		cfg, err := loadConfigTOML(v, tomlPath)
		if err != nil {
			return nil, err
		}
		cfg.ConfigFile = configTOMLFilename
		return withLocalOverlay(v, cfg, tomlPath, configDir, lo)
	}

	if legacy := legacyConfigIn(v, configDir); legacy != "" {
		return nil, newLegacyConfigError(legacy)
	}

	return nil, oops.
		With("config_dir", configDir).
		With("toml_path", tomlPath).
		Hint(fmt.Sprintf("Create %s in %s\nRun 'ai-rulez init' to initialize configuration", configTOMLFilename, configDir)).
		Errorf("no config file found (tried %s)", configTOMLFilename)
}

func loadConfigFilePath(v workspace.View, path string, lo loadOptions) (*Config, error) {
	cfg, err := loadConfigFilePathMain(v, path)
	if err != nil {
		return nil, err
	}
	return withLocalOverlay(v, cfg, path, filepath.Dir(path), lo)
}

func loadConfigFilePathMain(v workspace.View, path string) (*Config, error) {
	if filepath.Base(path) != configTOMLFilename {
		return nil, oops.
			With("path", path).
			Hint("Use config.toml inside a config directory").
			Errorf("unsupported config filename: %s", filepath.Base(path))
	}
	cfg, err := loadConfigTOML(v, path)
	if err != nil {
		return nil, err
	}
	cfg.ConfigFile = filepath.Base(path)
	return cfg, nil
}

// loadConfigTOML loads a config from TOML
func loadConfigTOML(v workspace.View, path string) (*Config, error) {
	if err := checkConfigFileInside(v, path); err != nil {
		return nil, err
	}
	data, err := readCapped(v, path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions", path)).
			Wrapf(err, "read config file")
	}
	return decodeConfigTOML(data, path)
}

// tomlConfig is the TOML decoding shape of Config: presets and builtins are
// mixed-type arrays that need a conversion step. Every toml-tagged field of
// Config must appear here (TestTomlConfigCoversConfig).
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
	AgentsMD        *bool                  `toml:"agents_md"`
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
	VerifiersSet    *VerifiersSettings     `toml:"verifiers_settings"`
	Usage           *UsageConfig           `toml:"usage"`
	Skills          *SkillsConfig          `toml:"skills"`
	DomainSettings  DomainConfigs          `toml:"domains"`
	SkillSources    []SkillSourceConfig    `toml:"skill_sources"`
	Roles           []RoleConfig           `toml:"roles"`
	RoleManifest    *RoleManifestConfig    `toml:"role_manifest"`
	Lock            *LockConfig            `toml:"lock"`
	Governance      *GovernanceConfig      `toml:"governance"`
	Catalog         *CatalogConfig         `toml:"catalog"`
	Signing         *SigningConfig         `toml:"signing"`
	Publish         *PublishConfig         `toml:"publish"`
	ARD             *ARDConfig             `toml:"ard"`
	LLM             *llm.Config            `toml:"llm"`
	Telemetry       *TelemetryConfig       `toml:"telemetry"`
	Review          *ReviewConfig          `toml:"review"`
	Improve         *ImproveConfig         `toml:"improve"`
	Search          *skillsearch.Config    `toml:"search"`
	Plugin          *PluginAuthoring       `toml:"plugin"`
	Marketplace     *MarketplaceAuthoring  `toml:"marketplace"`
	Placement       *PlacementConfig       `toml:"placement"`
	Claude          *ClaudeConfig          `toml:"claude"`
	Codex           *CodexConfig           `toml:"codex"`
	Hooks           []HookGroup            `toml:"hooks"`
	Guard           *GuardConfig           `toml:"guard"`
	Permissions     *Permissions           `toml:"permissions"`
	OKF             *OKFConfig             `toml:"okf"`
	LLMsTxt         *LLMsTxtConfig         `toml:"llms_txt"`
}

// decodeConfigTOML decodes TOML bytes; path is used for error context only.
func decodeConfigTOML(data []byte, path string) (*Config, error) {
	// TOML presets are plain strings; we unmarshal into an intermediate
	// struct then convert to []Preset. This avoids custom unmarshaler
	// issues with the TOML library.

	var raw tomlConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		if swapped := swappedLintTablesTOML(path, data); swapped != nil {
			return nil, swapped
		}
		if described := describeTOMLDecodeError(path, err); described != nil {
			return nil, described
		}
		return nil, oops.
			With("path", path).
			Hint("Check the TOML syntax - ensure proper formatting\nCommon issues: missing quotes around strings, incorrect table syntax").
			Wrapf(err, "parse TOML config")
	}

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
		AgentsMD:        raw.AgentsMD == nil || *raw.AgentsMD,
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
		OKF:             raw.OKF,
		LLMsTxt:         raw.LLMsTxt,
		Verifiers:       raw.Verifiers,
		Usage:           raw.Usage,
		Skills:          raw.Skills,
		DomainSettings:  raw.DomainSettings,
		SkillSources:    raw.SkillSources,
		Roles:           raw.Roles,
		RoleManifest:    raw.RoleManifest,
		Lock:            raw.Lock,
		Governance:      raw.Governance,
		Catalog:         raw.Catalog,
		Signing:         raw.Signing,
		Publish:         raw.Publish,
		ARD:             raw.ARD,
		LLM:             raw.LLM,
		Telemetry:       raw.Telemetry,
		Review:          raw.Review,
		Improve:         raw.Improve,
		Search:          raw.Search,
		Plugin:          raw.Plugin,
		Marketplace:     raw.Marketplace,
		Placement:       raw.Placement,
		Claude:          raw.Claude,
		Codex:           raw.Codex,
		Hooks:           raw.Hooks,
		Guard:           raw.Guard,
		Permissions:     raw.Permissions,
	}
	cfg.VerifiersSettings = raw.VerifiersSet
	if err := renamedRatchetTable(path, raw.Lint); err != nil {
		return nil, err
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

// SaveConfig writes a configuration back to config.toml in configDir. TOML is
// written with MarshalTOML (comments are not preserved).
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

	targetPath := filepath.Join(configDir, configTOMLFilename)
	data, err := MarshalTOML(cfg)
	if err != nil {
		return oops.With("path", targetPath).Wrapf(err, "marshal config to TOML")
	}
	return writeConfigAtomically(targetPath, data)
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

// loadHost is the host the load runs under: the one given by WithHost, with the
// runner of WithRunner when the host names none.
func loadHost(lo loadOptions) ambient.Host {
	h := lo.host
	if h.Runner == nil {
		h.Runner = lo.runner
	}
	return h
}

// warnMalformedFrontmatter reports a content file whose delimited frontmatter
// could not be parsed. The block is dropped from the content and the file is
// marked malformed, so validation fails; this tells the author which file and why.
func warnMalformedFrontmatter(log logger.Logger, path string) {
	log.Warn(malformedFrontmatterMsg, "path", path)
}

const malformedFrontmatterMsg = "Ignoring malformed YAML frontmatter — check for unquoted values containing ': '"

// quietFrontmatterLog drops the malformed-frontmatter warning (see
// WithFrontmatterErrors) and forwards everything else.
type quietFrontmatterLog struct{ logger.Logger }

// Unwrap returns the wrapped logger (see diag.OnceLogger).
func (q quietFrontmatterLog) Unwrap() logger.Logger { return q.Logger }

func (q quietFrontmatterLog) Warn(msg string, args ...any) {
	if msg != malformedFrontmatterMsg {
		q.Logger.Warn(msg, args...)
	}
}
