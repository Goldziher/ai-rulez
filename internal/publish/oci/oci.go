// Package oci packs a published plugin bundle as an OCI artifact and moves it
// to and from a registry with oras-go. Packing is pure and reproducible (the
// manifest carries a fixed creation time), so the artifact digest is known
// before anything is pushed. Pushing reads registry credentials from the Docker
// credential store (DOCKER_CONFIG or ~/.docker/config.json), the same place
// `docker login` writes them; they are used only for the registry the
// reference names and are never logged or returned.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/oops"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Media types of the artifact (docs/publish.md).
const (
	ArtifactType       = "application/vnd.ai-rulez.bundle.v1"
	ConfigMediaType    = "application/vnd.ai-rulez.manifest.v1+json"
	BundleMediaType    = "application/vnd.ai-rulez.bundle.v1.tar+gzip"
	LockMediaType      = "application/vnd.ai-rulez.lock.v1+toml"
	SBOMMediaType      = "application/vnd.cyclonedx+json"
	SignatureMediaType = "application/vnd.dev.sigstore.bundle.v0.3+json"
)

// Annotation keys beyond the OCI image-spec ones.
const (
	annotationTitle    = ocispec.AnnotationTitle
	annotationVersion  = ocispec.AnnotationVersion
	annotationSource   = ocispec.AnnotationSource
	annotationRevision = ocispec.AnnotationRevision
	annotationCreated  = ocispec.AnnotationCreated
)

const (
	requestTimeout = 5 * time.Minute
	maxBlobBytes   = 512 << 20
)

// Layer is one blob of the artifact. Title is the file name it came from.
type Layer struct {
	MediaType string
	Title     string
	Data      []byte
}

// Artifact is what gets pushed: the bundle manifest as the config blob and the
// files as layers, in order.
type Artifact struct {
	Config []byte
	Layers []Layer
	// Version, Source and Revision become the image-spec annotations of the same name.
	Version, Source, Revision string
	// Created is the fixed creation time (Unix seconds); the manifest never
	// records the time of the build.
	Created int64
}

// Packed is a packed artifact: the exact manifest bytes and their digest.
type Packed struct {
	Manifest []byte
	Digest   string
}

func (a Artifact) annotations() map[string]string {
	out := map[string]string{annotationCreated: time.Unix(a.Created, 0).UTC().Format(time.RFC3339)}
	for k, v := range map[string]string{annotationVersion: a.Version, annotationSource: a.Source, annotationRevision: a.Revision} {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func (a Artifact) descriptors() (config ocispec.Descriptor, layers []ocispec.Descriptor) {
	config = content.NewDescriptorFromBytes(ConfigMediaType, a.Config)
	for _, l := range a.Layers {
		d := content.NewDescriptorFromBytes(l.MediaType, l.Data)
		d.Annotations = map[string]string{annotationTitle: l.Title}
		layers = append(layers, d)
	}
	return config, layers
}

// Pack builds the image manifest into an in-memory store and returns its bytes.
// Equal inputs give equal bytes, so Packed.Digest can be recorded in a publish
// plan before anything is pushed.
func Pack(ctx context.Context, a Artifact) (Packed, error) {
	store := memory.New()
	if _, err := a.stage(ctx, store); err != nil {
		return Packed{}, err
	}
	return a.pack(ctx, store)
}

func (a Artifact) stage(ctx context.Context, store *memory.Store) (ocispec.Descriptor, error) {
	config, _ := a.descriptors()
	if err := store.Push(ctx, config, bytes.NewReader(a.Config)); err != nil {
		return ocispec.Descriptor{}, oops.Wrapf(err, "stage the config blob")
	}
	for _, l := range a.Layers {
		d := content.NewDescriptorFromBytes(l.MediaType, l.Data)
		if err := store.Push(ctx, d, bytes.NewReader(l.Data)); err != nil {
			return ocispec.Descriptor{}, oops.With("layer", l.Title).Wrapf(err, "stage a layer")
		}
	}
	return config, nil
}

func (a Artifact) pack(ctx context.Context, store *memory.Store) (Packed, error) {
	config, layers := a.descriptors()
	root, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, ArtifactType, oras.PackManifestOptions{
		ConfigDescriptor:    &config,
		Layers:              layers,
		ManifestAnnotations: a.annotations(),
	})
	if err != nil {
		return Packed{}, oops.Wrapf(err, "pack the OCI manifest")
	}
	if err := store.Tag(ctx, root, root.Digest.String()); err != nil {
		return Packed{}, oops.Wrapf(err, "tag the packed manifest")
	}
	rc, err := store.Fetch(ctx, root)
	if err != nil {
		return Packed{}, oops.Wrapf(err, "read the packed manifest")
	}
	defer rc.Close() //nolint:errcheck // in-memory reader
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return Packed{}, oops.Wrapf(err, "read the packed manifest")
	}
	return Packed{Manifest: buf.Bytes(), Digest: root.Digest.String()}, nil
}

// Target says where to push or pull.
type Target struct {
	// Ref is "host/path:tag" (push) or "host/path:tag" / "host/path@sha256:..." (pull).
	Ref string
	// Credentials overrides the Docker credential store (tests).
	Credentials auth.CredentialFunc
}

// ParseRepository checks that ref names a repository and a tag, with no digest.
func ParseRepository(ref string) (registry.Reference, error) {
	r, err := registry.ParseReference(ref)
	if err != nil {
		return registry.Reference{}, oops.Wrapf(err, "invalid OCI reference %q", ref)
	}
	return r, nil
}

func repoFor(t Target) (*remote.Repository, registry.Reference, error) {
	ref, err := ParseRepository(t.Ref)
	if err != nil {
		return nil, ref, err
	}
	repo, err := remote.NewRepository(ref.Registry + "/" + ref.Repository)
	if err != nil {
		return nil, ref, oops.Wrapf(err, "invalid OCI repository")
	}
	repo.PlainHTTP = isLoopback(ref.Registry)
	cred := t.Credentials
	if cred == nil {
		store, serr := credentials.NewStoreFromDocker(credentials.StoreOptions{})
		if serr == nil {
			cred = credentials.Credential(store)
		}
	}
	repo.Client = &auth.Client{Client: retry.DefaultClient, Cache: auth.NewCache(), Credential: cred}
	return repo, ref, nil
}

// isLoopback reports whether the registry host is the local machine: plain HTTP
// is allowed there only (a local test registry), never for a remote one.
func isLoopback(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Push uploads the artifact and tags it with the reference's tag. It returns the
// manifest digest, which must equal what Pack reported.
func Push(ctx context.Context, t Target, a Artifact) (string, error) {
	repo, ref, err := repoFor(t)
	if err != nil {
		return "", err
	}
	if ref.Reference == "" || strings.Contains(ref.Reference, ":") {
		return "", oops.Errorf("%q must carry a tag to push to", t.Ref)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	store := memory.New()
	if _, err := a.stage(ctx, store); err != nil {
		return "", err
	}
	packed, err := a.pack(ctx, store)
	if err != nil {
		return "", err
	}
	if _, err := oras.Copy(ctx, store, packed.Digest, repo, ref.Reference, oras.DefaultCopyOptions); err != nil {
		return "", oops.With("ref", t.Ref).Wrapf(err, "push the OCI artifact")
	}
	return packed.Digest, nil
}

// Pulled is an artifact read back from a registry.
type Pulled struct {
	Digest   string
	Manifest ocispec.Manifest
	Config   []byte
	Layers   []Layer
}

// Pull fetches the artifact ref names (a tag or a digest) and its blobs.
func Pull(ctx context.Context, t Target) (*Pulled, error) {
	repo, ref, err := repoFor(t)
	if err != nil {
		return nil, err
	}
	if ref.Reference == "" {
		return nil, oops.Errorf("%q names no tag or digest to pull", t.Ref)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	desc, rc, err := repo.FetchReference(ctx, ref.Reference)
	if err != nil {
		return nil, oops.With("ref", t.Ref).Wrapf(err, "fetch the OCI manifest")
	}
	raw, err := readLimited(rc)
	if err != nil {
		return nil, err
	}
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		return nil, oops.Errorf("%s is a %s, not an image manifest", t.Ref, desc.MediaType)
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, oops.Wrapf(err, "decode the OCI manifest")
	}
	if m.ArtifactType != ArtifactType {
		return nil, oops.Errorf("%s is not an ai-rulez bundle (artifact type %q)", t.Ref, m.ArtifactType)
	}
	out := &Pulled{Digest: desc.Digest.String(), Manifest: m}
	if out.Config, err = fetchBlob(ctx, repo, m.Config); err != nil {
		return nil, err
	}
	for _, l := range m.Layers {
		data, err := fetchBlob(ctx, repo, l)
		if err != nil {
			return nil, err
		}
		out.Layers = append(out.Layers, Layer{MediaType: l.MediaType, Title: l.Annotations[annotationTitle], Data: data})
	}
	return out, nil
}

func fetchBlob(ctx context.Context, repo *remote.Repository, d ocispec.Descriptor) ([]byte, error) {
	if d.Size < 0 || d.Size > maxBlobBytes {
		return nil, oops.Errorf("blob %s is larger than %d bytes", d.Digest, maxBlobBytes)
	}
	rc, err := repo.Fetch(ctx, d)
	if err != nil {
		return nil, oops.With("digest", d.Digest.String()).Wrapf(err, "fetch a blob")
	}
	data, err := readLimited(rc)
	if err != nil {
		return nil, err
	}
	if digest.FromBytes(data) != d.Digest {
		return nil, oops.Errorf("blob %s does not match its digest", d.Digest)
	}
	return data, nil
}
