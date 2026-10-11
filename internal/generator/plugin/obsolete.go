package plugin

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Obsolete is a file a previous run generated that the planned bundle no longer
// contains.
type Obsolete struct {
	// Path is the absolute path of the file.
	Path string
	// Rel is the bundle-relative, slash-separated path recorded in the sidecar.
	Rel string
	// ContentHash is the hash the sidecar recorded when the file was generated,
	// carried into the retained record so later runs still recognize it.
	ContentHash string
	// Edited is true when the file is not exactly what ai-rulez wrote (changed
	// content, or not a regular file), so it must be kept.
	Edited bool
	// Why says why an Edited file is kept, in words that fit "Kept ...: <Why>":
	// changed content, a symlink or other non-regular file, or a link on its path.
	Why string
}

// Reasons an obsolete file is kept.
const (
	WhyEdited       = "edited since it was generated"
	WhyNotRegular   = "it is a symlink or not a regular file"
	WhyLinkedParent = "its path crosses a symlink that leaves the bundle"
)

// ObsoleteFiles lists the files that previousSidecar records and plannedSidecar
// does not, and that still exist under bundleDir. A file counts as unchanged
// only when its content, minus the generated header, still hashes to the
// recorded hash. A file the previous sidecar kept (its "obsolete" record) was
// already edited, so it is always reported as kept. A file the previous sidecar
// never listed is never reported. The result is sorted by path.
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
		item, err := obsoleteFile(root, bundleDir, rel, recorded, previous.SourceHash, false)
		if err != nil {
			return nil, err
		}
		if item != nil {
			obsolete = append(obsolete, *item)
		}
	}
	// A file the previous run kept stays owned and stays kept: it was edited
	// once, so it is never pruned automatically. Its record also keeps verify
	// failing until it is reconciled into the sources or deleted by hand.
	for rel, recorded := range previous.Obsolete {
		if _, keep := planned.Outputs[rel]; keep {
			continue
		}
		if _, inOutputs := previous.Outputs[rel]; inOutputs {
			continue
		}
		item, err := obsoleteFile(root, bundleDir, rel, recorded, previous.SourceHash, true)
		if err != nil {
			return nil, err
		}
		if item != nil {
			obsolete = append(obsolete, *item)
		}
	}
	sort.Slice(obsolete, func(i, j int) bool { return obsolete[i].Rel < obsolete[j].Rel })
	return obsolete, nil
}

// obsoleteFile inspects one previously-recorded output. It returns nil when the
// file is gone. kept marks a file the previous run already kept because it was
// edited: such a file is always reported Edited so it is never pruned.
func obsoleteFile(root *os.Root, bundleDir, rel string, recorded provenanceOutput, sourceHash string, kept bool) (*Obsolete, error) {
	target, err := safeOutputPath(bundleDir, rel)
	if err != nil {
		return nil, err
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	info, statErr := root.Lstat(cleanRel)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, nil
		}
		// A link on the way that leaves the bundle is an edit, never a candidate.
		item := Obsolete{Path: target, Rel: filepath.ToSlash(rel), ContentHash: recorded.ContentHash, Edited: true, Why: WhyLinkedParent}
		return &item, nil
	}
	item := Obsolete{Path: target, Rel: filepath.ToSlash(rel), ContentHash: recorded.ContentHash, Edited: !info.Mode().IsRegular()}
	switch {
	case item.Edited:
		item.Why = WhyNotRegular
	case kept:
		item.Edited = true
		item.Why = WhyEdited
	default:
		body, readErr := readRootFile(root, cleanRel)
		if readErr != nil {
			return nil, oops.With("path", target).Wrapf(readErr, "read obsolete plugin file")
		}
		body = removeProvenanceHeader(body, target, recorded.ContentHash, sourceHash)
		if item.Edited = hashBytes(body) != recorded.ContentHash; item.Edited {
			item.Why = WhyEdited
		}
	}
	return &item, nil
}

// RemoveObsolete deletes one obsolete file and then every directory it leaves
// empty, stopping at bundleDir, which is never removed. The removal runs through
// an os.Root on bundleDir, so a symlinked parent that leads out of the bundle is
// refused instead of followed.
func RemoveObsolete(bundleDir, path string) error {
	bundleDir = filepath.Clean(bundleDir)
	rel, err := filepath.Rel(bundleDir, filepath.Clean(path))
	if err != nil || rel == "." || safefs.RelEscapes(rel) {
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
