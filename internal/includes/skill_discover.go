package includes

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// discoverSkillDirs lists the skill directories an installed_skills entry
// selected from root: root itself when it holds a SKILL.md, otherwise its
// immediate subdirectories that do. include and exclude are globs matched
// against the directory name (exclude wins); the result is sorted by directory
// name. Symlinks are never followed.
//
// This mirrors internal/skillsource.Discover. It is reimplemented here rather
// than imported because internal/skillsource imports internal/includes, and the
// reverse import would cycle.
func discoverSkillDirs(root string, include, exclude []string) []string {
	if hasSkillMarker(root) {
		return []string{root}
	}
	var chosen []string
	for _, dir := range skillSubdirs(root) {
		if selectedSkillDir(filepath.Base(dir), include, exclude) {
			chosen = append(chosen, dir)
		}
	}
	sort.Strings(chosen)
	return chosen
}

// skillSubdirs returns the immediate non-hidden subdirectories of root that hold
// a regular SKILL.md, sorted by directory name. A symlinked entry is skipped:
// it could point out of the source.
func skillSubdirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if hasSkillMarker(dir) {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
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

// scanSelectedSkills scans the skill directories an entry selected into
// ContentFiles. A root that holds a SKILL.md is one skill named namePrefix +
// entryName (so a path that names a skill directory behaves as before, the
// prefix defaulting to the empty string). Otherwise each selected subdirectory
// becomes a skill named namePrefix + the directory name. A skill that cannot be
// read is skipped with a warning rather than failing the whole entry.
func scanSelectedSkills(ctx context.Context, root, namePrefix, entryName string, include, exclude []string) ([]config.ContentFile, error) {
	log := logger.FromContext(ctx)
	if hasSkillMarker(root) {
		cf, err := ScanInstalledSkillDir(ctx, root, namePrefix+entryName)
		if err != nil {
			return nil, err
		}
		return []config.ContentFile{cf}, nil
	}
	var out []config.ContentFile
	for _, dir := range discoverSkillDirs(root, include, exclude) {
		cf, err := ScanInstalledSkillDir(ctx, dir, namePrefix+filepath.Base(dir))
		if err != nil {
			log.Warn("Skipping a skill in an installed skill entry", "path", dir, "error", err)
			continue
		}
		out = append(out, cf)
	}
	return out, nil
}
