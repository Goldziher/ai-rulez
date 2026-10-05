package plugin

import (
	"os"
	"path/filepath"
	"sort"

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
	var obsolete []Obsolete
	for rel, recorded := range previous.Outputs {
		if _, keep := planned.Outputs[rel]; keep {
			continue
		}
		target, err := safeOutputPath(bundleDir, rel)
		if err != nil {
			return nil, err
		}
		info, statErr := inspectOutputPath(bundleDir, rel)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, oops.With("path", target).Wrapf(statErr, "inspect obsolete plugin file")
		}
		item := Obsolete{Path: target, Rel: filepath.ToSlash(rel), Edited: !info.Mode().IsRegular()}
		if !item.Edited {
			body, readErr := os.ReadFile(target)
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
