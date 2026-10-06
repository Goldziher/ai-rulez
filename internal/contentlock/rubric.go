package contentlock

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// KindRubric pins one review rubric directory (.ai-rulez/rubrics/<id>/): the digest covers every
// file under it (rubric.toml, system.md, the golden cases and their fixtures, calibration.json),
// so a silent edit of a rubric, its prompt or its labels, and a re-calibration, show in review.
// The id is the directory name; Path is the directory relative to the configuration directory.
const KindRubric = "rubric"

// rubricsDir is the directory of user rubrics under the configuration directory.
const rubricsDir = "rubrics"

// collectRubrics pins the project's own rubric directories. A symlink, a file that cannot be
// read or a directory that cannot be walked is a problem a check fails on, never a skipped file.
func (c *collector) collectRubrics() error {
	if c.cfg.ConfigDir == "" {
		return nil
	}
	root := filepath.Join(c.cfg.ConfigDir, rubricsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		c.problems = append(c.problems, "the rubrics directory cannot be read, so rubrics cannot be pinned: "+err.Error())
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := rubricsDir + "/" + e.Name()
		dir := filepath.Join(root, e.Name())
		if info, lerr := os.Lstat(dir); lerr != nil || info.Mode()&os.ModeSymlink != 0 {
			c.problems = append(c.problems, "rubric directory "+rel+" is a symlink, so it cannot be pinned")
			continue
		}
		leaves, problems := c.rubricLeaves(dir, rel)
		c.problems = append(c.problems, problems...)
		if len(leaves) == 0 {
			continue
		}
		digest, derr := TreeDigest(KindRubric, leaves)
		if derr != nil {
			c.problems = append(c.problems, "rubric "+e.Name()+" cannot be pinned: "+derr.Error())
			continue
		}
		c.items = append(c.items, lockfile.Item{Kind: KindRubric, ID: e.Name(), Path: rel, Digest: digest})
	}
	return nil
}

// rubricLeaves lists the regular files under a rubric directory.
func (c *collector) rubricLeaves(dir, rel string) ([]Leaf, []string) {
	var leaves []Leaf
	var problems []string
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			problems = append(problems, "rubric path "+rel+" cannot be read: "+err.Error())
			return nil
		}
		if d.IsDir() {
			return nil
		}
		sub, _ := filepath.Rel(dir, p)
		sub = filepath.ToSlash(sub)
		info, ierr := d.Info()
		if ierr != nil || !info.Mode().IsRegular() {
			problems = append(problems, "rubric file "+rel+"/"+sub+" is not a regular file, so it cannot be pinned")
			return nil
		}
		data, rerr := os.ReadFile(p) //nolint:gosec // a regular file inside the rubric directory
		if rerr != nil {
			problems = append(problems, "rubric file "+rel+"/"+sub+" cannot be read: "+rerr.Error())
			return nil
		}
		leaves = append(leaves, Leaf{Path: sub, Mode: c.modes.mode(c.configRoot(), p, info), Data: data})
		return nil
	})
	if walkErr != nil {
		problems = append(problems, "rubric "+rel+" cannot be walked: "+walkErr.Error())
	}
	sort.Slice(leaves, func(i, j int) bool { return strings.Compare(leaves[i].Path, leaves[j].Path) < 0 })
	return leaves, problems
}
