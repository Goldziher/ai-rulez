package config

import (
	"encoding/json"
	"os"
	"time"
)

// Config represents the configuration format
type Config struct {
	Schema          string                 `yaml:"$schema,omitempty" json:"$schema,omitempty" toml:"schema,omitempty"`
	Version         string                 `yaml:"version" json:"version" toml:"version"`
	Name            string                 `yaml:"name" json:"name" toml:"name"`
	Description     string                 `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Presets         []Preset               `yaml:"presets,omitempty" json:"presets,omitempty" toml:"presets,omitempty"`
	Default         string                 `yaml:"default,omitempty" json:"default,omitempty" toml:"default,omitempty"`
	Profiles        map[string][]string    `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	Gitignore       *bool                  `yaml:"gitignore,omitempty" json:"gitignore,omitempty" toml:"gitignore,omitempty"`
	Includes        []IncludeConfig        `yaml:"includes,omitempty" json:"includes,omitempty" toml:"includes,omitempty"`
	InstalledSkills []InstalledSkillConfig `yaml:"installed_skills,omitempty" json:"installed_skills,omitempty" toml:"installed_skills,omitempty"` //nolint:tagliatelle
	Header          *HeaderConfig          `yaml:"header,omitempty" json:"header,omitempty" toml:"header,omitempty"`
	Defaults        *DefaultsConfig        `yaml:"defaults,omitempty" json:"defaults,omitempty" toml:"defaults,omitempty"`
	Builtins        *BuiltinsConfig        `yaml:"builtins,omitempty" json:"builtins,omitempty" toml:"builtins,omitempty"`
	Compact         *bool                  `yaml:"compact,omitempty" json:"compact,omitempty" toml:"compact,omitempty"`
	Plugins         []PluginConfig         `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`
	Marketplaces    []MarketplaceConfig    `yaml:"marketplaces,omitempty" json:"marketplaces,omitempty" toml:"marketplaces,omitempty"`
	Scopes          []ScopeConfig          `yaml:"scopes,omitempty" json:"scopes,omitempty" toml:"scopes,omitempty"`

	// Runtime fields (populated during load)
	BaseDir       string                `yaml:"-" json:"-" toml:"-"`
	ConfigDir     string                `yaml:"-" json:"-" toml:"-"`
	ConfigDirName string                `yaml:"-" json:"-" toml:"-"`
	ConfigFile    string                `yaml:"-" json:"-" toml:"-"` // Actual config filename (e.g. "config.toml")
	Content       *ContentTree          `yaml:"-" json:"-" toml:"-"`
	MCPServers    map[string]*MCPServer `yaml:"-" json:"-" toml:"-"`
	MCPServersRaw []MCPServer           `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty" toml:"mcp_servers,omitempty"`

	// SourceHash is a blake3 hash over all sources that contribute to generated
	// output for the active profile (config metadata, content tree, MCP servers,
	// and the generator schema version). Computed once per generation and embedded
	// in every output file's header so that subsequent runs can detect "nothing
	// changed in sources" without re-rendering. Format: "blake3:<hex>".
	SourceHash string `yaml:"-" json:"-" toml:"-"`

	// MCPEnvOverrides are generation-time KEY=VALUE overrides used to resolve
	// MCP env placeholders. They are intentionally not serialized.
	MCPEnvOverrides map[string]string `yaml:"-" json:"-" toml:"-"`

	// MCPEnvFiles are generation-time dotenv files used to resolve MCP env
	// placeholders. Empty means "load .env from BaseDir when present".
	MCPEnvFiles []string `yaml:"-" json:"-" toml:"-"`
}

// ScopeConfig configures an additional scoped output root for directory-aware
// assistants such as Codex and Claude Code.
type ScopeConfig struct {
	Name    string   `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Path    string   `yaml:"path" json:"path" toml:"path"`
	Profile string   `yaml:"profile,omitempty" json:"profile,omitempty" toml:"profile,omitempty"`
	Presets []string `yaml:"presets,omitempty" json:"presets,omitempty" toml:"presets,omitempty"`
}

// HeaderConfig represents header style configuration for generated files
type HeaderConfig struct {
	Style string `yaml:"style,omitempty" json:"style,omitempty" toml:"style,omitempty"` // "detailed", "compact", or "minimal"
}

// GetHeaderStyle returns the header style, defaulting to "detailed"
func (h *HeaderConfig) GetHeaderStyle() string {
	if h == nil || h.Style == "" {
		return "detailed"
	}
	return h.Style
}

// DefaultsConfig represents top-level defaults that propagate into generated outputs
// when individual content files do not override them.
type DefaultsConfig struct {
	// Effort is the default reasoning effort applied across providers that support it.
	// Accepted values: low, medium, high, xhigh, max, inherit. Empty string means "not set".
	// Each preset maps the value to its own vocabulary (see internal/generator/presets/effort.go).
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty" toml:"effort,omitempty"`

	// EffortByPreset overrides Effort for specific presets. Keys are preset names (e.g. "codex",
	// "claude", "windsurf"). Per-agent frontmatter still wins over this map where the preset
	// supports per-agent effort.
	EffortByPreset map[string]string `yaml:"effort_by_preset,omitempty" json:"effort_by_preset,omitempty" toml:"effort_by_preset,omitempty"`

	// ModelByPreset overrides the agent `model` per preset. Keys are preset names (e.g.
	// "claude", "copilot", "cursor"). Per-agent `<preset>_model` frontmatter still wins
	// over this map; the legacy `model` field is the lowest-priority fallback. Presets
	// that do not emit a per-agent model frontmatter ignore entries for their preset.
	ModelByPreset map[string]string `yaml:"model_by_preset,omitempty" json:"model_by_preset,omitempty" toml:"model_by_preset,omitempty"`
}

// BuiltinsConfig represents the builtins field which can be:
//   - boolean true: enable all builtins
//   - boolean false: disable all builtins (including auto-includes)
//   - array of strings: enable specific builtins (supports "!name" exclusion)
type BuiltinsConfig struct {
	All   *bool    // true = all, false = none
	Names []string // specific builtin names (when All is nil)
}

// IsEnabled returns true if builtins are configured (not nil)
func (b *BuiltinsConfig) IsEnabled() bool {
	return b != nil
}

// IsAll returns true if all builtins should be loaded
func (b *BuiltinsConfig) IsAll() bool {
	return b != nil && b.All != nil && *b.All
}

// IsNone returns true if all builtins should be disabled
func (b *BuiltinsConfig) IsNone() bool {
	return b != nil && b.All != nil && !*b.All
}

// GetNames returns the list of builtin names (empty if All is set)
func (b *BuiltinsConfig) GetNames() []string {
	if b == nil {
		return nil
	}
	return b.Names
}

// UnmarshalYAML implements custom YAML unmarshaling for BuiltinsConfig
func (b *BuiltinsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try boolean first
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		b.All = &boolVal
		return nil
	}

	// Try array of strings
	var names []string
	if err := unmarshal(&names); err != nil {
		return err
	}
	b.Names = names
	return nil
}

// MarshalYAML implements custom YAML marshaling for BuiltinsConfig
func (b BuiltinsConfig) MarshalYAML() (interface{}, error) { //nolint:gocritic // Value receiver required for marshaling
	if b.All != nil {
		return *b.All, nil
	}
	return b.Names, nil
}

// UnmarshalJSON implements custom JSON unmarshaling for BuiltinsConfig
func (b *BuiltinsConfig) UnmarshalJSON(data []byte) error {
	// Try boolean first
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		b.All = &boolVal
		return nil
	}

	// Try array of strings
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	b.Names = names
	return nil
}

// MarshalJSON implements custom JSON marshaling for BuiltinsConfig
func (b BuiltinsConfig) MarshalJSON() ([]byte, error) { //nolint:gocritic // Value receiver required for marshaling
	if b.All != nil {
		return json.Marshal(*b.All)
	}
	return json.Marshal(b.Names)
}

// Preset represents either a built-in preset name or a custom preset configuration
type Preset struct {
	// Built-in preset (e.g., "claude", "cursor")
	BuiltIn string `yaml:"-" json:"-" toml:"-"`

	// Custom preset fields
	Name     string     `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Type     PresetType `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
	Path     string     `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Template string     `yaml:"template,omitempty" json:"template,omitempty" toml:"template,omitempty"`
}

// PresetType defines the type of custom preset output
type PresetType string

const (
	PresetTypeMarkdown  PresetType = "markdown"
	PresetTypeDirectory PresetType = "directory"
	PresetTypeJSON      PresetType = "json"
)

// Config schema versions accepted by the loader and validator.
const (
	ConfigVersionV3 = "3.0"
	ConfigVersionV4 = "4.0"
)

// UnmarshalYAML implements custom YAML unmarshaling for Preset
func (p *Preset) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := unmarshal(&builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias Preset
	var custom presetAlias
	if err := unmarshal(&custom); err != nil {
		return err
	}

	p.Name = custom.Name
	p.Type = custom.Type
	p.Path = custom.Path
	p.Template = custom.Template
	return nil
}

// MarshalYAML implements custom YAML marshaling for Preset
func (p Preset) MarshalYAML() (interface{}, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return p.BuiltIn, nil
	}

	// Marshal as custom preset object
	type presetAlias Preset
	return presetAlias(p), nil
}

// UnmarshalJSON implements custom JSON unmarshaling for Preset
func (p *Preset) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := json.Unmarshal(data, &builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias Preset
	var custom presetAlias
	if err := json.Unmarshal(data, &custom); err != nil {
		return err
	}

	p.Name = custom.Name
	p.Type = custom.Type
	p.Path = custom.Path
	p.Template = custom.Template
	return nil
}

// MarshalJSON implements custom JSON marshaling for Preset
func (p Preset) MarshalJSON() ([]byte, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return json.Marshal(p.BuiltIn)
	}

	// Marshal as custom preset object
	type presetAlias Preset
	return json.Marshal(presetAlias(p))
}

// IsBuiltIn returns true if this is a built-in preset
func (p *Preset) IsBuiltIn() bool {
	return p.BuiltIn != ""
}

// GetName returns the preset name (built-in or custom)
func (p *Preset) GetName() string {
	if p.IsBuiltIn() {
		return p.BuiltIn
	}
	return p.Name
}

// IsValid returns true if the preset is valid
func (p *Preset) IsValid() bool {
	if p.IsBuiltIn() {
		return isValidBuiltInPreset(p.BuiltIn)
	}
	return p.Name != "" && p.Type != "" && p.Path != ""
}

// Built-in preset names
var builtInPresets = map[string]bool{
	string(PresetClaude):      true,
	string(PresetCursor):      true,
	string(PresetGemini):      true,
	string(PresetCopilot):     true,
	string(PresetContinue):    true,
	string(PresetWindsurf):    true,
	string(PresetCline):       true,
	string(PresetCodex):       true,
	string(PresetAmp):         true,
	string(PresetJunie):       true,
	"opencode":                true,
	string(PresetAntigravity): true,
	"mcp":                     true,
}

func isValidBuiltInPreset(name string) bool {
	return builtInPresets[name]
}

// MCPServer represents an MCP (Model Context Protocol) server configuration
type MCPServer struct {
	Name        string            `yaml:"name" json:"name" toml:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Command     string            `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty" json:"args,omitempty" toml:"args,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty" toml:"env,omitempty"`
	Transport   string            `yaml:"transport,omitempty" json:"transport,omitempty" toml:"transport,omitempty"`
	URL         string            `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`
	Enabled     *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`

	// SecretEnvKeys records env keys whose generated values should be treated as
	// sensitive. It is populated during generation and never serialized.
	SecretEnvKeys []string `yaml:"-" json:"-" toml:"-"`
}

// IsEnabled returns true if the MCP server is enabled (defaults to true if not specified)
func (m *MCPServer) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// GetTransport returns the transport protocol, defaulting to "stdio"
func (m *MCPServer) GetTransport() string {
	if m == nil || m.Transport == "" {
		return "stdio"
	}
	return m.Transport
}

// ContentTree represents the scanned content from .ai-rulez/ directory
type ContentTree struct {
	Rules    []ContentFile      `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context  []ContentFile      `yaml:"context,omitempty" json:"context,omitempty"`
	Skills   []ContentFile      `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents   []ContentFile      `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands []ContentFile      `yaml:"commands,omitempty" json:"commands,omitempty"`
	Domains  map[string]*Domain `yaml:"domains,omitempty" json:"domains,omitempty"`
}

// Domain represents content from a specific domain directory
type Domain struct {
	Name        string        `yaml:"name" json:"name"`
	Rules       []ContentFile `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context     []ContentFile `yaml:"context,omitempty" json:"context,omitempty"`
	Skills      []ContentFile `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents      []ContentFile `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands    []ContentFile `yaml:"commands,omitempty" json:"commands,omitempty"`
	Builtin     bool          `yaml:"-" json:"-"` // true if loaded from builtins
	FromInclude bool          `yaml:"-" json:"-"` // true if loaded from an external include
}

// ContentFile represents a single content file with optional frontmatter
type ContentFile struct {
	Name     string    `yaml:"name" json:"name"`
	Path     string    `yaml:"path" json:"path"`
	Content  string    `yaml:"content" json:"content"`
	Metadata *Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// Resources holds skill supporting files (references/, scripts/, assets/)
	// loaded alongside SKILL.md. Always empty for non-skill content.
	Resources []SkillResource `yaml:"-" json:"-"`
}

// SkillResource is one supporting file bundled with a skill.
//
// The canonical Agent Skills layout (followed by Claude Code, OpenAI Codex,
// and the agentskills.io standard) places these under three subdirectories:
//
//	references/  — markdown docs the agent reads on demand
//	scripts/     — executable scripts the agent invokes
//	assets/      — files used in output (templates, images, etc.)
//
// Resources are emitted as individual files under the rendered skill
// directory and indexed from SKILL.md so the agent can discover them.
type SkillResource struct {
	// Kind is one of "references", "scripts", "assets".
	Kind string
	// RelPath is the resource path relative to the skill root, including the
	// kind subdirectory (e.g. "references/api.md").
	RelPath string
	// Content holds the raw bytes of the file. Bytes (not string) so binary
	// assets round-trip without UTF-8 corruption.
	Content []byte
	// Mode is the file permission bits read from disk. Preserved through
	// generation so bundled scripts keep their executable bit.
	Mode os.FileMode
	// Description is parsed from a reference file's frontmatter `description`
	// field, or falls back to the first non-empty markdown line. Empty for
	// scripts and assets, or when no description is available.
	Description string
}

// Metadata represents parsed frontmatter metadata.
//
// Tools, Skills, and Keywords are list-valued and need typed handling because
// YAML sequences cannot round-trip through map[string]string — they would be
// stringified via fmt %v ("[a b c]") instead of preserved as proper lists.
type Metadata struct {
	Priority string            `yaml:"priority,omitempty" json:"priority,omitempty"`
	Targets  []string          `yaml:"targets,omitempty" json:"targets,omitempty"`
	Aliases  []string          `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Tools    []string          `yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills   []string          `yaml:"skills,omitempty" json:"skills,omitempty"`
	Keywords []string          `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Usage    string            `yaml:"usage,omitempty" json:"usage,omitempty"`
	Shortcut string            `yaml:"shortcut,omitempty" json:"shortcut,omitempty"`
	Category string            `yaml:"category,omitempty" json:"category,omitempty"`
	Effort   string            `yaml:"effort,omitempty" json:"effort,omitempty"`
	Extra    map[string]string `yaml:",inline" json:",inline"`
}

// GetPriority returns the priority as a Priority type, defaulting to medium
func (m *Metadata) GetPriority() Priority {
	if m == nil || m.Priority == "" {
		return PriorityMedium
	}
	p, err := ParsePriority(m.Priority)
	if err != nil {
		return PriorityMedium
	}
	return p
}

// HasTargets returns true if targets are specified
func (m *Metadata) HasTargets() bool {
	return m != nil && len(m.Targets) > 0
}

// Helper methods for Config

// ShouldUpdateGitignore returns whether .gitignore should be updated
func (c *Config) ShouldUpdateGitignore() bool {
	if c.Gitignore == nil {
		return true
	}
	return *c.Gitignore
}

// GetDefaultProfile returns the default profile name
func (c *Config) GetDefaultProfile() string {
	return c.Default
}

// GetProfileDomains returns the list of domains for a profile
func (c *Config) GetProfileDomains(profile string) []string {
	if profile == "" {
		profile = c.Default
	}
	if domains, ok := c.Profiles[profile]; ok {
		return domains
	}
	return nil
}

// HasProfile returns true if the profile exists
func (c *Config) HasProfile(profile string) bool {
	_, ok := c.Profiles[profile]
	return ok
}

// GetVersion returns the config version
func (c *Config) GetVersion() string {
	return c.Version
}

// IsV3 returns true if this is a V3 config (version == "3.0")
func (c *Config) IsV3() bool {
	return c.Version == ConfigVersionV3
}

// IsV4 returns true if this is a V4 config (version == "4.0")
func (c *Config) IsV4() bool {
	return c.Version == ConfigVersionV4
}

// IsCompact reports whether compact rendering is enabled. When true, presets
// omit per-rule "**Priority:**" annotations from inline rule sections to reduce
// output size. Defaults to false (full output).
func (c *Config) IsCompact() bool {
	return c != nil && c.Compact != nil && *c.Compact
}

// GetHeaderStyle returns the configured header style ("detailed", "compact", or "minimal")
func (c *Config) GetHeaderStyle() string {
	if c.Header == nil {
		return "detailed"
	}
	return c.Header.GetHeaderStyle()
}

// GetContentForProfile returns all content for a given profile.
// Root-level slices contain only root content. Domains are placed in the
// Domains map so that preset generators can combine them via
// combineContentFiles / getAllDomain* helpers without duplication.
func (c *Config) GetContentForProfile(profile string) (*ContentTree, error) {
	if c.Content == nil {
		return nil, ErrNoContent
	}

	profileDomains := c.GetProfileDomains(profile)

	// Build filtered domains map: profile-listed domains + builtin + FromInclude
	activeDomains := make(map[string]*Domain)

	// First pass: include all builtin and FromInclude domains unconditionally.
	// This ensures that domains from external includes are always available,
	// regardless of whether they are explicitly listed in the profile.
	for name, domain := range c.Content.Domains {
		if domain.Builtin || domain.FromInclude {
			activeDomains[name] = domain
		}
	}

	// Second pass: add profile-specified domains (may overlap with FromInclude).
	for _, name := range profileDomains {
		if domain, ok := c.Content.Domains[name]; ok {
			activeDomains[name] = domain
		}
	}

	return &ContentTree{
		Rules:    c.Content.Rules,
		Context:  c.Content.Context,
		Skills:   c.Content.Skills,
		Agents:   c.Content.Agents,
		Commands: c.Content.Commands,
		Domains:  activeDomains,
	}, nil
}

// Helper methods for ContentTree

// GetAllContentFiles returns all content files from the tree
func (t *ContentTree) GetAllContentFiles() []ContentFile {
	var files []ContentFile
	files = append(files, t.Rules...)
	files = append(files, t.Context...)
	files = append(files, t.Skills...)
	files = append(files, t.Agents...)
	files = append(files, t.Commands...)
	for _, domain := range t.Domains {
		files = append(files, domain.Rules...)
		files = append(files, domain.Context...)
		files = append(files, domain.Skills...)
		files = append(files, domain.Agents...)
		files = append(files, domain.Commands...)
	}
	return files
}

// GetRulesForDomains returns rules for specified domains (including root)
func (t *ContentTree) GetRulesForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Rules))
	copy(files, t.Rules)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Rules...)
		}
	}
	return files
}

// GetContextForDomains returns context files for specified domains (including root)
func (t *ContentTree) GetContextForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Context))
	copy(files, t.Context)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Context...)
		}
	}
	return files
}

// GetSkillsForDomains returns skills for specified domains (including root)
func (t *ContentTree) GetSkillsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Skills))
	copy(files, t.Skills)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Skills...)
		}
	}
	return files
}

// GetAgentsForDomains returns agents for specified domains (including root)
func (t *ContentTree) GetAgentsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Agents))
	copy(files, t.Agents)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Agents...)
		}
	}
	return files
}

// GetCommandsForDomains returns commands for specified domains (including root)
func (t *ContentTree) GetCommandsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Commands))
	copy(files, t.Commands)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Commands...)
		}
	}
	return files
}

// Helper methods for ContentFile

// GetFileExtension returns the file extension for the content file
func (f *ContentFile) GetFileExtension() string {
	if f == nil || f.Path == "" {
		return ""
	}
	if idx := len(f.Path) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if f.Path[i] == '.' {
				return f.Path[i:]
			}
			if f.Path[i] == '/' {
				break
			}
		}
	}
	return ""
}

// IsMarkdown returns true if the content file is markdown
func (f *ContentFile) IsMarkdown() bool {
	ext := f.GetFileExtension()
	return ext == ".md" || ext == ".markdown"
}

// IncludeConfig represents a content source (git repo or local path)
type IncludeConfig struct {
	Name          string   `yaml:"name" json:"name" toml:"name"`
	Source        string   `yaml:"source" json:"source" toml:"source"`
	Path          string   `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Include       []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Ref           string   `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	InstallTo     string   `yaml:"install_to,omitempty" json:"install_to,omitempty" toml:"install_to,omitempty"`             //nolint:tagliatelle
	MergeStrategy string   `yaml:"merge_strategy,omitempty" json:"merge_strategy,omitempty" toml:"merge_strategy,omitempty"` //nolint:tagliatelle
	LocalOverride string   `yaml:"local_override,omitempty" json:"local_override,omitempty" toml:"local_override,omitempty"` //nolint:tagliatelle
}

// InstalledSkillConfig represents a named skill to install from an external source
type InstalledSkillConfig struct {
	Name          string `yaml:"name" json:"name" toml:"name"`
	Source        string `yaml:"source" json:"source" toml:"source"`
	Path          string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Ref           string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	LocalOverride string `yaml:"local_override,omitempty" json:"local_override,omitempty" toml:"local_override,omitempty"` //nolint:tagliatelle
}

// GetPath returns the path within the repo, defaulting to "skills/<name>"
func (s *InstalledSkillConfig) GetPath() string {
	if s.Path != "" {
		return s.Path
	}
	return "skills/" + s.Name
}

// IncludeLock tracks resolved include sources
type IncludeLock struct {
	Includes map[string]IncludeLockEntry `yaml:"includes" json:"includes"`
}

// IncludeLockEntry represents a locked include source
type IncludeLockEntry struct {
	Source      string    `yaml:"source" json:"source"`
	Type        string    `yaml:"type" json:"type"`                                     // "git" or "local"
	ResolvedRef string    `yaml:"resolved_ref,omitempty" json:"resolved_ref,omitempty"` // git only
	ResolvedAt  time.Time `yaml:"resolved_at" json:"resolved_at"`
}
