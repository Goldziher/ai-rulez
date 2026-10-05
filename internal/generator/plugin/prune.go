package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// StaleOutput records an unchanged obsolete file and the bundle that owns it.
// The original bytes are retained to check again immediately before deletion.
type StaleOutput struct {
	Path string
	root string
	body []byte
}

// PlanStaleOutputs compares previous and planned inventories for every bundle
// being generated. Modified obsolete files refuse generation before any writes.
func PlanStaleOutputs(outputs []config.OutputFile) ([]StaleOutput, error) {
	keep := make(map[string]bool, len(outputs))
	for _, output := range outputs {
		keep[filepath.Clean(output.Path)] = true
	}
	var stale []StaleOutput
	for _, output := range outputs {
		if output.IsDir || filepath.Base(output.Path) != provenanceFileName {
			continue
		}
		bundleStale, err := planBundleStaleOutputs(output.Path, keep)
		if err != nil {
			return nil, err
		}
		stale = append(stale, bundleStale...)
	}

	sort.Slice(stale, func(i, j int) bool { return stale[i].Path < stale[j].Path })
	return stale, nil
}

func planBundleStaleOutputs(sidecar string, keep map[string]bool) ([]StaleOutput, error) {
	root := filepath.Dir(sidecar)
	if err := regularOutputPath(root, provenanceFileName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	data, err := os.ReadFile(sidecar)
	if err != nil {
		return nil, oops.Wrapf(err, "read previous plugin provenance")
	}
	var document provenanceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, oops.Wrapf(err, "parse previous plugin provenance")
	}
	if document.SchemaVersion != provenanceSchema {
		return nil, oops.Errorf("unsupported plugin provenance schema %q", document.SchemaVersion)
	}
	var stale []StaleOutput
	for rel, expected := range document.Outputs {
		target, err := safeOutputPath(root, rel)
		if err != nil {
			return nil, err
		}
		if keep[target] {
			continue
		}
		if err := regularOutputPath(root, rel); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		body, err := os.ReadFile(target)
		if err != nil {
			return nil, oops.With("path", target).Wrapf(err, "read obsolete plugin output")
		}
		payload := removeProvenanceHeader(body, target, expected.ContentHash, document.SourceHash)
		generated := payload
		if header := provenanceHeader(target, expected.ContentHash, document.SourceHash); header != "" {
			generated = insertProvenanceHeader(payload, target, header)
		}
		if hashBytes(payload) != expected.ContentHash || !bytes.Equal(body, generated) {
			return nil, oops.With("path", target).Errorf("modified obsolete plugin output; move or remove it explicitly before regenerating")
		}
		stale = append(stale, StaleOutput{Path: target, root: root, body: body})
	}
	return stale, nil
}

// regularOutputPath refuses directories and symlinks, including symlinked
// parent directories, so cleanup cannot follow a path outside its bundle.
func regularOutputPath(root, rel string) error {
	if _, err := safeOutputPath(root, rel); err != nil {
		return err
	}
	path := root
	parts := append([]string{""}, strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator))...)
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return oops.With("path", path).Wrapf(err, "inspect plugin output")
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return oops.With("path", path).Errorf("unsafe obsolete plugin output: expected regular file and directory parents")
		}
	}
	return nil
}

// RemoveStaleOutputs removes only planned files whose bytes remain unchanged,
// then removes only their empty parent directories, stopping at the bundle root.
func RemoveStaleOutputs(stale []StaleOutput) error {
	for _, output := range stale {
		rel, err := filepath.Rel(output.root, output.Path)
		if err != nil {
			return oops.Wrapf(err, "resolve obsolete plugin output")
		}
		if err := regularOutputPath(output.root, rel); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		body, err := os.ReadFile(output.Path)
		if err != nil {
			return oops.Wrapf(err, "read obsolete plugin output before deletion")
		}
		if !bytes.Equal(body, output.body) {
			return oops.With("path", output.Path).Errorf("obsolete plugin output changed during generation")
		}
		if err := os.Remove(output.Path); err != nil {
			return oops.Wrapf(err, "remove obsolete plugin output")
		}
		for dir := filepath.Dir(output.Path); dir != output.root; dir = filepath.Dir(dir) {
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	}
	return nil
}
