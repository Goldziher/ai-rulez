package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/oci"
)

// OCIManifestFile is the dist path of the packed OCI image manifest. Its digest
// is the digest the registry reports after the push.
const OCIManifestFile = "oci/manifest.json"

var ociTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// OCITag maps a plugin version to an OCI tag: "+" (semver build metadata) is
// not allowed in a tag and becomes "_".
func OCITag(version string) string { return strings.ReplaceAll(version, "+", "_") }

// ociReference joins the repository and the version tag and validates both.
func ociReference(repository, version string) (string, error) {
	if repository == "" {
		return "", newError(CodeConfig, ExitFailed, "set [publish.oci] ref or pass --oci-ref host/path", "the oci target needs a repository")
	}
	tag := OCITag(version)
	if !ociTagPattern.MatchString(tag) {
		return "", newError(CodeConfig, ExitFailed, "", "version %q cannot be an OCI tag", version)
	}
	ref := repository + ":" + tag
	parsed, err := oci.ParseRepository(ref)
	if err != nil || parsed.Reference != tag || parsed.Registry == "" || !strings.Contains(repository, "/") {
		return "", newError(CodeConfig, ExitFailed, "use host/path without a tag or digest, such as ghcr.io/acme/skills/conventions", "%q is not an OCI repository", repository)
	}
	return ref, nil
}

// ociUploadList names the files that become layers, in layer order.
func ociUploadList(m Manifest) []string {
	up := []string{m.Bundle.File, LockFile}
	if m.SBOM != nil {
		up = append(up, m.SBOM.File)
	}
	if m.Signature != nil {
		up = append(up, m.Signature.File)
		if m.Signature.Attestation != "" {
			up = append(up, m.Signature.Attestation)
		}
	}
	return up
}

func layerMediaType(m Manifest, name string) string {
	switch {
	case name == m.Bundle.File:
		return oci.BundleMediaType
	case name == LockFile:
		return oci.LockMediaType
	case m.SBOM != nil && name == m.SBOM.File:
		return oci.SBOMMediaType
	default:
		return oci.SignatureMediaType
	}
}

// ociArtifact builds the artifact from the dist files, which must hold every
// file ociUploadList names.
func ociArtifact(m Manifest, manifestBytes []byte, files map[string][]byte, mtime int64) (oci.Artifact, error) {
	if mtime < 0 {
		mtime = 0
	}
	a := oci.Artifact{
		Config: manifestBytes, Version: m.Version, Source: m.Source.Repo, Revision: m.Source.Commit, Created: mtime,
	}
	for _, name := range ociUploadList(m) {
		data, ok := files[name]
		if !ok {
			return a, newError(CodeTarget, ExitFailed, "", "the dist directory has no %s to push", name)
		}
		a.Layers = append(a.Layers, oci.Layer{MediaType: layerMediaType(m, name), Title: name, Data: data})
	}
	return a, nil
}

// packOCI packs the artifact of a built dist.
func packOCI(m Manifest, manifestBytes []byte, files map[string][]byte, mtime int64) (oci.Packed, error) {
	a, err := ociArtifact(m, manifestBytes, files, mtime)
	if err != nil {
		return oci.Packed{}, err
	}
	packed, err := oci.Pack(context.Background(), a)
	if err != nil {
		return oci.Packed{}, err //nolint:wrapcheck // already contextual
	}
	return packed, nil
}

// archiveMtime reads the fixed mtime of a release archive (every entry shares it).
func archiveMtime(archive []byte) int64 {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return 0
	}
	hdr, err := tar.NewReader(zr).Next()
	if err != nil {
		return 0
	}
	return hdr.ModTime.Unix()
}

// OCIExecuteOptions configure ExecuteOCI.
type OCIExecuteOptions struct {
	// Dir is the written dist directory the layers are read from.
	Dir string
	// Target overrides the registry client (tests).
	Target oci.Target
	// Force replaces a tag that already points at a different artifact.
	Force bool
}

// ExecuteOCI pushes the artifact the plan describes. It rebuilds the manifest
// from the dist files and refuses to push unless its digest equals the one the
// plan recorded, so an edited plan or file cannot push something other than what
// was reviewed. It returns the pushed reference with its digest.
func ExecuteOCI(ctx context.Context, plan Plan, opts OCIExecuteOptions) (string, error) {
	if plan.Target != TargetOCI || plan.Ref == "" || plan.OCIDigest == "" {
		return "", newError(CodeTarget, ExitFailed, "", "the plan has no oci push to run")
	}
	files, manifest, manifestBytes, err := readDistFiles(opts.Dir, plan)
	if err != nil {
		return "", err
	}
	a, err := ociArtifact(manifest, manifestBytes, files, archiveMtime(files[manifest.Bundle.File]))
	if err != nil {
		return "", err
	}
	packed, err := oci.Pack(ctx, a)
	if err != nil {
		return "", err //nolint:wrapcheck // already contextual
	}
	if packed.Digest != plan.OCIDigest || Digest(files[OCIManifestFile]) != plan.OCIDigest {
		return "", newError(CodeTarget, ExitFailed, "rebuild the dist directory with `ai-rulez publish`", "the dist files no longer match the plan: the OCI digest differs")
	}
	t := opts.Target
	t.Ref = plan.Ref
	repo := plan.Ref[:strings.LastIndex(plan.Ref, ":")]
	existing, err := CheckOCI(ctx, plan, opts)
	if err != nil {
		return "", err
	}
	if existing == plan.OCIDigest {
		return repo + "@" + existing, nil // already published: nothing to push
	}
	digest, err := oci.Push(ctx, t, a)
	if err != nil {
		return "", newError(CodeTarget, ExitFailed, "check `docker login` for the registry and that the repository exists",
			"push to %s failed: %s", plan.Ref, Redact(err.Error()))
	}
	if digest != plan.OCIDigest {
		return "", newError(CodeTarget, ExitFailed, "", "the registry holds %s, the plan recorded %s", digest, plan.OCIDigest)
	}
	return repo + "@" + digest, nil
}

// CheckOCI asks the registry what the plan's tag points at and refuses to
// overwrite a different artifact unless opts.Force is set (an OCI tag is
// mutable, so a silent push would replace a release consumers pinned by tag). It
// returns the digest the tag holds, "" when it is free.
func CheckOCI(ctx context.Context, plan Plan, opts OCIExecuteOptions) (string, error) {
	t := opts.Target
	t.Ref = plan.Ref
	existing, found, err := oci.Resolve(ctx, t)
	if err != nil {
		return "", newError(CodeTarget, ExitFailed, "check `docker login` for the registry and that it is reachable",
			"cannot tell whether %s already exists: %s", plan.Ref, Redact(err.Error()))
	}
	if found && existing != plan.OCIDigest && !opts.Force {
		return "", newError(CodeTarget, ExitFailed, "bump [plugin] version, or pass --force to replace the tag",
			"%s already exists and holds %s; OCI tags are mutable, so replacing a release needs --force", plan.Ref, existing)
	}
	return existing, nil
}

// readDistFiles reads the files an OCI push needs from a dist directory.
func readDistFiles(dir string, plan Plan) (map[string][]byte, Manifest, []byte, error) {
	manifestName := plan.Name + "-" + plan.Version + ".manifest.json"
	files := map[string][]byte{}
	read := func(name string) ([]byte, error) {
		data, err := readRegular(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, newError(CodeTarget, ExitFailed, "run `ai-rulez publish` first", "cannot read %s: %v", name, err)
		}
		return data, nil
	}
	manifestBytes, err := read(manifestName)
	if err != nil {
		return nil, Manifest{}, nil, err
	}
	m, err := decodeManifest(manifestBytes)
	if err != nil {
		return nil, Manifest{}, nil, newError(CodeTarget, ExitFailed, "", "invalid manifest: %v", err)
	}
	for _, name := range append(ociUploadList(m), OCIManifestFile) {
		if files[name], err = read(name); err != nil {
			return nil, Manifest{}, nil, err
		}
	}
	return files, m, manifestBytes, nil
}

// PullOCI fetches a published artifact into dir as a dist directory
// `publish verify` accepts: the manifest, the layers by their titles and a
// SHA256SUMS computed from what arrived (the registry's content addressing
// already guarantees each blob matches its digest, but a tag can point at
// anything: the SHA256SUMS prove nothing about authenticity). It returns the
// manifest digest the reference resolved to.
func PullOCI(ctx context.Context, t oci.Target, dir string) (string, error) {
	pulled, err := oci.Pull(ctx, t)
	if err != nil {
		return "", newError(CodeVerify, ExitFailed, "", "cannot pull %s: %s", t.Ref, Redact(err.Error()))
	}
	m, err := decodeManifest(pulled.Config)
	if err != nil {
		return "", newError(CodeVerify, ExitFailed, "", "%s does not carry a publish manifest: %v", t.Ref, err)
	}
	manifestName := m.Name + "-" + m.Version + ".manifest.json"
	files := map[string][]byte{manifestName: pulled.Config}
	for _, l := range pulled.Layers {
		if !ValidPath(l.Title) || strings.Contains(l.Title, "/") {
			return "", newError(CodeVerify, ExitFailed, "", "%s has a layer with an unusable title %q", t.Ref, l.Title)
		}
		// A layer titled like the manifest or the checksums, or like an earlier layer, would silently replace
		// that file: the sums are then computed over the replacement and verify agrees with itself.
		if _, dup := files[l.Title]; dup || l.Title == SumsFile || l.Title == PlanFile {
			return "", newError(CodeVerify, ExitFailed, "", "%s has a layer titled %q, which would replace another file of the release", t.Ref, l.Title)
		}
		files[l.Title] = l.Data
	}
	sums := make([]SumEntry, 0, len(files))
	for p, data := range files {
		sums = append(sums, SumEntry{Path: p, Digest: Digest(data)})
	}
	files[SumsFile] = FormatSums(sums)
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return "", oops.With("path", name).Wrapf(err, "write pulled file")
		}
	}
	return pulled.Digest, nil
}

// OCIRefIsMutable reports whether ref names its artifact by a tag, which the
// registry's owner can move, rather than by digest.
func OCIRefIsMutable(ref string) bool {
	parsed, err := oci.ParseRepository(ref)
	return err != nil || !strings.HasPrefix(parsed.Reference, "sha256:")
}
