package plugin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Obsolete is a file a previous run generated that the planned bundle no longer
// contains.
type Obsolete struct {
	// Path is the absolute path of the file.
	Path string
	// Rel is the bundle-relative, slash-separated path recorded in the sidecar.
	Rel string
	// Edited is true when the file is not exactly what ai-rulez wrote (changed
	// content, or not a regular file), so it must be kept.
	Edited bool
}

// ObsoleteFiles lists the files that previousSidecar records and plannedSidecar
// does not, and that still exist under bundleDir. A file counts as unchanged
// only when its content, minus the generated header, still hashes to the
// recorded hash. A file the previous sidecar never listed is never reported.
// The result is sorted by path.
func ObsoleteFiles(bundleDir string, previousSidecar, plannedSidecar []byte) ([]Obsolete, error) {
	var previous provenanceDocument
	if err := unmarshalProvenance(previousSidecar, &previous); err != nil {
		return nil, err
	}
	var planned provenanceDocument
	if err := unmarshalProvenance(plannedSidecar, &planned); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(bundleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", bundleDir).Wrapf(err, "open plugin bundle")
	}
	defer root.Close() //nolint:errcheck // read only
	var obsolete []Obsolete
	for rel, recorded := range previous.Outputs {
		if _, keep := planned.Outputs[rel]; keep {
			continue
		}
		target, err := safeOutputPath(bundleDir, rel)
		if err != nil {
			return nil, err
		}
		cleanRel := filepath.Clean(filepath.FromSlash(rel))
		info, statErr := root.Lstat(cleanRel)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			// A link on the way that leaves the bundle is an edit, never a candidate.
			obsolete = append(obsolete, Obsolete{Path: target, Rel: filepath.ToSlash(rel), Edited: true})
			continue
		}
		item := Obsolete{Path: target, Rel: filepath.ToSlash(rel), Edited: !info.Mode().IsRegular()}
		if !item.Edited {
			body, readErr := readRootFile(root, cleanRel)
			if readErr != nil {
				return nil, oops.With("path", target).Wrapf(readErr, "read obsolete plugin file")
			}
			body = removeProvenanceHeader(body, target, recorded.ContentHash, previous.SourceHash)
			item.Edited = hashBytes(body) != recorded.ContentHash
		}
		obsolete = append(obsolete, item)
	}
	sort.Slice(obsolete, func(i, j int) bool { return obsolete[i].Rel < obsolete[j].Rel })
	return obsolete, nil
}

// RemoveObsolete deletes one obsolete file and then every directory it leaves
// empty, stopping at bundleDir, which is never removed. The removal runs through
// an os.Root on bundleDir, so a symlinked parent that leads out of the bundle is
// refused instead of followed.
func RemoveObsolete(bundleDir, path string) error {
	bundleDir = filepath.Clean(bundleDir)
	rel, err := filepath.Rel(bundleDir, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return oops.With("path", path).With("bundle", bundleDir).Errorf("obsolete plugin file is outside its bundle")
	}
	root, err := os.OpenRoot(bundleDir)
	if err != nil {
		return oops.With("path", bundleDir).Wrapf(err, "open plugin bundle")
	}
	defer root.Close() //nolint:errcheck // nothing is written through the handle
	if err := root.Remove(rel); err != nil && !os.IsNotExist(err) {
		return oops.With("path", path).Wrapf(err, "remove obsolete plugin file")
	}
	for dir := filepath.Dir(rel); dir != "." && dir != ""; dir = filepath.Dir(dir) {
		if err := root.Remove(dir); err != nil {
			break // not empty: something that is not generated lives there
		}
	}
	return nil
}
