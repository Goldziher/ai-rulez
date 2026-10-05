package plugin

import (
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
		if _, statErr := os.Stat(filepath.Join(dir, provenanceFileName)); statErr == nil {
			stale = append(stale, dir)
		}
	}
	sort.Strings(stale)
	return stale, nil
}

// RemoveGeneratedPluginDir deletes the files a generated plugin directory's
// provenance sidecar lists, the sidecar itself, and the directories that are
// left empty. A file the sidecar does not list (something a person added) is
// kept, so are the directories holding it, and its path is returned.
func RemoveGeneratedPluginDir(dir string) (kept []string, err error) {
	sidecarPath := filepath.Join(dir, provenanceFileName)
	stale, err := planBundleStaleOutputs(sidecarPath, nil)
	if err != nil {
		return nil, err
	}
	if err := RemoveStaleOutputs(stale); err != nil {
		return nil, err
	}
	if err := os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "remove plugin provenance")
	}
	pruneEmptyDirs(dir)
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
