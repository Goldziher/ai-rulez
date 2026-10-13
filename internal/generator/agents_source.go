package generator

import (
	"path/filepath"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// agentsSourceFiles lists the instruction files the "agents" source reads: the
// root AGENTS.md and the AGENTS.md of every configured scope. Absolute paths.
func agentsSourceFiles(cfg *config.Config) []string {
	files := []string{filepath.Join(cfg.BaseDir, config.AgentsMDFilename)}
	for _, scope := range cfg.Scopes {
		files = append(files, filepath.Join(cfg.BaseDir, filepath.FromSlash(scope.Path), config.AgentsMDFilename))
	}
	return files
}

// isAgentsSourcePath reports whether the absolute path abs belongs to the
// "agents" source of cfg: an AGENTS.md it reads, or anything under .agents.
// Always false for a project with another source.
func isAgentsSourcePath(cfg *config.Config, abs string) bool {
	if !cfg.UsesAgentsSource() {
		return false
	}
	if underAny(abs, []string{filepath.Join(cfg.BaseDir, config.AgentsDirName)}) {
		return true
	}
	for _, file := range agentsSourceFiles(cfg) {
		if samePath(abs, file) {
			return true
		}
	}
	return false
}

// isAgentsSource reports whether abs is source content of an "agents" project
// that no generate run owns: an AGENTS.md or .agents file without the generated
// header. A leftover that carries the header is output of an earlier run (from
// before the source was switched) and may be retired like any generated file.
func (g *Generator) isAgentsSource(abs string) bool {
	return isAgentsSourcePath(g.config, abs) && !g.looksGenerated(abs)
}

// withoutAgentsSource drops from files (absolute paths) the source of an "agents"
// project: whatever a manifest lists, clean never removes AGENTS.md or .agents,
// not even with --include-edited. User scope has no such source.
func (g *Generator) withoutAgentsSource(files []string) []string {
	if g.userMode {
		return files
	}
	return slices.DeleteFunc(files, g.isAgentsSource)
}

// removeEmptyAgentsStateDir removes the state folder beside .agents/ai-rulez.toml
// once clean has emptied it: it holds only what ai-rulez writes, so an empty one
// is clutter.
func (g *Generator) removeEmptyAgentsStateDir() {
	if g.config.AgentsLayout && !g.userMode {
		g.removeEmptyDir(g.config.ConfigDir)
	}
}

// dropAgentsSourceOutputs removes, from every preset, the outputs that would
// land on the files the "agents" source reads. They are the project's own
// content: writing generated output over them would make the next load read its
// own output back (and replace what a person wrote). CLAUDE.md and every other
// tool's files are untouched; consumers of the shared files read the originals.
// One info line says how many were left alone.
func (g *Generator) dropAgentsSourceOutputs(allOutputs map[string][]config.OutputFile, cfg *config.Config) {
	if !cfg.UsesAgentsSource() {
		return
	}
	dropped := 0
	for name, outputs := range allOutputs {
		kept := outputs[:0:0]
		for _, output := range outputs {
			path := output.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(cfg.BaseDir, path)
			}
			if isAgentsSourcePath(cfg, path) {
				dropped++
				continue
			}
			kept = append(kept, output)
		}
		allOutputs[name] = kept
	}
	if dropped > 0 && g.config.OnceKey("agents-source-skip") {
		g.log().Info("source = agents: AGENTS.md and .agents/ are the source and are not written", "skipped", dropped)
	}
}

// withoutAgentsSourcePluginOutputs drops, from the plugin outputs, the paths of
// the "agents" source (the Codex index .agents/plugins/marketplace.json, or a
// bundle placed under .agents). They are the project's own files, so a plugin
// run never overwrites them; each skipped path is named in a warning.
func (g *Generator) withoutAgentsSourcePluginOutputs(outputs []config.OutputFile) []config.OutputFile {
	if !g.config.UsesAgentsSource() || g.userMode {
		return outputs
	}
	kept := outputs[:0:0]
	for i := range outputs {
		abs := g.absOutputPath(outputs[i].Path)
		if isAgentsSourcePath(g.config, abs) {
			g.warnOnce("source = agents: not writing "+g.relSlash(abs)+" (AGENTS.md and .agents/ are the source and are never written)",
				"hint", "point the plugin output elsewhere, or keep that file by hand")
			continue
		}
		kept = append(kept, outputs[i])
	}
	return kept
}
