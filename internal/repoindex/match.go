package repoindex

import (
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

const (
	skillFile = "SKILL.md"
	// skillMaxDepth bounds how far below a skills directory a skill directory is looked for.
	skillMaxDepth = 6
	// itemMaxDepth bounds how far below a rules, agents or commands directory an item is looked for.
	itemMaxDepth = 4

	sourceClaude  = "claude"
	sourceAgents  = "agents"
	sourceCodex   = "codex"
	sourceCursor  = "cursor"
	sourceCopilot = "copilot"
	sourceGemini  = "gemini"
	sourceSkills  = "skills-dir"

	dirClaude = ".claude"
	dirAgents = ".agents"
	dirCodex  = ".codex"
	dirCursor = ".cursor"
	dirGithub = ".github"
	dirSkills = "skills"

	fileAgents  = "AGENTS.md"
	fileClaude  = "CLAUDE.md"
	fileGemini  = "GEMINI.md"
	fileCopilot = "copilot-instructions.md"
)

// dirDef is a directory name sequence where a kind of item lives.
type dirDef struct {
	segs   []string
	kind   Kind
	source string
}

// matcher recognizes the units of a file listing.
type matcher struct {
	byFirst map[string][]dirDef
	skip    []string
	exclude *Excluder
}

func newMatcher(presets, skip []string, exclude *Excluder) *matcher {
	m := &matcher{byFirst: map[string][]dirDef{}, skip: skip, exclude: exclude}
	seen := map[string]bool{}
	add := func(dir string, kind Kind, source string) {
		key := string(kind) + "|" + dir
		if dir == "" || seen[key] {
			return
		}
		seen[key] = true
		segs := strings.Split(dir, "/")
		m.byFirst[segs[0]] = append(m.byFirst[segs[0]], dirDef{segs: segs, kind: kind, source: source})
	}
	add(dirSkills, KindSkill, sourceSkills)
	add(dirClaude+"/"+dirSkills, KindSkill, sourceClaude)
	// Rules, agents and commands are indexed wherever a known convention keeps
	// them: the files present decide, not the presets. Skill directories follow
	// the presets.
	every := config.IndividualPresetNames()
	for _, kind := range []struct {
		importer importer.Kind
		kind     Kind
		presets  []string
	}{
		{importer.KindSkill, KindSkill, presets}, {importer.KindRule, KindRule, every},
		{importer.KindAgent, KindAgent, every}, {importer.KindCommand, KindCommand, every},
	} {
		for _, dir := range importer.KindDirectories(kind.presets, kind.importer) {
			add(dir, kind.kind, sourceOfDir(dir))
		}
	}
	for _, defs := range m.byFirst {
		// The longest directory first: .claude/skills before a bare skills.
		sort.SliceStable(defs, func(i, j int) bool { return len(defs[i].segs) > len(defs[j].segs) })
	}
	return m
}

// sourceOfDir names the convention a directory belongs to from its first segment.
func sourceOfDir(dir string) string {
	first, _, _ := strings.Cut(dir, "/")
	switch first {
	case dirClaude:
		return sourceClaude
	case dirAgents:
		return sourceAgents
	case dirCodex:
		return sourceCodex
	case dirCursor:
		return sourceCursor
	case dirGithub:
		return sourceCopilot
	case ".gemini":
		return sourceGemini
	case dirSkills:
		return sourceSkills
	}
	return strings.TrimLeft(first, ".")
}

// skipped reports whether a listed file lies in a place the index never enters.
func (m *matcher) skipped(segs []string, file string) bool {
	for _, seg := range segs {
		if seg == dotGit || seg == nodeModules || seg == configDir {
			return true
		}
	}
	for _, prefix := range m.skip {
		if file == prefix || strings.HasPrefix(file, prefix+"/") {
			return true
		}
	}
	return m.excluded(file)
}

// excluded reports whether an Exclude glob matches the file or a directory above it.
func (m *matcher) excluded(file string) bool { return m.exclude.Excluded(file) }

// candidate is a file that marks an item, before it is read.
type candidate struct {
	kind   Kind
	source string
	scope  string
	// file is the marker file; unit is the skill directory for a skill and the file otherwise.
	file, unit string
	// root is the skills directory a skill was found below.
	root string
	// rel is the path below the kind's directory, without the extension (rules, agents, commands).
	rel string
}

// classify recognizes the unit a listed file marks. ok is false for any other file.
func (m *matcher) classify(segs []string, file string) (candidate, bool) {
	base := segs[len(segs)-1]
	dirs := segs[:len(segs)-1]
	if base == skillFile {
		if c, ok := m.skill(dirs, file); ok {
			return c, true
		}
	}
	if c, ok := m.instruction(dirs, base, file); ok {
		return c, true
	}
	return m.pooled(dirs, base, file)
}

// skill matches a SKILL.md below a skills directory.
func (m *matcher) skill(dirs []string, file string) (candidate, bool) {
	for i := range dirs {
		for _, def := range m.byFirst[dirs[i]] {
			end := i + len(def.segs)
			if def.kind != KindSkill || end > len(dirs) || !equalSegs(dirs[i:end], def.segs) {
				continue
			}
			if below := len(dirs) - end; below < 1 || below > skillMaxDepth {
				continue
			}
			if def.source == sourceSkills && i > 0 && strings.HasPrefix(dirs[i-1], ".") {
				continue // a skills directory inside a hidden directory belongs to that convention
			}
			return candidate{
				kind: KindSkill, source: def.source, scope: joinScope(dirs[:i]),
				file: file, unit: strings.Join(dirs, "/"), root: strings.Join(dirs[:end], "/"),
			}, true
		}
	}
	return candidate{}, false
}

// instruction matches AGENTS.md, CLAUDE.md, GEMINI.md and .github/copilot-instructions.md.
func (m *matcher) instruction(dirs []string, base, file string) (candidate, bool) {
	source := ""
	switch base {
	case fileAgents:
		source = sourceAgents
	case fileClaude:
		source = sourceClaude
	case fileGemini:
		source = sourceGemini
	case fileCopilot:
		if n := len(dirs); n == 0 || dirs[n-1] != dirGithub {
			return candidate{}, false
		}
		source, dirs = sourceCopilot, dirs[:len(dirs)-1]
	default:
		return candidate{}, false
	}
	for _, seg := range dirs {
		if strings.HasPrefix(seg, ".") {
			return candidate{}, false
		}
	}
	return candidate{kind: KindInstruction, source: source, scope: joinScope(dirs), file: file, unit: file}, true
}

// pooled matches a rule, agent or command file below the directory a preset keeps them in.
func (m *matcher) pooled(dirs []string, base, file string) (candidate, bool) {
	ext := path.Ext(base)
	if ext != ".md" && ext != ".mdc" {
		return candidate{}, false
	}
	for i := range dirs {
		for _, def := range m.byFirst[dirs[i]] {
			end := i + len(def.segs)
			if def.kind == KindSkill || end > len(dirs) || !equalSegs(dirs[i:end], def.segs) {
				continue
			}
			if len(dirs)-end+1 > itemMaxDepth {
				continue
			}
			inner := append(append([]string{}, dirs[end:]...), strings.TrimSuffix(base, ext))
			return candidate{
				kind: def.kind, source: def.source, scope: joinScope(dirs[:i]), file: file, unit: file,
				rel: strings.Join(inner, "."),
			}, true
		}
	}
	return candidate{}, false
}

// marker returns the scope and source a listed file marks by its directory
// alone: .claude/, .agents/, .codex/, .cursor/rules.
func marker(segs []string) (scope, source string, ok bool) {
	dirs := segs[:len(segs)-1]
	for i, seg := range dirs {
		switch {
		case seg == dirClaude:
			return joinScope(dirs[:i]), sourceClaude, true
		case seg == dirAgents:
			return joinScope(dirs[:i]), sourceAgents, true
		case seg == dirCodex:
			return joinScope(dirs[:i]), sourceCodex, true
		case seg == dirCursor && i+1 < len(dirs) && dirs[i+1] == "rules":
			return joinScope(dirs[:i]), sourceCursor, true
		}
	}
	return "", "", false
}

func equalSegs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinScope(segs []string) string {
	if len(segs) == 0 {
		return RootScope
	}
	return strings.Join(segs, "/")
}

// outermost drops a skill directory that sits inside another one: its SKILL.md
// belongs to the skill around it (an example, a template).
func outermost(skills []candidate) []candidate {
	sort.Slice(skills, func(i, j int) bool { return skills[i].unit < skills[j].unit })
	var kept []candidate
	for _, c := range skills {
		if n := len(kept); n > 0 && strings.HasPrefix(c.unit, kept[n-1].unit+"/") {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}
