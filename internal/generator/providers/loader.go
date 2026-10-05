package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
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
	registerSplitRulesDir(spec)

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

// validateLocalFile checks root.local_file: "none", or a slash-separated path
// inside the project that is not the root file itself (writing the local variant
// over the shared root would replace it).
func validateLocalFile(root *RootSpec) error {
	local := root.LocalFile
	if local == "" || local == LocalFileNone {
		return nil
	}
	clean := path.Clean(local)
	switch {
	case !fs.ValidPath(local) || local == "." || strings.ContainsAny(local, `\:`):
		return fmt.Errorf("root.local_file: %q must be %q or a relative, slash-separated path inside the project",
			local, LocalFileNone)
	case strings.EqualFold(clean, path.Clean(root.File)):
		return fmt.Errorf("root.local_file: %q is the root file itself; it must name a separate file", local)
	}
	return nil
}

// specName is the shape of a provider name: it becomes a preset name and appears
// in paths and manifests, so it is allowlisted rather than checked for bad characters.
var specName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// permissionsAndHooksEnabled gates the reserved sidecar kinds. They have no
// renderer yet, so a spec declaring one fails at load time instead of at
// generation; flip it (and add the renderers in sidecars_generic.go) to enable.
const permissionsAndHooksEnabled = false

// validateSpec enforces the closed-set enum constraints documented in
// schema/provider.schema.json. Strict TOML/YAML/JSON decoding catches unknown
// fields; this layer catches unknown enum values and missing required combos.
//
//nolint:gocyclo // Linear validation over a finite enum set — splitting hurts readability.
func validateSpec(s *ProviderSpec) error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !specName.MatchString(s.Name) {
		return fmt.Errorf("name: %q must match %s (lowercase letters, digits and dashes)", s.Name, specName)
	}
	for i, dir := range s.Directories {
		if err := validateRelativeFile(fmt.Sprintf("directories[%d]", i), dir); err != nil {
			return err
		}
	}

	if s.Root != nil {
		if s.Root.File == "" {
			return fmt.Errorf("root.file is required when root is set")
		}
		if err := validateRelativeFile("root.file", s.Root.File); err != nil {
			return err
		}
		if err := validateLocalFile(s.Root); err != nil {
			return err
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
		if out.Dir != "" && !out.Split { // a split rules dir has its own, stricter check
			if err := validateRelativeFile(fmt.Sprintf("outputs[%q].dir", typ), out.Dir); err != nil {
				return err
			}
		}
		if err := validateOutputMode(typ, out); err != nil {
			return err
		}
		if out.Filter != "" && out.Filter != FilterIncludeIfTargetingProvider && out.Filter != FilterPathScoped && out.Filter != FilterPlacementCore {
			return fmt.Errorf("outputs[%q].filter: unknown filter %q", typ, out.Filter)
		}
		if err := validateSplitFields(typ, out, rootSections(s)); err != nil {
			return err
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
		if err := validateSidecar(i, sidecar); err != nil {
			return err
		}
	}

	return validateGlobal(s.Global)
}

// validateOutputMode checks outputs.<type>.mode and the fields that belong to a
// mode: aggregate (checks only) takes a file and header and no per-item layout.
func validateOutputMode(typ string, out *OutputSpec) error {
	switch out.Mode {
	case OutputModePerItemFile:
		if out.File != "" || out.Header != "" {
			return fmt.Errorf("outputs[%q]: file and header are only valid with mode %q", typ, OutputModeAggregate)
		}
		if fm := out.Frontmatter; fm != nil && fm.JoinLists && !fm.Tools && !fm.Skills {
			return fmt.Errorf("outputs[%q].frontmatter.join_lists needs tools or skills to be true", typ)
		}
	case OutputModeAggregate:
		if typ != OutputTypeChecks {
			return fmt.Errorf("outputs[%q].mode: %q is only valid on outputs.%s", typ, out.Mode, OutputTypeChecks)
		}
		if err := validateRelativeFile(fmt.Sprintf("outputs[%q].file", typ), out.File); err != nil {
			return err
		}
		if out.Dir != "" || out.Filename != "" || out.Resources || out.Body != nil || out.Frontmatter != nil {
			return fmt.Errorf("outputs[%q]: mode %q takes only file and header", typ, OutputModeAggregate)
		}
	default:
		return fmt.Errorf("outputs[%q].mode: unknown mode %q", typ, out.Mode)
	}
	return nil
}

// validateRelativeFile checks a slash-separated path that stays inside its root.
func validateRelativeFile(field, p string) error {
	if p == "" {
		return fmt.Errorf("%s is required", field)
	}
	if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, `\:`) {
		return fmt.Errorf("%s: %q must be a relative, slash-separated path without \"..\"", field, p)
	}
	return nil
}

// validateSidecar checks one sidecars[] entry: the closed enums, and that the
// generic-only fields (format, key, dialect) appear only on a generic kind.
func validateSidecar(i int, sc *SidecarSpec) error {
	if !isValidSidecarKind(sc.Kind) {
		return fmt.Errorf("sidecars[%d].kind: unknown kind %q", i, sc.Kind)
	}
	if sc.EmitWhen != "" && !isValidPredicate(sc.EmitWhen) {
		return fmt.Errorf("sidecars[%d].emit_when: unknown predicate %q", i, sc.EmitWhen)
	}
	if err := validateRelativeFile(fmt.Sprintf("sidecars[%d].path", i), sc.Path); err != nil {
		return err
	}
	if !permissionsAndHooksEnabled && (sc.Kind == SidecarPermissions || sc.Kind == SidecarHooks) {
		return fmt.Errorf("sidecars[%d].kind: %q is not yet supported", i, sc.Kind)
	}
	if sc.GlobalPath != "" {
		if err := validateRelativeFile(fmt.Sprintf("sidecars[%d].global_path", i), sc.GlobalPath); err != nil {
			return err
		}
	}
	if sc.GlobalMCPPath != "" {
		if err := validateRelativeFile(fmt.Sprintf("sidecars[%d].global_mcp_path", i), sc.GlobalMCPPath); err != nil {
			return err
		}
		if !IsMCPSidecarKind(sc.Kind) {
			return fmt.Errorf("sidecars[%d].global_mcp_path is only valid on a sidecar that holds MCP servers", i)
		}
	}
	if !isGenericSidecarKind(sc.Kind) {
		if sc.Format != "" || len(sc.Key) > 0 || sc.Dialect != "" || sc.Elements != nil {
			return fmt.Errorf("sidecars[%d]: format, key and dialect are only valid on the generic kinds (%s, %s, %s)",
				i, SidecarMCP, SidecarPermissions, SidecarHooks)
		}
		return nil
	}
	return validateGenericSidecar(i, sc)
}

// validateGenericSidecar checks the format, key and dialect of a generic sidecar.
func validateGenericSidecar(i int, sc *SidecarSpec) error {
	if sc.Format != "" && !isDocFormat(sc.Format) {
		return fmt.Errorf("sidecars[%d].format: unknown format %q (want json, jsonc, toml or yaml)", i, sc.Format)
	}
	if sc.DocFormat() == "" {
		return fmt.Errorf("sidecars[%d].format is required: cannot infer it from the extension of %q", i, sc.Path)
	}
	for _, segment := range sc.Key {
		if segment == "" {
			return fmt.Errorf("sidecars[%d].key: segments must not be empty", i)
		}
	}
	if sc.Dialect != "" && sc.Kind != SidecarMCP {
		return fmt.Errorf("sidecars[%d].dialect is only valid on kind %q", i, SidecarMCP)
	}
	if sc.Kind == SidecarMCP {
		if _, err := mcpDialectFor(sc.Dialect); err != nil {
			return fmt.Errorf("sidecars[%d].dialect: %w", i, err)
		}
		if sc.EmitWhen == "" {
			// No servers, no document: an mcp sidecar is not an always-on file,
			// unless it also owns array elements, which need no server.
			sc.EmitWhen = PredicateHasMCPServers
			if sc.Elements != nil {
				sc.EmitWhen = PredicateAlways
			}
		}
	}
	return validateElements(i, sc)
}

// validateGlobal checks the [global] block: every path is a relative,
// slash-separated path under the user's home, and home_env comes with home_dir.
func validateGlobal(g *GlobalSpec) error {
	if g == nil {
		return nil
	}
	if (g.HomeEnv == "") != (g.HomeDir == "") {
		return fmt.Errorf("global.home_env and global.home_dir must be set together")
	}
	if g.HomeEnv != "" && !isEnvName(g.HomeEnv) {
		return fmt.Errorf("global.home_env: %q is not a valid environment variable name", g.HomeEnv)
	}
	if g.HomeDir != "" {
		if err := validateRelativeFile("global.home_dir", g.HomeDir); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{
		"root_file": g.RootFile, "skills_dir": g.SkillsDir, "agents_dir": g.AgentsDir,
		"commands_dir": g.CommandsDir, "rules_dir": g.RulesDir,
	} {
		if value == "" {
			continue
		}
		if err := validateRelativeFile("global."+field, value); err != nil {
			return err
		}
		if g.HomeDir != "" && !isUnder(value, g.HomeDir) {
			// A path outside home_dir would not follow the variable.
			return fmt.Errorf("global.%s: %q must be inside global.home_dir %q", field, value, g.HomeDir)
		}
	}
	return nil
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isEnvName(name string) bool { return envName.MatchString(name) }

// isUnder reports whether p is dir or lies inside it (slash-separated, relative).
func isUnder(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

func validateSplitEnums(typ string, out *OutputSpec) error {
	if out.InlineFilter != "" && out.InlineFilter != InlineFilterPathScoped {
		return fmt.Errorf("outputs[%q].inline_filter: unknown value %q", typ, out.InlineFilter)
	}
	if out.Dialect != "" && !rulefiles.IsDialect(out.Dialect) {
		return fmt.Errorf("outputs[%q].dialect: unknown dialect %q", typ, out.Dialect)
	}
	return nil
}

// validateSplitFields checks the split/inline_filter/dialect trio of a rules
// output.
func validateSplitFields(typ string, out *OutputSpec, rootSections []string) error {
	if out.InlineUnscoped && (typ != OutputTypeRules || !out.Split) {
		return fmt.Errorf("outputs[%q].inline_unscoped is only valid on outputs.rules with split = true", typ)
	}
	if !out.Split && out.InlineFilter == "" && out.Dialect == "" && out.Activation == nil && !out.AlwaysFiles {
		return nil
	}
	if out.Activation != nil && out.Dialect == "" {
		out.Dialect = DialectMapped // an activation block is the mapped dialect
	}
	if typ != OutputTypeRules {
		return fmt.Errorf("outputs[%q]: split, inline_filter, dialect and activation are only valid on outputs.rules", typ)
	}
	if err := validateActivation(typ, out); err != nil {
		return err
	}
	if err := validateSplitEnums(typ, out); err != nil {
		return err
	}
	if out.Dialect != "" && (out.Body != nil || out.Frontmatter != nil) {
		return fmt.Errorf("outputs[%q]: dialect cannot be combined with body or frontmatter blocks", typ)
	}
	if !out.Split {
		return fmt.Errorf("outputs[%q]: inline_filter, dialect, activation and always_files require split = true", typ)
	}
	if !out.AlwaysFiles && !slices.Contains(rootSections, SectionRootRulesInline) {
		return fmt.Errorf("outputs[%q]: split requires root.sections to include %q", typ, SectionRootRulesInline)
	}
	if out.Filter != "" {
		return fmt.Errorf("outputs[%q]: split cannot be combined with filter %q", typ, out.Filter)
	}
	if out.Dialect == "" {
		return fmt.Errorf("outputs[%q].dialect is required when split = true", typ)
	}
	if err := validateSplitDir(typ, out.Dir); err != nil {
		return err
	}
	return validateSplitFilename(typ, out.Filename)
}

// registerSplitRulesDir marks the folder of a split rules output as a shared
// rules folder, so it gets the protections the built-in folders have: the
// overwrite guard for hand-written files, hashes in the banner, and per-file
// gitignore entries. Registering on load is the one point every spec passes
// through, built-in or custom, before any path check can run.
func registerSplitRulesDir(s *ProviderSpec) {
	if out := s.Outputs[OutputTypeRules]; out != nil && out.Split {
		config.RegisterRulesDir(out.Dir)
	}
}

// driveLetter matches a Windows drive prefix such as "C:".
var driveLetter = regexp.MustCompile(`^[A-Za-z]:`)

// validateActivation checks the dialect/activation pairing and the shape of the
// activation tables.
func validateActivation(typ string, out *OutputSpec) error {
	if out.Dialect == DialectMapped && out.Activation == nil {
		return fmt.Errorf("outputs[%q].dialect %q requires an [outputs.%s.activation] block", typ, DialectMapped, typ)
	}
	a := out.Activation
	if a == nil {
		return nil
	}
	if out.Dialect != DialectMapped {
		return fmt.Errorf("outputs[%q]: an activation block needs dialect %q, got %q", typ, DialectMapped, out.Dialect)
	}
	if a.Format != "" && a.Format != ActivationFormatYAML && a.Format != ActivationFormatLines {
		return fmt.Errorf("outputs[%q].activation.format: unknown format %q", typ, a.Format)
	}
	for mode, table := range map[string]map[string]any{"always": a.Always, "glob": a.Glob, "auto": a.Auto, "manual": a.Manual} {
		for key, value := range table {
			if !rulefiles.ValidActivationKey(key) {
				return fmt.Errorf("outputs[%q].activation.%s: key %q must match [A-Za-z_][A-Za-z0-9_-]*", typ, mode, key)
			}
			if err := validateActivationValue(value); err != nil {
				return fmt.Errorf("outputs[%q].activation.%s.%s: %w", typ, mode, key, err)
			}
		}
	}
	return nil
}

// validateActivationValue allows the scalars a frontmatter value can be.
func validateActivationValue(value any) error {
	switch v := value.(type) {
	case string:
		if rulefiles.ActivationValueHasEmbeddedList(v) {
			return fmt.Errorf("{globs_list} must be the whole value, not part of %q", v)
		}
		return nil
	case bool, int, int64, float64:
		return nil
	}
	return fmt.Errorf("must be a string, boolean or number, got %T", value)
}

func validateSplitDir(typ, dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("outputs[%q].dir is required when split = true", typ)
	}
	slashed := strings.ReplaceAll(dir, "\\", "/")
	clean := path.Clean(slashed)
	if strings.HasPrefix(slashed, "/") || driveLetter.MatchString(slashed) || path.IsAbs(clean) ||
		clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("outputs[%q].dir: split needs a relative path inside the project, got %q", typ, dir)
	}
	return nil
}

func validateSplitFilename(typ, filename string) error {
	if !strings.HasPrefix(filename, "{id}") || strings.Contains(filename, "/") {
		return fmt.Errorf("outputs[%q].filename: split needs a flat {id}<ext> template, got %q", typ, filename)
	}
	return nil
}

func rootSections(s *ProviderSpec) []string {
	if s.Root == nil {
		return nil
	}
	return s.Root.Sections
}

func isValidOutputType(typ string) bool {
	switch typ {
	case "rules", "context", OutputTypeSkills, OutputTypeAgents, OutputTypeCommands, OutputTypeChecks:
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
	case PredicateAlways, PredicateHasMCPServersOrPluginSettings, PredicateHasMCPServers, PredicateHasMCPJSONEntries, PredicateHasPlugins, PredicateHasResolvedEffort, PredicateHasResolvedEffortOrMCPServers:
		return true
	}
	return false
}

func isValidSidecarKind(k string) bool {
	switch k {
	case SidecarClaudeSettingsJSON, SidecarClaudePluginsJSON,
		SidecarMCPJSON, SidecarAmpSettingsJSON, SidecarPiMCPJSON,
		SidecarMCP, SidecarPermissions, SidecarHooks:
		return true
	}
	return false
}
