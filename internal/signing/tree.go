package signing

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

// Sidecar file names. A bundle or skill directory carries its own attestation
// next to the files it covers; the signature files are not part of what they sign.
const (
	// SidecarName is the attestation of a bundle or skill directory.
	SidecarName = ".ai-rulez.sigstore.json"
	// ProvenanceSidecarName is the SLSA provenance attestation of a bundle.
	ProvenanceSidecarName = ".ai-rulez.provenance.sigstore.json"
	bundleSuffix          = ".sigstore.json"
)

// Tree kinds of the directories a signature covers. Each is its own digest
// domain, so a bundle's tree never equals a skill's with the same files.
const (
	KindBundleTree = "plugin-bundle"
	KindSkillTree  = "published-skill"
)

// Bounds on what is read to compute a directory subject.
const (
	maxTreeFiles = 20000
	maxTreeBytes = 512 << 20
)

// IsSignatureFile reports whether rel, a path relative to a signed directory, is
// one of its attestation files: .ai-rulez.sigstore.json, a further signature
// (.ai-rulez.2.sigstore.json) or the provenance statement. Only the root of the
// directory holds them; the same name deeper in the tree is content.
func IsSignatureFile(rel string) bool {
	if rel == SidecarName {
		return true
	}
	const prefix = ".ai-rulez."
	if strings.Contains(rel, "/") || !strings.HasPrefix(rel, prefix) || !strings.HasSuffix(rel, bundleSuffix) || len(rel) <= len(prefix)+len(bundleSuffix) {
		return false
	}
	mid := rel[len(prefix) : len(rel)-len(bundleSuffix)]
	return mid == "provenance" || isDigits(mid)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// DirTree is the files of a directory as a signature sees them.
type DirTree struct {
	// Leaves are the files, sorted by the digest function; signature files and
	// version-control metadata are left out.
	Leaves []contentlock.Leaf
}

// ReadDirTree reads dir into leaves: regular files only, with the executable bit
// as their mode. A symlink or any other irregular entry is refused: a signature
// that covered "where the link pointed" would not cover what an agent reads
// through it. VCS metadata (.git) and the directory root's signature files are
// skipped.
func ReadDirTree(dir string) (*DirTree, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read the directory to sign")
	}
	if !info.IsDir() {
		return nil, oops.With("dir", dir).Errorf("%s is not a directory (a symlink is not followed)", dir)
	}
	var leaves []contentlock.Leaf
	total := 0
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if IsSignatureFile(rel) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		if !fi.Mode().IsRegular() {
			return oops.With("path", rel).Errorf("%s is a symlink or not a regular file: a signed directory holds regular files only", rel)
		}
		if len(leaves) >= maxTreeFiles {
			return oops.Errorf("%s holds more than %d files", dir, maxTreeFiles)
		}
		if total += int(fi.Size()); total > maxTreeBytes {
			return oops.Errorf("%s holds more than %d bytes", dir, maxTreeBytes)
		}
		data, err := readRegular(path)
		if err != nil {
			return err
		}
		leaves = append(leaves, contentlock.Leaf{Path: rel, Mode: contentlock.ModeFor(uint32(fi.Mode().Perm())), Data: data})
		return nil
	})
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read the directory to sign")
	}
	if len(leaves) == 0 {
		return nil, oops.With("dir", dir).Errorf("%s has no files to sign", dir)
	}
	return &DirTree{Leaves: leaves}, nil
}

func readRegular(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from the walk below the directory the user named
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	return data, nil
}

// Digest is the tree digest of the directory under kind (KindBundleTree or
// KindSkillTree), "sha256:<hex>".
func (t *DirTree) Digest(kind string) (string, error) {
	d, err := contentlock.TreeDigest(kind, t.Leaves)
	if err != nil {
		return "", oops.Wrapf(err, "digest the directory")
	}
	return d, nil
}

// Content returns the bytes of the file at rel, or nil when the tree has none.
func (t *DirTree) Content(rel string) []byte {
	for i := range t.Leaves {
		if t.Leaves[i].Path == rel {
			return t.Leaves[i].Data
		}
	}
	return nil
}
