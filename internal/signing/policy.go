package signing

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore-go/pkg/root"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// maxKeyBytes bounds a key or trusted root file.
const maxKeyBytes = 4 << 20

// TrustedRootFile is the cached public-good trusted root's file name, written by
// `ai-rulez trust update` under ~/.cache/ai-rulez/sigstore.
const TrustedRootFile = "trusted_root.json"

// VerifyOptions are the command-line additions to the [signing] policy. They add
// trusted signers and name files; they cannot remove what the config requires.
type VerifyOptions struct {
	// TrustedRoot is a trusted root file path (anywhere); overrides [signing] trusted_root.
	TrustedRoot string
	// PublicKeys are PEM public key paths to trust for the subject, in addition to the config.
	PublicKeys []string
	// Identity and Issuer trust one more keyless signer.
	Identity, Issuer string
	// Subject is the trust subject the flag-supplied signers apply to; default
	// SubjectLock. Only PrepareArtifactCheck reads it.
	Subject string
	// BundlePath is the attestation file; default [signing] attestation, then the
	// lock's sidecar.
	BundlePath string
	// NoState skips the rollback state: nothing is read or written.
	NoState bool
	Env     ambient.Env
	Now     time.Time
}

func (o VerifyOptions) subject() string {
	if o.Subject == "" {
		return SubjectLock
	}
	return o.Subject
}

// LockCheck is a prepared verification of one project's lock attestation.
type LockCheck struct {
	Policy LockPolicy
	Lock   *lockfile.File
	// BundlePath is the attestation file that was (or would be) read.
	BundlePath string
	// Warnings are non-fatal problems building the policy (an unusable rollback state).
	Warnings []string
	state    *State
}

// ReadBundles reads the attestation files of the lock: the primary and its
// co-signatures. Nothing found is AR720.
func (c *LockCheck) ReadBundles() ([][]byte, error) {
	data, err := ReadBundleFiles(c.BundlePath)
	var se *Error
	if errors.As(err, &se) && se.Code == CodeMissing {
		return nil, Errorf(CodeMissing, "no attestation at %s; sign the lock with `ai-rulez sign --lock`", c.BundlePath)
	}
	return data, err
}

// ReadBundleFiles reads the attestation at path and its co-signatures (see
// BundleFiles), each bounded by MaxBundleBytes. Nothing found is AR720.
func ReadBundleFiles(path string) ([][]byte, error) {
	files, err := BundleFiles(path)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(files))
	for _, p := range files {
		data, err := readBundleFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, nil
}

func readBundleFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the configured, flagged or sidecar attestation
	if errors.Is(err, os.ErrNotExist) {
		return nil, Errorf(CodeMissing, "no attestation at %s", path)
	}
	if err != nil {
		return nil, wrap(CodeMissing, err, "cannot read the attestation at %s", path)
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, MaxBundleBytes+1))
	if err != nil {
		return nil, wrap(CodeInvalid, err, "cannot read the attestation at %s", path)
	}
	return data, nil
}

// Verify reads and verifies the attestation files. It never writes the rollback
// state; call Commit on the report after accepting it.
func (c *LockCheck) Verify() (*LockReport, error) {
	bundles, err := c.ReadBundles()
	if err != nil {
		return nil, err
	}
	return VerifyLockSet(bundles, c.Lock, c.Policy)
}

// Commit records an accepted report in the rollback state, if there is one.
func (c *LockCheck) Commit(r *LockReport) error { return r.Commit(c.state) }

// checkBase is what every verification shares: who is trusted, how a bundle is
// verified, how fresh it must be, and the rollback state.
type checkBase struct {
	trust              TrustSet
	verifier           Verifier
	maxAge             time.Duration
	threshold          int
	state              *State
	scopeRel, scopeAbs string
	warnings           []string
}

// prepareBase builds the trust set and verifier for subject from the [signing]
// policy and the options. It fails when nobody is trusted for the subject: a
// verification without a trust set would accept anyone.
func prepareBase(cfg *config.Config, subject string, o VerifyOptions) (*checkBase, error) {
	s := cfg.Signing
	trust, err := buildTrust(cfg, o)
	if err != nil {
		return nil, err
	}
	if trust.CountFor(subject) == 0 {
		return nil, oops.Hint("set [signing] identity and issuer, key_file or [[signing.trust]] (with subject = \""+subject+"\"), or pass --public-key or --identity with --issuer").
			Errorf("no trusted signer is configured for %s: a valid signature alone does not say who may sign it", subject)
	}
	b := &checkBase{trust: trust, threshold: 1}
	if len(o.PublicKeys) == 0 && o.Identity == "" {
		b.warnings = append(b.warnings, "the trusted signers come from this repository's own [signing] configuration: whoever can change the repository can change who may sign. "+
			"Pass --public-key or --identity with --issuer from CI configuration outside the repository to pin them")
	}
	b.scopeRel, b.scopeAbs = stateScope(cfg.ConfigDir)
	b.verifier = Verifier{Keys: trust.Keys(subject), TLog: effectiveTLog(s, trust, subject)}
	if s != nil {
		if s.MaxAge != "" {
			if b.maxAge, err = config.ParseApprovalMaxAge(s.MaxAge); err != nil {
				return nil, oops.Wrap(err)
			}
		}
		if k := s.Thresholds[subject]; k > 1 {
			b.threshold = k
		}
	}
	if b.verifier.TrustedRoot, err = loadTrustedRoot(cfg, o); err != nil {
		return nil, err
	}
	if !o.NoState {
		statePath, secretPath := StatePaths(o.Env)
		st, serr := OpenState(statePath, secretPath)
		switch {
		case serr != nil:
			b.warnings = append(b.warnings, "rollback detection is off: "+serr.Error())
		default:
			if st.Reset {
				b.warnings = append(b.warnings, "the rollback state failed its integrity check and was reset")
			}
			b.state = st
		}
	}
	return b, nil
}

// PrepareLockCheck builds the verification of cfg's lock from the [signing]
// policy and the options. It fails when the lock cannot be read or no signer is
// trusted: a verification without a trust set would accept anyone.
func PrepareLockCheck(cfg *config.Config, o VerifyOptions) (*LockCheck, error) {
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if lock == nil || !lock.HasContentPins() {
		return nil, oops.Hint("run `ai-rulez lock` first").Errorf("%s has no content pins to verify an attestation against", lockfile.FileName)
	}
	o.Subject = SubjectLock
	base, err := prepareBase(cfg, SubjectLock, o)
	if err != nil {
		return nil, err
	}
	check := &LockCheck{Lock: lock, BundlePath: bundlePathFor(cfg, o), Warnings: base.warnings, state: base.state}
	check.Policy = LockPolicy{
		Trust: base.trust, Verifier: base.verifier, MaxAge: base.maxAge, Threshold: base.threshold, State: base.state,
		ScopeRel: base.scopeRel, ScopeAbs: base.scopeAbs, Now: o.Now,
	}
	if cfg.Signing != nil {
		check.Policy.MinHashVersion = cfg.Signing.MinHashVersion
	}
	return check, nil
}

// ArtifactCheck is a prepared verification of the attestations of non-lock
// subjects (bundles, published skills, SBOMs) under one policy.
type ArtifactCheck struct {
	Subject string
	Policy  ArtifactPolicy
	// Builders and RequireProvenance come from [signing] (bundles).
	Builders          []string
	RequireProvenance bool
	// Warnings are non-fatal problems building the policy.
	Warnings []string
	state    *State
}

// PrepareArtifactCheck builds the verification of subject (SubjectBundle,
// SubjectSkill or SubjectSBOM) from the [signing] policy and the options.
func PrepareArtifactCheck(cfg *config.Config, subject string, o VerifyOptions) (*ArtifactCheck, error) {
	o.Subject = subject
	base, err := prepareBase(cfg, subject, o)
	if err != nil {
		return nil, err
	}
	c := &ArtifactCheck{Subject: subject, Warnings: base.warnings, state: base.state}
	c.Policy = ArtifactPolicy{
		Subject: subject, Verifier: base.verifier, Trust: base.trust, MaxAge: base.maxAge, Threshold: base.threshold,
		State: base.state, ScopeRel: base.scopeRel, ScopeAbs: base.scopeAbs, Now: o.Now,
	}
	if cfg.Signing != nil {
		c.Builders, c.RequireProvenance = cfg.Signing.Builders, cfg.Signing.RequireProvenance
	}
	return c, nil
}

// VerifyFor verifies the attestations of one artifact (from source, for a skill
// scoped to its origin) against what the caller recomputed.
func (c *ArtifactCheck) VerifyFor(source string, bundles [][]byte, exp Expectation) (*ArtifactReport, error) {
	p := c.Policy
	p.Source = source
	return VerifyArtifact(bundles, exp, p)
}

// Commit records an accepted report in the rollback state, if there is one.
func (c *ArtifactCheck) Commit(r *ArtifactReport) error { return r.Commit(c.state) }

func effectiveTLog(s *config.SigningConfig, trust TrustSet, subject string) TLogMode {
	if s != nil && s.TLog != "" {
		return TLogMode(s.TLog)
	}
	if trust.HasIdentities(subject) {
		return TLogRequired
	}
	return TLogOff
}

func bundlePathFor(cfg *config.Config, o VerifyOptions) string {
	if o.BundlePath != "" {
		return o.BundlePath
	}
	name := config.SigningAttestationFile
	if cfg.Signing != nil && cfg.Signing.Attestation != "" {
		name = cfg.Signing.Attestation
	}
	return filepath.Join(cfg.ConfigDir, filepath.FromSlash(name))
}

// buildTrust turns the config's trust entries and the options' extra signers into
// a TrustSet, reading each key file.
func buildTrust(cfg *config.Config, o VerifyOptions) (TrustSet, error) {
	var set TrustSet
	for _, t := range cfg.Signing.SigningTrustEntries() {
		e := TrustEntry{Subject: t.Subject, Source: t.Source, Identity: t.Identity, IdentityRegexp: t.IdentityRegexp, Issuer: t.Issuer, Reviewer: t.Reviewer}
		var err error
		if t.ValidFrom != "" {
			if e.ValidFrom, err = config.ParseSigningTime(t.ValidFrom, false); err != nil {
				return set, oops.Wrap(err)
			}
		}
		if t.ValidUntil != "" {
			if e.ValidUntil, err = config.ParseSigningTime(t.ValidUntil, true); err != nil {
				return set, oops.Wrap(err)
			}
		}
		if t.KeyFile != "" {
			data, err := readInProject(cfg.BaseDir, t.KeyFile)
			if err != nil {
				return set, oops.With("key_file", t.KeyFile).Wrapf(err, "read the trusted key")
			}
			if e.Key, err = ParsePublicKey(data); err != nil {
				return set, oops.With("key_file", t.KeyFile).Wrap(err)
			}
		}
		set.Entries = append(set.Entries, e)
	}
	for _, path := range o.PublicKeys {
		data, err := readLimited(path)
		if err != nil {
			return set, oops.With("path", path).Wrapf(err, "read the public key")
		}
		key, err := ParsePublicKey(data)
		if err != nil {
			return set, oops.With("path", path).Wrap(err)
		}
		set.Entries = append(set.Entries, TrustEntry{Subject: o.subject(), Key: key})
	}
	if o.Identity != "" || o.Issuer != "" {
		if o.Identity == "" || o.Issuer == "" {
			return set, oops.Errorf("--identity and --issuer go together")
		}
		set.Entries = append(set.Entries, TrustEntry{Subject: o.subject(), Identity: o.Identity, Issuer: o.Issuer})
	}
	return set, nil
}

// loadTrustedRoot resolves the trusted root: the option, else [signing]
// trusted_root (inside the project), else the user cache's. A named root that is
// missing is an error; the cached default is optional (nil when absent), since
// key bundles do not need it.
func loadTrustedRoot(cfg *config.Config, o VerifyOptions) (root.TrustedMaterial, error) {
	var data []byte
	var err error
	switch {
	case o.TrustedRoot != "":
		data, err = readLimited(o.TrustedRoot)
		if err != nil {
			return nil, wrap(CodeRootUnavailable, err, "cannot read the trusted root %s", o.TrustedRoot)
		}
	case cfg.Signing != nil && cfg.Signing.TrustedRoot != "":
		data, err = readInProject(cfg.BaseDir, cfg.Signing.TrustedRoot)
		if err != nil {
			return nil, wrap(CodeRootUnavailable, err, "cannot read [signing] trusted_root %s", cfg.Signing.TrustedRoot)
		}
	default:
		dir, derr := config.CacheDirIn(o.Env, "sigstore")
		if derr != nil {
			return nil, nil //nolint:nilnil // no home directory: no cached root
		}
		data, err = readLimited(filepath.Join(dir, TrustedRootFile))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil //nolint:nilnil // key bundles need no root
		}
		if err != nil {
			return nil, wrap(CodeRootUnavailable, err, "cannot read the cached trusted root")
		}
	}
	tr, err := root.NewTrustedRootFromJSON(data)
	if err != nil {
		return nil, wrap(CodeRootUnavailable, err, "the trusted root is not valid")
	}
	return tr, nil
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path is named by the user
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	defer f.Close() //nolint:errcheck // read-only
	return readAllLimited(f)
}

func readAllLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxKeyBytes+1))
	if err != nil {
		return nil, err //nolint:wrapcheck // contextualized by the caller
	}
	if len(data) > maxKeyBytes {
		return nil, errors.New("file is too large")
	}
	return data, nil
}

// readInProject reads rel under base without leaving it: a symlink or ".." that
// resolves outside the project is refused (a committed config cannot reach files
// elsewhere on the machine).
func readInProject(base, rel string) ([]byte, error) {
	r, err := os.OpenRoot(base)
	if err != nil {
		return nil, err //nolint:wrapcheck // contextualized by the caller
	}
	defer r.Close() //nolint:errcheck // read-only
	f, err := r.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, err //nolint:wrapcheck // contextualized by the caller
	}
	defer f.Close() //nolint:errcheck // read-only
	return readAllLimited(f)
}

// RequiredLockFindings verifies the lock attestation when [signing] require names
// the lock, for `validate --strict`. It returns nil when nothing is required or
// the attestation is good, else one *Error per problem (a policy that cannot be
// built is reported as AR720 with the reason). It reads the rollback state but
// never writes it.
func RequiredLockFindings(cfg *config.Config, env ambient.Env, now time.Time) []*Error {
	findings, err := RequiredLockCheck(cfg, env, now)
	if err != nil {
		return []*Error{{Code: CodeMissing, Reason: "cannot verify the lock attestation: " + err.Error()}}
	}
	return findings
}

// RequiredLockCheck is RequiredLockFindings for `lock --check` and
// `generate --locked`: a policy that cannot be built (an unreadable key_file, no
// trusted signer) is the returned error, "could not run" as in
// `verify --attestation` (exit 1), and findings are verification failures.
func RequiredLockCheck(cfg *config.Config, env ambient.Env, now time.Time) ([]*Error, error) {
	if !cfg.Signing.Requires(config.SigningSubjectLock) {
		return nil, nil
	}
	return CheckLockAttestation(cfg, env, now)
}

// CheckLockAttestation verifies the lock attestation under the [signing] policy
// whether or not require names the lock: `mcp --serve-skills` needs it when
// require names "served". The results are RequiredLockCheck's. It reads the
// rollback state and never writes it.
func CheckLockAttestation(cfg *config.Config, env ambient.Env, now time.Time) ([]*Error, error) {
	check, err := PrepareLockCheck(cfg, VerifyOptions{Env: env, Now: now})
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return []*Error{e}, nil
		}
		return nil, err
	}
	if _, err := check.Verify(); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return []*Error{e}, nil
		}
		return []*Error{{Code: CodeInvalid, Reason: err.Error()}}, nil
	}
	return nil, nil
}
