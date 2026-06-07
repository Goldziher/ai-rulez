package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// LoadFormat hints which decoder to use. Auto-detection falls back to TOML
// when the format is unspecified — TOML is the canonical builtin format and
// the closest match to `.ai-rulez/config.toml`.
type LoadFormat string

const (
	FormatAuto LoadFormat = ""
	FormatTOML LoadFormat = "toml"
	FormatYAML LoadFormat = "yaml"
	FormatJSON LoadFormat = "json"
)

// LoadProviderSpec decodes a provider spec from raw bytes. Format is detected
// from the filename extension when format is FormatAuto; pass an explicit
// format when no filename is available.
func LoadProviderSpec(raw []byte, filename string, format LoadFormat) (*ProviderSpec, error) {
	spec := &ProviderSpec{}
	resolved := resolveFormat(filename, format)
	switch resolved {
	case FormatTOML:
		dec := toml.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(spec); err != nil {
			return nil, oops.With("filename", filename).Wrapf(err, "decode provider TOML")
		}
	case FormatYAML:
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(spec); err != nil {
			return nil, oops.With("filename", filename).Wrapf(err, "decode provider YAML")
		}
	case FormatJSON:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(spec); err != nil {
			return nil, oops.With("filename", filename).Wrapf(err, "decode provider JSON")
		}
	default:
		return nil, oops.With("filename", filename).Errorf("unsupported provider format %q", format)
	}

	if err := validateSpec(spec); err != nil {
		return nil, oops.With("filename", filename).Wrapf(err, "invalid provider spec")
	}

	return spec, nil
}

func resolveFormat(filename string, format LoadFormat) LoadFormat {
	if format != FormatAuto {
		return format
	}
	switch {
	case strings.HasSuffix(filename, ".toml"):
		return FormatTOML
	case strings.HasSuffix(filename, ".yaml"), strings.HasSuffix(filename, ".yml"):
		return FormatYAML
	case strings.HasSuffix(filename, ".json"):
		return FormatJSON
	default:
		return FormatTOML
	}
}

// validateSpec enforces the closed-set enum constraints documented in
// schema/provider.schema.json. Strict TOML/YAML/JSON decoding catches unknown
// fields; this layer catches unknown enum values and missing required combos.
//
//nolint:gocyclo // Linear validation over a finite enum set — splitting hurts readability.
func validateSpec(s *ProviderSpec) error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}

	if s.Root != nil {
		if s.Root.File == "" {
			return fmt.Errorf("root.file is required when root is set")
		}
		for _, section := range s.Root.Sections {
			if !isValidRootSection(section) {
				return fmt.Errorf("root.sections[]: unknown section %q", section)
			}
		}
	}

	for typ, out := range s.Outputs {
		if !isValidOutputType(typ) {
			return fmt.Errorf("outputs[%q]: unknown content type", typ)
		}
		if out.Mode != OutputModePerItemFile {
			return fmt.Errorf("outputs[%q].mode: unknown mode %q", typ, out.Mode)
		}
		if out.Filter != "" && out.Filter != FilterIncludeIfTargetingProvider {
			return fmt.Errorf("outputs[%q].filter: unknown filter %q", typ, out.Filter)
		}
		if out.Body != nil {
			for _, section := range out.Body.Sections {
				if !isValidBodySection(section) {
					return fmt.Errorf("outputs[%q].body.sections[]: unknown section %q", typ, section)
				}
			}
		}
	}

	if s.EffortMap != nil && s.EffortMap.Style != EffortMapStyleString {
		return fmt.Errorf("effort_map.style: unknown style %q", s.EffortMap.Style)
	}

	for i, sidecar := range s.Sidecars {
		if !isValidSidecarKind(sidecar.Kind) {
			return fmt.Errorf("sidecars[%d].kind: unknown kind %q", i, sidecar.Kind)
		}
		if sidecar.EmitWhen != "" && !isValidPredicate(sidecar.EmitWhen) {
			return fmt.Errorf("sidecars[%d].emit_when: unknown predicate %q", i, sidecar.EmitWhen)
		}
		if sidecar.Path == "" {
			return fmt.Errorf("sidecars[%d].path is required", i)
		}
	}

	return nil
}

func isValidOutputType(typ string) bool {
	switch typ {
	case "rules", "context", OutputTypeSkills, OutputTypeAgents, OutputTypeCommands:
		return true
	}
	return false
}

func isValidRootSection(section string) bool {
	switch section {
	case SectionRootHeader, SectionRootTitle, SectionRootDescription,
		SectionRootRulesInline, SectionRootContextInline, SectionRootAgentsDelegation:
		return true
	}
	return false
}

func isValidBodySection(section string) bool {
	switch section {
	case SectionBodyFrontmatter, SectionBodyContent, SectionBodyResourceIndex,
		SectionBodyTargetedRules, SectionBodyTargetedContext:
		return true
	}
	return false
}

func isValidPredicate(p string) bool {
	switch p {
	case PredicateAlways, PredicateHasMCPServers, PredicateHasPlugins:
		return true
	}
	return false
}

func isValidSidecarKind(k string) bool {
	switch k {
	case SidecarClaudeSettingsJSON, SidecarClaudePluginsJSON:
		return true
	}
	return false
}
