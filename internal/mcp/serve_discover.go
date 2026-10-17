package mcp

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// Skill discovery (`mcp --serve-skills --discover-skills`, [mcp] discover_skills):
// the skill directories a repository already has are served beside the skills
// ai-rulez defines.

const (
	// discoverMaxDepth bounds how far below a skill root a skill directory is looked for.
	discoverMaxDepth = 6
	// discoverMaxSkills bounds the skill directories one discovery serves.
	discoverMaxSkills = 500
	// discoverMaxWalk bounds the files a walk of a repository without git lists.
	discoverMaxWalk = 200000

	rootSkillsDir = "skills"
	claudeDir     = ".claude"
	nodeModules   = "node_modules"
	dotGit        = ".git"
	configDirName = ".ai-rulez"
	// ownerAiRulez names the winner of a name collision with a skill that ai-rulez
	// defines or fetches (a skill source).
	ownerAiRulez = "an ai-rulez skill or skill source"
)

// DiscoveryReport says what skill discovery found. The ai-rulez://catalog
// resource carries it.
type DiscoveryReport struct {
	// Mode is the discovery mode in effect: "repo" or "all".
	Mode string `json:"mode"`
	// Served lists the repository skills that are served.
	Served []DiscoveredSkill `json:"served"`
	// Shadowed lists the repository skills that are not served because a skill
	// with the same name has precedence.
	Shadowed []ShadowedSkill `json:"shadowed"`
	// Generated counts the skill directories left out because ai-rulez wrote them.
	Generated int `json:"generated_skipped"`
}

// DiscoveredSkill is a repository skill that is served.
type DiscoveredSkill struct {
	// Name is the served name (see namespacedName for a renamed skill).
	Name string `json:"name"`
	// Path is the skill directory relative to the project root, slash separated.
	Path string `json:"path"`
}

// ShadowedSkill is a repository skill that is not served.
type ShadowedSkill struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// ShadowedBy says what has the name: an ai-rulez skill, or another repository
	// skill (by its path).
	ShadowedBy string `json:"shadowed_by"`
}

// Discovery reports what skill discovery found; nil when it did not run.
func (c *Catalog) Discovery() *DiscoveryReport {
	if c == nil {
		return nil
	}
	return c.discovery
}

// discoverMode is the discovery mode of the build: the setup's (the flag), else
// the configuration's [mcp] discover_skills, else off.
func (st *ServeSetup) discoverMode(cfg *config.Config) (string, error) {
	value := st.DiscoverSkills
	if value == "" && cfg != nil && cfg.MCP != nil {
		value = cfg.MCP.DiscoverSkills
	}
	mode, err := config.ParseDiscoverSkills(value)
	return mode, err //nolint:wrapcheck // names the field and the values
}

// discovery is one run of skill discovery.
type discovery struct {
	st     *ServeSetup
	cfg    *config.Config
	report *DiscoveryReport
	// roots are the absolute skill roots whose contents the build depends on.
	roots []string
	// generated holds the absolute paths the manifests record.
	generated map[string]bool
}

// reportOf is the report of a discovery run; nil for no run.
func (d *discovery) reportOf() *DiscoveryReport {
	if d == nil {
		return nil
	}
	return d.report
}

// warnShadowed reports at startup each repository skill that is not served
// because a skill with its name has precedence.
func (st *ServeSetup) warnShadowed(report *DiscoveryReport) {
	if report == nil {
		return
	}
	for _, s := range report.Shadowed {
		st.logger().Warn("A repository skill is shadowed by a skill with the same name and is not served",
			"skill", s.Name, "path", s.Path, "shadowed_by", s.ShadowedBy)
	}
}

// skillDir is a skill directory found in the working tree.
type skillDir struct {
	// rel is the directory relative to the project root, slash separated.
	rel string
	// root is the skill root it was found below (skills, .claude/skills, ...).
	root string
}

func (d skillDir) depth() int { return strings.Count(d.rel, "/") }

// appendDiscovered serves the skills found in the working tree after the
// project's own skills and the skill sources: those keep every name they take.
func (st *ServeSetup) appendDiscovered(ctx context.Context, d *discovery, r *renderedSkills, served []generator.ServedSkill, roleKeeps func(string) bool) []generator.ServedSkill {
	dirs := d.find(ctx)
	owners := map[string]string{}
	for _, dir := range dirs {
		skill, ok := d.load(dir)
		if !ok {
			continue
		}
		name := catalogName(&skill)
		if !validSkillSegment(name) {
			st.logger().Warn("Not serving a repository skill whose name is not a valid skill name", "skill", name, "path", dir.rel)
			continue
		}
		if r.taken[name] {
			holder, byRepo := owners[name]
			if !byRepo {
				d.shadow(name, dir, ownerAiRulez)
				continue
			}
			// Another repository skill has the name: this one is namespaced by its path.
			renamed := namespacedName(dir.rel)
			if r.taken[renamed] || !validSkillSegment(renamed) {
				d.shadow(name, dir, holder)
				continue
			}
			skill.ID, skill.Files[0].Content = renamed, skillsource.RewriteName(skill.Files[0].Content, renamed)
			name = renamed
		}
		if roleKeeps != nil && !roleKeeps(name) {
			continue
		}
		r.taken[name] = true
		owners[name] = dir.rel
		d.report.Served = append(d.report.Served, DiscoveredSkill{Name: name, Path: dir.rel})
		served = append(served, skill)
	}
	return served
}

func (d *discovery) shadow(name string, dir skillDir, by string) {
	d.report.Shadowed = append(d.report.Shadowed, ShadowedSkill{Name: name, Path: dir.rel, ShadowedBy: by})
}

// load reads a discovered skill directory into a served skill. ok is false for
// a directory that is skipped: unreadable, or generated by ai-rulez.
func (d *discovery) load(dir skillDir) (generator.ServedSkill, bool) {
	abs := filepath.Join(d.cfg.BaseDir, filepath.FromSlash(dir.rel))
	files, err := skillsource.ReadSkillDir(d.st.Log, abs)
	if err != nil {
		d.st.logger().Warn("Skipping a repository skill that cannot be read", "path", dir.rel, "error", err.Error())
		return generator.ServedSkill{}, false
	}
	if d.isGenerated(abs, files[0].Content) {
		d.report.Generated++
		return generator.ServedSkill{}, false
	}
	served := make([]generator.ServedSkillFile, 0, len(files))
	for _, f := range files {
		served = append(served, generator.ServedSkillFile{RelPath: f.Path, Content: f.Content, Mode: f.Mode})
	}
	return generator.ServedSkill{
		ID: path.Base(dir.rel), Source: dir.rel, Delivery: config.DeliveryServed, Verbatim: true, Files: served,
	}, true
}

// isGenerated reports whether a skill directory is an output ai-rulez wrote: a
// file in it is recorded in the generated manifest, or its SKILL.md still carries
// the generated header.
func (d *discovery) isGenerated(abs string, skillMD []byte) bool {
	prefix := abs + string(filepath.Separator)
	for p := range d.generated {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return genmark.IsGeneratedOutput(filepath.Join(abs, skillMarkdown), skillMD)
}

// find lists the skill directories of the working tree in precedence order: the
// shallowest first, then by path. Names that two of them claim are settled by
// resolveNames.
func (d *discovery) find(ctx context.Context) []skillDir {
	base := d.cfg.BaseDir
	files := listProjectFiles(ctx, base)
	presets := make([]string, 0, len(d.cfg.Presets))
	for i := range d.cfg.Presets {
		presets = append(presets, d.cfg.Presets[i].GetName())
	}
	rootSet := map[string]bool{}
	for _, dir := range importer.SkillDirectories(presets) {
		rootSet[dir] = true
	}
	skip := d.skippedPrefixes()
	var dirs []skillDir
	for _, f := range files {
		if dir, ok := matchSkillDir(f, rootSet, skip); ok {
			dirs = append(dirs, dir)
		}
	}
	dirs = outermostOnly(dirs)
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].depth() != dirs[j].depth() {
			return dirs[i].depth() < dirs[j].depth()
		}
		return dirs[i].rel < dirs[j].rel
	})
	if len(dirs) > discoverMaxSkills {
		d.st.logger().Warn("Too many repository skills; serving the first ones", "found", len(dirs), "limit", discoverMaxSkills)
		dirs = dirs[:discoverMaxSkills]
	}
	d.watch(dirs, rootSet)
	return dirs
}

// skippedPrefixes are the project-relative directories discovery never enters:
// the configuration directory, whose skills ai-rulez serves itself.
func (d *discovery) skippedPrefixes() []string {
	var skip []string
	if d.cfg.ConfigDir != "" {
		if rel, err := filepath.Rel(d.cfg.BaseDir, d.cfg.ConfigDir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			skip = append(skip, filepath.ToSlash(rel))
		}
	}
	return skip
}

// watch records the skill roots the build depends on, so a change below one
// reloads the catalog: those of the skills found and the root-level ones that exist.
func (d *discovery) watch(dirs []skillDir, rootSet map[string]bool) {
	seen := map[string]bool{}
	add := func(rel string) {
		abs := filepath.Join(d.cfg.BaseDir, filepath.FromSlash(rel))
		if !seen[abs] {
			seen[abs] = true
			d.roots = append(d.roots, abs)
		}
	}
	for _, dir := range dirs {
		add(dir.root)
	}
	candidates := append([]string{rootSkillsDir, claudeDir + "/" + rootSkillsDir}, mapKeys(rootSet)...)
	for _, rel := range candidates {
		if info, err := os.Stat(filepath.Join(d.cfg.BaseDir, filepath.FromSlash(rel))); err == nil && info.IsDir() {
			add(rel)
		}
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// listProjectFiles lists the files below base that are not ignored: git's own
// listing (tracked files and untracked files that .gitignore does not exclude)
// in a repository, else a walk that skips .git and node_modules.
func listProjectFiles(ctx context.Context, base string) []string {
	if files, ok, err := (gitutil.Git{}).ListFilesContext(ctx, base); ok && err == nil {
		return files
	}
	var files []string
	_ = filepath.WalkDir(base, func(p string, entry fs.DirEntry, err error) error { //nolint:errcheck // an unreadable entry is left out
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is left out
		}
		if entry.IsDir() {
			if name := entry.Name(); name == dotGit || name == nodeModules {
				return fs.SkipDir
			}
			return nil
		}
		if rel, relErr := filepath.Rel(base, p); relErr == nil && entry.Type().IsRegular() {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= discoverMaxWalk {
			return fs.SkipAll
		}
		return nil
	})
	return files
}

// matchSkillDir reports the skill directory a listed file marks: a SKILL.md
// below a skill root, at most discoverMaxDepth levels down.
func matchSkillDir(file string, roots map[string]bool, skip []string) (skillDir, bool) {
	if path.Base(file) != skillMarkdown {
		return skillDir{}, false
	}
	dir := path.Dir(file)
	segments := strings.Split(dir, "/")
	for _, seg := range segments {
		if seg == configDirName || seg == dotGit || seg == nodeModules {
			return skillDir{}, false
		}
	}
	for _, prefix := range skip {
		if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
			return skillDir{}, false
		}
	}
	for n := 1; n < len(segments); n++ {
		if len(segments)-n > discoverMaxDepth {
			continue
		}
		if root := path.Join(segments[:n]...); isSkillRoot(segments[:n], roots) {
			return skillDir{rel: dir, root: root}, true
		}
	}
	return skillDir{}, false
}

// isSkillRoot reports whether a path is a place skills live: the root skills
// directory, a skill directory of a configured preset, or .claude/skills at any depth.
func isSkillRoot(segments []string, roots map[string]bool) bool {
	joined := strings.Join(segments, "/")
	if joined == rootSkillsDir || roots[joined] {
		return true
	}
	n := len(segments)
	return n >= 2 && segments[n-2] == claudeDir && segments[n-1] == rootSkillsDir
}

// outermostOnly drops a skill directory that sits inside another one: its
// SKILL.md belongs to the skill around it (an example, a template).
func outermostOnly(dirs []skillDir) []skillDir {
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].rel < dirs[j].rel })
	var kept []skillDir
	for _, dir := range dirs {
		if n := len(kept); n > 0 && strings.HasPrefix(dir.rel, kept[n-1].rel+"/") {
			continue
		}
		kept = append(kept, dir)
	}
	return kept
}

// namespacedName is the name a repository skill gets when another repository
// skill already has its name: the skill directory's path from the project root,
// with the separators turned into dots and the leading dot of each segment
// dropped (pkg/.claude/skills/baz is served as pkg.claude.skills.baz).
func namespacedName(rel string) string {
	segments := strings.Split(rel, "/")
	for i, seg := range segments {
		segments[i] = strings.TrimLeft(seg, ".")
	}
	return strings.Join(segments, ".")
}

// newDiscovery prepares a discovery run for a build. gen is nil when the build
// has no project (a server of --source skills only): there is then no manifest.
func (st *ServeSetup) newDiscovery(cfg *config.Config, gen *generator.Generator, mode string) *discovery {
	d := &discovery{st: st, cfg: cfg, report: &DiscoveryReport{Mode: mode}, generated: map[string]bool{}}
	if gen != nil {
		for _, p := range gen.GeneratedPaths() {
			d.generated[filepath.Clean(p)] = true
		}
	}
	return d
}
