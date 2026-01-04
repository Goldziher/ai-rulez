package config

import (
	"encoding/json"
	"time"
)

// ConfigV3 represents the V3 configuration format
type ConfigV3 struct {
	Schema      string              `yaml:"$schema,omitempty" json:"$schema,omitempty"`
	Version     string              `yaml:"version" json:"version"`
	Name        string              `yaml:"name" json:"name"`
	Description string              `yaml:"description,omitempty" json:"description,omitempty"`
	Presets     []PresetV3          `yaml:"presets,omitempty" json:"presets,omitempty"`
	Default     string              `yaml:"default,omitempty" json:"default,omitempty"`
	Profiles    map[string][]string `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Gitignore   *bool               `yaml:"gitignore,omitempty" json:"gitignore,omitempty"`
	Includes    []IncludeConfig     `yaml:"includes,omitempty" json:"includes,omitempty"`
	Header      *HeaderConfig       `yaml:"header,omitempty" json:"header,omitempty"`
	Compression *CompressionConfig  `yaml:"compression,omitempty" json:"compression,omitempty"`

	// Runtime fields (populated during load)
	BaseDir    string                  `yaml:"-" json:"-"`
	Content    *ContentTreeV3          `yaml:"-" json:"-"`
	MCPServers map[string]*MCPServerV3 `yaml:"-" json:"-"`
}

// HeaderConfig represents header style configuration for generated files
type HeaderConfig struct {
	Style string `yaml:"style,omitempty" json:"style,omitempty"` // "detailed", "compact", or "minimal"
}

// GetHeaderStyle returns the header style, defaulting to "detailed"
func (h *HeaderConfig) GetHeaderStyle() string {
	if h == nil || h.Style == "" {
		return "detailed"
	}
	return h.Style
}

// CompressionConfig represents compression settings for generated files
type CompressionConfig struct {
	Level              string `yaml:"level,omitempty" json:"level,omitempty"`                             // "none", "minimal", "standard", "aggressive"
	RemoveDuplicates   *bool  `yaml:"remove_duplicates,omitempty" json:"remove_duplicates,omitempty"`     // Extract common patterns
	UseAbbreviations   *bool  `yaml:"use_abbreviations,omitempty" json:"use_abbreviations,omitempty"`     // Use shorter markdown syntax
	PreserveFormatting *bool  `yaml:"preserve_formatting,omitempty" json:"preserve_formatting,omitempty"` // Disable whitespace optimization
}

// GetCompressionLevel returns the compression level, defaulting to "none"
func (c *CompressionConfig) GetCompressionLevel() string {
	if c == nil || c.Level == "" {
		return "none"
	}
	return c.Level
}

// ShouldRemoveDuplicates returns true if deduplication is enabled
func (c *CompressionConfig) ShouldRemoveDuplicates() bool {
	if c == nil || c.RemoveDuplicates == nil {
		level := c.GetCompressionLevel()
		return level == "standard" || level == "aggressive"
	}
	return *c.RemoveDuplicates
}

// ShouldUseAbbreviations returns true if abbreviations are enabled
func (c *CompressionConfig) ShouldUseAbbreviations() bool {
	if c == nil || c.UseAbbreviations == nil {
		return c.GetCompressionLevel() == "aggressive"
	}
	return *c.UseAbbreviations
}

// ShouldPreserveFormatting returns false by default (allow optimization)
func (c *CompressionConfig) ShouldPreserveFormatting() bool {
	if c == nil || c.PreserveFormatting == nil {
		return false
	}
	return *c.PreserveFormatting
}

// PresetV3 represents either a built-in preset name or a custom preset configuration
type PresetV3 struct {
	// Built-in preset (e.g., "claude", "cursor")
	BuiltIn string `yaml:"-" json:"-"`

	// Custom preset fields
	Name     string     `yaml:"name,omitempty" json:"name,omitempty"`
	Type     PresetType `yaml:"type,omitempty" json:"type,omitempty"`
	Path     string     `yaml:"path,omitempty" json:"path,omitempty"`
	Template string     `yaml:"template,omitempty" json:"template,omitempty"`
}

// PresetType defines the type of custom preset output
type PresetType string

const (
	PresetTypeMarkdown  PresetType = "markdown"
	PresetTypeDirectory PresetType = "directory"
	PresetTypeJSON      PresetType = "json"
)

// UnmarshalYAML implements custom YAML unmarshaling for PresetV3
func (p *PresetV3) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := unmarshal(&builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias PresetV3
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

// MarshalYAML implements custom YAML marshaling for PresetV3
func (p PresetV3) MarshalYAML() (interface{}, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return p.BuiltIn, nil
	}

	// Marshal as custom preset object
	type presetAlias PresetV3
	return presetAlias(p), nil
}

// UnmarshalJSON implements custom JSON unmarshaling for PresetV3
func (p *PresetV3) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := json.Unmarshal(data, &builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias PresetV3
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

// MarshalJSON implements custom JSON marshaling for PresetV3
func (p PresetV3) MarshalJSON() ([]byte, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return json.Marshal(p.BuiltIn)
	}

	// Marshal as custom preset object
	type presetAlias PresetV3
	return json.Marshal(presetAlias(p))
}

// IsBuiltIn returns true if this is a built-in preset
func (p *PresetV3) IsBuiltIn() bool {
	return p.BuiltIn != ""
}

// GetName returns the preset name (built-in or custom)
func (p *PresetV3) GetName() string {
	if p.IsBuiltIn() {
		return p.BuiltIn
	}
	return p.Name
}

// IsValid returns true if the preset is valid
func (p *PresetV3) IsValid() bool {
	if p.IsBuiltIn() {
		return isValidBuiltInPreset(p.BuiltIn)
	}
	return p.Name != "" && p.Type != "" && p.Path != ""
}

// Built-in preset names
var builtInPresetsV3 = map[string]bool{
	"claude":       true,
	"cursor":       true,
	"gemini":       true,
	"copilot":      true,
	"continue-dev": true,
	"windsurf":     true,
	"cline":        true,
	"codex":        true,
	"amp":          true,
	"junie":        true,
	"opencode":     true,
}

func isValidBuiltInPreset(name string) bool {
	return builtInPresetsV3[name]
}

// MCPServerV3 represents an MCP (Model Context Protocol) server configuration
type MCPServerV3 struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Command     string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Transport   string            `yaml:"transport,omitempty" json:"transport,omitempty"`
	URL         string            `yaml:"url,omitempty" json:"url,omitempty"`
	Enabled     *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// MCPConfigV3 represents loaded MCP configuration from mcp.yaml or mcp.json
type MCPConfigV3 struct {
	Schema  string        `yaml:"$schema,omitempty" json:"$schema,omitempty"`
	Version string        `yaml:"version" json:"version"`
	Servers []MCPServerV3 `yaml:"mcp_servers" json:"mcp_servers"`
}

// IsEnabled returns true if the MCP server is enabled (defaults to true if not specified)
func (m *MCPServerV3) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// GetTransport returns the transport protocol, defaulting to "stdio"
func (m *MCPServerV3) GetTransport() string {
	if m == nil || m.Transport == "" {
		return "stdio"
	}
	return m.Transport
}

// ContentTreeV3 represents the scanned content from .ai-rulez/ directory
type ContentTreeV3 struct {
	Rules    []ContentFile        `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context  []ContentFile        `yaml:"context,omitempty" json:"context,omitempty"`
	Skills   []ContentFile        `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents   []ContentFile        `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands []ContentFile        `yaml:"commands,omitempty" json:"commands,omitempty"`
	Domains  map[string]*DomainV3 `yaml:"domains,omitempty" json:"domains,omitempty"`
}

// DomainV3 represents content from a specific domain directory
type DomainV3 struct {
	Name       string                  `yaml:"name" json:"name"`
	Rules      []ContentFile           `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context    []ContentFile           `yaml:"context,omitempty" json:"context,omitempty"`
	Skills     []ContentFile           `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents     []ContentFile           `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands   []ContentFile           `yaml:"commands,omitempty" json:"commands,omitempty"`
	MCPServers map[string]*MCPServerV3 `yaml:"-" json:"-"`
}

// ContentFile represents a single content file with optional frontmatter
type ContentFile struct {
	Name     string      `yaml:"name" json:"name"`
	Path     string      `yaml:"path" json:"path"`
	Content  string      `yaml:"content" json:"content"`
	Metadata *MetadataV3 `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// MetadataV3 represents parsed frontmatter metadata
type MetadataV3 struct {
	Priority string            `yaml:"priority,omitempty" json:"priority,omitempty"`
	Targets  []string          `yaml:"targets,omitempty" json:"targets,omitempty"`
	Aliases  []string          `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Usage    string            `yaml:"usage,omitempty" json:"usage,omitempty"`
	Shortcut string            `yaml:"shortcut,omitempty" json:"shortcut,omitempty"`
	Category string            `yaml:"category,omitempty" json:"category,omitempty"`
	Extra    map[string]string `yaml:",inline" json:",inline"`
}

// GetPriority returns the priority as a Priority type, defaulting to medium
func (m *MetadataV3) GetPriority() Priority {
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
func (m *MetadataV3) HasTargets() bool {
	return m != nil && len(m.Targets) > 0
}

// Helper methods for ConfigV3

// ShouldUpdateGitignore returns whether .gitignore should be updated
func (c *ConfigV3) ShouldUpdateGitignore() bool {
	if c.Gitignore == nil {
		return true
	}
	return *c.Gitignore
}

// GetDefaultProfile returns the default profile name
func (c *ConfigV3) GetDefaultProfile() string {
	return c.Default
}

// GetProfileDomains returns the list of domains for a profile
func (c *ConfigV3) GetProfileDomains(profile string) []string {
	if profile == "" {
		profile = c.Default
	}
	if domains, ok := c.Profiles[profile]; ok {
		return domains
	}
	return nil
}

// HasProfile returns true if the profile exists
func (c *ConfigV3) HasProfile(profile string) bool {
	_, ok := c.Profiles[profile]
	return ok
}

// GetVersion returns the config version
func (c *ConfigV3) GetVersion() string {
	return c.Version
}

// IsV3 returns true if this is a V3 config (version == "3.0")
func (c *ConfigV3) IsV3() bool {
	return c.Version == "3.0"
}

// GetHeaderStyle returns the configured header style ("detailed", "compact", or "minimal")
func (c *ConfigV3) GetHeaderStyle() string {
	if c.Header == nil {
		return "detailed"
	}
	return c.Header.GetHeaderStyle()
}

// GetContentForProfile returns all content for a given profile
func (c *ConfigV3) GetContentForProfile(profile string) (*ContentTreeV3, error) {
	if c.Content == nil {
		return nil, ErrNoContent
	}

	domains := c.GetProfileDomains(profile)

	return &ContentTreeV3{
		Rules:    c.Content.GetRulesForDomains(domains),
		Context:  c.Content.GetContextForDomains(domains),
		Skills:   c.Content.GetSkillsForDomains(domains),
		Agents:   c.Content.GetAgentsForDomains(domains),
		Commands: c.Content.GetCommandsForDomains(domains),
		Domains:  c.Content.Domains, // Include all domains for reference
	}, nil
}

// Helper methods for ContentTreeV3

// GetAllContentFiles returns all content files from the tree
func (t *ContentTreeV3) GetAllContentFiles() []ContentFile {
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
func (t *ContentTreeV3) GetRulesForDomains(domains []string) []ContentFile {
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
func (t *ContentTreeV3) GetContextForDomains(domains []string) []ContentFile {
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
func (t *ContentTreeV3) GetSkillsForDomains(domains []string) []ContentFile {
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
func (t *ContentTreeV3) GetAgentsForDomains(domains []string) []ContentFile {
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
func (t *ContentTreeV3) GetCommandsForDomains(domains []string) []ContentFile {
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
	Name          string   `yaml:"name" json:"name"`
	Source        string   `yaml:"source" json:"source"`                       // Git repo URL OR local path
	Path          string   `yaml:"path,omitempty" json:"path,omitempty"`       // Path within git repo (git only)
	Include       []string `yaml:"include,omitempty" json:"include,omitempty"` // [rules, context, skills, mcp]
	Ref           string   `yaml:"ref,omitempty" json:"ref,omitempty"`         // branch/tag/commit (git only)
	InstallTo     string   `yaml:"install_to,omitempty" json:"install_to,omitempty"`
	MergeStrategy string   `yaml:"merge_strategy,omitempty" json:"merge_strategy,omitempty"`
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
