package generator

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/registry"
	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// HarnessReach implements lint.HarnessReach: for an item of the project it names the
// harnesses that load its output. Those are the enabled presets that write it (the plan says
// whether they do, so placement, targets and plugin-only content are honored) and every
// registered harness that reads the directory it lands in, whether or not its preset is
// enabled: a skill in .agents/skills is read by each harness that lists that directory.
type HarnessReach struct {
	cfg     *config.Config
	planned lint.PlannedFiles

	once    sync.Once
	written map[string]bool     // "dir/id" and "dir/id/..." prefixes of the planned files
	layouts map[string]layout   // enabled preset -> its project layout
	readers map[string][]string // skills directory -> harnesses that read it
	noPlan  bool                // the plan could not be rendered: every enabled preset counts as a writer
}

type layout struct {
	skills, commands, agents string
}

// NewHarnessReach returns the reach of cfg's outputs; planned says what a run writes.
func NewHarnessReach(cfg *config.Config, planned lint.PlannedFiles) *HarnessReach {
	return &HarnessReach{cfg: cfg, planned: planned}
}

func (h *HarnessReach) load() {
	h.once.Do(func() {
		reg := h.cfg.Registry
		if reg == nil {
			reg = registry.Default()
		}
		h.indexPlan()
		enabled := map[string]bool{}
		for _, p := range h.cfg.Presets {
			for _, n := range []string{p.BuiltIn, p.Name} {
				if n != "" {
					enabled[strings.ToLower(n)] = true
				}
			}
		}
		h.layouts = map[string]layout{}
		h.readers = map[string][]string{}
		for _, name := range reg.Names() {
			gen, err := reg.Generator(name)
			if err != nil {
				continue
			}
			l, ok := projectLayout(gen, h.cfg)
			if enabled[name] && ok {
				h.layouts[name] = layout{skills: cleanDir(l.SkillsDir), commands: cleanDir(l.CommandsDir), agents: cleanDir(l.AgentsDir)}
			}
			for dir := range skillDirs(gen, l, ok) {
				h.readers[dir] = append(h.readers[dir], name)
			}
		}
		for dir := range h.readers {
			sort.Strings(h.readers[dir])
		}
	})
}

// indexPlan records every planned path, its stem and its directories, folded to lower case: an
// output file name may be folded (.claude/agents/explore.md for the agent Explore).
func (h *HarnessReach) indexPlan() {
	h.written = map[string]bool{}
	paths := h.planned.PlannedPaths()
	h.noPlan = len(paths) == 0
	for _, p := range paths {
		p = strings.ToLower(p)
		h.written[p] = true
		h.written[strings.TrimSuffix(p, path.Ext(p))] = true
		for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
			h.written[dir] = true
		}
	}
}

// skillDirs returns the project directories a harness reads skills from: its own, and the
// ones its user-level layout lists (the project layout mirrors the user one).
func skillDirs(gen config.PresetGenerator, l presets.ProjectLayout, hasLayout bool) map[string]bool {
	dirs := map[string]bool{}
	if hasLayout && l.SkillsDir != "" {
		dirs[cleanDir(l.SkillsDir)] = true
	}
	gp, ok := gen.(presets.GlobalOutputProvider)
	if !ok {
		return dirs
	}
	home := fakeHome()
	paths := gp.GlobalOutputPaths(home, func(string) string { return "" })
	if paths == nil {
		return dirs
	}
	for _, reader := range paths.SkillReaders {
		if rel, err := filepath.Rel(home, reader); err == nil && !strings.HasPrefix(rel, "..") {
			dirs[cleanDir(filepath.ToSlash(rel))] = true
		}
	}
	return dirs
}

// fakeHome is an absolute directory to resolve user-level layouts against; only the part
// below it is used.
func fakeHome() string {
	for _, h := range []string{"/h", `C:\h`} {
		if filepath.IsAbs(h) {
			return h
		}
	}
	return "/h"
}

func projectLayout(gen config.PresetGenerator, cfg *config.Config) (presets.ProjectLayout, bool) {
	if c, ok := gen.(presets.ConfiguredProjectLayoutProvider); ok {
		return c.ProjectLayoutFor(cfg), true
	}
	if p, ok := gen.(presets.ProjectLayoutProvider); ok {
		return p.ProjectLayout(), true
	}
	return presets.ProjectLayout{}, false
}

func cleanDir(d string) string {
	if d == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(d))
}

// Reach implements lint.HarnessReach.
func (h *HarnessReach) Reach(kind, id string) []lint.ReachTarget {
	h.load()
	seen := map[string]bool{}
	var out []lint.ReachTarget
	add := func(t lint.ReachTarget) {
		key := t.Preset + "\x00" + t.Role + "\x00" + t.Dir
		if !seen[key] {
			seen[key] = true
			out = append(out, t)
		}
	}
	for preset, l := range h.layouts {
		dir, role := l.dirFor(kind)
		if dir == "" || (!h.noPlan && !h.written[strings.ToLower(path.Join(dir, id))]) {
			continue
		}
		add(lint.ReachTarget{Preset: preset, Role: role, Dir: dir, Writer: true})
		if role != harnessnames.KindSkill {
			continue
		}
		for _, reader := range h.readers[dir] {
			add(lint.ReachTarget{Preset: reader, Role: role, Dir: dir})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Preset != b.Preset {
			return a.Preset < b.Preset
		}
		return a.Role+a.Dir < b.Role+b.Dir
	})
	return out
}

// dirFor returns the directory a preset writes items of the kind to and the role the harness
// reads them in. Commands kept in the skills directory are skills to the harness.
func (l layout) dirFor(kind string) (dir, role string) {
	switch kind {
	case harnessnames.KindSkill:
		return l.skills, harnessnames.KindSkill
	case harnessnames.KindCommand:
		if l.commands != "" && l.commands == l.skills {
			return l.commands, harnessnames.KindSkill
		}
		return l.commands, harnessnames.KindCommand
	case harnessnames.KindAgent:
		return l.agents, harnessnames.KindAgent
	}
	return "", ""
}
