package mcp

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// saveGuard refuses a rebuild made while the SKILL.md of a served skill is
// caught mid-save: an editor that truncates the file and then writes it leaves an
// empty file, or a frontmatter that is not closed yet, for a moment. Swapping in
// such a catalog would make the skill vanish (its delivery key is gone) or serve
// it empty, so the rebuild counts as failed and the previous catalog keeps
// serving until the save completes. A deleted SKILL.md, or a frontmatter that is
// complete but invalid, is a real edit and is not affected; neither is a file
// that was already incomplete when the current catalog was built, so a skill
// left that way does not stop every later reload.
type saveGuard struct {
	roots []string
	known map[string]bool
}

func newSaveGuard(roots []string, current *Catalog) *saveGuard {
	g := &saveGuard{roots: roots, known: map[string]bool{}}
	partial, err := partialSkillFiles(current, roots)
	if err == nil {
		for _, p := range partial {
			g.known[p] = true
		}
	}
	return g
}

// check fails while a skill of current has a SKILL.md that became incomplete
// since current was built; otherwise it records the incomplete files and passes.
func (g *saveGuard) check(current *Catalog) error {
	partial, err := partialSkillFiles(current, g.roots)
	if err != nil {
		return err
	}
	var fresh []string
	for _, p := range partial {
		if !g.known[p] {
			fresh = append(fresh, p)
		}
	}
	if len(fresh) > 0 {
		return oops.Errorf("%s is empty or its frontmatter is not closed (a save in progress?); keeping the previous version", strings.Join(fresh, ", "))
	}
	g.known = map[string]bool{}
	for _, p := range partial {
		g.known[p] = true
	}
	return nil
}

// partialSkillFiles lists, sorted, the SKILL.md files below roots that belong to
// a skill of current (by directory name) and look cut short by a save.
func partialSkillFiles(current *Catalog, roots []string) ([]string, error) {
	if current == nil || len(current.byName) == 0 {
		return nil, nil
	}
	served := current.byName
	var partial []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // an unreadable entry is not a partial save
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return fs.SkipDir
				}
				return nil
			}
			if d.Name() != skillMarkdown || served[filepath.Base(filepath.Dir(p))] == nil {
				return nil
			}
			content, err := os.ReadFile(p) //nolint:gosec // a file under a watched skill root
			if err == nil && incompleteSkillFile(content) {
				partial = append(partial, p)
			}
			return nil
		})
		if err != nil {
			return nil, oops.Wrapf(err, "check skill files")
		}
	}
	sort.Strings(partial)
	return partial, nil
}

// incompleteSkillFile reports whether a SKILL.md looks cut short by a save in
// progress: empty, part of the opening delimiter, or a frontmatter never closed.
func incompleteSkillFile(content []byte) bool {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if strings.TrimSpace(text) == "" || strings.HasPrefix("---\n", text) {
		return true
	}
	rest, ok := strings.CutPrefix(text, "---\n")
	return ok && !strings.Contains(rest, "\n---")
}
