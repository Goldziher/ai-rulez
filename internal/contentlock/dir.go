package contentlock

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// Tree kinds of fetched or cached directories. Each is its own domain, so the
// digest of an include never equals the digest of a skill source with the same
// files.
const (
	KindInclude        = "include"
	KindOKFInclude     = "okf-include"
	KindInstalledSkill = "installed-skill"
	KindSkillSource    = "skill-source"
)

// cacheMetaPrefix is the ai-rulez cache bookkeeping file kept next to a cached
// clone. It is not content.
const cacheMetaPrefix = ".cache_meta.json"

type treeFile struct {
	rel  string // "/"-separated, relative to the root
	abs  string
	info os.FileInfo
	data []byte
}

// readTree returns the regular files below dir: VCS metadata (.git below the
// root) and cache bookkeeping are left out, so the digest of a fresh clone equals
// the digest of the same tree re-read later. A symlinked root is refused:
// WalkDir does not follow it, so every such tree would share one constant digest.
func readTree(dir string) ([]treeFile, error) {
	if info, err := os.Lstat(dir); err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "digest directory")
	} else if !info.IsDir() {
		return nil, oops.With("dir", dir).Errorf("digest directory: %s is not a real directory (a symlink is not followed)", dir)
	}
	var files []treeFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, cacheMetaPrefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir below a trusted cache directory
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		files = append(files, treeFile{rel: filepath.ToSlash(rel), abs: path, info: info, data: data})
		return nil
	})
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "digest directory")
	}
	return files, nil
}

// DigestDir returns the digest of the regular files below dir under the one
// hashing scheme of the lock (TreeDigest over the files, kind being one of the
// Kind* tree kinds). Modes are normalized like every other pin (fileMode).
func DigestDir(kind, dir string) (string, error) {
	files, err := readTree(dir)
	if err != nil {
		return "", err
	}
	leaves := make([]Leaf, len(files))
	for i, f := range files {
		leaves[i] = Leaf{Path: f.rel, Mode: fileMode(f.abs, f.info), Data: f.data}
	}
	digest, err := TreeDigest(kind, leaves)
	if err != nil {
		return "", oops.With("dir", dir).Wrapf(err, "digest directory")
	}
	return digest, nil
}
