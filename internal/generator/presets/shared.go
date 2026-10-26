package presets

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/markdown"
)

// SharedAgentsMD renders the shared AGENTS.md written once when agents_md is on.
// It is the file codex, opencode, xum and amp each render today (their roots are
// byte-identical by design), so the shared file matches what any one of them
// produced alone. owners are the configured presets relying on the file; they
// extend the default AGENTS.md owners, so an item targeted at claude or gemini
// lands in the shared file when that preset imports or reads it, and so does one
// targeted at the root file such a preset replaces (CLAUDE.md, GEMINI.md, ...).
// inlining limits the scoped rules and context to what a preset without a rules
// folder needs.
func SharedAgentsMD(content *config.ContentTree, baseDir string, cfg *config.Config, owners []string,
	inlining config.AgentsMDInlining,
) config.OutputFile {
	all, aliases := sharedAgentsMDOwners(owners)
	shared := &sharedAgentsMDOpts{owners: all, aliases: aliases, inlining: inlining, negatedOnly: !rulefiles.InScope(cfg)}
	return config.OutputFile{
		Path:    filepath.Join(baseDir, agentsMDFile),
		Content: (&CodexPresetGenerator{}).renderAgentsMarkdownFor(content, cfg, shared),
	}
}

// sharedAgentsMDOwners extends the default AGENTS.md owners with the configured
// presets relying on the file, and returns the root files those presets replace
// (CLAUDE.md, GEMINI.md, ...), which frontmatter targets may name instead.
func sharedAgentsMDOwners(owners []string) (all, aliases []string) {
	all = rulefiles.RootOwners(agentsMDFile)
	for _, owner := range owners {
		if !slices.Contains(all, owner) {
			all = append(all, owner)
		}
		if consumer, ok := config.SharedOutputConsumerFor(owner); ok && consumer.HasRootFile() {
			aliases = append(aliases, consumer.ReplacedRootFile())
		}
	}
	return all, aliases
}

// scopedSkillDescriptionLimit is the listing budget harnesses give a skill
// description; a generated one is cut to fit.
const scopedSkillDescriptionLimit = 250

// ScopedFallbackSkills is what rules.scoped_fallback = "skill" puts in the shared
// .agents/skills tree in place of the glob-scoped rules and context the shared
// AGENTS.md no longer inlines: one skill per item, named rule-<id> or
// context-<id>, carrying the item's positive globs as paths (Claude Code and
// Cursor load such a skill only for matching files; other readers list it and
// load it on demand). The items are exactly the ones SharedAgentsMD leaves out:
// selected for AGENTS.md by their targets, glob-activated with at least one
// positive glob, and not moved into a nested AGENTS.md by the baz preset.
func ScopedFallbackSkills(content *config.ContentTree, cfg *config.Config, owners []string) []config.ContentFile {
	root := rulefiles.RootTarget(codexPresetName, agentsMDFile)
	root.Owners, root.RootAliases = sharedAgentsMDOwners(owners)
	var skills []config.ContentFile
	for _, group := range []struct {
		prefix string
		items  []config.ContentFile
	}{
		{"rule-", withoutBazNested(rulefiles.FilterInline(allInlineRules(content), root), cfg)},
		{"context-", withoutBazNested(rulefiles.FilterInline(allInlineContext(content), root), cfg)},
	} {
		for idx := range group.items {
			if skill, ok := scopedFallbackSkill(group.items[idx], group.prefix); ok {
				skills = append(skills, skill)
			}
		}
	}
	return skills
}

// scopedFallbackSkill renders one glob-scoped item as a skill; false when the
// item is not glob-scoped by at least one positive glob.
func scopedFallbackSkill(cf config.ContentFile, prefix string) (config.ContentFile, bool) {
	act := cf.Metadata.ResolveActivation()
	if act.Mode != config.ActivationGlob || rulefiles.EffectiveModeOf(cf) != config.ActivationGlob {
		return config.ContentFile{}, false
	}
	var globs []string
	for _, g := range act.Globs {
		if !strings.HasPrefix(g, "!") {
			globs = append(globs, g)
		}
	}
	if len(globs) == 0 {
		return config.ContentFile{}, false
	}
	id := prefix + rulefiles.ItemID(cf.Name)
	title := cf.Name
	if cf.Metadata != nil && cf.Metadata.OKFTitle != "" {
		title = cf.Metadata.OKFTitle
	}
	description := strings.Join(strings.Fields(act.Description), " ")
	if description == "" {
		description = title + ": conventions for files matching " + strings.Join(globs, ", ") +
			". Use when reading or editing those files."
	}
	if len(description) > scopedSkillDescriptionLimit {
		description = strings.TrimSpace(description[:scopedSkillDescriptionLimit-3]) + "..."
	}
	spans := make([]string, len(globs))
	for i, g := range globs {
		spans[i] = "`" + g + "`"
	}
	body := "# " + title + "\n\n_Applies to: " + strings.Join(spans, ", ") + "_\n\n" +
		strings.TrimSpace(markdown.ProcessEmbeddedContent(cf.Content)) + "\n"
	return config.ContentFile{
		Name:     id,
		ID:       id,
		Path:     cf.Path,
		Content:  body,
		Metadata: &config.Metadata{Paths: globs, Extra: map[string]string{"description": description}},
	}, true
}

// SkillTargets maps the id of every skill that restricts itself with frontmatter
// targets to those targets. Such skills stay out of the shared .agents/skills
// tree, which every reader sees whatever the targets say.
func SkillTargets(content *config.ContentTree) map[string][]string {
	targeted := map[string][]string{}
	skillList := allSkills(content)
	for idx := range skillList {
		skill := skillList[idx]
		if skill.Metadata != nil && len(skill.Metadata.Targets) > 0 {
			targeted[skillIDOf(skill)] = skill.Metadata.Targets
		}
	}
	return targeted
}

// SharedCommandSkills renders the commands without targets as skills, for the
// shared .agents/skills tree: codex, antigravity, devin and warp run a command
// as a skill (config.SharedOutputConsumer.CommandsAsSkills), and the tree
// replaces the directory they would write it to. A command with targets stays on
// each preset's own path (see CommandSkillTargets).
func SharedCommandSkills(d *diag.Collector, content *config.ContentTree) []config.ContentFile {
	return commandSkillsWhere(d, content, string(config.SharedAgentSkills), func(command config.ContentFile) bool {
		return command.Metadata == nil || len(command.Metadata.Targets) == 0
	})
}

// SharedCodexCommandPolicy is the agents/openai.yaml of each command skill in
// the shared tree: Codex has no SKILL.md key for invocation control, so a
// command that must run only on an explicit $name gets the policy file, as in
// Codex's own skills directory.
func SharedCodexCommandPolicy(commands []config.ContentFile, baseDir string) []config.OutputFile {
	var outputs []config.OutputFile
	for idx := range commands {
		agentsDir := filepath.Join(baseDir, ".agents", "skills", skillIDOf(commands[idx]), "agents")
		outputs = append(outputs,
			config.OutputFile{Path: agentsDir, IsDir: true},
			config.OutputFile{Path: filepath.Join(agentsDir, "openai.yaml"), Content: codexImplicitInvocationOff},
		)
	}
	return outputs
}

// CommandSkillTargets maps the id of every command that restricts itself with
// frontmatter targets to those targets, the counterpart of SkillTargets for the
// commands written as skills. A skill of the same id wins, as it does when the
// command is rendered.
func CommandSkillTargets(content *config.ContentTree) map[string][]string {
	targeted := map[string][]string{}
	skillIDs := map[string]bool{}
	skillList := allSkills(content)
	for idx := range skillList {
		skillIDs[skillIDOf(skillList[idx])] = true
	}
	commandList := allCommands(content)
	for idx := range commandList {
		command := commandList[idx]
		id := sanitizeName(command.Name)
		if !skillIDs[id] && command.Metadata != nil && len(command.Metadata.Targets) > 0 {
			targeted[id] = command.Metadata.Targets
		}
	}
	return targeted
}

// SharedAgentSkills renders the shared .agents/skills tree: one
// <name>/SKILL.md per skill, in the generic Agent Skills format (name and
// description frontmatter), plus bundled resources. Skills with targets are left
// to the per-preset path (see SkillTargets). It returns nothing when the content
// has no such skills.
func SharedAgentSkills(content *config.ContentTree, baseDir string, extra ...config.ContentFile) []config.OutputFile {
	skills := slices.Clone(extra)
	skillList := allSkills(content)
	for idx := range skillList {
		skill := skillList[idx]
		if skill.Metadata == nil || len(skill.Metadata.Targets) == 0 {
			skills = append(skills, skill)
		}
	}
	if len(skills) == 0 {
		return nil
	}
	root := filepath.Join(baseDir, ".agents", "skills")
	outputs := []config.OutputFile{
		{Path: filepath.Join(baseDir, ".agents"), IsDir: true},
		{Path: root, IsDir: true},
	}
	for idx := range skills {
		skill := skills[idx]
		skillDir := filepath.Join(root, skillIDOf(skill))
		outputs = append(outputs,
			config.OutputFile{Path: skillDir, IsDir: true},
			config.OutputFile{Path: filepath.Join(skillDir, "SKILL.md"), Content: renderAgentSkillFile(skill)},
		)
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}
	return outputs
}

// renderAgentSkillFile is the one rendering of a skill in the shared
// .agents/skills tree. Every preset that writes that tree (the Go presets and
// the DSL specs, through RenderAgentSkillFrontmatter) must produce these exact
// bytes, so the frontmatter carries the union of the keys any writer emits and
// does not depend on which presets are enabled. Tools ignore keys they do not
// know, so a key meant for one harness is harmless to the others.
func renderAgentSkillFile(skill config.ContentFile) string {
	var builder strings.Builder
	builder.WriteString(RenderAgentSkillFrontmatter(skill))
	builder.WriteString(skill.Content)
	builder.WriteString(RenderSkillResourcesIndex(&skill))
	return builder.String()
}

// RenderAgentSkillFrontmatter renders the canonical SKILL.md frontmatter block
// (--- fences and the blank line after them) of the shared .agents/skills tree:
// name, then the quoted description, then the remaining keys sorted:
//   - the Agent Skills specification fields with their original types
//     (license, compatibility, metadata, allowed-tools), the Codex
//     short-description merged into metadata;
//   - paths, the Claude Code and Cursor glob gate;
//   - disable-model-invocation (Cursor, Claude Code) and user-invocable, as
//     booleans.
func RenderAgentSkillFrontmatter(skill config.ContentFile) string {
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n")
	writeSkillSpecFields(&builder, skill, agentSkillExtraFields(skill))
	builder.WriteString("---\n\n")
	return builder.String()
}

// agentSkillExtraFields are the keys of the canonical skill frontmatter beyond
// name, description and the specification fields writeSkillSpecFields adds.
func agentSkillExtraFields(skill config.ContentFile) map[string]any {
	extra := map[string]any{}
	if shortDesc := config.SkillShortDescription(skill.Metadata); shortDesc != "" {
		extra["metadata"] = mergeShortDescription(skill.Metadata.SkillSpecFields()["metadata"], shortDesc)
	}
	if scope := skill.Metadata.PathScope(); len(scope) > 0 {
		extra["paths"] = scope
	}
	for _, key := range []string{metaDisableModelInvocation, metaUserInvocable} {
		if val, set := skill.Metadata.ExtraBool(key); set {
			extra[key] = val
		}
	}
	return extra
}

// typedAgentField is an agent frontmatter field as the DSL renderer carries it,
// so a file both write is identical: the invocation switches as canonical
// booleans, every other key with its original YAML type (list, map, bool,
// date), a plain string as text. The second result is false when it is unset.
func typedAgentField(meta *config.Metadata, key string) (any, bool) {
	if val, set := meta.ExtraBool(key); set && (key == metaUserInvocable || key == metaDisableModelInvocation) {
		return val, true
	}
	if val, ok := meta.TypedExtra(key); ok {
		return val, true
	}
	return nil, false
}

// inlinedInAgentsMD drops, from the items AGENTS.md would inline, the ones
// every preset relying on it takes from its own rules folder. A nil shared
// keeps everything. The folders hold exactly the items the routing sends to
// files: not always-on rules, glob-scoped context. An item therefore stays when
// it is always-on, when only negated globs scope it (no folder can express that,
// so the folders skip it and AGENTS.md is its single home; a scope run leaves it
// to its scope-qualified file), when inlining asks for its kind (auto and manual
// ones, which include a glob rule without globs). Other context stays unless it
// is glob-scoped.
func inlinedInAgentsMD(items []config.ContentFile, shared *sharedAgentsMDOpts, context bool) []config.ContentFile {
	if shared == nil || shared.inlining.Scoped {
		return items
	}
	kept := make([]config.ContentFile, 0, len(items))
	for idx := range items {
		cf := items[idx]
		raw := cf.Metadata.ResolveActivation().Mode
		effective := rulefiles.EffectiveModeOf(cf)
		autoManual := effective == config.ActivationAuto || effective == config.ActivationManual
		var keep bool
		switch {
		case raw == config.ActivationAlways:
			keep = true
		case shared.negatedOnly && rulefiles.OnlyNegatedGlobs(cf):
			keep = true
		case context:
			keep = raw != config.ActivationGlob || (autoManual && shared.inlining.AutoManual)
		case autoManual:
			keep = shared.inlining.AutoManual
		}
		if keep {
			kept = append(kept, cf)
		}
	}
	return kept
}

// routingWithSharedAgentsMD adapts a rules-folder routing to agents_md: a preset
// that reads the shared AGENTS.md finds its always-on rules and context there,
// so its folder keeps only the items with a narrower activation.
func routingWithSharedAgentsMD(cfg *config.Config, preset string, r rulefiles.Routing) rulefiles.Routing {
	if cfg.ReadsSharedAgentsMD(preset) {
		return rulefiles.WithoutAlwaysOn(r)
	}
	return r
}

// writeSkillSpecFields appends the optional Agent Skills specification fields
// the skill sets (license, compatibility, metadata, allowed-tools), plus any
// preset-specific extra fields, as YAML with their original types. Keys are
// sorted, so output is deterministic. Nothing is written when there are none,
// which keeps a skill without those fields byte-identical to before.
func writeSkillSpecFields(b *strings.Builder, skill config.ContentFile, extra map[string]any) {
	fields := skill.Metadata.SkillSpecFields()
	for k, v := range extra {
		fields[k] = v
	}
	if len(fields) == 0 {
		return
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(fields); err != nil {
		return
	}
	if err := enc.Close(); err != nil {
		return
	}
	b.Write(buf.Bytes())
}

// mergeShortDescription returns the metadata value with short-description added
// as a nested key. existing is the author's metadata (a mapping node) or nil; a
// metadata value that is not a map is returned unchanged, and an author-set
// short-description wins.
func mergeShortDescription(existing any, shortDesc string) any {
	entry := func() (*yaml.Node, *yaml.Node) {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "short-description"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: shortDesc, Style: yaml.DoubleQuotedStyle}
	}
	switch m := existing.(type) {
	case nil:
		k, v := entry()
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{k, v}}
	case *yaml.Node:
		if m.Kind != yaml.MappingNode {
			return existing
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == "short-description" {
				return existing
			}
		}
		k, v := entry()
		m.Content = append(m.Content, k, v)
		return m
	}
	return existing
}
