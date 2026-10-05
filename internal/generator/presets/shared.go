package presets

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
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
	all := rulefiles.RootOwners(agentsFileName)
	var aliases []string
	for _, owner := range owners {
		if !slices.Contains(all, owner) {
			all = append(all, owner)
		}
		if consumer, ok := config.SharedOutputConsumerFor(owner); ok && consumer.HasRootFile() {
			aliases = append(aliases, consumer.ReplacedRootFile())
		}
	}
	shared := &sharedAgentsMDOpts{owners: all, aliases: aliases, inlining: inlining, negatedOnly: !rulefiles.InScope(cfg)}
	return config.OutputFile{
		Path:    filepath.Join(baseDir, agentsFileName),
		Content: (&CodexPresetGenerator{}).renderAgentsMarkdownFor(content, cfg, shared),
	}
}

// SkillTargets maps the id of every skill that restricts itself with frontmatter
// targets to those targets. Such skills stay out of the shared .agents/skills
// tree, which every reader sees whatever the targets say.
func SkillTargets(content *config.ContentTree) map[string][]string {
	targeted := map[string][]string{}
	for _, skill := range allSkills(content) {
		if skill.Metadata != nil && len(skill.Metadata.Targets) > 0 {
			targeted[extractSkillID(skill.Path)] = skill.Metadata.Targets
		}
	}
	return targeted
}

// SharedAgentSkills renders the shared .agents/skills tree: one
// <name>/SKILL.md per skill, in the generic Agent Skills format (name and
// description frontmatter), plus bundled resources. Skills with targets are left
// to the per-preset path (see SkillTargets). It returns nothing when the content
// has no such skills.
func SharedAgentSkills(content *config.ContentTree, baseDir string) []config.OutputFile {
	var skills []config.ContentFile
	for _, skill := range allSkills(content) {
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
	for _, skill := range skills {
		skillDir := filepath.Join(root, extractSkillID(skill.Path))
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
	for _, key := range []string{keyDisableModelInvocation, keyUserInvocable} {
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
	if val, set := meta.ExtraBool(key); set && (key == keyUserInvocable || key == keyDisableModelInvocation) {
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
	for _, cf := range items {
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
