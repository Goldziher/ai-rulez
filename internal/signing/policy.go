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
	// PublicKeys are PEM public key paths to trust for the lock, in addition to the config.
	PublicKeys []string
	// Identity and Issuer trust one more keyless signer.
	Identity, Issuer string
	// BundlePath is the attestation file; default [signing] attestation, then the
	// lock's sidecar.
	BundlePath string
	// NoState skips the rollback state: nothing is read or written.
	NoState bool
	Env     ambient.Env
	Now     time.Time
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

// ReadBundle reads the attestation file. A missing file is AR720.
func (c *LockCheck) ReadBundle() ([]byte, error) {
	f, err := os.Open(c.BundlePath) //nolint:gosec // the path is the configured or flagged sidecar
	if errors.Is(err, os.ErrNotExist) {
		return nil, Errorf(CodeMissing, "no attestation at %s; sign the lock with `ai-rulez sign --lock`", c.BundlePath)
	}
	if err != nil {
		return nil, wrap(CodeMissing, err, "cannot read the attestation at %s", c.BundlePath)
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, MaxBundleBytes+1))
	if err != nil {
		return nil, wrap(CodeInvalid, err, "cannot read the attestation at %s", c.BundlePath)
	}
	return data, nil
}

// Verify reads and verifies the attestation. It never writes the rollback state;
// call Commit on the report after accepting it.
func (c *LockCheck) Verify() (*LockReport, error) {
	data, err := c.ReadBundle()
	if err != nil {
		return nil, err
	}
	return VerifyLock(data, c.Lock, c.Policy)
}

// Commit records an accepted report in the rollback state, if there is one.
func (c *LockCheck) Commit(r *LockReport) error { return r.Commit(c.state) }

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
	s := cfg.Signing
	trust, err := buildTrust(cfg, o)
	if err != nil {
		return nil, err
	}
	if len(trust.Entries) == 0 {
		return nil, oops.Hint("set [signing] identity and issuer, key_file or [[signing.trust]], or pass --public-key or --identity with --issuer").
			Errorf("no trusted signer is configured: a valid signature alone does not say who may sign the lock")
	}
	check := &LockCheck{Lock: lock, BundlePath: bundlePathFor(cfg, o)}
	check.Policy = LockPolicy{Trust: trust, Now: o.Now}
	check.Policy.Verifier = Verifier{Keys: trust.Keys(SubjectLock), TLog: effectiveTLog(s, trust)}
	if s != nil {
		if s.MaxAge != "" {
			if check.Policy.MaxAge, err = config.ParseApprovalMaxAge(s.MaxAge); err != nil {
				return nil, oops.Wrap(err)
			}
		}
		check.Policy.MinHashVersion = s.MinHashVersion
	}
	if check.Policy.Verifier.TrustedRoot, err = loadTrustedRoot(cfg, o); err != nil {
		return nil, err
	}
	if !o.NoState {
		statePath, secretPath := StatePaths(o.Env)
		st, serr := OpenState(statePath, secretPath)
		switch {
		case serr != nil:
			check.Warnings = append(check.Warnings, "rollback detection is off: "+serr.Error())
		default:
			if st.Reset {
				check.Warnings = append(check.Warnings, "the rollback state failed its integrity check and was reset")
			}
			check.state = st
			check.Policy.State = st
		}
	}
	return check, nil
}

func effectiveTLog(s *config.SigningConfig, trust TrustSet) TLogMode {
	if s != nil && s.TLog != "" {
		return TLogMode(s.TLog)
	}
	if trust.HasIdentities(SubjectLock) {
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
		e := TrustEntry{Subject: t.Subject, Identity: t.Identity, IdentityRegexp: t.IdentityRegexp, Issuer: t.Issuer}
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
		set.Entries = append(set.Entries, TrustEntry{Subject: SubjectLock, Key: key})
	}
	if o.Identity != "" || o.Issuer != "" {
		if o.Identity == "" || o.Issuer == "" {
			return set, oops.Errorf("--identity and --issuer go together")
		}
		set.Entries = append(set.Entries, TrustEntry{Subject: SubjectLock, Identity: o.Identity, Issuer: o.Issuer})
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
// the lock, for `lock --check`, `generate --locked` and `validate --strict`. It
// returns nil when nothing is required or the attestation is good, else one
// *Error per problem (a policy that cannot be built is reported as AR720 with the
// reason). It reads the rollback state but never writes it.
func RequiredLockFindings(cfg *config.Config, env ambient.Env, now time.Time) []*Error {
	if !cfg.Signing.Requires(config.SigningSubjectLock) {
		return nil
	}
	check, err := PrepareLockCheck(cfg, VerifyOptions{Env: env, Now: now})
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return []*Error{e}
		}
		return []*Error{{Code: CodeMissing, Reason: "cannot verify the lock attestation: " + err.Error()}}
	}
	if _, err := check.Verify(); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return []*Error{e}
		}
		return []*Error{{Code: CodeInvalid, Reason: err.Error()}}
	}
	return nil
}
