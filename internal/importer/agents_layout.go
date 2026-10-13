package importer

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// stateDirName is where ai-rulez keeps its own files next to .agents/ai-rulez.toml.
const stateDirName = ".ai-rulez"

// agentsInto reports whether intoAbs is the .agents directory (or its state
// directory): the conversion then produces a project with the "agents" source.
func agentsInto(intoAbs string) bool {
	return config.IsAgentsConfigDir(intoAbs)
}

// diskRel maps a path of the planned tree, which is relative to the config
// directory (config.toml, rules/x.md, local/...), to its path below intoAbs. They
// are the same unless intoAbs is .agents: the config is then ai-rulez.toml and
// the machine-local tree sits in the .ai-rulez state subfolder.
func diskRel(intoAbs, rel string) string {
	if !agentsInto(intoAbs) {
		return rel
	}
	switch {
	case rel == configTOML:
		return config.AgentsConfigFilename
	case rel == localDir || strings.HasPrefix(rel, localDir+"/"):
		return stateDirName + "/" + rel
	}
	return rel
}

// stateAbs is the directory that holds the conversion record: the config
// directory, or the .ai-rulez subfolder of .agents.
func stateAbs(intoAbs string) string {
	if agentsInto(intoAbs) {
		return filepath.Join(intoAbs, stateDirName)
	}
	return intoAbs
}

// dropInPlace removes from the plan what the "agents" source already reads where
// it is: the root AGENTS.md and everything under .agents. Copying it into
// .agents would duplicate it, and a generated copy is never source anyway.
func dropInPlace(plan *Plan) {
	inPlace := func(source string) bool {
		source = path.Clean(filepath.ToSlash(source))
		return source == config.AgentsMDFilename || source == config.AgentsDirName || strings.HasPrefix(source, config.AgentsDirName+"/")
	}
	kept := plan.Items[:0:0]
	for i := range plan.Items {
		it := &plan.Items[i]
		stays := len(it.Sources) > 0
		for _, s := range it.Sources {
			stays = stays && inPlace(s)
		}
		if !stays {
			kept = append(kept, *it)
			continue
		}
		plan.add(newFinding(StatusMapped, it.Sources[0], "", "", "already the source of an agents project; left in place"))
	}
	plan.Items = kept
	dropInPlacePresets(plan)
}

// dropInPlacePresets removes the presets that only the files kept in place imply:
// a preset such as antigravity writes .agents/rules, and in an agents project
// nothing is ever written under .agents, so reading its folder says nothing
// about the tools to generate for.
func dropInPlacePresets(plan *Plan) {
	implied := map[string]bool{}
	for _, s := range nativeSources() {
		if strings.HasPrefix(s.Path, config.AgentsDirName+"/") {
			if preset := s.impliedPreset(); preset != "" {
				implied[preset] = true
			}
		}
	}
	kept := plan.Presets[:0:0]
	for _, preset := range plan.Presets {
		if !implied[preset] {
			kept = append(kept, preset)
		}
	}
	plan.Presets = kept
}
