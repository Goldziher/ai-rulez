package signing

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
)

// Subjects and predicate types of the artifacts besides the lock.
const (
	// SubjectBundle is a generated plugin bundle; SubjectSkill one published
	// skill directory; SubjectSBOM an SBOM file; SubjectServed the gate on a
	// signed lock when skills are served (it has no attestation of its own).
	SubjectBundle = "bundle"
	SubjectSkill  = "skill"
	SubjectSBOM   = "sbom"
	SubjectServed = "served"

	// PredicateBundle, PredicateSkill and PredicateSBOM are the predicate types
	// of the attestations `sign --bundle`, `--skill` and `--sbom` write.
	PredicateBundle = "https://github.com/Goldziher/ai-rulez/attestations/bundle/v1"
	PredicateSkill  = "https://github.com/Goldziher/ai-rulez/attestations/skill/v1"
	PredicateSBOM   = "https://github.com/Goldziher/ai-rulez/attestations/sbom/v1"
)

// maxSBOMBytes bounds the file `sign --sbom` reads.
const maxSBOMBytes = 256 << 20

// ArtifactMeta is what signing records besides the artifact: informational
// claims, except IssuedAt, which stands in for a signing time when no log saw
// the signature.
type ArtifactMeta struct {
	Version    string
	Repository string
	Ref        string
	Now        time.Time
}

// TreePredicate is the predicate of a bundle or skill attestation. The tree
// digest is the one the statement's subject carries; the rest is informational.
type TreePredicate struct {
	Tree           string    `json:"tree"`
	Files          int       `json:"files"`
	Name           string    `json:"name"`
	AIRulezVersion string    `json:"ai_rulez_version"`
	Repository     string    `json:"repository,omitempty"`
	Ref            string    `json:"ref,omitempty"`
	IssuedAt       time.Time `json:"issued_at"`
}

// SBOMPredicate is the predicate of an SBOM attestation. The file's own format
// (SPDX, CycloneDX, the ai-rulez SBOM) does not matter: the signature covers its
// bytes.
type SBOMPredicate struct {
	File           string    `json:"file"`
	Size           int64     `json:"size"`
	AIRulezVersion string    `json:"ai_rulez_version"`
	Repository     string    `json:"repository,omitempty"`
	Ref            string    `json:"ref,omitempty"`
	IssuedAt       time.Time `json:"issued_at"`
}

// TreeSubject is a directory as a signature covers it.
type TreeSubject struct {
	Name string
	// Digest is "sha256:<hex>".
	Digest string
	Tree   *DirTree
}

// HexDigest is Digest without the algorithm prefix.
func (s TreeSubject) HexDigest() string { return hexDigest(s.Digest) }

// ReadTreeSubject reads dir and computes its digest under kind. The name is the
// directory's base name: it ties an attestation to one bundle or skill.
func ReadTreeSubject(kind, dir string) (TreeSubject, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return TreeSubject{}, oops.With("dir", dir).Wrapf(err, "resolve the directory")
	}
	tree, err := ReadDirTree(abs)
	if err != nil {
		return TreeSubject{}, err
	}
	digest, err := tree.Digest(kind)
	if err != nil {
		return TreeSubject{}, err
	}
	return TreeSubject{Name: filepath.Base(abs), Digest: digest, Tree: tree}, nil
}

// TreeStatement builds the attestation statement of a bundle (SubjectBundle) or a
// published skill (SubjectSkill) directory.
func TreeStatement(subject, dir string, meta ArtifactMeta) (*Statement, TreeSubject, error) {
	kind, predicateType := KindBundleTree, PredicateBundle
	if subject == SubjectSkill {
		kind, predicateType = KindSkillTree, PredicateSkill
	}
	ts, err := ReadTreeSubject(kind, dir)
	if err != nil {
		return nil, ts, err
	}
	pred := TreePredicate{
		Tree: ts.Digest, Files: len(ts.Tree.Leaves), Name: ts.Name, AIRulezVersion: meta.Version,
		Repository: meta.Repository, Ref: meta.Ref, IssuedAt: meta.Now.UTC().Truncate(time.Second),
	}
	st, err := NewStatement(predicateType, []Subject{{Name: ts.Name, Digest: map[string]string{"sha256": ts.HexDigest()}}}, pred)
	return st, ts, err
}

// TreeExpectation is what a verifier expects of the attestation of ts. The
// directory's name is not part of the match: a bundle or skill checked out under
// another name verifies, and the digest decides.
func TreeExpectation(subject string, ts TreeSubject) Expectation {
	predicateType := PredicateBundle
	if subject == SubjectSkill {
		predicateType = PredicateSkill
	}
	return Expectation{PredicateType: predicateType, DigestHex: ts.HexDigest()}
}

// FileSubject is a single file (an SBOM) as a signature covers it.
type FileSubject struct {
	Name      string
	DigestHex string
	Size      int64
}

// ReadFileSubject hashes the file at path (which is never interpreted).
func ReadFileSubject(path string) (FileSubject, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileSubject{}, oops.With("path", path).Wrapf(err, "read the file to sign")
	}
	if !info.Mode().IsRegular() {
		return FileSubject{}, oops.With("path", path).Errorf("%s is not a regular file", path)
	}
	if info.Size() > maxSBOMBytes {
		return FileSubject{}, oops.With("path", path).Errorf("%s is larger than %d bytes", path, maxSBOMBytes)
	}
	f, err := os.Open(path) //nolint:gosec // the user names the file to sign
	if err != nil {
		return FileSubject{}, oops.With("path", path).Wrapf(err, "read the file to sign")
	}
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxSBOMBytes+1))
	if err != nil {
		return FileSubject{}, oops.With("path", path).Wrapf(err, "read the file to sign")
	}
	if n > maxSBOMBytes {
		return FileSubject{}, oops.With("path", path).Errorf("%s is larger than %d bytes", path, maxSBOMBytes)
	}
	return FileSubject{Name: filepath.Base(path), DigestHex: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

// SBOMStatement builds the attestation statement of the file at path.
func SBOMStatement(path string, meta ArtifactMeta) (*Statement, FileSubject, error) {
	fs, err := ReadFileSubject(path)
	if err != nil {
		return nil, fs, err
	}
	pred := SBOMPredicate{
		File: fs.Name, Size: fs.Size, AIRulezVersion: meta.Version,
		Repository: meta.Repository, Ref: meta.Ref, IssuedAt: meta.Now.UTC().Truncate(time.Second),
	}
	st, err := NewStatement(PredicateSBOM, []Subject{{Name: fs.Name, Digest: map[string]string{"sha256": fs.DigestHex}}}, pred)
	return st, fs, err
}

// SBOMExpectation is what a verifier expects of the attestation of fs. The file
// name is not part of the match: a renamed copy of the same bytes verifies.
func SBOMExpectation(fs FileSubject) Expectation {
	return Expectation{PredicateType: PredicateSBOM, DigestHex: fs.DigestHex}
}
