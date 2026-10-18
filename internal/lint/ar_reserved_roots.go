package lint

import (
	"path"
	"strings"
)

// A name only collides with a harness built-in where the harness reads it, so AR808 looks at a
// hand-written skill or command only inside a root the harness loads from (#457): its own
// directory (.claude/skills), a plugin's skills/ or commands/ directory, a local skill source,
// or a directory [lint] skill_roots names. Content the project authors under .ai-rulez/ is
// always checked, because generate writes it into those roots.

// fileCommand is the file name of a command kept in a directory of its own.
const fileCommand = "COMMAND.md"

// inHarnessRoot reports whether the item lies where the harness of preset loads it from. Authored
// content and items without a path are always in a root; the files found by convention or named
// with --path and --glob must lie in one.
func (r *runner) inHarnessRoot(it *item, preset string) bool {
	if !it.targetOnly {
		return true
	}
	rel := r.rel(it.abs)
	if rel == "" {
		return true
	}
	dir := path.Dir(rel)
	if base := path.Base(rel); base == fileSkill || base == fileCommand {
		dir = path.Dir(dir) // <root>/<name>/SKILL.md: the root is the directory above the item's own
	}
	if len(r.lc.SkillRoots) > 0 {
		return globsMatch(compileGlobs(r.lc.SkillRoots), dir)
	}
	table, ok := reservedPresetTable()[preset]
	if !ok {
		return true
	}
	roots := table.SkillRoots
	pluginDir := "skills"
	if it.kind == kindCommand {
		roots, pluginDir = table.CommandRoots, keyCommands
	}
	for _, root := range roots {
		if dir == root || strings.HasSuffix(dir, "/"+root) {
			return true
		}
	}
	if table.Plugin && path.Base(dir) == pluginDir && r.hasPluginManifest(path.Dir(dir)) {
		return true
	}
	return r.inLocalSkillSource(it, dir)
}

// hasPluginManifest reports whether dir holds a plugin manifest, so its skills/ and commands/
// directories are the plugin's.
func (r *runner) hasPluginManifest(dir string) bool {
	join := func(parts ...string) string { return path.Join(append([]string{dir}, parts...)...) }
	return r.tree.Exists(join(dirPluginMeta, filePlugin)) || r.tree.Exists(join(filePlugin))
}

// inLocalSkillSource reports whether dir is, or is below, the directory of a [[skill_sources]]
// entry that points at a directory of the project.
func (r *runner) inLocalSkillSource(it *item, dir string) bool {
	if it.kind != kindSkill || r.cfg == nil {
		return false
	}
	for i := range r.cfg.SkillSources {
		s := &r.cfg.SkillSources[i]
		url := strings.ReplaceAll(s.URL, `\`, "/")
		if strings.Contains(url, "://") || strings.HasPrefix(url, "git@") || path.IsAbs(url) || hasDriveLetter(url) {
			continue
		}
		root := path.Clean(path.Join(strings.TrimPrefix(url, "./"), strings.TrimPrefix(strings.ReplaceAll(s.GetPath(), `\`, "/"), "./")))
		if root == "." || dir == root || strings.HasPrefix(dir, root+"/") {
			return true
		}
	}
	return false
}

// hasDriveLetter reports whether p starts with a Windows drive such as C:.
func hasDriveLetter(p string) bool {
	return len(p) >= 2 && p[1] == ':' && (p[0]|0x20) >= 'a' && (p[0]|0x20) <= 'z'
}

// globsMatch reports whether rel matches any of the globs.
func globsMatch(globs []globMatcher, rel string) bool {
	for _, g := range globs {
		if g.match(rel) {
			return true
		}
	}
	return false
}
