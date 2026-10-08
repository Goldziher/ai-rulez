package config

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
)

// ContentTree represents the scanned content from .ai-rulez/ directory
type ContentTree struct {
	Rules    []ContentFile      `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context  []ContentFile      `yaml:"context,omitempty" json:"context,omitempty"`
	Skills   []ContentFile      `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents   []ContentFile      `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands []ContentFile      `yaml:"commands,omitempty" json:"commands,omitempty"`
	Checks   []ContentFile      `yaml:"checks,omitempty" json:"checks,omitempty"`
	Domains  map[string]*Domain `yaml:"domains,omitempty" json:"domains,omitempty"`
	// ImportedVerifiers are the verifier declaration files of includes; the
	// project's own are read from its configuration directory.
	ImportedVerifiers []ImportedVerifierFile `yaml:"-" json:"-"`
}

// Domain represents content from a specific domain directory
type Domain struct {
	Name          string        `yaml:"name" json:"name"`
	Rules         []ContentFile `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context       []ContentFile `yaml:"context,omitempty" json:"context,omitempty"`
	Skills        []ContentFile `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents        []ContentFile `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands      []ContentFile `yaml:"commands,omitempty" json:"commands,omitempty"`
	Checks        []ContentFile `yaml:"checks,omitempty" json:"checks,omitempty"`
	Builtin       bool          `yaml:"-" json:"-"` // true if loaded from builtins
	BuiltinScoped bool          `yaml:"-" json:"-"` // true if a builtin loaded only for the profiles that name it (builtin:<name>)
	FromInclude   bool          `yaml:"-" json:"-"` // true if loaded from an external include
}

// ContentFile represents a single content file with optional frontmatter
type ContentFile struct {
	Name     string    `yaml:"name" json:"name"`
	Path     string    `yaml:"path" json:"path"`
	Content  string    `yaml:"content" json:"content"`
	Metadata *Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// Profiles scopes this file to the named profiles (used by installed
	// skills). Empty means every profile. Runtime-only; not serialized.
	Profiles []string `yaml:"-" json:"-"`

	// Resources holds skill supporting files (references/, scripts/, assets/)
	// loaded alongside SKILL.md. Always empty for non-skill content.
	Resources []SkillResource `yaml:"-" json:"-"`

	// Verifiers holds the verifier declaration files an installed skill ships in
	// its verifiers/ directory. They are data: see ImportedVerifierFile.
	Verifiers []ImportedVerifierFile `yaml:"-" json:"-"`

	// MalformedFrontmatter is true when the source file contained a delimited
	// YAML frontmatter block (---...---) but its content was unparseable.
	// It is set during loading and used by Config.Validate to fail fast.
	MalformedFrontmatter bool `yaml:"-" json:"-"`
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
	Priority string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Targets  []string `yaml:"targets,omitempty" json:"targets,omitempty"`
	Aliases  []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Tools    []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills   []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Keywords []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Usage    string   `yaml:"usage,omitempty" json:"usage,omitempty"`
	Shortcut string   `yaml:"shortcut,omitempty" json:"shortcut,omitempty"`
	Category string   `yaml:"category,omitempty" json:"category,omitempty"`
	Effort   string   `yaml:"effort,omitempty" json:"effort,omitempty"`
	// Activation selects when a rule or context file applies: always, glob,
	// auto, or manual. See ResolveActivation for precedence and defaults.
	Activation string `yaml:"activation,omitempty" json:"activation,omitempty"`
	// Globs and Paths declare the files a rule applies to. They are two
	// spellings of the same idea (Cursor calls them globs, Claude Code calls
	// them paths); either populates the same path-scope used by presets.
	Globs []string          `yaml:"globs,omitempty" json:"globs,omitempty"`
	Paths []string          `yaml:"paths,omitempty" json:"paths,omitempty"`
	Extra map[string]string `yaml:",inline" json:",inline"`

	// OKFType and OKFTitle are the `type` and `title` an OKF concept declares.
	// They carry nothing for ai-rulez; the loader keeps them so that an export
	// of an OKF-shaped tree reproduces the type and title it was written with.
	OKFType  string `yaml:"-" json:"-"`
	OKFTitle string `yaml:"-" json:"-"`

	// extraNodes holds the original, typed YAML value of every Extra key (nested
	// maps, lists, booleans, numbers, dates). Extra keeps a string form for the
	// lookups that only need text; generators emit the typed value through
	// TypedExtra so the frontmatter round-trips without Go syntax.
	extraNodes map[string]*yaml.Node
}

// PathScope returns the file globs a rule declares, from either `globs` or
// `paths` frontmatter. Empty means the rule is not path-scoped.
func (m *Metadata) PathScope() []string {
	if m == nil {
		return nil
	}
	if len(m.Globs) > 0 {
		return NormalizeGlobs(m.Globs)
	}
	return NormalizeGlobs(m.Paths)
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

// GetContentForProfile returns all content for a given profile.
// Root-level slices contain only root content. Domains are placed in the
// Domains map so that preset generators can combine them via
// combineContentFiles / getAllDomain* helpers without duplication.
func (c *Config) GetContentForProfile(profile string) (*ContentTree, error) {
	return c.SelectContentForProfile(c.Content, profile)
}

// SelectContentForProfile applies a profile's domain selection to a content tree:
// the root content plus the domains the profile names, the globally active
// builtins and every domain that came from an include. It is the single
// implementation of that selection, used for the shared tree and for the
// machine-local one (.ai-rulez/local/) alike.
func (c *Config) SelectContentForProfile(content *ContentTree, profile string) (*ContentTree, error) {
	if content == nil {
		return nil, ErrNoContent
	}

	// Installed skills may be scoped to profiles; drop the ones not active.
	activeProfile := profile
	if activeProfile == "" {
		activeProfile = c.Default
	}
	return c.selectContent(content, c.GetProfileDomains(profile), activeProfile)
}

// selectContent is the shared body of profile and role selection: the root
// content, the globally active builtins, every included domain and the listed
// domains. Installed skills scoped to profiles are filtered by activeProfile; an
// empty value keeps them all.
func (c *Config) selectContent(content *ContentTree, profileDomains []string, activeProfile string) (*ContentTree, error) {
	rootSkills := FilterContentFilesByProfile(content.Skills, activeProfile)

	// Build filtered domains map: profile-listed domains + global builtins + FromInclude
	activeDomains := make(map[string]*Domain)

	// First pass: include globally-active builtins and every FromInclude domain
	// unconditionally. A builtin loaded only because a profile named it is
	// excluded here and re-added by the second pass for that profile alone.
	for name, domain := range content.Domains {
		if domain.FromInclude || (domain.Builtin && !domain.BuiltinScoped) {
			activeDomains[name] = domain
		}
	}

	// Second pass: add profile-specified domains (may overlap with FromInclude).
	// A "builtin:<name>" element selects a profile-scoped builtin; a bare name
	// selects an on-disk or include domain and never widens a scoped builtin
	// into a profile that did not ask for it.
	for _, ref := range profileDomains {
		name := builtins.TrimRefPrefix(ref)
		domain, ok := content.Domains[name]
		if !ok {
			continue
		}
		if domain.BuiltinScoped && !builtins.HasRefPrefix(ref) {
			continue
		}
		activeDomains[name] = domain
	}

	return &ContentTree{
		Rules:    content.Rules,
		Context:  content.Context,
		Skills:   rootSkills,
		Agents:   content.Agents,
		Commands: content.Commands,
		Checks:   content.Checks,
		Domains:  activeDomains,
	}, nil
}

// Helper methods for ContentTree

// IsEmpty reports whether the tree carries no content of any kind.
func (t *ContentTree) IsEmpty() bool {
	if t == nil {
		return true
	}
	if len(t.Rules) > 0 || len(t.Context) > 0 || len(t.Skills) > 0 ||
		len(t.Agents) > 0 || len(t.Commands) > 0 || len(t.Checks) > 0 {
		return false
	}
	for _, domain := range t.Domains {
		if domain.hasContent() {
			return false
		}
	}
	return true
}

// hasContent reports whether the domain carries content of any kind. A nil
// domain carries none.
func (d *Domain) hasContent() bool {
	return d != nil && (len(d.Rules) > 0 || len(d.Context) > 0 ||
		len(d.Skills) > 0 || len(d.Agents) > 0 || len(d.Commands) > 0 || len(d.Checks) > 0)
}

// GetAllContentFiles returns all content files from the tree
func (t *ContentTree) GetAllContentFiles() []ContentFile {
	var files []ContentFile
	files = append(files, t.Rules...)
	files = append(files, t.Context...)
	files = append(files, t.Skills...)
	files = append(files, t.Agents...)
	files = append(files, t.Commands...)
	files = append(files, t.Checks...)
	for _, domain := range t.Domains {
		files = append(files, domain.Rules...)
		files = append(files, domain.Context...)
		files = append(files, domain.Skills...)
		files = append(files, domain.Agents...)
		files = append(files, domain.Commands...)
		files = append(files, domain.Checks...)
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

// GetChecksForDomains returns checks for specified domains (including root)
func (t *ContentTree) GetChecksForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Checks))
	copy(files, t.Checks)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Checks...)
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
	return ext == markdownExt || ext == ".markdown"
}
