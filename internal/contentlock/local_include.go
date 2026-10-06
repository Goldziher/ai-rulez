package contentlock

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// localIncludeDirs are the top-level directories of an include tree that become
// content. Everything else in the directory (its own config, lock, build output)
// is not content and does not change the pin.
var localIncludeDirs = map[string]bool{
	"rules": true, "context": true, "skills": true, "agents": true,
	"commands": true, "checks": true, "domains": true, "verifiers": true,
}

const aiRulezDirName = ".ai-rulez"

// collectLocalIncludes pins the content tree of every include whose source is a
// local path. Remote includes are pinned by commit and digest; a local include
// outside the configuration directory is otherwise read unpinned, so editing
// the shared directory would change generated output without touching the lock.
// A tree that cannot be pinned (missing, or containing a symlink) is recorded as
// a problem, so a check fails on it.
func (c *collector) collectLocalIncludes() error {
	for i := range c.cfg.Includes {
		inc := &c.cfg.Includes[i]
		if lockfile.IsGitSource(inc.Source) || inc.Source == "" || inc.LocalOverride != "" {
			continue
		}
		dir, err := c.localIncludeDir(inc)
		if err != nil {
			c.problems = append(c.problems, "local include "+inc.Name+" cannot be pinned: "+err.Error())
			continue
		}
		kind, keep := KindInclude, localIncludeDirs
		if inc.Format == config.IncludeFormatOKF {
			kind, keep = KindOKFInclude, nil // a bundle has its own layout: pin it whole
		}
		digest, err := digestTree(kind, dir, keep)
		if err != nil {
			c.problems = append(c.problems, "local include "+inc.Name+" cannot be pinned: "+err.Error())
			continue
		}
		c.items = append(c.items, lockfile.Item{
			Kind: KindLocalInclude, ID: inc.Name, Path: filepath.ToSlash(inc.Source), Digest: digest,
		})
	}
	return nil
}

// localIncludeDir resolves the directory a local include is read from, the way
// the include resolver does: the source itself when it is an .ai-rulez
// directory, its .ai-rulez subdirectory, else the source (a bare tree with the
// content directories directly inside). A symlink on the configured path itself
// is followed, since the configuration chose it; symlinks inside the tree are
// refused by the digest.
func (c *collector) localIncludeDir(inc *config.IncludeConfig) (string, error) {
	src := inc.Source
	if !filepath.IsAbs(src) {
		src = filepath.Join(c.cfg.BaseDir, src)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(src))
	if err != nil {
		return "", oops.Errorf("path %q not found", inc.Source)
	}
	if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() {
		return "", oops.Errorf("path %q is not a directory", inc.Source)
	}
	if filepath.Base(resolved) == aiRulezDirName {
		return resolved, nil
	}
	if sub, evalErr := filepath.EvalSymlinks(filepath.Join(resolved, aiRulezDirName)); evalErr == nil && isDir(sub) {
		return sub, nil
	}
	return resolved, nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
