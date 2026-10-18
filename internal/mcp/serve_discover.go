package mcp

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// Skill discovery (`mcp --serve-skills --discover-skills`, [mcp] discover_skills):
// the skill directories a repository already has are served beside the skills
// ai-rulez defines.

const (
	// discoverMaxSkills bounds the skill directories one discovery serves.
	discoverMaxSkills = 500

	rootSkillsDir = "skills"
	claudeDir     = ".claude"
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
	// Renamed lists the repository skills served under their path because
	// another repository skill kept their name.
	Renamed []RenamedSkill `json:"renamed,omitempty"`
	// Generated counts the skill directories left out because ai-rulez wrote them.
	Generated int `json:"generated_skipped"`
	// Index sizes the repository index the discovery built.
	Index *IndexReport `json:"index,omitempty"`
}

// DiscoveredSkill is a repository skill that is served.
type DiscoveredSkill struct {
	// Name is the served name (see namespacedName for a renamed skill).
	Name string `json:"name"`
	// Path is the skill directory relative to the project root, slash separated.
	Path string `json:"path"`
	// Scope is the repository scope the skill lives in (the directory that holds
	// its convention directory); empty for a skill ai-rulez defines.
	Scope string `json:"scope"`
}

// ShadowedSkill is a repository skill that is not served.
type ShadowedSkill struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// ShadowedBy says what has the name: an ai-rulez skill, or another repository
	// skill (by its path).
	ShadowedBy string `json:"shadowed_by"`
}

// RenamedSkill is a repository skill served under another name than its own.
type RenamedSkill struct {
	// Name is the name the skill declares.
	Name string `json:"name"`
	// ServedAs is the name it is served under.
	ServedAs string `json:"served_as"`
	// Path is the skill directory, relative to the project root.
	Path string `json:"path"`
	// KeptBy is the directory of the repository skill that kept Name.
	KeptBy string `json:"kept_by"`
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

// checkExclude validates the --exclude globs and refuses them without discovery,
// where they would silently do nothing.
func (st *ServeSetup) checkExclude(mode string) error {
	if len(st.Exclude) == 0 {
		return nil
	}
	if mode == config.DiscoverOff {
		return oops.
			Hint("Add --discover-skills (or set [mcp] discover_skills), or drop --exclude").
			Errorf("--exclude applies to repository discovery, which is off")
	}
	return config.ValidateExcludeGlobs(st.Exclude) //nolint:wrapcheck // names the field and the glob
}

// excludeGlobs are the globs discovery leaves out: [mcp] exclude, then the
// --exclude flag's, without repeats.
func (st *ServeSetup) excludeGlobs(cfg *config.Config) []string {
	var globs []string
	if cfg != nil && cfg.MCP != nil {
		globs = append(globs, cfg.MCP.Exclude...)
	}
	globs = append(globs, st.Exclude...)
	seen := make(map[string]bool, len(globs))
	out := globs[:0:0]
	for _, g := range globs {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
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
	// index is the repository index of this run; nil until find ran.
	index *repoindex.Index
	// store is the index cache of this run; nil until find ran.
	store *indexcache.Store
	// cacheWarning says why the cache is not where it was asked to be.
	cacheWarning string
}

// reportOf is the report of a discovery run; nil for no run.
func (d *discovery) reportOf() *DiscoveryReport {
	if d == nil {
		return nil
	}
	return d.report
}

// warnIndexLimit reports at startup that the repository has more items than the
// index holds, so a planner is not told a scope is empty when it was cut.
func (st *ServeSetup) warnIndexLimit(report *DiscoveryReport) {
	if report == nil || report.Index == nil || !report.Index.Truncated {
		return
	}
	st.logger().Warn("The repository index is over its limit and holds only the shallowest scopes",
		"found", report.Index.Found, "limit", report.Index.Limit)
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

// warnRenamed reports at startup each repository skill served under its path
// because another repository skill kept its name.
func (st *ServeSetup) warnRenamed(report *DiscoveryReport) {
	if report == nil {
		return
	}
	for _, r := range report.Renamed {
		st.logger().Warn("A repository skill shares its name with another and is served under its path",
			"skill", r.Name, "served_as", r.ServedAs, "path", r.Path, "kept_by", r.KeptBy)
	}
}

// appendDiscovered serves the skills found in the working tree after the
// project's own skills and the skill sources: those keep every name they take.
func (st *ServeSetup) appendDiscovered(ctx context.Context, d *discovery, r *renderedSkills, served []generator.ServedSkill, roleKeeps func(string) bool) []generator.ServedSkill {
	items := d.find(ctx)
	owners := map[string]string{}
	for _, item := range items {
		skill, ok := d.load(item)
		if !ok {
			continue
		}
		name := catalogName(&skill)
		if !validSkillSegment(name) {
			st.logger().Warn("Not serving a repository skill whose name is not a valid skill name", "skill", name, "path", item.Path)
			continue
		}
		if source, isBuiltin := r.builtin[name]; isBuiltin {
			// A repository skill is the project's own: it replaces a builtin of
			// the same name, as a skill under .ai-rulez/skills does.
			served = withoutSkill(served, name, source)
			delete(r.builtin, name)
			delete(r.taken, name)
			st.logger().Info("A repository skill replaces a builtin skill with the same name",
				"skill", name, "path", item.Path, "replaced", source)
		}
		if r.taken[name] {
			holder, byRepo := owners[name]
			if !byRepo {
				d.shadow(name, item, ownerAiRulez)
				continue
			}
			// Another repository skill has the name: this one is namespaced by its path.
			renamed := repoindex.NamespacedName(item.Path)
			if r.taken[renamed] || !validSkillSegment(renamed) {
				d.shadow(name, item, holder)
				continue
			}
			skill.ID, skill.Files[0].Content = renamed, skillsource.RewriteName(skill.Files[0].Content, renamed)
			d.report.Renamed = append(d.report.Renamed, RenamedSkill{Name: name, ServedAs: renamed, Path: item.Path, KeptBy: holder})
			name = renamed
		}
		if roleKeeps != nil && !roleKeeps(name) {
			continue
		}
		r.taken[name] = true
		owners[name] = item.Path
		d.report.Served = append(d.report.Served, DiscoveredSkill{Name: name, Path: item.Path, Scope: item.Scope})
		served = append(served, skill)
	}
	return served
}

// withoutSkill returns served without the skill with the catalog name and source.
func withoutSkill(served []generator.ServedSkill, name, source string) []generator.ServedSkill {
	out := make([]generator.ServedSkill, 0, len(served))
	for i := range served {
		if catalogName(&served[i]) == name && served[i].Source == source {
			continue
		}
		out = append(out, served[i])
	}
	return out
}

func (d *discovery) shadow(name string, item repoindex.Item, by string) {
	d.report.Shadowed = append(d.report.Shadowed, ShadowedSkill{Name: name, Path: item.Path, ShadowedBy: by})
}

// load reads a discovered skill directory into a served skill. ok is false for
// a directory that is skipped: unreadable.
func (d *discovery) load(item repoindex.Item) (generator.ServedSkill, bool) {
	abs := filepath.Join(d.cfg.BaseDir, filepath.FromSlash(item.Path))
	files, err := skillsource.ReadSkillDir(d.st.Log, abs)
	if err != nil {
		d.st.logger().Warn("Skipping a repository skill that cannot be read", "path", item.Path, "error", err.Error())
		return generator.ServedSkill{}, false
	}
	served := make([]generator.ServedSkillFile, 0, len(files))
	for _, f := range files {
		served = append(served, generator.ServedSkillFile{RelPath: f.Path, Content: f.Content, Mode: f.Mode})
	}
	return generator.ServedSkill{
		ID: path.Base(item.Path), Source: item.Path, Delivery: config.DeliveryServed, Verbatim: true, Files: served,
	}, true
}

// isGenerated reports whether a file the index found is an output ai-rulez
// wrote: it is recorded in the generated manifest (for a skill, any file of its
// directory is), or it still carries the generated header.
func (d *discovery) isGenerated(file string, content []byte) bool {
	if filepath.Base(file) == skillMarkdown {
		prefix := filepath.Dir(file) + string(filepath.Separator)
		for p := range d.generated {
			if strings.HasPrefix(p, prefix) {
				return true
			}
		}
	} else if d.generated[filepath.Clean(file)] {
		return true
	}
	return genmark.IsGeneratedOutput(file, content)
}

// find indexes the working tree and lists its skill directories in precedence
// order: the shallowest first, then by path. Names that two of them claim are
// settled by appendDiscovered.
func (d *discovery) find(ctx context.Context) []repoindex.Item {
	presets := make([]string, 0, len(d.cfg.Presets))
	for i := range d.cfg.Presets {
		presets = append(presets, d.cfg.Presets[i].GetName())
	}
	d.store, d.cacheWarning = d.st.openIndexCache(ctx, d.cfg)
	if d.cacheWarning != "" {
		d.st.logger().Warn("The index cache is not where it was asked to be", "reason", d.cacheWarning)
	}
	opts := repoindex.Options{
		Root: d.cfg.BaseDir, Presets: presets, Skip: d.skippedPrefixes(), Exclude: d.st.excludeGlobs(d.cfg), Generated: d.isGenerated, Screen: d.screen,
		MaxItems: d.st.indexLimit,
	}
	if d.store.Enabled() {
		opts.Cache, opts.ScreenKnown = d.store, d.screenKnown
	}
	ix, err := repoindex.Build(ctx, opts)
	if err != nil {
		d.st.logger().Warn("Could not index the repository for skills", "error", err.Error())
		return nil
	}
	d.index = ix
	d.report.Index = &IndexReport{Items: len(ix.Items()), Limit: ix.Limit(), Truncated: ix.Truncated(), Found: ix.Found()}
	d.report.Index.Cache = reportOfCache(d.store, d.cacheWarning)
	for _, p := range ix.Problems() {
		if path.Base(p.Path) == skillMarkdown {
			d.st.logger().Warn("Skipping a repository skill that cannot be read", "path", path.Dir(p.Path), "error", p.Err)
		}
	}
	refusals := ix.Refusals()
	for i := range refusals {
		d.st.logger().Warn("Withholding repository context: the security scan found an error", "kind", string(refusals[i].Item.Kind), "path", refusals[i].Item.Path, "reason", refusals[i].Reason)
	}
	d.report.Generated += ix.GeneratedSkipped(repoindex.KindSkill)
	skills := ix.Skills()
	if len(skills) > discoverMaxSkills {
		d.st.logger().Warn("Too many repository skills; serving the first ones", "found", len(skills), "limit", discoverMaxSkills)
		skills = skills[:discoverMaxSkills]
	}
	d.watch(ix.SkillRoots())
	return skills
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
func (d *discovery) watch(found []string) {
	seen := map[string]bool{}
	add := func(rel string) {
		abs := filepath.Join(d.cfg.BaseDir, filepath.FromSlash(rel))
		if !seen[abs] {
			seen[abs] = true
			d.roots = append(d.roots, abs)
		}
	}
	for _, rel := range found {
		add(rel)
	}
	for _, rel := range d.rootCandidates() {
		if info, err := os.Stat(filepath.Join(d.cfg.BaseDir, filepath.FromSlash(rel))); err == nil && info.IsDir() {
			add(rel)
		}
	}
}

// rootCandidates are the root-level skill directories whose appearance should reload the catalog.
func (d *discovery) rootCandidates() []string {
	presets := make([]string, 0, len(d.cfg.Presets))
	for i := range d.cfg.Presets {
		presets = append(presets, d.cfg.Presets[i].GetName())
	}
	return append([]string{rootSkillsDir, claudeDir + "/" + rootSkillsDir}, importer.SkillDirectories(presets)...)
}

// newDiscovery prepares a discovery run for a build. gen is nil when the build
// has no project (a server of --source skills only): there is then no manifest.
func (st *ServeSetup) newDiscovery(cfg *config.Config, gen *generator.Generator, mode string) *discovery {
	d := &discovery{st: st, cfg: cfg, report: &DiscoveryReport{Mode: mode, Served: []DiscoveredSkill{}, Shadowed: []ShadowedSkill{}}, generated: map[string]bool{}}
	if gen != nil {
		for _, p := range gen.GeneratedPaths() {
			d.generated[filepath.Clean(p)] = true
		}
	}
	return d
}

// configDirSkip is the configuration directory relative to the project root,
// as a list: the file watcher leaves it to the poll of the configuration.
func configDirSkip(cfg *config.Config) []string {
	d := &discovery{cfg: cfg}
	return d.skippedPrefixes()
}
