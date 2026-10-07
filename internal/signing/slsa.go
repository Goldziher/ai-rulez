package signing

import (
	"slices"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

const (
	// PredicateSLSA is the in-toto predicate type of SLSA provenance v1.
	PredicateSLSA = "https://slsa.dev/provenance/v1"
	// BuildTypeBundle is the build type ai-rulez records for a plugin bundle.
	BuildTypeBundle = "https://github.com/Goldziher/ai-rulez/buildtypes/plugin-bundle/v1"
	// DefaultBuilderID is the builder id recorded outside a CI system that names
	// its workflow.
	DefaultBuilderID = "https://github.com/Goldziher/ai-rulez/builders/cli/v1"
)

// SLSAProvenance is the SLSA v1 provenance predicate (https://slsa.dev/spec/v1.0/provenance).
type SLSAProvenance struct {
	BuildDefinition SLSABuildDefinition `json:"buildDefinition"`
	RunDetails      SLSARunDetails      `json:"runDetails"`
}

// SLSABuildDefinition says what was built and from what.
type SLSABuildDefinition struct {
	BuildType            string            `json:"buildType"`
	ExternalParameters   map[string]any    `json:"externalParameters"`
	InternalParameters   map[string]any    `json:"internalParameters,omitempty"`
	ResolvedDependencies []SLSAResourceRef `json:"resolvedDependencies,omitempty"`
}

// SLSAResourceRef is a resolved input: a source repository at a commit, or a file by digest.
type SLSAResourceRef struct {
	URI    string            `json:"uri,omitempty"`
	Name   string            `json:"name,omitempty"`
	Digest map[string]string `json:"digest,omitempty"`
}

// SLSARunDetails says who ran the build.
type SLSARunDetails struct {
	Builder  SLSABuilder  `json:"builder"`
	Metadata SLSAMetadata `json:"metadata,omitempty"`
}

// SLSABuilder identifies the build platform.
type SLSABuilder struct {
	ID      string            `json:"id"`
	Version map[string]string `json:"version,omitempty"`
}

// SLSAMetadata is the invocation's identity and time.
type SLSAMetadata struct {
	InvocationID string `json:"invocationId,omitempty"`
	StartedOn    string `json:"startedOn,omitempty"`
	FinishedOn   string `json:"finishedOn,omitempty"`
}

// ProvenanceInput is what `sign --bundle --provenance` knows about the build.
// ai-rulez does not run a hermetic build, so the provenance is the signer's
// account of where the bundle was generated, not a builder's attestation: it
// reaches SLSA build level 1 at most. Use it to tie a bundle to a repository,
// commit and workflow, and verify it with a builder allowlist.
type ProvenanceInput struct {
	// BuilderID identifies the build platform: a CI workflow reference, or DefaultBuilderID.
	BuilderID string
	// Version is the ai-rulez version that generated the bundle.
	Version string
	// Repository, Ref and Commit identify the source.
	Repository, Ref, Commit string
	// InvocationID identifies the run (a CI run URL), when known.
	InvocationID string
	// LockSubject is the sha256 (hex) of the lock subject the bundle was
	// generated under, when there is a lock.
	LockSubject string
	Now         time.Time
}

// ProvenanceStatement builds the SLSA provenance statement of the bundle ts.
func ProvenanceStatement(ts TreeSubject, in ProvenanceInput) (*Statement, error) {
	if in.BuilderID == "" {
		in.BuilderID = DefaultBuilderID
	}
	now := in.Now.UTC().Truncate(time.Second).Format(time.RFC3339)
	ext := map[string]any{"bundle": ts.Name}
	if in.Repository != "" {
		ext["repository"] = in.Repository
	}
	if in.Ref != "" {
		ext["ref"] = in.Ref
	}
	var deps []SLSAResourceRef
	if in.Repository != "" && in.Commit != "" {
		deps = append(deps, SLSAResourceRef{URI: "git+" + in.Repository, Digest: map[string]string{"gitCommit": in.Commit}})
	}
	if in.LockSubject != "" {
		deps = append(deps, SLSAResourceRef{Name: LockSubjectName, Digest: map[string]string{contentlock.Algorithm: in.LockSubject}})
	}
	pred := SLSAProvenance{
		BuildDefinition: SLSABuildDefinition{
			BuildType: BuildTypeBundle, ExternalParameters: ext,
			InternalParameters:   map[string]any{"ai_rulez_version": in.Version},
			ResolvedDependencies: deps,
		},
		RunDetails: SLSARunDetails{
			Builder:  SLSABuilder{ID: in.BuilderID, Version: map[string]string{"ai-rulez": in.Version}},
			Metadata: SLSAMetadata{InvocationID: in.InvocationID, StartedOn: now, FinishedOn: now},
		},
	}
	return NewStatement(PredicateSLSA, []Subject{{Name: ts.Name, Digest: map[string]string{contentlock.Algorithm: ts.HexDigest()}}}, pred)
}

// decodeClaim reads the signer's own claim of time and repository from a
// verified statement: issued_at and repository for the ai-rulez predicates, the
// run's finishedOn and the repository parameter for SLSA provenance.
func decodeClaim(st *Statement) (artifactClaim, error) {
	if st.PredicateType != PredicateSLSA {
		var c artifactClaim
		err := st.DecodePredicate(&c)
		return c, err
	}
	var p SLSAProvenance
	if err := st.DecodePredicate(&p); err != nil {
		return artifactClaim{}, err
	}
	var c artifactClaim
	if t, err := time.Parse(time.RFC3339, p.RunDetails.Metadata.FinishedOn); err == nil {
		c.IssuedAt = t
	}
	if repo, ok := p.BuildDefinition.ExternalParameters["repository"].(string); ok {
		c.Repository = repo
	}
	return c, nil
}

// VerifyProvenance verifies the SLSA provenance attestation of the bundle ts:
// the signature and signer as for the bundle itself (trust subject bundle), a
// statement that names this bundle's digest, and, when builders is not empty, a
// builder id from that list. Anything the provenance itself gets wrong is AR729.
func (c *ArtifactCheck) VerifyProvenance(bundles [][]byte, ts TreeSubject, builders []string) (*ArtifactReport, *SLSAProvenance, error) {
	if len(bundles) == 0 {
		return nil, nil, Errorf(CodeProvenance, "no SLSA provenance next to the bundle; sign it with `ai-rulez sign --bundle --provenance`")
	}
	// The builder that attests provenance is one signer: the bundle's threshold does not apply to it.
	p := c.Policy
	p.Threshold = 1
	rep, err := VerifyArtifact(bundles, Expectation{PredicateType: PredicateSLSA, DigestHex: ts.HexDigest()}, p)
	if err != nil {
		return nil, nil, err
	}
	var prov SLSAProvenance
	if err := rep.Statement.DecodePredicate(&prov); err != nil {
		return nil, nil, Errorf(CodeProvenance, "the provenance predicate is not SLSA provenance v1: %v", err)
	}
	if prov.RunDetails.Builder.ID == "" {
		return nil, nil, Errorf(CodeProvenance, "the provenance names no builder id")
	}
	if len(builders) > 0 && !slices.Contains(builders, prov.RunDetails.Builder.ID) {
		return nil, nil, Errorf(CodeProvenance, "the provenance was produced by builder %q, which [signing] builders does not list", prov.RunDetails.Builder.ID)
	}
	return rep, &prov, nil
}

// ProvenanceSidecarFor is the provenance attestation path next to the bundle
// attestation at path: "X.sigstore.json" becomes "X.provenance.sigstore.json".
func ProvenanceSidecarFor(path string) string {
	if len(path) > len(bundleSuffix) && path[len(path)-len(bundleSuffix):] == bundleSuffix {
		return path[:len(path)-len(bundleSuffix)] + ".provenance" + bundleSuffix
	}
	return path + ".provenance" + bundleSuffix
}
