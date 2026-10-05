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
func PlanStaleOutputs(outputs []config.OutputFile, staleRoots ...string) ([]StaleOutput, error) {
	keep := make(map[string]bool, len(outputs))
	for _, output := range outputs {
		keep[filepath.Clean(output.Path)] = true
	}
	var stale []StaleOutput
	var inventories []string
	for _, output := range outputs {
		if output.PluginInventory {
			inventories = append(inventories, output.Path)
		}
	}
	for _, root := range staleRoots {
		inventories = append(inventories, filepath.Join(root, provenanceFileName))
	}
	for _, sidecar := range inventories {
		bundleStale, err := planBundleStaleOutputs(sidecar, keep)
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
	document, err := readPreviousProvenance(sidecar)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var stale []StaleOutput
	for rel, expected := range document.Outputs {
		output, err := planStaleOutput(root, rel, expected, document.SourceHash, keep)
		if err != nil {
			return nil, err
		}
		if output != nil {
			stale = append(stale, *output)
		}
	}

	return stale, nil
}

func planStaleOutput(root, rel string, expected provenanceOutput, sourceHash string, keep map[string]bool) (*StaleOutput, error) {
	target, err := safeOutputPath(root, rel)
	if err != nil {
		return nil, err
	}
	if keep[target] {
		return nil, nil
	}
	// A previous run may have completed a layout replacement but failed
	// before committing its inventory. The obsolete file is already gone
	// when its path is now a directory for planned descendants.
	info, err := inspectOutputPath(root, rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if info.IsDir() && hasPlannedDescendant(target, keep) {
		return nil, nil
	}
	if !info.Mode().IsRegular() {
		return nil, oops.With("path", target).Errorf("unsafe obsolete plugin output: expected regular file")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		return nil, oops.With("path", target).Wrapf(err, "read obsolete plugin output")
	}
	payload := removeProvenanceHeader(body, target, expected.ContentHash, sourceHash)
	generated := payload
	if header := provenanceHeader(target, expected.ContentHash, sourceHash); header != "" {
		generated = insertProvenanceHeader(payload, target, header)
	}
	if hashBytes(payload) != expected.ContentHash || !bytes.Equal(body, generated) {
		return nil, oops.With("path", target).Errorf("modified obsolete plugin output; move or remove it explicitly before regenerating")
	}
	return &StaleOutput{Path: target, root: root, body: body}, nil
}

func readPreviousProvenance(sidecar string) (*provenanceDocument, error) {
	if err := regularOutputPath(filepath.Dir(sidecar), provenanceFileName); err != nil {
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
	if document.Outputs == nil || provenanceSourceHash(document.Outputs) != document.SourceHash {
		return nil, oops.With("path", sidecar).Errorf("invalid previous plugin provenance inventory or source hash")
	}
	return &document, nil
}

// regularOutputPath refuses directories and symlinks, including symlinked
// parent directories, so cleanup cannot follow a path outside its bundle.
func regularOutputPath(root, rel string) error {
	info, err := inspectOutputPath(root, rel)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return oops.With("path", rel).Errorf("unsafe obsolete plugin output: expected regular file")
	}
	return nil
}

func hasPlannedDescendant(path string, keep map[string]bool) bool {
	prefix := path + string(filepath.Separator)
	for planned := range keep {
		if strings.HasPrefix(planned, prefix) {
			return true
		}
	}
	return false
}

func inspectOutputPath(root, rel string) (os.FileInfo, error) {
	if _, err := safeOutputPath(root, rel); err != nil {
		return nil, err
	}
	path := root
	parts := append([]string{""}, strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator))...)
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, oops.With("path", path).Wrapf(err, "inspect plugin output")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, oops.With("path", path).Errorf("unsafe obsolete plugin output: symlink")
		}
		if i == len(parts)-1 {
			return info, nil
		}
		if info.Mode().IsRegular() {
			// A descendant beneath a regular file no longer exists. This also
			// permits retry after a directory-to-file layout replacement.
			return nil, oops.With("path", path).Wrapf(os.ErrNotExist, "obsolete plugin output has a file parent")
		}
		if !info.IsDir() {
			return nil, oops.With("path", path).Errorf("unsafe obsolete plugin output: expected directory parent")
		}
	}
	return nil, oops.Errorf("invalid plugin output path")
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
