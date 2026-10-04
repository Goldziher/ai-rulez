package plugin

import (
	"encoding/json"
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
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "read plugin provenance")
	}
	var document provenanceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "parse plugin provenance")
	}
	for rel := range document.Outputs {
		target, pathErr := safeOutputPath(dir, rel)
		if pathErr != nil {
			return nil, pathErr
		}
		if rmErr := os.Remove(target); rmErr != nil && !os.IsNotExist(rmErr) {
			return nil, oops.With("path", target).Wrapf(rmErr, "remove stale plugin file")
		}
	}
	if err := os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
		return nil, oops.With("path", sidecarPath).Wrapf(err, "remove plugin provenance")
	}
	pruneEmptyDirs(dir)
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			kept = append(kept, path)
		}
		return nil
	})
	sort.Strings(kept)
	return kept, nil
}

// pruneEmptyDirs removes empty directories under root, deepest first, and root
// itself when nothing is left in it.
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // fails, harmlessly, when the directory is not empty
	}
}
