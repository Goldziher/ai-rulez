package config

import (
	"encoding/json"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers/builtin"
)

// Preset represents either a built-in preset name or a custom preset configuration
type Preset struct {
	// Built-in preset (e.g., "claude", "cursor")
	BuiltIn string `yaml:"-" json:"-" toml:"-"`

	// Custom preset fields
	Name     string     `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Type     PresetType `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
	Path     string     `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Template string     `yaml:"template,omitempty" json:"template,omitempty" toml:"template,omitempty"`

	// Provider references a declarative provider spec (schema/provider.schema.json)
	// relative to the project root. A provider-backed preset has full parity with
	// a built-in preset (root file, skills/agents/commands, frontmatter, MCP
	// sidecars). When set, Type and Path must be empty.
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" toml:"provider,omitempty"`
}

// PresetType defines the type of custom preset output
type PresetType string

const (
	PresetTypeMarkdown  PresetType = "markdown"
	PresetTypeDirectory PresetType = "directory"
	PresetTypeJSON      PresetType = "json"
)

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
	p.Provider = custom.Provider
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
//
// For a built-in name the answer depends on registration: provider-spec presets
// are known only once internal/generator/providers is linked into the binary
// (see AllPresetNames). Import internal/generator before validating presets.
func (p *Preset) IsValid() bool {
	if p.IsBuiltIn() {
		return isValidBuiltInPreset(p.BuiltIn)
	}
	// Provider-backed presets carry a spec reference instead of type/path.
	if p.Provider != "" {
		return p.Name != "" && p.Type == "" && p.Path == ""
	}
	return p.Name != "" && p.Type != "" && p.Path != ""
}

// goPresetNames are the presets implemented in Go (their PresetName constants);
// every other built-in preset is a declarative spec embedded in
// internal/generator/providers/builtin, whose names are read from there.
var goPresetNames = []string{
	string(PresetClaude), string(PresetCursor), string(PresetGemini), string(PresetCopilot), string(PresetDevin),
	string(PresetCline), string(PresetCodex), string(PresetAmp), string(PresetJunie), string(PresetHermes),
	string(PresetOpenCode), string(PresetAntigravity), string(PresetMCP), string(PresetXum), string(PresetPi),
	string(PresetBaz), PresetOKF, PresetLLMsTxt,
}

var (
	builtInPresetsOnce sync.Once
	builtInPresets     map[string]bool
)

// builtInPresetSet is every name accepted as `presets = ["<name>"]`: the Go
// presets plus the embedded provider specs. It is derived from embedded data on
// first use and never changes.
func builtInPresetSet() map[string]bool {
	builtInPresetsOnce.Do(func() {
		builtInPresets = make(map[string]bool, len(goPresetNames)+len(builtin.Names()))
		for _, name := range goPresetNames {
			builtInPresets[name] = true
		}
		for _, name := range builtin.Names() {
			builtInPresets[name] = true
		}
	})
	return builtInPresets
}

func isValidBuiltInPreset(name string) bool { return builtInPresetSet()[name] }
