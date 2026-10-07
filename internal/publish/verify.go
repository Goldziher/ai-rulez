package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/oci"
)

// maxVerifyBytes caps what verify reads: each dist file and the total
// uncompressed size of the archive. A var so tests can lower it.
var maxVerifyBytes int64 = 512 << 20

// Problem is one mismatch `publish verify` found.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// VerifyResult is the outcome of verifying a dist directory.
type VerifyResult struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Files   int    `json:"files"`
	// Signature is "none" (the bundle is unsigned), "unverified" (it is signed
	// and no trusted signer was given) or "verified" (the signature checks out
	// and its signer is trusted).
	Signature string    `json:"signature,omitempty"`
	Signer    string    `json:"signer,omitempty"`
	Problems  []Problem `json:"problems"`
}

// VerifyChecks refine Verify.
type VerifyChecks struct {
	// Signature says whom to trust; with no signer named the signature is not checked.
	Signature VerifyOptions
	// RequireSignature turns an unsigned or unverified bundle into a problem.
	// Naming a trusted signer implies it.
	RequireSignature bool
}

// OK reports whether nothing mismatched.
func (r VerifyResult) OK() bool { return len(r.Problems) == 0 }

func (r *VerifyResult) add(path, format string, args ...any) {
	r.Problems = append(r.Problems, Problem{Path: path, Message: sprintf(format, args...)})
}

// Verify recomputes every digest of a dist directory: SHA256SUMS against the
// files, the manifest against the archive's entries, the lock copy against the
// manifest, and the archive's headers against the determinism rules. Nothing
// is written. A dist directory that cannot be read at all is an error; a
// mismatch is a Problem in the result.
func Verify(dir string) (VerifyResult, error) { return VerifyWith(dir, VerifyChecks{}) }

// VerifyWith is Verify plus the signature checks.
func VerifyWith(dir string, checks VerifyChecks) (VerifyResult, error) {
	// Naming a trusted signer asks for a signed release: a stripped signature
	// must not verify clean.
	checks.RequireSignature = checks.RequireSignature || checks.Signature.Trusts()
	res := VerifyResult{Problems: []Problem{}}
	recorded, ok, err := verifySums(dir, &res)
	if err != nil || !ok {
		return res, err
	}
	manifest, ok := loadManifest(dir, &res)
	if !ok {
		sort.SliceStable(res.Problems, func(i, j int) bool { return res.Problems[i].Path < res.Problems[j].Path })
		return res, nil
	}
	res.Name, res.Version = manifest.Name, manifest.Version
	checkManifest(dir, manifest, recorded, &res)
	checkPlan(dir, manifest, recorded, &res)
	checkLockCopy(dir, manifest, &res)
	checkSignature(dir, manifest, checks, &res)
	sort.SliceStable(res.Problems, func(i, j int) bool { return res.Problems[i].Path < res.Problems[j].Path })
	return res, nil
}

// verifySums checks SHA256SUMS against the files of dir and flags files it does
// not list. ok is false when the sums file itself is unusable (a problem is
// recorded); an unreadable sums file is an error.
func verifySums(dir string, res *VerifyResult) (recorded map[string]string, ok bool, err error) {
	sumsRaw, err := readRegular(filepath.Join(dir, SumsFile))
	if err != nil {
		return nil, false, newError(CodeVerify, ExitFailed, "run `ai-rulez publish` first", "cannot read %s: %v", SumsFile, err)
	}
	sums, err := ParseSums(sumsRaw)
	if err != nil {
		res.add(SumsFile, "%v", err)
		return nil, false, nil
	}
	recorded = map[string]string{}
	for _, s := range sums {
		if _, dup := recorded[s.Path]; dup {
			res.add(s.Path, "listed more than once in %s", SumsFile)
			continue
		}
		recorded[s.Path] = s.Digest
		data, rerr := readRegular(filepath.Join(dir, filepath.FromSlash(s.Path)))
		if rerr != nil {
			res.add(s.Path, "listed in %s but unreadable: %v", SumsFile, rerr)
			continue
		}
		res.Files++
		if got := Digest(data); got != s.Digest {
			res.add(s.Path, "digest is %s, %s records %s", got, SumsFile, s.Digest)
		}
	}
	checkUnlisted(dir, recorded, res)
	return recorded, true, nil
}

// VerifySums verifies a directory that has checksums but no bundle manifest,
// such as the aggregate directory of a multi-plugin publish.
func VerifySums(dir string) (VerifyResult, error) {
	res := VerifyResult{Problems: []Problem{}}
	if _, _, err := verifySums(dir, &res); err != nil {
		return res, err
	}
	sort.SliceStable(res.Problems, func(i, j int) bool { return res.Problems[i].Path < res.Problems[j].Path })
	return res, nil
}

// checkSignature verifies the release signature when a trusted signer was
// named, and enforces RequireSignature. Without a named signer a present
// signature is reported as unverified: a valid bundle alone only says who
// signed, never that the signer is one the caller trusts.
func checkSignature(dir string, m Manifest, checks VerifyChecks, res *VerifyResult) {
	if m.Signature == nil {
		res.Signature = "none"
		if checks.RequireSignature {
			res.add(m.Bundle.File, "%s the bundle is not signed", CodeUnsigned)
		}
		return
	}
	res.Signature = "unverified"
	if !checks.Signature.Trusts() {
		if checks.RequireSignature {
			res.add(m.Signature.File, "%s a signature is present but no trusted key or identity was given to verify it", CodeUnsigned)
		}
		return
	}
	if !expectedAttachmentNames(m) {
		return // checkAttachments reported the name; a file the manifest chose is never opened
	}
	bundle, err := readRegular(filepath.Join(dir, filepath.FromSlash(m.Signature.File)))
	if err != nil {
		res.add(m.Signature.File, "%s unreadable: %v", CodeUnsigned, err)
		return
	}
	archive, err := readRegular(filepath.Join(dir, m.Bundle.File))
	if err != nil {
		return // reported by the SHA256SUMS pass
	}
	out, err := VerifyArchiveSignature(bundle, archive, checks.Signature)
	if err != nil {
		res.add(m.Signature.File, "%s the signature does not verify: %v", CodeUnsigned, err)
		return
	}
	if !verifyAttestation(dir, m, checks, res) {
		return
	}
	res.Signature = "verified"
	if out.Signer.Kind == "key" {
		res.Signer = "key " + out.Signer.KeyID
	} else {
		res.Signer = out.Signer.Identity + " (issuer " + out.Signer.Issuer + ")"
	}
}

// expectedAttachmentNames reports whether every file name the manifest gives for the archive, its signature,
// the attestation and the SBOM is exactly the name Build derives from the plugin name and version. The manifest
// is untrusted input, so a verifier reads only files under those names, never a path it chose.
func expectedAttachmentNames(m Manifest) bool {
	base := m.Name + "-" + m.Version
	s := m.Signature
	ok := s != nil && ValidPath(m.Bundle.File) && m.Bundle.File == base+".tar.gz" && s.File == m.Bundle.File+".sigstore.json" &&
		(s.Attestation == "" || s.Attestation == base+".attestation.sigstore.json")
	if m.SBOM != nil {
		ok = ok && m.SBOM.File == base+".sbom.cdx.json"
	}
	return ok && ValidPath(s.File)
}

// verifyAttestation checks the signed release statement, which binds the name,
// version and the archive, lock and SBOM digests the archive signature alone
// does not. A signed release without one cannot be trusted: its manifest could
// have been relabelled.
func verifyAttestation(dir string, m Manifest, checks VerifyChecks, res *VerifyResult) bool {
	att := m.Signature.Attestation
	if att == "" {
		res.add(m.Signature.File, "%s the release has no signed attestation binding its name, version and digests; re-sign it", CodeUnsigned)
		return false
	}
	bundle, err := readRegular(filepath.Join(dir, filepath.FromSlash(att)))
	if err != nil {
		res.add(att, "%s unreadable: %v", CodeUnsigned, err)
		return false
	}
	files := ReleaseFiles{}
	if files.Archive, err = readRegular(filepath.Join(dir, m.Bundle.File)); err != nil {
		return false // reported by the SHA256SUMS pass
	}
	if files.Lock, err = readRegular(filepath.Join(dir, LockFile)); err != nil {
		return false // reported by the SHA256SUMS pass
	}
	if m.SBOM != nil {
		if files.SBOM, err = readRegular(filepath.Join(dir, filepath.FromSlash(m.SBOM.File))); err != nil {
			return false // reported by the SHA256SUMS pass
		}
	}
	if _, err := VerifyReleaseAttestation(bundle, m, files, checks.Signature); err != nil {
		res.add(att, "%s the attestation does not verify: %v", CodeUnsigned, err)
		return false
	}
	return true
}

// checkUnlisted flags every file below dir that SHA256SUMS does not list. The
// sums file and the plan are the only files that cannot be listed.
func checkUnlisted(dir string, recorded map[string]string, res *VerifyResult) {
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if _, ok := recorded[rel]; !ok && rel != SumsFile && rel != PlanFile {
			res.add(rel, "present but not listed in %s", SumsFile)
		}
		return nil
	})
	if err != nil {
		res.add(".", "cannot list the directory: %v", err)
	}
}

// checkLockCopy compares the lock tree and version the manifest records with
// the shipped lock copy's own.
func checkLockCopy(dir string, m Manifest, res *VerifyResult) {
	raw, err := readRegular(filepath.Join(dir, LockFile))
	if err != nil {
		return // reported by the SHA256SUMS pass
	}
	var lock struct {
		Version int    `toml:"version"`
		Tree    string `toml:"tree"`
	}
	if err := toml.Unmarshal(raw, &lock); err != nil {
		res.add(LockFile, "not valid TOML: %v", err)
		return
	}
	if lock.Tree != m.Lock.Tree {
		res.add(LockFile, "lock tree %q differs from the manifest's lock.tree %q", lock.Tree, m.Lock.Tree)
	}
	if lock.Version != m.Lock.Version {
		res.add(LockFile, "lock version %d differs from the manifest's lock.version %d", lock.Version, m.Lock.Version)
	}
}

func loadManifest(dir string, res *VerifyResult) (Manifest, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.manifest.json"))
	if err != nil || len(matches) != 1 {
		res.add(".", "expected exactly one *.manifest.json, found %d", len(matches))
		return Manifest{}, false
	}
	raw, err := readRegular(matches[0])
	if err != nil {
		res.add(filepath.Base(matches[0]), "unreadable: %v", err)
		return Manifest{}, false
	}
	m, err := decodeManifest(raw)
	if err != nil {
		res.add(filepath.Base(matches[0]), "%v", err)
		return Manifest{}, false
	}
	if want := m.Name + "-" + m.Version + ".manifest.json"; filepath.Base(matches[0]) != want {
		res.add(filepath.Base(matches[0]), "named for a different bundle than its content (%s)", want)
	}
	return m, true
}

// decodeManifest parses and sanity-checks a manifest document.
func decodeManifest(raw []byte) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, oops.Errorf("invalid manifest: %v", err)
	}
	if m.SchemaVersion != SchemaVersion {
		return Manifest{}, oops.Errorf("unsupported schema_version %d", m.SchemaVersion)
	}
	if err := ValidateName(m.Name, m.Version); err != nil {
		return Manifest{}, err //nolint:wrapcheck // a publish.Error carries the code
	}
	return m, nil
}

func checkManifest(dir string, m Manifest, recorded map[string]string, res *VerifyResult) {
	if _, ok := recorded[m.Name+"-"+m.Version+".manifest.json"]; !ok {
		res.add(SumsFile, "does not list the manifest")
	}
	if want := m.Name + "-" + m.Version + ".tar.gz"; m.Bundle.File != want {
		res.add(m.Bundle.File, "bundle.file must be %s", want)
		return
	}
	if d, ok := recorded[m.Bundle.File]; !ok || d != m.Bundle.Digest {
		res.add(m.Bundle.File, "manifest digest %s differs from %s (%s)", m.Bundle.Digest, SumsFile, d)
	}
	if d, ok := recorded[LockFile]; !ok || d != m.Lock.FileDigest {
		res.add(LockFile, "manifest lock.file_digest %s differs from %s (%s)", m.Lock.FileDigest, SumsFile, d)
	}
	checkAttachments(m, recorded, res)
	checkArchiveContents(dir, m, res)
}

// checkArchiveContents compares the bundle archive's size, identity and file
// list with the manifest.
func checkArchiveContents(dir string, m Manifest, res *VerifyResult) {
	archive, err := readRegular(filepath.Join(dir, m.Bundle.File))
	if err != nil {
		return // already reported as unreadable by the SHA256SUMS pass
	}
	if len(archive) != m.Bundle.Size {
		res.add(m.Bundle.File, "size is %d, manifest records %d", len(archive), m.Bundle.Size)
	}
	entries, manifests, err := readArchiveManifests(archive)
	if err != nil {
		res.add(m.Bundle.File, "%v", err)
		return
	}
	checkInArchiveIdentity(m, manifests, res)
	got := map[string]FileEntry{}
	for _, e := range entries {
		got[e.Path] = e
	}
	want := map[string]FileEntry{}
	for _, f := range m.Files {
		want[f.Path] = f
	}
	for p, w := range want {
		g, ok := got[p]
		switch {
		case !ok:
			res.add(p, "in the manifest but not in the archive")
		case g.Digest != w.Digest || g.Size != w.Size:
			res.add(p, "archive holds %s (%d bytes), manifest records %s (%d bytes)", g.Digest, g.Size, w.Digest, w.Size)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			res.add(p, "in the archive but not in the manifest")
		}
	}
}

// checkPlan verifies publish-plan.json when present: its artifacts (path, size,
// digest) against the files on disk and SHA256SUMS, and its commands against
// what `publish --execute` would build for the manifest. A plan that names a
// different command than the one the manifest implies is a mismatch, because
// --execute runs the plan's argv.
func checkPlan(dir string, m Manifest, recorded map[string]string, res *VerifyResult) {
	raw, err := readRegular(filepath.Join(dir, PlanFile))
	if err != nil {
		return // the plan is optional for verification
	}
	var plan Plan
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&plan); err != nil {
		res.add(PlanFile, "invalid plan: %v", err)
		return
	}
	if plan.Name != m.Name || plan.Version != m.Version {
		res.add(PlanFile, "plan is for %s %s, the manifest for %s %s", plan.Name, plan.Version, m.Name, m.Version)
	}
	checkPlanArtifacts(dir, plan, recorded, res)
	checkPlanCommands(dir, plan, m, res)
}

// checkPlanArtifacts checks each artifact the plan lists against the file on
// disk and SHA256SUMS, and that SHA256SUMS lists nothing the plan does not.
func checkPlanArtifacts(dir string, plan Plan, recorded map[string]string, res *VerifyResult) {
	for _, a := range plan.Artifacts {
		if !ValidPath(a.Path) {
			res.add(PlanFile, "plan lists an unsafe path %q", a.Path)
			continue
		}
		if d, ok := recorded[a.Path]; ok && d != a.Digest {
			res.add(a.Path, "plan digest %s differs from %s (%s)", a.Digest, SumsFile, d)
		}
		data, rerr := readRegular(filepath.Join(dir, filepath.FromSlash(a.Path)))
		if rerr != nil {
			res.add(a.Path, "listed in the plan but unreadable: %v", rerr)
			continue
		}
		if len(data) != a.Size {
			res.add(a.Path, "plan records %d bytes, the file has %d", a.Size, len(data))
		}
		if got := Digest(data); got != a.Digest {
			res.add(a.Path, "plan digest %s differs from the file's %s", a.Digest, got)
		}
	}
	for p := range recorded {
		listed := false
		for _, a := range plan.Artifacts {
			listed = listed || a.Path == p
		}
		if !listed {
			res.add(p, "listed in %s but not in the plan", SumsFile)
		}
	}
}

// checkAttachments requires the signature and SBOM files the manifest names to
// be listed in SHA256SUMS with the digest the manifest records for the SBOM.
func checkAttachments(m Manifest, recorded map[string]string, res *VerifyResult) {
	if s := m.Signature; s != nil {
		switch {
		case s.Type != SignatureSigstoreBundle:
			res.add(m.Bundle.File, "unknown signature type %q", s.Type)
		case s.File != m.Bundle.File+".sigstore.json":
			res.add(s.File, "signature.file must be %s.sigstore.json", m.Bundle.File)
		default:
			if _, ok := recorded[s.File]; !ok {
				res.add(s.File, "the manifest names this signature but %s does not list it", SumsFile)
			}
		}
		if s.Attestation != "" {
			if s.Attestation != m.Name+"-"+m.Version+".attestation.sigstore.json" {
				res.add(s.Attestation, "signature.attestation must be %s-%s.attestation.sigstore.json", m.Name, m.Version)
			} else if _, ok := recorded[s.Attestation]; !ok {
				res.add(s.Attestation, "the manifest names this attestation but %s does not list it", SumsFile)
			}
		}
	}
	if s := m.SBOM; s != nil {
		d, ok := recorded[s.File]
		switch {
		case s.Format != SBOMCycloneDX || s.File != m.Name+"-"+m.Version+".sbom.cdx.json":
			res.add(s.File, "the manifest names an unsupported SBOM format or file")
		case !ok:
			res.add(s.File, "the manifest names this SBOM but %s does not list it", SumsFile)
		case d != s.Digest:
			res.add(s.File, "manifest sbom.digest %s differs from %s (%s)", s.Digest, SumsFile, d)
		}
	}
}

func checkPlanCommands(dir string, plan Plan, m Manifest, res *VerifyResult) {
	if plan.Target == "" {
		if len(plan.Commands) != 0 || len(plan.Upload) != 0 || plan.NPM != nil || plan.Ref != "" {
			res.add(PlanFile, "plan has no target but lists commands or uploads")
		}
		return
	}
	switch plan.Target {
	case TargetGitHubRelease:
		checkGitHubPlan(plan, m, res)
	case TargetNPM:
		checkNPMPlanFile(plan, m, res)
	case TargetOCI:
		checkOCIPlan(dir, plan, m, res)
	default:
		res.add(PlanFile, "unknown plan target %q", plan.Target)
	}
}

func checkNPMPlanFile(plan Plan, m Manifest, res *VerifyResult) {
	if plan.NPM == nil {
		res.add(PlanFile, "an npm plan needs its npm section")
		return
	}
	if err := checkNPMPlan(*plan.NPM, m.Name, m.Version); err != nil {
		res.add(PlanFile, "%v", err)
		return
	}
	want := []Command{{Argv: NPMPackArgv(), Cwd: "."}, {Argv: NPMPublishArgv(*plan.NPM), Cwd: "."}}
	if !reflect.DeepEqual(plan.Commands, want) || len(plan.Upload) != 0 {
		res.add(PlanFile, "plan commands differ from the ones `publish --execute` builds for the npm target")
	}
}

func checkOCIPlan(dir string, plan Plan, m Manifest, res *VerifyResult) {
	ref, err := ociReference(strings.TrimSuffix(plan.Ref, ":"+OCITag(m.Version)), m.Version)
	if err != nil || ref != plan.Ref {
		res.add(PlanFile, "plan ref %q is not a repository tagged %s", plan.Ref, OCITag(m.Version))
		return
	}
	if len(plan.Commands) != 0 {
		res.add(PlanFile, "an oci plan runs no process but lists commands")
	}
	if !reflect.DeepEqual(plan.Upload, ociUploadList(m)) {
		res.add(PlanFile, "plan uploads %v, expected %v", plan.Upload, ociUploadList(m))
	}
	packed, err := readRegular(filepath.Join(dir, filepath.FromSlash(OCIManifestFile)))
	if err != nil {
		res.add(OCIManifestFile, "an oci plan needs its packed manifest: %v", err)
		return
	}
	if got := Digest(packed); got != plan.OCIDigest {
		res.add(PlanFile, "plan oci_digest %s differs from %s (%s)", plan.OCIDigest, OCIManifestFile, got)
	}
	files, manifest, manifestBytes, err := readDistFiles(dir, plan)
	if err != nil {
		res.add(PlanFile, "cannot rebuild the OCI manifest: %v", err)
		return
	}
	a, err := ociArtifact(manifest, manifestBytes, files, archiveMtime(files[manifest.Bundle.File]))
	if err != nil {
		res.add(PlanFile, "%v", err)
		return
	}
	rebuilt, err := oci.Pack(context.Background(), a)
	if err != nil {
		res.add(OCIManifestFile, "cannot rebuild the OCI manifest: %v", err)
		return
	}
	if rebuilt.Digest != plan.OCIDigest {
		res.add(OCIManifestFile, "the dist files pack to %s, the plan records %s", rebuilt.Digest, plan.OCIDigest)
	}
}

func checkGitHubPlan(plan Plan, m Manifest, res *VerifyResult) {
	if err := ValidateTarget(plan.Tag, plan.Repo); err != nil {
		res.add(PlanFile, "%v", err)
		return
	}
	upload := uploadList(m)
	want := Command{Argv: ReleaseCreateArgv(m.Name, m.Version, plan.Tag, plan.Repo, upload), Cwd: "."}
	if !reflect.DeepEqual(plan.Upload, upload) {
		res.add(PlanFile, "plan uploads %v, expected %v", plan.Upload, upload)
	}
	if len(plan.Commands) != 1 || !reflect.DeepEqual(plan.Commands[0], want) {
		res.add(PlanFile, "plan commands differ from the one `publish --execute` builds: %s", strings.Join(want.Argv, " "))
	}
}

// pluginManifestFiles are the runtime manifests that name the plugin (and mostly
// its version) inside the archive.
var pluginManifestFiles = []string{
	".claude-plugin/plugin.json", ".cursor-plugin/plugin.json", ".codex-plugin/plugin.json",
	".factory-plugin/plugin.json", "gemini-extension.json", "kimi.plugin.json", "plugin.json",
}

// checkInArchiveIdentity compares the name and version the archive's own
// runtime manifests carry with the release manifest's: a manifest relabelled
// after the archive was built (an old archive under a new version, or another
// plugin's under this name) is a mismatch.
func checkInArchiveIdentity(m Manifest, manifests map[string][]byte, res *VerifyResult) {
	for _, path := range pluginManifestFiles {
		raw, ok := manifests[path]
		if !ok {
			continue
		}
		var doc struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue // not this runtime's JSON manifest; the digest pass covers the bytes
		}
		if doc.Name != "" && doc.Name != m.Name {
			res.add(path, "the archive names the plugin %q, the manifest %q", doc.Name, m.Name)
		}
		if doc.Version != "" && doc.Version != m.Version {
			res.add(path, "the archive is version %q, the manifest %q", doc.Version, m.Version)
		}
	}
}

// readArchive reads a bundle archive and checks the determinism rules.
func readArchive(data []byte) ([]FileEntry, error) {
	entries, _, err := readArchiveManifests(data)
	return entries, err
}

// readArchiveManifests is readArchive that also returns the bodies of the
// runtime manifests (pluginManifestFiles).
func readArchiveManifests(data []byte) ([]FileEntry, map[string][]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, oops.Wrapf(err, "not a gzip archive")
	}
	if zr.Name != "" || zr.Comment != "" || !zr.ModTime.IsZero() && zr.ModTime.Unix() != 0 {
		return nil, nil, oops.Errorf("gzip header carries a name, comment or time")
	}
	tr := tar.NewReader(zr)
	var out []FileEntry
	manifests := map[string][]byte{}
	var mtime int64 = -1
	prev := ""
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, manifests, nil
		}
		if err != nil {
			return nil, nil, oops.Wrapf(err, "read archive")
		}
		if err := checkHeader(hdr, mtime, prev); err != nil {
			return nil, nil, err
		}
		mtime, prev = hdr.ModTime.Unix(), hdr.Name
		if hdr.Size < 0 || hdr.Size > maxVerifyBytes-total {
			return nil, nil, oops.Errorf("archive exceeds the %d byte verification limit", maxVerifyBytes)
		}
		total += hdr.Size
		body, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil || int64(len(body)) != hdr.Size {
			return nil, nil, oops.Errorf("archive entry %q is truncated", hdr.Name)
		}
		if slices.Contains(pluginManifestFiles, hdr.Name) {
			manifests[hdr.Name] = body
		}
		out = append(out, FileEntry{Path: hdr.Name, Size: len(body), Digest: Digest(body)})
	}
}

// readRegular reads a regular file and refuses a symlink.
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if !info.Mode().IsRegular() {
		return nil, oops.Errorf("%s is not a regular file", filepath.Base(path))
	}
	if info.Size() > maxVerifyBytes {
		return nil, oops.Errorf("%s exceeds the %d byte verification limit", filepath.Base(path), maxVerifyBytes)
	}
	data, err := os.ReadFile(path) //nolint:gosec // an explicit dist directory chosen by the caller
	return data, oops.Wrap(err)
}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}

// checkHeader applies the determinism rules to one entry; mtime is -1 for the first.
func checkHeader(hdr *tar.Header, mtime int64, prev string) error {
	if hdr.Typeflag != tar.TypeReg || !ValidPath(hdr.Name) {
		return oops.Errorf("archive entry %q is not a regular file with a safe path", hdr.Name)
	}
	if hdr.Uid != 0 || hdr.Gid != 0 || hdr.Uname != "" || hdr.Gname != "" ||
		(hdr.Mode != modeFile && hdr.Mode != modeExecutable) || len(hdr.PAXRecords) > 0 {
		return oops.Errorf("archive entry %q has a non-normalised owner, mode or PAX record", hdr.Name)
	}
	if mtime >= 0 && hdr.ModTime.Unix() != mtime {
		return oops.Errorf("archive entry %q has a different mtime from its siblings", hdr.Name)
	}
	if prev != "" && hdr.Name <= prev {
		return oops.Errorf("archive entry %q is out of order or duplicated", hdr.Name)
	}
	return nil
}
