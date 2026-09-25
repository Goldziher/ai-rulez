package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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

	if err := c.validateProfiles(); err != nil {
		return err
	}

	if err := c.validateSkillDescriptions(); err != nil {
		return err
	}

	if err := c.validateMalformedFrontmatter(); err != nil {
		return err
	}

	if err := c.validateInstalledSkills(); err != nil {
		return err
	}

	if err := c.validateDefaults(); err != nil {
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

	if err := c.validateDuplicateOutputIDs(); err != nil {
		return err
	}

	if err := c.validateOutputNamespaceCollisions(); err != nil {
		return err
	}

	// Warn about missing domain references (non-fatal)
	c.warnMissingDomainReferences()

	// Warn about inert argument-hint on skills (non-fatal)
	c.warnSkillArgumentHint()

	return nil
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
				Hint("Use a built-in preset name as the key (e.g. claude, codex, windsurf).").
				Errorf("unknown preset %q in defaults.effort_by_preset", preset)
		}
		fieldPath := fmt.Sprintf("defaults.effort_by_preset.%s", preset)
		if err := validateEffort(value, fieldPath); err != nil {
			return err
		}
	}
	return nil
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
		if c.Plugin != nil || (c.Marketplace != nil && len(c.Marketplace.Members) > 0) {
			return nil
		}
		return oops.
			With("field", "presets").
			Hint("Add at least one preset to your config file\nExample: presets: [claude]\nAvailable built-in presets: claude, cursor, gemini, windsurf, copilot, continue-dev, cline").
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
		return oops.
			With("field", "default").
			With("default_profile", c.Default).
			Hint("If you specify a default profile, you must define profiles\nRemove the 'default' field or add a 'profiles' section").
			Errorf("default profile %q specified but no profiles defined", c.Default)
	}

	// If default is specified, it must exist in profiles
	if c.Default != "" {
		if _, exists := c.Profiles[c.Default]; !exists {
			profileNames := make([]string, 0, len(c.Profiles))
			for name := range c.Profiles {
				profileNames = append(profileNames, name)
			}
			return oops.
				With("field", "default").
				With("default_profile", c.Default).
				With("available_profiles", profileNames).
				Hint(fmt.Sprintf("Set default to one of the defined profiles: %v\nOr add a profile named %q", profileNames, c.Default)).
				Errorf("default profile %q does not exist in profiles", c.Default)
		}
	}

	return nil
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

	// Collect all domain names referenced in profiles
	referencedDomains := make(map[string]bool)
	for _, domains := range c.Profiles {
		for _, domain := range domains {
			referencedDomains[domain] = true
		}
	}

	// Check which domains are missing
	for domain := range referencedDomains {
		if _, exists := c.Content.Domains[domain]; !exists {
			if hasIncludes {
				logger.Debug("profile references domain not found in merged content (may be missing from include source)",
					"domain", domain)
			} else {
				logger.Warn("profile references non-existent domain", "domain", domain)
			}
		}
	}
}

// getBuiltInPresetNames returns a list of built-in preset names
func getBuiltInPresetNames() []string {
	names := make([]string, 0, len(builtInPresets))
	for name := range builtInPresets {
		names = append(names, name)
	}
	return names
}

// scopeRoot names root content in diagnostics. Domain scopes read
// "domain <name>".
const scopeRoot = "root"

// namespaceEntry is the first item seen for an output id: the id as authored
// (for the message) plus its source path (so both sides of a collision are
// nameable).
type namespaceEntry struct {
	id   string
	path string
}

// validateDuplicateOutputIDs detects two skills, or two commands, that resolve
// to the same output id within one scope. Both render to
// .claude/skills/{id}/SKILL.md and nothing downstream deduplicates them —
// combineContentFiles in internal/generator/presets concatenates and
// stable-sorts — so whichever is written last silently replaces the other. The
// directory form opened this hole: before it, two commands in one directory
// could not share an id.
//
// A scope is root, or a single domain, and never a pool of the two. Cross-scope
// duplicates are resolved on purpose: the scanner drops the root copy when a
// domain defines the same item (resolveCollisions) and namespaces domain ids, so
// two scopes never compete for one output path. Only duplicates inside a single
// scope are resolved by nobody.
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
// .claude/skills/{id}/SKILL.md, differing only in the user_invocable frontmatter
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

// warnInertSkillArgumentHint is advisory rather than fatal: argument-hint is
// inert on skills because user_invocable=false is a hard constant in the
// claude.toml [outputs.skills] block, but a config that declares it still
// generates correctly.
const warnInertSkillArgumentHint = "skill declares argument-hint but it is inert " +
	"(skills have user_invocable=false) — move it to commands/ instead"

// skillWarning is one non-fatal skill advisory, carried as data rather than
// logged at the point of detection so the detection logic stays testable.
type skillWarning struct {
	Scope   string
	Skill   string
	Path    string
	Message string
}

// warnSkillArgumentHint logs the advisories collected by
// skillArgumentHintWarnings.
func (c *Config) warnSkillArgumentHint() {
	for _, warning := range c.skillArgumentHintWarnings() {
		logger.Warn(warning.Message, "scope", warning.Scope, "skill", warning.Skill, "path", warning.Path)
	}
}

// skillArgumentHintWarnings reports skills declaring argument-hint in their
// frontmatter, in root order followed by domains in alphabetical order so the
// warning sequence is reproducible.
func (c *Config) skillArgumentHintWarnings() []skillWarning {
	if c.Content == nil {
		return nil
	}

	var warnings []skillWarning
	collect := func(skills []ContentFile, scope string) {
		for _, skill := range skills {
			if skill.Metadata == nil || skill.Metadata.Extra == nil {
				continue
			}
			if _, hasHint := skill.Metadata.Extra["argument-hint"]; !hasHint {
				continue
			}
			warnings = append(warnings, skillWarning{
				Scope:   scope,
				Skill:   SkillID(skill),
				Path:    skill.Path,
				Message: warnInertSkillArgumentHint,
			})
		}
	}

	collect(c.Content.Skills, scopeRoot)

	domainNames := make([]string, 0, len(c.Content.Domains))
	for domainName := range c.Content.Domains {
		domainNames = append(domainNames, domainName)
	}
	sort.Strings(domainNames)

	for _, domainName := range domainNames {
		if domain := c.Content.Domains[domainName]; domain != nil {
			collect(domain.Skills, "domain "+domainName)
		}
	}

	return warnings
}
