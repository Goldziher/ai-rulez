package lint

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// stageFilter selects which files of a staged skill directory reach the scanner
// (stage_globs, stage_exclude). The zero value keeps every file. SKILL.md is the
// skill itself and is never filtered.
type stageFilter struct {
	globs, exclude []globMatcher
	// unchanged, when set, reports a skill none of whose files can have changed
	// (--changed, --since). Such a skill is neither read nor expanded: the run
	// skips it, so only its SKILL.md is listed to keep the skill directory known.
	unchanged func(*item) bool
}

func newStageFilter(ex config.LintExternal) stageFilter {
	return stageFilter{globs: compileGlobs(ex.StageGlobs), exclude: compileGlobs(ex.StageExclude)}
}

// keep reports whether the file at rel (slash path below the skill directory) is staged.
func (f stageFilter) keep(rel string) bool {
	for _, g := range f.exclude {
		if g.match(rel) {
			return false
		}
	}
	if len(f.globs) == 0 {
		return true
	}
	for _, g := range f.globs {
		if g.match(rel) {
			return true
		}
	}
	return false
}

// stageGlobProblems lists what is wrong with an entry's stage_globs and stage_exclude.
func stageGlobProblems(ex config.LintExternal) []string {
	problems := globListProblems("stage_globs", "skill directory", ex.StageGlobs)
	return append(problems, globListProblems("stage_exclude", "skill directory", ex.StageExclude)...)
}

// globListProblems lists the patterns of key that are empty, absolute, leave the
// scope (a ".." segment) or are not valid globs.
func globListProblems(key, scope string, patterns []string) []string {
	var problems []string
	for _, p := range patterns {
		clean := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
		switch {
		case clean == "":
			problems = append(problems, key+" holds an empty pattern")
		case strings.HasPrefix(clean, "/") || (len(clean) > 1 && clean[1] == ':'):
			problems = append(problems, fmt.Sprintf("%s %q must be relative to the %s", key, p, scope))
		case hasDotDot(clean):
			problems = append(problems, fmt.Sprintf("%s %q must stay inside the %s", key, p, scope))
		case malformedGlob(clean):
			problems = append(problems, fmt.Sprintf("%s %q is not a valid glob", key, p))
		}
	}
	return problems
}

// stageSkillFiles lists the files of a skill to stage: the resources the loader
// found (references/, scripts/, assets/) and every other regular file of an
// owned skill's directory, so a file at the skill root or in a folder of its own
// is scanned too. The extra files obey the installed-skill limits (symbolic
// links, credential-like names, size, bundle excludes). The result is kept per
// directory for the run.
func (r *runner) stageSkillFiles(it *item) []config.SkillResource {
	if !it.owned || it.kind != kindSkill || it.itemDir == "" {
		return it.cf.Resources
	}
	if cached, ok := r.skillFiles.Load(it.itemDir); ok {
		return cached.([]config.SkillResource) //nolint:forcetypeassert,errcheck // only this function stores here
	}
	all := append([]config.SkillResource(nil), it.cf.Resources...)
	have := map[string]bool{}
	for _, res := range all {
		have[path.Clean(strings.ReplaceAll(res.RelPath, `\`, "/"))] = true
	}
	ctx := logger.WithContext(scanContext(), r.log())
	extra, err := config.LoadAllSkillFiles(ctx, it.itemDir, r.cfg.BundleExclude)
	if err != nil {
		r.log().Warn("Staging only the canonical resources of a skill", "skill", it.itemDir, "error", err)
	}
	for _, res := range extra {
		if rel := path.Clean(strings.ReplaceAll(res.RelPath, `\`, "/")); !have[rel] {
			have[rel] = true
			all = append(all, res)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].RelPath < all[j].RelPath })
	r.skillFiles.Store(it.itemDir, all)
	return all
}
