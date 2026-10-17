package includes

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

const (
	// DefaultMaxSkills bounds the skills one installed_skills entry or one
	// [[skill_sources]] entry loads.
	DefaultMaxSkills = 200
	// maxSkillDepth bounds how far below a root a skill directory is looked for.
	maxSkillDepth = 6
	// maxSkillHits bounds the skill directories one walk collects before
	// include/exclude pick from them, so a huge tree cannot grow the walk without limit.
	maxSkillHits = 5000
	nodeModules  = "node_modules"
)

// ConventionalSkillDirs lists the directories inside a repository where agent
// harnesses keep skills, in probing order. internal/importer's layout table
// (every preset's project skills directory) is the canonical list; a test there
// checks that each entry is one of them.
func ConventionalSkillDirs() []string {
	return []string{
		"skills",
		".claude/skills",
		".agents/skills",
		".codex/skills",
		".cursor/skills",
		".gemini/skills",
		".github/skills",
		".opencode/skills",
	}
}

// skillHit is a directory holding a SKILL.md, found below one of the roots.
type skillHit struct {
	dir   string
	depth int
	root  int
}

// discoverSkillDirs lists the skill directories an installed_skills entry
// selected from roots: every directory at or below a root that holds a SKILL.md,
// to maxSkillDepth levels. A skill
// directory is not searched further. include and exclude are globs matched
// against the directory name (exclude wins). When two skills share a directory
// name the shallowest wins, ties going to the earlier root and then the path; the
// others are skipped with a warning. The result is sorted by directory name.
// Symlinks, hidden directories, node_modules are never entered.
//
// This mirrors internal/skillsource.Discover. It is reimplemented here rather
// than imported because internal/skillsource imports internal/includes, and the
// reverse import would cycle.
func discoverSkillDirs(ctx context.Context, roots []string, include, exclude []string) ([]string, error) {
	log := logger.FromContext(ctx)
	var hits []skillHit
	for i, root := range roots {
		walkSkillDirs(root, 0, i, &hits)
	}
	if len(hits) >= maxSkillHits {
		log.Warn("Stopped looking for skills after the limit; later skills in name order are not considered", "limit", maxSkillHits)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		if a.root != b.root {
			return a.root < b.root
		}
		return a.dir < b.dir
	})
	taken := map[string]string{}
	var chosen []string
	for _, h := range hits {
		name := filepath.Base(h.dir)
		if !selectedSkillDir(name, include, exclude) {
			continue
		}
		if first, dup := taken[name]; dup {
			log.Warn("Skipping a skill whose directory name another skill already has", "name", name, "kept", first, "skipped", h.dir)
			continue
		}
		taken[name] = h.dir
		chosen = append(chosen, h.dir)
	}
	if len(chosen) > DefaultMaxSkills {
		return nil, oops.Hint("Narrow the entry with `include`/`exclude` or `path`").
			Errorf("installed skill source holds %d skills, more than the limit of %d", len(chosen), DefaultMaxSkills)
	}
	sort.Slice(chosen, func(i, j int) bool {
		bi, bj := filepath.Base(chosen[i]), filepath.Base(chosen[j])
		if bi != bj {
			return bi < bj
		}
		return chosen[i] < chosen[j]
	})
	return chosen, nil
}

// walkSkillDirs appends dir to hits when it holds a SKILL.md, and otherwise
// searches its subdirectories. Entries are visited in name order.
func walkSkillDirs(dir string, depth, root int, hits *[]skillHit) {
	if len(*hits) >= maxSkillHits {
		return
	}
	if hasSkillMarker(dir) {
		*hits = append(*hits, skillHit{dir: dir, depth: depth, root: root})
		return
	}
	if depth >= maxSkillDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() || strings.HasPrefix(name, ".") || name == nodeModules {
			continue
		}
		walkSkillDirs(filepath.Join(dir, name), depth+1, root, hits)
	}
}

// holdsSkills reports whether dir is a skill or has one below it.
func holdsSkills(dir string) bool {
	var hits []skillHit
	walkSkillDirs(dir, 0, 0, &hits)
	return len(hits) > 0
}

// probeRoots finds where a source keeps its skills when the entry names no path:
// base itself when it holds a SKILL.md, else each conventional harness directory
// below base that holds skills. A directory reached through a symlink is skipped.
func probeRoots(base string) []string {
	if hasSkillMarker(base) {
		return []string{base}
	}
	var roots []string
	for _, rel := range ConventionalSkillDirs() {
		dir := filepath.Join(base, filepath.FromSlash(rel))
		if !pathHasSymlink(logger.Or(nil), base, dir) && holdsSkills(dir) {
			roots = append(roots, dir)
		}
	}
	return roots
}

// selectedSkillDir reports whether a skill directory name passes the include and
// exclude globs; exclude wins, and an empty include list selects everything.
func selectedSkillDir(name string, include, exclude []string) bool {
	match := func(globs []string) bool {
		for _, g := range globs {
			if ok, err := path.Match(strings.TrimSpace(g), name); err == nil && ok {
				return true
			}
		}
		return false
	}
	if match(exclude) {
		return false
	}
	return len(include) == 0 || match(include)
}

// scanEntrySkills scans the skill directories an entry selected into
// ContentFiles. A base that holds a SKILL.md is one skill named namePrefix +
// entryName (so a path that names a skill directory behaves as before, the
// prefix defaulting to the empty string). Otherwise each selected skill
// directory becomes a skill named namePrefix + the directory name. When probed,
// base is a whole source and its skills are looked for in the conventional
// harness directories; an entry that selects no glob then names one skill, the
// one whose directory is called entryName. A skill that cannot be read is
// skipped with a warning rather than failing the whole entry. An entry that asked
// for skills by glob but matched none is an error: silently resolving nothing
// would drop the entry's outputs without a trace.
func scanEntrySkills(ctx context.Context, base string, probed bool, namePrefix, entryName string, include, exclude []string) ([]config.ContentFile, error) {
	log := logger.FromContext(ctx)
	if hasSkillMarker(base) {
		cf, err := scanPrefixedSkillDir(ctx, base, namePrefix, entryName)
		if err != nil {
			return nil, err
		}
		return []config.ContentFile{cf}, nil
	}
	roots := []string{base}
	if probed {
		roots = probeRoots(base)
	}
	dirs, err := discoverSkillDirs(ctx, roots, include, exclude)
	if err != nil {
		return nil, err
	}
	if probed && !selectsSeveral(include, exclude) {
		dirs, err = skillNamed(dirs, entryName, base)
		if err != nil {
			return nil, err
		}
	}
	if len(dirs) == 0 && selectsSeveral(include, exclude) {
		return nil, oops.
			With("path", base).
			Errorf("installed skill %q: no skill directory under %s matched include=%v exclude=%v", entryName, base, include, exclude)
	}
	var out []config.ContentFile
	for _, dir := range dirs {
		cf, err := scanPrefixedSkillDir(ctx, dir, namePrefix, filepath.Base(dir))
		var limit *config.SkillBundleLimitError
		if errors.As(err, &limit) {
			return nil, err //nolint:wrapcheck // names the limit and how to narrow it
		}
		if err != nil {
			log.Warn("Skipping a skill in an installed skill entry", "path", dir, "error", err)
			continue
		}
		out = append(out, cf)
	}
	return out, nil
}

// scanPrefixedSkillDir is ScanInstalledSkillDir for a skill named namePrefix +
// name. The prefix applies to the output id (the skill's directory in the
// generated trees) as well as to the name, so two entries with different
// prefixes never write to the same path.
func scanPrefixedSkillDir(ctx context.Context, skillDir, namePrefix, name string) (config.ContentFile, error) {
	cf, err := ScanInstalledSkillDir(ctx, skillDir, namePrefix+name)
	if err == nil && namePrefix != "" {
		cf.ID = namePrefix + filepath.Base(skillDir)
	}
	return cf, err
}

// skillNamed picks the skill directory called name among the ones a probe found.
func skillNamed(dirs []string, name, base string) ([]string, error) {
	var found []string
	for _, d := range dirs {
		if filepath.Base(d) == name {
			return []string{d}, nil
		}
		found = append(found, filepath.Base(d))
	}
	return nil, oops.
		With("path", base).
		Hint("Name the skill directory with `name`, select several with `include`, or give a `path`").
		Errorf("installed skill %q: no skill directory called %q in %s (found: %s)", name, name, base, strings.Join(found, ", "))
}

// selectsSeveral reports whether an entry picks skills by glob (include/exclude)
// rather than naming a single skill directory.
func selectsSeveral(include, exclude []string) bool {
	return len(include) > 0 || len(exclude) > 0
}
