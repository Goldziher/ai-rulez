package worktreeprovision

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
)

type ownedPaths map[string]bool

func loadOwnedPaths(w *worktrees) (ownedPaths, error) {
	paths := make(ownedPaths)
	names := []string{manifestName, localManifestName}
	if w.srcState.StateDirRel == "." {
		names = config.RootManifestNames()
	}
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Join(w.srcState.StateDirRel, name))
		paths[rel] = true
		data, err := readCopyFile(w.src, rel)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read generated manifest %s: %w", rel, err)
		}
		var manifest struct {
			Files   []string                   `json:"files"`
			Merged  map[string]json.RawMessage `json:"merged"`
			Digests map[string]string          `json:"digests"`
			Assets  map[string]string          `json:"assets"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode generated manifest %s: %w", rel, err)
		}
		for _, file := range manifest.Files {
			paths[file] = true
		}
		for file := range manifest.Merged {
			paths[file] = true
		}
		for file := range manifest.Digests {
			paths[file] = true
		}
		for file := range manifest.Assets {
			paths[file] = true
		}
	}
	return paths, nil
}

func (p ownedPaths) generated(w *worktrees, path string) (bool, error) {
	rel := relTo(w.src, path)
	if p[rel] {
		return true, nil
	}
	data, err := readCopyFile(w.src, rel)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", rel, err)
	}
	return genmark.IsGeneratedOutput(path, data), nil
}
