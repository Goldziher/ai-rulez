package generator

import (
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
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
func applySharedOutputs(allOutputs map[string][]config.OutputFile, cfg *config.Config, content *config.ContentTree) {
	if !cfg.AgentsMD {
		return
	}
	var wantAgentsMD, wantSkills bool
	targeted := skillTargets(presets.SkillTargets(content))
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
		allOutputs[preset.BuiltIn] = dropOwnSharedOutputs(allOutputs[preset.BuiltIn], cfg.BaseDir, preset.BuiltIn, consumer, targeted)
	}
	if !wantAgentsMD && !wantSkills {
		return
	}

	inlining := config.SharedAgentsMDInlining(cfg)
	owners := agentsMDOwners(cfg)
	hash := computeSharedSourceHash(cfg, content, inlining, owners)
	var shared []config.OutputFile
	if wantAgentsMD {
		shared = append(shared, presets.SharedAgentsMD(content, cfg.BaseDir, cfg, owners, inlining))
	}
	if wantSkills {
		shared = append(shared, presets.SharedAgentSkills(content, cfg.BaseDir)...)
	}
	for i := range shared {
		if !shared[i].IsDir && shared[i].RawContent == nil {
			shared[i].SourceHash = hash
		}
	}
	dropShadowedByShared(allOutputs, cfg.BaseDir, wantAgentsMD, wantSkills, targeted)
	allOutputs[sharedOutputsKey] = shared
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
func agentsMDOwners(cfg *config.Config) []string {
	var owners []string
	for _, preset := range cfg.Presets {
		if !preset.IsBuiltIn() {
			continue
		}
		if consumer, ok := config.SharedOutputConsumerFor(preset.BuiltIn); ok && consumer.NeedsAgentsMD() {
			owners = append(owners, preset.BuiltIn)
		}
	}
	return owners
}

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
		root = cleanPath(root)
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
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
