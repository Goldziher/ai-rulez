package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/builtins"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// Validate validates a configuration
func (c *Config) Validate() error {
	if err := c.validateVersion(); err != nil {
		return err
	}

	if err := c.validateName(); err != nil {
		return err
	}

	if err := c.validatePresets(); err != nil {
		return err
	}

	if err := c.validateCodexSkillsDir(); err != nil {
		return err
	}

	if err := c.validateProfiles(); err != nil {
		return err
	}

	if err := c.validateRoles(); err != nil {
		return err
	}

	if err := c.validateLock(); err != nil {
		return err
	}

	if err := c.validateSkillDescriptions(); err != nil {
		return err
	}

	if err := c.validateFrontmatter(); err != nil {
		return err
	}

	if err := c.validateInstalledSkills(); err != nil {
		return err
	}

	if err := c.validateDynamicSkills(); err != nil {
		return err
	}

	if err := c.validateDefaults(); err != nil {
		return err
	}

	if err := c.validateMCP(); err != nil {
		return err
	}

	if err := c.validateAgentEffort(); err != nil {
		return err
	}

	if err := c.validatePluginAuthoring(); err != nil {
		return err
	}

	if err := c.validateMarketplaceAuthoring(); err != nil {
		return err
	}

	if err := c.validateOutputCollisions(); err != nil {
		return err
	}

	// Warn about missing domain references (non-fatal)
	c.warnMissingDomainReferences()

	return nil
}

func (c *Config) validateOutputCollisions() error {
	if err := c.validateDuplicateOutputIDs(); err != nil {
		return err
	}
	return c.validateOutputNamespaceCollisions()
}

func (c *Config) validateCodexSkillsDir() error {
	if strings.TrimSpace(c.CodexSkillsDir) == "" {
		return nil
	}
	return ValidateOutputSubdir("codex_skills_dir", c.CodexSkillsDirOrDefault())
}

// validEffortValues lists the reasoning-effort values accepted by Claude Code
// subagent frontmatter. Lowercase only. Empty string means "not set" and is
// always valid; this list governs explicit values only.
// effortXHigh is the "xhigh" reasoning-effort level (between high and max).
const effortXHigh = "xhigh"

var validEffortValues = []string{"low", "medium", string(PriorityHigh), effortXHigh, "max", "inherit"}

// validateEffort returns nil for the empty string or any value in validEffortValues.
// Returns an oops-wrapped error otherwise. The fieldPath is embedded in the error
// for actionable messages (e.g., "defaults.effort", "agent[my-agent].effort").
func validateEffort(value, fieldPath string) error {
	if value == "" {
		return nil
	}
	for _, v := range validEffortValues {
		if value == v {
			return nil
		}
	}
	return oops.
		With("field", fieldPath).
		With("actual_value", value).
		With("valid_values", validEffortValues).
		Hint("Use one of: low, medium, high, xhigh, max, inherit (lowercase). Available levels depend on the model.").
		Errorf("invalid effort value %q at %s", value, fieldPath)
}

func (c *Config) validateDefaults() error {
	if err := c.validateHeaderHashes(); err != nil {
		return err
	}
	if err := c.validateRules(); err != nil {
		return err
	}
	if err := c.validateScopes(); err != nil {
		return err
	}
	if c.Defaults == nil {
		return nil
	}
	if err := validateEffort(c.Defaults.Effort, "defaults.effort"); err != nil {
		return err
	}
	for preset, value := range c.Defaults.EffortByPreset {
		if !isValidBuiltInPreset(preset) {
			return oops.
				With("field", "defaults.effort_by_preset").
				With("preset", preset).
				With("available_presets", getBuiltInPresetNames()).
				Hint("Use a built-in preset name as the key (e.g. claude, codex, devin).").
				Errorf("unknown preset %q in defaults.effort_by_preset", preset)
		}
		fieldPath := fmt.Sprintf("defaults.effort_by_preset.%s", preset)
		if err := validateEffort(value, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

// validateScopes checks that every [[scopes]] path is a plain relative subdirectory.
func (c *Config) validateScopes() error {
	for _, scope := range c.Scopes {
		if err := ValidateScopePath(scope.Path); err != nil {
			return oops.With("field", "scopes.path").With("scope_name", scope.Name).Wrap(err)
		}
	}
	return nil
}

// validateRules checks rules.mode and rules.mode_by_preset values and preset keys.
func (c *Config) validateRules() error {
	if c.Rules == nil {
		return nil
	}
	if err := validateRulesMode(c.Rules.Mode, "rules.mode"); err != nil {
		return err
	}
	if err := validateBazScoped(c.Rules.BazScoped); err != nil {
		return err
	}
	for preset, value := range c.Rules.ModeByPreset {
		if !c.isKnownPreset(preset) {
			return oops.
				With("field", "rules.mode_by_preset").
				With("preset", preset).
				With("available_presets", getBuiltInPresetNames()).
				Hint("Use a built-in preset name or a custom/provider preset name from `presets` as the key (e.g. claude, copilot, devin).").
				Errorf("unknown preset %q in rules.mode_by_preset", preset)
		}
		if value == "" {
			return oops.
				With("field", "rules.mode_by_preset."+preset).
				With("valid_values", validRulesModes).
				Hint("Use one of: split, inline.").
				Errorf("empty rules mode for preset %q in rules.mode_by_preset", preset)
		}
		if err := validateRulesMode(value, "rules.mode_by_preset."+preset); err != nil {
			return err
		}
	}
	return nil
}

func validateBazScoped(value string) error {
	if value == "" || slices.Contains(validBazScoped, value) {
		return nil
	}
	return oops.
		With("field", "rules.baz_scoped").
		With("actual_value", value).
		With("valid_values", validBazScoped).
		Hint("Use one of: nested, root (lowercase).").
		Errorf("invalid baz_scoped value %q at rules.baz_scoped", value)
}

func validateRulesMode(value, fieldPath string) error {
	if value == "" {
		return nil
	}
	for _, v := range validRulesModes {
		if value == v {
			return nil
		}
	}
	return oops.
		With("field", fieldPath).
		With("actual_value", value).
		With("valid_values", validRulesModes).
		Hint("Use one of: split, inline (lowercase).").
		Errorf("invalid rules mode %q at %s", value, fieldPath)
}

// isKnownPreset reports whether name is a built-in preset or the name of a
// custom/provider preset declared in the config.
func (c *Config) isKnownPreset(name string) bool {
	if isValidBuiltInPreset(name) {
		return true
	}
	for i := range c.Presets {
		if c.Presets[i].Name == name {
			return true
		}
	}
	return false
}

func (c *Config) validateAgentEffort() error {
	if c.Content == nil {
		return nil
	}

	if err := validateAgentEffortSlice(c.Content.Agents, "root"); err != nil {
		return err
	}

	for domainName, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		if err := validateAgentEffortSlice(domain.Agents, "domain "+domainName); err != nil {
			return err
		}
	}

	return nil
}

func validateAgentEffortSlice(agents []ContentFile, scope string) error {
	for _, agent := range agents {
		if agent.Metadata == nil {
			continue
		}
		fieldPath := fmt.Sprintf("%s agent[%s].effort", scope, agent.Name)
		if err := validateEffort(agent.Metadata.Effort, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

// validateFrontmatter runs the content frontmatter checks.
func (c *Config) validateFrontmatter() error {
	if err := c.validateMalformedFrontmatter(); err != nil {
		return err
	}
	return c.validateRuleActivation()
}

// validateMalformedFrontmatter fails validation when any content file carried a
// delimited frontmatter block whose YAML could not be parsed. A skill or agent
// with malformed frontmatter is silently invisible downstream (no name, no
// description, no tools), so `validate` — the CI gate — must exit non-zero
// rather than warn-and-continue (#175).
func (c *Config) validateMalformedFrontmatter() error {
	if c.Content == nil {
		return nil
	}

	var bad []string
	visit := func(files []ContentFile) {
		for _, f := range files {
			if f.MalformedFrontmatter {
				bad = append(bad, f.Path)
			}
		}
	}

	visit(c.Content.Rules)
	visit(c.Content.Context)
	visit(c.Content.Skills)
	visit(c.Content.Agents)
	visit(c.Content.Commands)
	for _, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		visit(domain.Rules)
		visit(domain.Context)
		visit(domain.Skills)
		visit(domain.Agents)
		visit(domain.Commands)
	}

	if len(bad) == 0 {
		return nil
	}

	return oops.
		With("paths", bad).
		With("hint", "Check for unquoted values containing ': ' in the frontmatter block, e.g. description: key: value").
		Errorf("malformed YAML frontmatter in %d file(s): %q", len(bad), bad)
}

// validateSkillDescriptions checks that every skill carries a description.
func (c *Config) validateSkillDescriptions() error {
	if c.Content == nil {
		return nil
	}

	if err := validateSkillSlice(c.Content.Skills, "root"); err != nil {
		return err
	}

	for domainName, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		if err := validateSkillSlice(domain.Skills, "domain "+domainName); err != nil {
			return err
		}
	}

	return nil
}

func validateSkillSlice(skills []ContentFile, scope string) error {
	for _, skill := range skills {
		if SkillDescription(skill.Metadata) != "" {
			continue
		}

		skillID := SkillID(skill)
		logger.Warn("skill missing 'description' field in frontmatter — using skill name as fallback",
			"scope", scope, "skill", skillID, "path", skill.Path)
	}

	return nil
}

// validateVersion checks that version is "3.0" or "4.0"
func (c *Config) validateVersion() error {
	if c.Version != ConfigVersionV3 && c.Version != ConfigVersionV4 {
		return oops.
			With("field", "version").
			With("actual_version", c.Version).
			Hint("Set version to \"3.0\" or \"4.0\" in your config file").
			Errorf("invalid version: expected \"3.0\" or \"4.0\", got %q", c.Version)
	}
	return nil
}

// validateName checks that name is non-empty
func (c *Config) validateName() error {
	if c.Name == "" {
		return oops.
			With("field", "name").
			Hint("Add a 'name' field to your config file\nExample: name: my-project").
			Errorf("required field 'name' is missing")
	}
	return nil
}

// validatePresets validates that at least one preset exists and all are valid
func (c *Config) validatePresets() error {
	// A config that only authors a plugin bundle ([plugin] block) or a monorepo
	// marketplace ([marketplace] with members) need not declare any presets: the
	// plugin generator renders runtime manifests directly rather than through the
	// preset pipeline.
	if len(c.Presets) == 0 {
		if c.HasPluginAuthoring() {
			return nil
		}
		return oops.
			With("field", "presets").
			Hint("Add at least one preset to your config file\nExample: presets: [claude]\nAvailable built-in presets: " + strings.Join(AllPresetNames(), ", ")).
			Errorf("at least one preset is required")
	}

	for i := range c.Presets {
		if err := c.validatePreset(&c.Presets[i], i); err != nil {
			return err
		}
	}

	return nil
}

// validatePreset validates a single preset
func (c *Config) validatePreset(preset *Preset, index int) error {
	// Check if it's a built-in preset
	if preset.IsBuiltIn() {
		if !isValidBuiltInPreset(preset.BuiltIn) {
			return oops.
				With("field", fmt.Sprintf("presets[%d]", index)).
				With("preset", preset.BuiltIn).
				With("available_presets", getBuiltInPresetNames()).
				Hint(fmt.Sprintf("Use a valid built-in preset name\nAvailable presets: %s", getBuiltInPresetNames())).
				Errorf("unknown built-in preset: %q", preset.BuiltIn)
		}
		return nil
	}

	// Custom preset validation
	if preset.Name == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].name", index)).
			Hint("Custom presets must have a 'name' field\nExample: {name: my-preset, type: markdown, path: CUSTOM.md}").
			Errorf("custom preset missing required field 'name'")
	}

	// Provider-backed custom presets reference a declarative provider spec
	// instead of carrying type/path inline.
	if preset.Provider != "" {
		if preset.Type != "" || preset.Path != "" {
			return oops.
				With("field", fmt.Sprintf("presets[%d]", index)).
				With("preset_name", preset.Name).
				Hint("A provider-backed preset gets its type/path from the spec; drop 'type' and 'path'").
				Errorf("custom preset %q sets both 'provider' and 'type'/'path'", preset.Name)
		}
		if ProviderSpecValidator != nil {
			if err := ProviderSpecValidator(*preset, c.BaseDir); err != nil {
				return oops.
					With("field", fmt.Sprintf("presets[%d].provider", index)).
					With("preset_name", preset.Name).
					With("provider", preset.Provider).
					Wrapf(err, "invalid provider spec for custom preset %q", preset.Name)
			}
		}
		return nil
	}

	if preset.Type == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].type", index)).
			With("preset_name", preset.Name).
			Hint("Custom presets must have a 'type' field\nValid types: markdown, directory, json").
			Errorf("custom preset %q missing required field 'type'", preset.Name)
	}

	// Validate preset type
	validTypes := []PresetType{PresetTypeMarkdown, PresetTypeDirectory, PresetTypeJSON}
	isValidType := false
	for _, validType := range validTypes {
		if preset.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return oops.
			With("field", fmt.Sprintf("presets[%d].type", index)).
			With("preset_name", preset.Name).
			With("actual_type", preset.Type).
			With("valid_types", validTypes).
			Hint("Use a valid preset type: markdown, directory, or json").
			Errorf("custom preset %q has invalid type: %q", preset.Name, preset.Type)
	}

	if preset.Path == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].path", index)).
			With("preset_name", preset.Name).
			Hint("Custom presets must have a 'path' field\nExample: path: docs/AI_GUIDE.md").
			Errorf("custom preset %q missing required field 'path'", preset.Name)
	}

	return nil
}

// validateProfiles validates the profiles section
func (c *Config) validateProfiles() error {
	// If default is specified, profiles must be defined
	if c.Default != "" && len(c.Profiles) == 0 {
		if c.defaultFromOverlay() {
			return oops.
				With("field", "default").
				Hint("If you specify a default profile, you must define profiles\nRemove the 'default' field or add a 'profiles' section").
				Errorf("the default profile set by the local overlay is specified but no profiles are defined")
		}
		return oops.
			With("field", "default").
			With("default_profile", c.Default).
			Hint("If you specify a default profile, you must define profiles\nRemove the 'default' field or add a 'profiles' section").
			Errorf("default profile %q specified but no profiles defined", c.Default)
	}

	// A comma separates the elements of a composed profile value, so a name
	// containing one could never be selected.
	for name := range c.Profiles {
		if strings.Contains(name, ProfileSeparator) {
			return oops.
				With("field", "profiles").
				With("profile_name", name).
				Hint("A comma composes several profiles into one value (profile: \"base,backend\"), so it cannot appear in a profile name\nRename the profile without a comma").
				Errorf("profile name %q contains %q", name, ProfileSeparator)
		}
	}

	// A "builtin:<name>" element must name a real builtin pack. A misspelled
	// reference loads nothing and would otherwise surface only as a vague
	// missing-domain warning.
	for name, domains := range c.Profiles {
		for _, domain := range domains {
			if !builtins.HasRefPrefix(domain) {
				continue
			}
			ref := builtins.TrimRefPrefix(domain)
			if !builtins.IsValid(ref) {
				return oops.
					With("field", fmt.Sprintf("profiles.%s", name)).
					With("builtin", domain).
					Hint("Use a name from `ai-rulez builtins list`, or drop the 'builtin:' prefix to reference a local domain").
					Errorf("profile %q references unknown builtin %q", name, ref)
			}
		}
	}

	// If default is specified, every element of it must exist in profiles. The
	// default may be composed, the same as a --profile value.
	if c.Default != "" && !c.HasProfile(c.Default) {
		profileNames := make([]string, 0, len(c.Profiles))
		for name := range c.Profiles {
			profileNames = append(profileNames, name)
		}
		sort.Strings(profileNames)
		unknown := c.UnknownProfileNames(c.Default)
		if c.defaultFromOverlay() {
			return oops.
				With("field", "default").
				With("available_profiles", profileNames).
				Hint(fmt.Sprintf("Set default to one of the defined profiles: %v", profileNames)).
				Errorf("the default profile set by the local overlay does not exist in profiles")
		}
		return oops.
			With("field", "default").
			With("default_profile", c.Default).
			With("unknown_profiles", unknown).
			With("available_profiles", profileNames).
			Hint(fmt.Sprintf("Set default to one of the defined profiles: %v\nOr add a profile named %q", profileNames, strings.Join(unknown, ", "))).
			Errorf("default profile %q does not exist in profiles", strings.Join(unknown, ", "))
	}

	return nil
}

// defaultFromOverlay reports whether the default profile was set by the local
// overlay, whose values must not be echoed in messages (a mistyped key can hold
// a secret).
func (c *Config) defaultFromOverlay() bool {
	if c.LocalOverlay == nil {
		return false
	}
	_, ok := c.LocalOverlay.Doc["default"]
	return ok
}

// validateInstalledSkills validates the installed_skills section
func (c *Config) validateInstalledSkills() error {
	seen := make(map[string]bool)
	for i, skill := range c.InstalledSkills {
		if skill.Name == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].name", i)).
				Hint("Each installed skill must have a non-empty 'name' field").
				Errorf("installed skill at index %d missing required field 'name'", i)
		}
		if skill.Source == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].source", i)).
				With("skill_name", skill.Name).
				Hint("Provide a git URL or local path as the 'source'").
				Errorf("installed skill %q missing required field 'source'", skill.Name)
		}
		if seen[skill.Name] {
			return oops.
				With("field", "installed_skills").
				With("skill_name", skill.Name).
				Hint("Each installed skill must have a unique name").
				Errorf("duplicate installed skill name: %q", skill.Name)
		}
		seen[skill.Name] = true
	}
	return nil
}

// warnMissingDomainReferences logs warnings for domains referenced in profiles but not found in content.
// Domains from includes (FromInclude=true) are checked in the merged content tree.
// If includes are configured but a domain is missing, we emit a debug hint instead
// of a warning since the domain may exist in the include source but failed to resolve.
func (c *Config) warnMissingDomainReferences() {
	if c.Content == nil || len(c.Profiles) == 0 {
		return
	}

	hasIncludes := len(c.Includes) > 0

	// Collect all domain names referenced in profiles. A "builtin:<name>"
	// reference addresses the builtin's bare domain name in the content tree.
	referencedDomains := make(map[string]bool)
	for _, domains := range c.Profiles {
		for _, domain := range domains {
			referencedDomains[builtins.TrimRefPrefix(domain)] = true
		}
	}

	// Check which domains are missing
	for domain := range referencedDomains {
		if _, exists := c.Content.Domains[domain]; !exists && !c.hasLocalDomain(domain) {
			if hasIncludes {
				logger.Debug("profile references domain not found in merged content (may be missing from include source)",
					"domain", domain)
			} else {
				logger.Warn("profile references non-existent domain", "domain", domain)
			}
		}
	}
}

// hasLocalDomain reports whether machine-local content defines the domain.
func (c *Config) hasLocalDomain(domain string) bool {
	if c.LocalContent == nil {
		return false
	}
	_, ok := c.LocalContent.Domains[domain]
	return ok
}

// getBuiltInPresetNames returns a list of built-in preset names
func getBuiltInPresetNames() []string {
	return AllPresetNames()
}

// namespaceEntry is the first item seen for an output id: the id as authored
// (for the message) plus its source path (so both sides of a collision are
// nameable).
type namespaceEntry struct {
	id   string
	path string
}

// validateDuplicateOutputIDs detects two skills, or two commands, that resolve
// to the same output id within one scope. Both render to
// .claude/skills/{id}/SKILL.md, so whichever is written last silently replaces
// the other. The directory form opened this hole: before it, two commands in one
// directory could not share an id.
//
// A scope is root, or a single domain, and never a pool of the two. Cross-scope
// duplicates are resolved on purpose by allSkills/allCommands in
// internal/generator/presets, which drop the lower-precedence copy (root beats
// on-disk domain beats include beats builtin) and warn, so two scopes never
// compete for one output path. Within a single scope there is no precedence to
// apply and nothing resolves the clash — hence this check.
func (c *Config) validateDuplicateOutputIDs() error {
	if c.Content == nil {
		return nil
	}

	var collisions []string
	collectScope := func(skills, commands []ContentFile) {
		collisions = append(collisions, duplicateOutputIDs(skills, skillDirectoryOutputID, ItemKindSkill)...)
		collisions = append(collisions, duplicateOutputIDs(commands, commandOutputID, ItemKindCommand)...)
	}

	collectScope(c.Content.Skills, c.Content.Commands)

	for _, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		collectScope(domain.Skills, domain.Commands)
	}

	if len(collisions) == 0 {
		return nil
	}

	// Domain map iteration is unordered; sort so the message is reproducible.
	sort.Strings(collisions)

	return oops.
		With("collisions", collisions).
		Hint("Two skills, or two commands, in one directory cannot share an output id: both render to "+
			".claude/skills/{id}/SKILL.md and one silently replaces the other. The directory form "+
			"(commands/name/COMMAND.md) and the flat form (commands/name.md) resolve to the same id — "+
			"keep one, or rename one side.").
		Errorf("duplicate output ids: %s", strings.Join(collisions, "; "))
}

// duplicateOutputIDs reports items of one kind, within one scope, that resolve
// to the same output id. Ids are compared case-folded for the same reason as the
// cross-kind check: on a case-insensitive checkout two ids differing only in
// case are one output directory.
func duplicateOutputIDs(items []ContentFile, outputID func(ContentFile) string, kind string) []string {
	firstByKey := make(map[string]namespaceEntry, len(items))

	var duplicates []string
	for _, item := range items {
		id := outputID(item)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		first, seen := firstByKey[key]
		if !seen {
			firstByKey[key] = namespaceEntry{id: id, path: item.Path}
			continue
		}
		// One source listed twice — include merges carry the same entry into
		// more than one slice — cannot overwrite itself.
		if first.path == item.Path {
			continue
		}
		duplicates = append(duplicates, fmt.Sprintf("%s %q (%s) vs %s %q (%s)",
			kind, first.id, first.path, kind, id, item.Path))
	}

	return duplicates
}

// skillDirectoryOutputID returns the output id a skill competes for, or "" for a
// flat skills/name.md file. A flat file resolves to its *parent directory* name
// (computeItemID in internal/generator/providers/render.go takes
// base(dir(path))), so every flat skill in one directory reports the same id.
// That derivation is a defect in flat-skill support rather than an authoring
// collision, and rejecting it here would refuse bare-structure includes that
// generate today.
func skillDirectoryOutputID(skill ContentFile) string {
	if skill.Path != "" && filepath.Base(skill.Path) != skillMarkerFile {
		return ""
	}

	return SkillID(skill)
}

// validateOutputNamespaceCollisions detects a skill and a command that would
// write to the same output path. Skills and commands both render to
// .claude/skills/{id}/SKILL.md, differing only in the user-invocable frontmatter
// constant, so a shared id silently overwrites one with the other — data loss
// that no other check catches.
//
// Root and every domain are pooled together because the output layout has no
// domain segment: a skill in one domain and a command in another still land on
// the same path for any profile that activates both. Collisions *within* one
// kind are reported by validateDuplicateOutputIDs instead, which pools nothing:
// root shadowing a domain is documented design, resolved by the scanner.
func (c *Config) validateOutputNamespaceCollisions() error {
	if c.Content == nil {
		return nil
	}

	// Skill and command ids are derived differently by the generator, and the
	// difference matters: a directory named "Foo_Bar" yields the skill id
	// "Foo_Bar" but the command id "foo-bar". Mirroring each rule exactly is
	// what makes this check agree with what is actually written to disk.
	// Keyed case-folded: macOS and Windows checkouts are case-insensitive, so
	// .claude/skills/Review/ and .claude/skills/review/ are one directory there.
	// Treating that as a collision everywhere keeps the diagnosis portable.
	skills := make(map[string]namespaceEntry)
	commands := make(map[string]namespaceEntry)

	collect := func(into map[string]namespaceEntry, items []ContentFile, id func(ContentFile) string) {
		for _, item := range items {
			itemID := id(item)
			if itemID == "" {
				continue
			}
			key := strings.ToLower(itemID)
			if _, seen := into[key]; !seen {
				into[key] = namespaceEntry{id: itemID, path: item.Path}
			}
		}
	}

	collect(skills, c.Content.Skills, SkillID)
	collect(commands, c.Content.Commands, commandOutputID)

	for _, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		collect(skills, domain.Skills, SkillID)
		collect(commands, domain.Commands, commandOutputID)
	}

	var collisions []string
	for key, skill := range skills {
		command, clash := commands[key]
		if !clash {
			continue
		}
		collisions = append(collisions, fmt.Sprintf(
			"skill %q (%s) vs command %q (%s)", skill.id, skill.path, command.id, command.path))
	}
	if len(collisions) == 0 {
		return nil
	}

	// Map iteration is unordered; sort so the message is reproducible.
	sort.Strings(collisions)

	return oops.
		With("collisions", collisions).
		Hint("Skills and commands share one output namespace (.claude/skills/{id}/SKILL.md), so an id must be unique across skills/ and commands/ in every domain. Rename one side.").
		Errorf("skill and command ids collide in the output namespace: %s", strings.Join(collisions, "; "))
}

// commandOutputID mirrors sanitizeAgentID in
// internal/generator/providers/render.go, the function commands actually resolve
// through: lowercase, spaces and underscores to dashes. Duplicated rather than
// shared because that function is unexported in a package this one cannot
// import without a cycle; render.go remains the source of truth, so a change
// there must be mirrored here.
func commandOutputID(command ContentFile) string {
	id := strings.ToLower(command.Name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")

	return id
}

// validateMCP checks [[mcp_servers]] headers and the [mcp] options. The
// self-server tuning fields are rejected when self_server is off, since they
// would otherwise be silently inert.
func (c *Config) validateMCP() error {
	if err := c.validateMCPServerHeaders(); err != nil {
		return err
	}
	if c.MCP == nil {
		return nil
	}
	m := c.MCP
	if !m.SelfServer && (m.SelfServerVersion != "" || len(m.SelfServerCommand) > 0) {
		return oops.
			Hint("Set `self_server = true` under [mcp], or remove self_server_version / self_server_command").
			Errorf("mcp.self_server_version and mcp.self_server_command require mcp.self_server = true")
	}
	if len(m.SelfServerCommand) > 0 && strings.TrimSpace(m.SelfServerCommand[0]) == "" {
		return oops.
			Hint("The first element of self_server_command is the executable, e.g. [\"ai-rulez\", \"mcp\"]").
			Errorf("mcp.self_server_command executable must not be empty")
	}
	if len(m.SelfServerCommand) > 0 && m.SelfServerVersion != "" {
		return oops.
			Hint("self_server_command replaces the whole launch command, so a pinned version has no effect; remove one of them").
			Errorf("mcp.self_server_version and mcp.self_server_command are mutually exclusive")
	}
	if strings.ContainsAny(m.SelfServerVersion, " \t\n@/") {
		return oops.
			Hint("Use a bare version or npm dist-tag such as \"4.19.0\" or \"latest\"").
			Errorf("mcp.self_server_version %q is not a valid version", m.SelfServerVersion)
	}
	return nil
}

func (c *Config) validateHeaderHashes() error {
	switch mode := c.GetHeaderHashes(); mode {
	case HeaderHashesFull, HeaderHashesContent, HeaderHashesNone:
		return nil
	default:
		return oops.
			With("field", "header.hashes").
			With("value", mode).
			Hint(`Use "full", "content" or "none".`).
			Errorf("invalid header.hashes %q", mode)
	}
}

// validateMCPServerHeaders checks [[mcp_servers]] headers: they only apply to
// remote transports, names must be RFC 9110 tokens, and values must not carry
// CR/LF (which would let a value inject further headers).
func (c *Config) validateMCPServerHeaders() error {
	names := make([]string, 0, len(c.MCPServers))
	for name := range c.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		server := c.MCPServers[name]
		if server == nil || len(server.Headers) == 0 {
			continue
		}
		if t := server.GetTransport(); t != TransportHTTP && t != TransportSSE {
			return oops.
				With("server", name).
				Hint(`Set transport = "http" or "sse", or remove headers; stdio servers take env instead`).
				Errorf("mcp_servers.%s.headers require transport http or sse", name)
		}
		seen := make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			if other, dup := seen[strings.ToLower(key)]; dup {
				return oops.
					With("server", name).
					Hint("HTTP header names are case-insensitive; keep one of them").
					Errorf("mcp_servers.%s.headers: duplicate header %q and %q", name, other, key)
			}
			seen[strings.ToLower(key)] = key
			if !isHTTPToken(key) {
				return oops.
					With("server", name).
					Hint("Header names may contain only letters, digits and !#$%&'*+-.^_`|~").
					Errorf("mcp_servers.%s.headers: invalid header name %q", name, key)
			}
			if strings.ContainsAny(value, "\r\n") {
				return oops.
					With("server", name).
					Errorf("mcp_servers.%s.headers.%s must not contain a line break", name, key)
			}
		}
	}
	return nil
}

// isHTTPToken reports whether s is a non-empty RFC 9110 token (a valid header name).
func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

// validateRuleActivation checks the `activation` frontmatter of rules and
// context files in root content, domains and local content. Problems in
// builtin or included domains are warnings, since the user cannot fix them
// locally; everything else is an error.
func (c *Config) validateRuleActivation() error {
	for _, tree := range []*ContentTree{c.Content, c.LocalContent} {
		if tree == nil {
			continue
		}
		if err := validateActivationSlice(tree.Rules, false); err != nil {
			return err
		}
		if err := validateActivationSlice(tree.Context, false); err != nil {
			return err
		}
		names := make([]string, 0, len(tree.Domains))
		for name := range tree.Domains {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			domain := tree.Domains[name]
			if domain == nil {
				continue
			}
			lenient := domain.Builtin || domain.FromInclude
			if err := validateActivationSlice(domain.Rules, lenient); err != nil {
				return err
			}
			if err := validateActivationSlice(domain.Context, lenient); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateActivationSlice(files []ContentFile, lenient bool) error {
	for _, f := range files {
		err := validateActivation(f)
		if err == nil {
			continue
		}
		if !lenient {
			return err
		}
		logger.Warn("invalid activation in included content", "file", f.Path, "error", err.Error())
	}
	return nil
}

const logKeyFile = "file"

// legacyValueWarning is one advisory about a legacy activation field.
type legacyValueWarning struct {
	msg   string
	attrs []any
}

// unknownLegacyValues lists legacy `trigger` and `alwaysApply` values that are
// not recognized and so are ignored when resolving activation, and always-on
// legacy activations whose globs are ignored.
func unknownLegacyValues(f ContentFile) []legacyValueWarning {
	if f.Metadata == nil {
		return nil
	}
	var out []legacyValueWarning
	if v := f.Metadata.Extra["trigger"]; strings.TrimSpace(v) != "" && triggerMode(v) == "" {
		out = append(out, legacyValueWarning{
			"unknown trigger value ignored; use always_on, glob, model_decision or manual",
			[]any{logKeyFile, f.Path, "trigger", v},
		})
	}
	switch v := f.Metadata.Extra["alwaysApply"]; strings.ToLower(strings.TrimSpace(v)) {
	case "", boolTrue, boolFalse:
	default:
		out = append(out, legacyValueWarning{
			"unknown alwaysApply value ignored; use true or false",
			[]any{logKeyFile, f.Path, "alwaysApply", v},
		})
	}
	if f.Metadata.Activation == "" {
		if act := f.Metadata.ResolveActivation(); act.Mode == ActivationAlways && len(act.Globs) > 0 {
			out = append(out, legacyValueWarning{
				"legacy always-on trigger with globs/paths: the globs are ignored and the rule applies everywhere",
				[]any{logKeyFile, f.Path, "source", act.Source},
			})
		}
	}
	return out
}

// invalidTargetWarned remembers the (file, target) pairs already reported so a
// config validated several times warns once.
var invalidTargetWarned sync.Map

// warnInvalidTargets reports frontmatter targets that are malformed glob
// patterns: they never match, so the item would be silently dropped from every
// output that is restricted by targets.
func warnInvalidTargets(f ContentFile) {
	for _, target := range invalidTargets(f) {
		if _, seen := invalidTargetWarned.LoadOrStore(f.Path+"\x00"+target, struct{}{}); seen {
			continue
		}
		logger.Warn("invalid glob in targets never matches any output", logKeyFile, f.Path, "name", f.Name, "target", target)
	}
}

// invalidTargets lists the malformed glob patterns among f's targets.
func invalidTargets(f ContentFile) []string {
	if f.Metadata == nil {
		return nil
	}
	var out []string
	for _, target := range f.Metadata.Targets {
		if targetmatch.InvalidGlob(target) {
			out = append(out, target)
		}
	}
	return out
}

func validateActivation(f ContentFile) error {
	warnInvalidTargets(f)
	for _, w := range unknownLegacyValues(f) {
		logger.Warn(w.msg, w.attrs...)
	}
	if f.Metadata == nil || strings.TrimSpace(f.Metadata.Activation) == "" {
		return nil
	}
	m := f.Metadata
	mode := m.ActivationValue()
	fail := func(hint, format string, args ...any) error {
		return oops.With("file", f.Path).With("hint", hint).Errorf(format, args...)
	}

	if !mode.IsValid() {
		return fail("Use one of: always, glob, auto, manual",
			"%s: unknown activation %q", f.Path, m.Activation)
	}

	act := m.ResolveActivation()
	switch {
	case mode == ActivationGlob && len(act.Globs) == 0:
		return fail("Add globs/paths, or choose a different activation",
			"%s: activation %q requires globs or paths", f.Path, m.Activation)
	case mode == ActivationAuto && act.Description == "":
		return fail("Add a description so the model can decide when to apply the rule",
			"%s: activation %q requires a description", f.Path, m.Activation)
	case mode == ActivationAlways && len(act.Globs) > 0:
		return fail("Drop the globs/paths, or drop activation = \"always\"",
			"%s: activation %q conflicts with globs/paths", f.Path, m.Activation)
	}

	warnLegacyActivation(f.Path, m, mode)
	return nil
}

// warnLegacyActivation warns when a legacy field contradicts the explicit
// activation, which takes precedence.
func warnLegacyActivation(path string, m *Metadata, mode ActivationMode) {
	if legacy := triggerMode(m.Extra["trigger"]); legacy != "" && legacy != mode {
		logger.Warn("legacy trigger contradicts activation; activation wins",
			"file", path, "trigger", m.Extra["trigger"], "activation", m.Activation)
	}
	switch strings.ToLower(strings.TrimSpace(m.Extra["alwaysApply"])) {
	case "true":
		if mode != ActivationAlways {
			logger.Warn("legacy alwaysApply contradicts activation; activation wins",
				"file", path, "alwaysApply", "true", "activation", m.Activation)
		}
	case "false":
		if mode == ActivationAlways {
			logger.Warn("legacy alwaysApply contradicts activation; activation wins",
				"file", path, "alwaysApply", "false", "activation", m.Activation)
		}
	}
}
