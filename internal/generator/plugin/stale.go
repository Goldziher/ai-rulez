package plugin

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"
)

// StalePluginDirs lists the generated plugin directories under
// <outputRoot>/plugins that are not in keep (a set of plugin names). A directory
// counts as generated only when it carries ai-rulez's provenance sidecar, so a
// hand-made directory next to the generated ones is never reported. The result
// is sorted.
func StalePluginDirs(outputRoot string, keep map[string]bool) ([]string, error) {
	base := filepath.Join(outputRoot, DomainPluginsDir)
	if info, err := os.Lstat(base); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, nil // a linked plugins directory leads out of the output root
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", base).Wrapf(err, "list plugin directories")
	}
	var stale []string
	for _, entry := range entries {
		if !entry.IsDir() || keep[entry.Name()] {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		if info, statErr := os.Lstat(filepath.Join(dir, provenanceFileName)); statErr == nil && info.Mode().IsRegular() {
			stale = append(stale, dir)
		}
	}
	sort.Strings(stale)
	return stale, nil
}

// RemoveGeneratedPluginDir deletes the files a generated plugin directory's
// provenance sidecar lists, the sidecar itself, and the directories that are
// left empty. A file the sidecar does not list (something a person added), one
// whose content no longer matches its recorded hash, and one reached through a
// symlink is kept, so are the directories holding it, and its path is returned.
// The directory is opened as an os.Root, so no removal can leave it.
func RemoveGeneratedPluginDir(dir string) (kept []string, err error) {
	sidecarPath := filepath.Join(dir, provenanceFileName)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "open stale plugin directory")
	}
	defer root.Close() //nolint:errcheck // nothing is written through the handle
	if info, statErr := root.Lstat(provenanceFileName); statErr != nil || !info.Mode().IsRegular() {
		return nil, oops.With("path", sidecarPath).Errorf("plugin provenance is missing or not a regular file")
	}
	data, err := readRootFile(root, provenanceFileName)
	if err != nil {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "read plugin provenance")
	}
	var document provenanceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "parse plugin provenance")
	}
	if err := removeListedFiles(root, dir, &document); err != nil {
		return nil, err
	}
	if err := root.Remove(provenanceFileName); err != nil && !os.IsNotExist(err) {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "remove plugin provenance")
	}
	// Windows cannot remove a directory while a handle on it is open.
	_ = root.Close() //nolint:errcheck // the deferred Close covers the early returns; a second one is harmless
	pruneEmptyDirs(dir)
	kept, err = filesUnder(dir)
	if err != nil {
		return nil, err
	}
	sort.Strings(kept)
	return kept, nil
}

// removeListedFiles removes each output the provenance document lists that is
// still unchanged; the others are left for the caller to report.
func removeListedFiles(root *os.Root, dir string, document *provenanceDocument) error {
	for rel, recorded := range document.Outputs {
		target, pathErr := safeOutputPath(dir, rel)
		if pathErr != nil {
			return pathErr
		}
		if !unchangedInRoot(root, filepath.Clean(filepath.FromSlash(rel)), target, recorded, document.SourceHash) {
			continue
		}
		if rmErr := root.Remove(filepath.Clean(filepath.FromSlash(rel))); rmErr != nil && !os.IsNotExist(rmErr) {
			return oops.With("path", target).Wrapf(rmErr, "remove stale plugin file")
		}
	}
	return nil
}

// filesUnder lists the files left below dir, sorted; a missing dir holds none.
func filesUnder(dir string) ([]string, error) {
	var kept []string
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err //nolint:wrapcheck // reported by the caller below
		}
		if !d.IsDir() {
			kept = append(kept, path)
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, oops.With("path", dir).Wrapf(walkErr, "list files left in the stale plugin directory")
	}
	sort.Strings(kept)
	return kept, nil
}

// unchangedInRoot reports whether rel is a regular file inside root (no link on
// the way) whose content, minus the generated header, still hashes to the
// recorded value. A missing file is not "unchanged": there is nothing to remove.
func unchangedInRoot(root *os.Root, rel, target string, recorded provenanceOutput, sourceHash string) bool {
	info, err := root.Lstat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	body, err := readRootFile(root, rel)
	if err != nil {
		return false
	}
	body = removeProvenanceHeader(body, target, recorded.ContentHash, sourceHash)
	return hashBytes(body) == recorded.ContentHash
}

func readRootFile(root *os.Root, rel string) ([]byte, error) {
	f, err := root.Open(rel)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers wrap with the path
	}
	defer f.Close() //nolint:errcheck // read only
	return io.ReadAll(io.LimitReader(f, maxProvenanceFileBytes))
}

// maxProvenanceFileBytes bounds one generated file read to verify its hash.
const maxProvenanceFileBytes = 64 << 20

// pruneEmptyDirs removes empty directories under root, deepest first, and root
// itself when nothing is left in it.
func pruneEmptyDirs(root string) {
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr //nolint:wrapcheck // the caller only needs to know the walk stopped
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil {
			continue // not empty: a file that is not generated lives there
		}
	}
}
