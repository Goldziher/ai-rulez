package signing

import (
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const (
	// SubjectRelease is an ai-rulez release binary (`verify --self`).
	SubjectRelease = "release"

	// ReleaseIssuer is the OIDC issuer of the GitHub Actions workflow that
	// publishes ai-rulez releases.
	ReleaseIssuer = "https://token.actions.githubusercontent.com"
	// ReleaseIdentityRegexp matches the publish workflow of the ai-rulez
	// repository on a version tag; it is anchored (AR722).
	ReleaseIdentityRegexp = `^https://github\.com/Goldziher/ai-rulez/\.github/workflows/publish\.yaml@refs/tags/v[0-9][^/]*$`
)

// SelfOptions are the inputs of VerifySelf.
type SelfOptions struct {
	// Exe is the binary to verify (the running executable).
	Exe string
	// BundlePath is the attestation; default Exe + ".sigstore.json".
	BundlePath string
	// TrustedRoot is a trusted root file; default the cache of `trust update`.
	TrustedRoot string
	// PublicKeys and Identity with Issuer replace the pinned release workflow
	// identity: they name who signed a build that did not come from the
	// official workflow (a fork, a local build).
	PublicKeys       []string
	Identity, Issuer string
	Env              ambient.Env
	Now              time.Time
}

// SelfReport is a verified release attestation.
type SelfReport struct {
	*ArtifactReport
	// Attestation is the bundle file that was verified.
	Attestation string
	// Digest is the sha256 (hex) of the binary.
	Digest string
}

// VerifySelf verifies an ai-rulez binary against the Sigstore bundle its release
// published: an in-toto SLSA provenance statement that names the binary's sha256
// and was signed by the release workflow's identity (a log proof is required for
// a certificate identity). It is offline, writes nothing and keeps no rollback
// state: the question is whether these bytes are an official build.
func VerifySelf(o SelfOptions) (*SelfReport, error) {
	fs, err := ReadFileSubject(o.Exe)
	if err != nil {
		return nil, err
	}
	path := o.BundlePath
	if path == "" {
		path = o.Exe + ".sigstore.json"
	}
	cfg := &config.Config{BaseDir: filepath.Dir(o.Exe)}
	vo := VerifyOptions{TrustedRoot: o.TrustedRoot, PublicKeys: o.PublicKeys, Identity: o.Identity, Issuer: o.Issuer, Subject: SubjectRelease, Env: o.Env}
	trust, err := buildTrust(cfg, vo)
	if err != nil {
		return nil, err
	}
	if len(trust.Entries) == 0 {
		trust.Entries = []TrustEntry{{Subject: SubjectRelease, IdentityRegexp: ReleaseIdentityRegexp, Issuer: ReleaseIssuer}}
	}
	verifier := Verifier{Keys: trust.Keys(SubjectRelease), TLog: TLogOff}
	if trust.HasIdentities(SubjectRelease) {
		verifier.TLog = TLogRequired
	}
	if verifier.TrustedRoot, err = loadTrustedRoot(cfg, vo); err != nil {
		return nil, err
	}
	bundles, err := ReadBundleFiles(path)
	if err != nil {
		return nil, err
	}
	rep, err := VerifyArtifact(bundles, Expectation{PredicateType: PredicateSLSA, DigestHex: fs.DigestHex}, ArtifactPolicy{
		Subject: SubjectRelease, Verifier: verifier, Trust: trust, Threshold: 1, Now: o.Now,
	})
	if err != nil {
		return nil, err
	}
	return &SelfReport{ArtifactReport: rep, Attestation: path, Digest: fs.DigestHex}, nil
}
