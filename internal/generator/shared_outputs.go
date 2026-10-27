package generator

import (
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// sharedOutputsKey keys the shared outputs in the per-preset output map.
const sharedOutputsKey = "<shared>"

// applySharedOutputs implements the agents_md flag. When it is on and at least
// one configured preset reads a shared output (config.SharedOutputConsumerFor),
// the shared AGENTS.md and .agents/skills tree are rendered once, and the
// outputs those presets would have written for themselves (their root AGENTS.md
// and their own skills directory) are dropped. Dropping after the render keeps
// the participating generators, including declarative providers, unaware of the
// flag; paths a preset no longer emits fall out through the generated manifest.
// It is a no-op when the flag is off, so default output is unchanged.
func applySharedOutputs(allOutputs map[string][]config.OutputFile, cfg *config.Config, content *config.ContentTree) { //nolint:gocyclo // linear preset walk and drop
	if !cfg.AgentsMD {
		return
	}
	var wantAgentsMD, wantSkills bool
	var skillReaders []string
	targeted := sharedSkillTargets(content)
	for _, preset := range cfg.Presets {
		if !preset.IsBuiltIn() {
			continue
		}
		consumer, ok := config.SharedOutputConsumerFor(preset.BuiltIn)
		if !ok {
			continue
		}
		wantAgentsMD = wantAgentsMD || consumer.NeedsAgentsMD()
		wantSkills = wantSkills || consumer.Reads(config.SharedAgentSkills)
		if consumer.Reads(config.SharedAgentSkills) {
			skillReaders = append(skillReaders, preset.BuiltIn)
		}
		allOutputs[preset.BuiltIn] = dropOwnSharedOutputs(allOutputs[preset.BuiltIn], cfg.BaseDir, preset.BuiltIn, consumer, targeted)
	}
	if !wantAgentsMD && !wantSkills {
		return
	}

	inlining := config.SharedAgentsMDInlining(cfg)
	owners := agentsMDOwners(cfg)
	hash := computeSharedSourceHash(cfg, content, inlining, owners)
	var shared []config.OutputFile
	// A plugin-authoring root delivers its content through the plugin, so a root
	// with no content would write a header-only AGENTS.md. It is skipped rather
	// than written empty; the preset's own root file reports the skip (see
	// providers.renderRootFile).
	if wantAgentsMD && !cfg.SkipsRootFiles() && (!content.IsEmpty() || !cfg.HasPluginAuthoring()) {
		shared = append(shared, presets.SharedAgentsMD(content, cfg.BaseDir, cfg, owners, inlining))
	}
	var fallback []config.ContentFile
	if inlining.ScopedAsSkills {
		fallback = scopedFallbackSkills(cfg, content, owners)
	} else if inlining.Scoped {
		warnScopedInlinedTwice(cfg, content, owners)
	}
	if wantSkills {
		// The tree is rendered once for all readers, so delivery is resolved for
		// them together (served skills out and the stub in, as for claude).
		shared = append(shared, sharedSkillOutputs(cfg, content, skillReaders, fallback)...)
	}
	for i := range shared {
		if !shared[i].IsDir && shared[i].RawContent == nil {
			shared[i].SourceHash = hash
		}
	}
	dropShadowedByShared(allOutputs, cfg.BaseDir, wantAgentsMD, wantSkills, targeted)
	allOutputs[sharedOutputsKey] = shared
}

// scopedFallbackSkills is the skills rules.scoped_fallback = "skill" adds to the
// shared tree. A generated skill whose id an authored skill or a command already
// uses is left out with a warning: the authored item keeps its directory.
func scopedFallbackSkills(cfg *config.Config, content *config.ContentTree, owners []string) []config.ContentFile {
	return presets.ScopedFallbackSkillsUnclaimed(content, cfg, owners, func(skill *config.ContentFile) {
		cfg.Diag.Warn("a skill or command already uses the name of a glob-scoped item's fallback skill, so the item "+
			"is not served to presets without a rules folder; rename one of them", "skill", skill.ID, "source", skill.Path)
	})
}

// warnScopedInlinedTwice reports, once per run, that the shared AGENTS.md
// inlines glob-scoped items for a preset without a rules folder while a preset
// with one also reads (or, for claude, imports) the file: that preset then loads
// every such item at the start of each session, beside its own path-scoped copy.
func warnScopedInlinedTwice(cfg *config.Config, content *config.ContentTree, owners []string) {
	var folderReaders []string
	for _, preset := range cfg.Presets {
		if !preset.IsBuiltIn() {
			continue
		}
		consumer, ok := config.SharedOutputConsumerFor(preset.BuiltIn)
		if ok && consumer.NeedsAgentsMD() && consumer.Folder != config.RulesFolderNone {
			folderReaders = append(folderReaders, preset.BuiltIn)
		}
	}
	if len(folderReaders) == 0 {
		return
	}
	scoped := presets.ScopedFallbackItemCount(content, cfg, owners)
	if scoped == 0 {
		return
	}
	cfg.Diag.OnceWarn("agents_md", "scoped-inlined-twice",
		"AGENTS.md inlines glob-scoped rules and context for presets without a rules folder; presets with one also read it, "+
			"so those items load in every session. Set [rules] scoped_fallback = \"skill\" to serve them as path-gated skills",
		"items", scoped, "presets", strings.Join(folderReaders, ","))
}

// sharedSkillTargets is the frontmatter targets of the skills and of the commands
// written as skills: those items keep their per-preset path.
func sharedSkillTargets(content *config.ContentTree) skillTargets {
	targeted := skillTargets(presets.SkillTargets(content))
	for id, targets := range presets.CommandSkillTargets(content) {
		targeted[id] = targets
	}
	return targeted
}

// sharedSkillOutputs renders the shared .agents/skills tree for readers: the
// skills, plus the commands (and Codex's invocation policy for them) when a
// reader runs a command as a skill.
func sharedSkillOutputs(cfg *config.Config, content *config.ContentTree, readers []string,
	fallback []config.ContentFile,
) []config.OutputFile {
	var commands []config.ContentFile
	if slices.ContainsFunc(readers, func(preset string) bool {
		consumer, ok := config.SharedOutputConsumerFor(preset)
		return ok && consumer.CommandsAsSkills
	}) {
		commands = presets.SharedCommandSkills(cfg.Diag, content)
	}
	outputs := presets.SharedAgentSkills(sharedSkillsContent(cfg, content, readers), cfg.BaseDir,
		append(commands, fallback...)...)
	if slices.Contains(readers, string(config.PresetCodex)) {
		outputs = append(outputs, presets.SharedCodexCommandPolicy(commands, cfg.BaseDir)...)
	}
	return outputs
}

// sharedSkillsContent is the content the shared .agents/skills tree renders:
// content with delivery applied for the presets that read the tree. content is
// the profile-filtered tree the presets rendered, which cfg.Content may differ
// from in a scope.
func sharedSkillsContent(cfg *config.Config, content *config.ContentTree, readers []string) *config.ContentTree {
	scoped := *cfg
	scoped.Content = content
	return scoped.ContentForSharedSkills(readers)
}

// dropShadowedByShared removes, from every preset, the outputs that would land on
// a shared path: flattenPresetOutputs lets the last preset in name order win a
// path, so a preset that is not a consumer (a custom provider writing AGENTS.md,
// a tool writing .agents/skills in its own format) would otherwise overwrite the
// shared file depending on how its name sorts. The shared outputs win.
func dropShadowedByShared(allOutputs map[string][]config.OutputFile, baseDir string, agentsMD, skills bool,
	targeted skillTargets,
) {
	var roots []string
	if skills {
		roots = append(roots, filepath.Join(baseDir, filepath.FromSlash(string(config.SharedAgentSkills))))
	}
	agentsMDPath := filepath.Join(baseDir, string(config.SharedAgentsMD))
	for name, outputs := range allOutputs {
		kept := outputs[:0:0]
		for _, output := range outputs {
			if (agentsMD && samePath(output.Path, agentsMDPath)) ||
				(underAny(output.Path, roots) && !targeted.keeps(name, baseDir, output.Path, roots)) {
				continue
			}
			kept = append(kept, output)
		}
		allOutputs[name] = kept
	}
}

// agentsMDOwners lists the configured built-in presets that rely on the shared
// AGENTS.md, so frontmatter targets naming any of them select an item for it.
func agentsMDOwners(cfg *config.Config) []string { return cfg.AgentsMDOwners() }

// dropOwnSharedOutputs removes the outputs the shared ones replace: the root
// AGENTS.md, the preset's own skills directory and .agents/skills itself. A
// preset's own root file (GEMINI.md, .hermes.md, ...) is not rendered in the
// first place, see config.SharedOutputConsumer.OwnRootFile.
func dropOwnSharedOutputs(outputs []config.OutputFile, baseDir, preset string, consumer config.SharedOutputConsumer,
	targeted skillTargets,
) []config.OutputFile {
	return dropOwnOutputs(dropSharedRootSkills(outputs, baseDir, preset, consumer, targeted), baseDir, preset, consumer, targeted)
}

// dropSharedRootSkills removes the preset's outputs below the shared
// .agents/skills tree.
func dropSharedRootSkills(outputs []config.OutputFile, baseDir, preset string, consumer config.SharedOutputConsumer,
	targeted skillTargets,
) []config.OutputFile {
	if !consumer.Reads(config.SharedAgentSkills) {
		return outputs
	}
	return dropUnder(outputs, baseDir, preset, targeted,
		[]string{filepath.Join(baseDir, filepath.FromSlash(string(config.SharedAgentSkills)))})
}

// dropOwnOutputs removes the preset's own root AGENTS.md and its own skills
// directory, the outputs the shared ones replace, leaving whatever it writes to
// .agents/skills.
func dropOwnOutputs(outputs []config.OutputFile, baseDir, preset string, consumer config.SharedOutputConsumer,
	targeted skillTargets,
) []config.OutputFile {
	if consumer.Reads(config.SharedAgentsMD) {
		agentsMD := filepath.Join(baseDir, string(config.SharedAgentsMD))
		kept := outputs[:0:0]
		for _, output := range outputs {
			if !samePath(output.Path, agentsMD) {
				kept = append(kept, output)
			}
		}
		outputs = kept
	}
	if consumer.Reads(config.SharedAgentSkills) && consumer.OwnSkillsDir != "" {
		outputs = dropUnder(outputs, baseDir, preset, targeted,
			[]string{filepath.Join(baseDir, filepath.FromSlash(consumer.OwnSkillsDir))})
	}
	return outputs
}

// dropUnder removes the outputs below roots, except those of skills whose
// targets keep them on the preset's own path.
func dropUnder(outputs []config.OutputFile, baseDir, preset string, targeted skillTargets, roots []string,
) []config.OutputFile {
	kept := outputs[:0:0]
	for _, output := range outputs {
		if underAny(output.Path, roots) && !targeted.keeps(preset, baseDir, output.Path, roots) {
			continue
		}
		kept = append(kept, output)
	}
	return kept
}

// skillTargets maps a skill id to its frontmatter targets (presets.SkillTargets).
type skillTargets map[string][]string

// keeps reports whether path, below one of the skill roots, is an output of a
// skill restricted by targets that the targets allow for preset: those skills
// are not in the shared tree and keep the per-preset path they had before.
func (s skillTargets) keeps(preset, baseDir, path string, roots []string) bool {
	if len(s) == 0 {
		return false
	}
	path = cleanPath(path)
	for _, root := range roots {
		rel, ok := strings.CutPrefix(path, cleanPath(root)+string(filepath.Separator))
		if !ok {
			continue
		}
		id, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		targets, ok := s[id]
		if !ok {
			return false
		}
		fromBase, err := filepath.Rel(cleanPath(baseDir), path)
		if err != nil {
			return false
		}
		fromBase = filepath.ToSlash(fromBase)
		return targetmatch.Allow(targets, []string{preset}, fromBase, pathpkg.Base(fromBase))
	}
	return false
}

func underAny(path string, roots []string) bool {
	path = cleanPath(path)
	for _, root := range roots {
		// Within compares like the platform's file system does (case-insensitively on Windows).
		if safefs.Within(cleanPath(root), path) {
			return true
		}
	}
	return false
}

// samePath compares two paths after cleaning them, so "./a//AGENTS.md" and
// "a/AGENTS.md" are the same file.
func samePath(a, b string) bool {
	return cleanPath(a) == cleanPath(b)
}

// cleanPath is the absolute, cleaned form of path (the cleaned path itself when
// the working directory cannot be resolved).
func cleanPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}
