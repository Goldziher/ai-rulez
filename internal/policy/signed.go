package policy

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

const (
	// SubjectPolicy is the signing subject a policy signer is trusted for.
	SubjectPolicy = "policy"
	// PredicatePolicy is the predicate type of a policy attestation.
	PredicatePolicy = "https://github.com/Goldziher/ai-rulez/attestations/policy/v1"
	// SidecarSuffix is appended to a policy's path or URL to name its signature.
	SidecarSuffix = ".sigstore.json"
	// policyStatementName is the subject name of a policy statement.
	policyStatementName = "policy.toml"
	// clockSkew is how far into the future a signing time may be.
	clockSkew = 5 * time.Minute
)

// Environment variables that configure signature checking, next to the flags and
// the user config. They belong to the machine or the CI image, not the repository.
const (
	EnvRequireSigned  = "AI_RULEZ_POLICY_REQUIRE_SIGNED"
	EnvSignerIdentity = "AI_RULEZ_POLICY_SIGNER_IDENTITY"
	EnvSignerIssuer   = "AI_RULEZ_POLICY_SIGNER_ISSUER"
	EnvSignerKey      = "AI_RULEZ_POLICY_SIGNER_KEY"
	EnvTrustedRoot    = "AI_RULEZ_POLICY_TRUSTED_ROOT"
)

// PolicyPredicate is the predicate of a policy attestation. The subject is the
// SHA-256 of the policy with CRLF normalized (the digest --show-policy prints).
type PolicyPredicate struct {
	PolicyVersion int       `json:"policy_version"`
	Name          string    `json:"name,omitempty"`
	IssuedAt      time.Time `json:"issued_at"`
}

// PolicyStatement builds the statement that vouches for a policy file.
func PolicyStatement(data []byte, now time.Time) (*signing.Statement, error) {
	d, err := parseDoc("policy", data)
	if err != nil {
		return nil, err
	}
	return signing.NewStatement(PredicatePolicy,
		[]signing.Subject{{Name: policyStatementName, Digest: map[string]string{"sha256": strings.TrimPrefix(digest(data), "sha256:")}}},
		PolicyPredicate{PolicyVersion: Version, Name: d.name, IssuedAt: now.UTC()})
}

// SignPolicy signs a policy file with signer and returns the Sigstore bundle to
// publish next to it as <policy>.sigstore.json. A bundle made with `cosign
// sign-blob --bundle` over the file's exact bytes is accepted as well.
func SignPolicy(ctx context.Context, signer signing.Signer, data []byte, now time.Time) ([]byte, error) {
	st, err := PolicyStatement(data, now)
	if err != nil {
		return nil, err
	}
	return signing.SignStatement(ctx, signer, st) //nolint:wrapcheck // the signing package contextualizes
}

// SignPolicyFile reads the policy at path, checks that the loader accepts it, and
// signs it (see SignPolicy). A file that is not a valid policy is *ParseError
// (AR743): nothing is signed that would be rejected on load.
func SignPolicyFile(ctx context.Context, signer signing.Signer, path string, now time.Time) ([]byte, error) {
	data, err := readPolicyFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the policy %s: %w", path, err)
	}
	return SignPolicy(ctx, signer, data, now)
}

// SignatureError is a policy whose signature is missing where one is required, or
// does not verify (AR746).
type SignatureError struct {
	Path string
	Err  error
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("%s: policy %s: %v", lint.CodePolicySignature, e.Path, e.Err)
}

func (e *SignatureError) Unwrap() error { return e.Err }

// SignatureOptions configures the check of a policy's signature. Trust comes
// from outside the repository: these options, the AI_RULEZ_POLICY_* environment
// and the [policy] table of the user config; they are merged (every source adds
// trusted signers, any of them can require signatures).
type SignatureOptions struct {
	// Require makes an unsigned policy an error (AR746).
	Require bool
	// Identity and Issuer trust one keyless signer (--policy-signer-identity).
	Identity, Issuer string
	// KeyFiles are PEM public keys to trust (--policy-signer-key).
	KeyFiles []string
	// TrustedRoot is a Sigstore trusted root file (--policy-trusted-root); the
	// root `ai-rulez trust update` cached is the default.
	TrustedRoot string
}

// UserSigner is a [[policy.signers]] entry of the user config.
type UserSigner struct {
	Identity       string
	IdentityRegexp string
	Issuer         string
	KeyFile        string
}

// verifier is a configured signature check.
type verifier struct {
	v       signing.Verifier
	trust   signing.TrustSet
	require bool
	state   *signing.State
	now     func() time.Time
}

// sidecarRef names the signature next to a policy reference.
func sidecarRef(ref Ref) Ref {
	if !ref.Remote {
		return Ref{Location: ref.Location + SidecarSuffix}
	}
	u, err := url.Parse(ref.Location)
	if err != nil {
		return Ref{}
	}
	u.Path += SidecarSuffix
	u.RawPath = ""
	return Ref{Location: u.String(), Remote: true}
}

// verifier builds the signature check once per load. It is nil when no trust is
// configured and nothing is required: signatures are then not looked at.
func (l *loader) verifier() (*verifier, error) {
	if l.verifierDone {
		return l.pv, l.pvErr
	}
	l.verifierDone = true
	l.pv, l.pvErr = l.buildVerifier()
	return l.pv, l.pvErr
}

func (l *loader) buildVerifier() (*verifier, error) {
	env := l.opts.Env
	so := l.opts.Signature
	us, err := LoadUserSettings(env)
	if err != nil {
		return nil, err
	}
	require := so.Require || us.RequireSignature || truthy(ambient.Getenv(env, EnvRequireSigned))
	var entries []signing.TrustEntry
	addIdentity := func(identity, regexp, issuer string) {
		entries = append(entries, signing.TrustEntry{Subject: SubjectPolicy, Identity: identity, IdentityRegexp: regexp, Issuer: issuer})
	}
	var keys []crypto.PublicKey
	addKey := func(path string) error {
		data, rerr := readRegularLimited(path, maxKeyFileBytes)
		if rerr != nil {
			return &SignatureError{Path: path, Err: fmt.Errorf("cannot read the trusted signer key: %w", rerr)}
		}
		key, perr := signing.ParsePublicKey(data)
		if perr != nil {
			return &SignatureError{Path: path, Err: fmt.Errorf("the trusted signer key is invalid: %w", perr)}
		}
		keys = append(keys, key)
		entries = append(entries, signing.TrustEntry{Subject: SubjectPolicy, Key: key})
		return nil
	}
	for _, s := range us.Signers {
		if s.KeyFile != "" {
			if err := addKey(s.KeyFile); err != nil {
				return nil, err
			}
			continue
		}
		addIdentity(s.Identity, s.IdentityRegexp, s.Issuer)
	}
	identity, issuer := firstNonEmpty(so.Identity, ambient.Getenv(env, EnvSignerIdentity)), firstNonEmpty(so.Issuer, ambient.Getenv(env, EnvSignerIssuer))
	if (identity == "") != (issuer == "") {
		return nil, &ParseError{Path: "--policy-signer-identity", Msg: "the signer identity and issuer go together"}
	}
	if identity != "" {
		addIdentity(identity, "", issuer)
	}
	keyFiles := append([]string(nil), so.KeyFiles...)
	if k := strings.TrimSpace(ambient.Getenv(env, EnvSignerKey)); k != "" {
		keyFiles = append(keyFiles, filepath.SplitList(k)...)
	}
	for _, k := range keyFiles {
		if err := addKey(k); err != nil {
			return nil, err
		}
	}
	if len(entries) == 0 {
		if require {
			return nil, &SignatureError{Path: "(policy signature)", Err: errors.New("signatures are required, but no trusted signer is configured; set --policy-signer-key, --policy-signer-identity and --policy-signer-issuer, or [[policy.signers]] in the user config")}
		}
		return nil, nil //nolint:nilnil // no trust configured: signatures are not checked
	}
	pv := &verifier{trust: signing.TrustSet{Entries: entries}, require: require, now: l.opts.Clock.Now}
	tlog := signing.TLogOff
	if pv.trust.HasIdentities(SubjectPolicy) {
		tlog = signing.TLogRequired
	}
	switch strings.ToLower(strings.TrimSpace(us.TLog)) {
	case "":
	case "required", "optional", "off":
		tlog = signing.TLogMode(strings.ToLower(strings.TrimSpace(us.TLog)))
	default:
		return nil, &ParseError{Path: "[policy] tlog", Msg: fmt.Sprintf("%q is not required, optional or off", us.TLog)}
	}
	tr, err := loadTrustedRoot(env, firstNonEmpty(so.TrustedRoot, ambient.Getenv(env, EnvTrustedRoot), us.TrustedRoot))
	if err != nil {
		return nil, err
	}
	pv.v = signing.Verifier{TrustedRoot: tr, Keys: keys, TLog: tlog}
	statePath, secretPath := signing.StatePaths(env)
	if st, serr := signing.OpenState(statePath, secretPath); serr != nil {
		logger.Debug("policy rollback detection is off", "error", serr)
	} else {
		pv.state = st
	}
	return pv, nil
}

// maxKeyFileBytes bounds a trusted key or root file.
const maxKeyFileBytes = 4 << 20

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// loadTrustedRoot reads the named trusted root, else the one `trust update`
// cached; nil when there is none (key signatures need no root).
func loadTrustedRoot(env ambient.Env, path string) (root.TrustedMaterial, error) {
	explicit := path != ""
	if !explicit {
		dir, err := config.CacheDirIn(env, "sigstore")
		if err != nil {
			return nil, nil //nolint:nilnil // no home directory: no cached root
		}
		path = filepath.Join(dir, signing.TrustedRootFile)
	}
	data, err := readRegularLimited(path, maxKeyFileBytes)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return nil, nil //nolint:nilnil // key bundles need no root
	}
	if err != nil {
		return nil, &SignatureError{Path: path, Err: fmt.Errorf("cannot read the trusted root: %w", err)}
	}
	tr, err := root.NewTrustedRootFromJSON(data)
	if err != nil {
		return nil, &SignatureError{Path: path, Err: fmt.Errorf("the trusted root is not valid: %w", err)}
	}
	return tr, nil
}

// verify checks that bundle is a valid signature over raw by a trusted signer
// and is not older than one already seen (rollback). The bundle is a DSSE
// attestation over a policy statement (SignPolicy) or a `cosign sign-blob` bundle
// over the file's bytes. It returns who signed.
func (v *verifier) verify(ref Ref, raw, bundle []byte) (signer string, err error) {
	now := v.now()
	var res *signing.Result
	var claimed time.Time
	if signing.IsBlobBundle(bundle) {
		res, err = v.v.VerifyBlob(bundle, raw)
	} else {
		res, err = v.v.Verify(bundle)
		if err == nil {
			claimed, err = checkPolicyStatement(res, raw)
		}
	}
	if err != nil {
		return "", err //nolint:wrapcheck // the signing error carries its AR72x code
	}
	if err := v.trust.Check(res, SubjectPolicy, now); err != nil {
		return "", err //nolint:wrapcheck // AR722
	}
	at := res.SignedAt
	if at.IsZero() {
		at = claimed
	}
	if !at.IsZero() && at.After(now.Add(clockSkew)) {
		return "", fmt.Errorf("signed in the future (%s)", at.UTC().Format(time.RFC3339))
	}
	key := "policy|" + signerKey(res.Signer) + "|" + redactURL(ref.Location)
	if v.state != nil && at.IsZero() {
		// No signing time at all (a key-signed blob bundle without a log entry):
		// neither skew nor time-ordered rollback can be judged, so the digests
		// already replaced on this machine are the rollback record.
		d := digest(raw)
		if err := v.state.CheckDigest(key, d); err != nil {
			return "", err //nolint:wrapcheck // AR727
		}
		if err := v.state.AdvanceDigest(key, d); err != nil {
			logger.Debug("cannot record the policy digest", "error", err)
		}
	}
	if v.state != nil && !at.IsZero() {
		if err := v.state.Check(key, at); err != nil {
			return "", err //nolint:wrapcheck // AR727
		}
		if err := v.state.Advance(key, at); err != nil {
			logger.Debug("cannot record the policy signing time", "error", err)
		}
	}
	return signerKey(res.Signer), nil
}

func signerKey(s signing.SignerInfo) string {
	if s.Kind == signing.KindKeyless {
		return s.Identity
	}
	return "key:" + s.KeyID
}

// checkPolicyStatement checks the verified statement covers raw and returns the
// signer's claimed issue time.
func checkPolicyStatement(res *signing.Result, raw []byte) (time.Time, error) {
	if res.Statement == nil {
		return time.Time{}, signing.Errorf(signing.CodeSubjectMismatch, "the signed payload is not an in-toto statement")
	}
	if res.Statement.PredicateType != PredicatePolicy {
		return time.Time{}, signing.Errorf(signing.CodeSubjectMismatch, "the signed statement is a %q attestation, not a policy attestation", res.Statement.PredicateType)
	}
	if err := res.Statement.RequireSubject("sha256", strings.TrimPrefix(digest(raw), "sha256:")); err != nil {
		return time.Time{}, signing.Errorf(signing.CodeSubjectMismatch, "the policy changed since it was signed (it is %s)", digest(raw))
	}
	var pred PolicyPredicate
	if err := res.Statement.DecodePredicate(&pred); err != nil {
		return time.Time{}, err //nolint:wrapcheck // AR721
	}
	return pred.IssuedAt, nil
}

// checkSignature verifies bundle over data when a trust set is configured. signed
// says a trusted signer vouches for data; an unsigned policy is an error only
// when signatures are required.
func (l *loader) checkSignature(ref Ref, data, bundle []byte) (signer string, err error) {
	pv, err := l.verifier()
	if err != nil || pv == nil {
		return "", err
	}
	if len(bundle) == 0 {
		if pv.require {
			return "", &SignatureError{Path: ref.Display(), Err: fmt.Errorf("it is not signed (no %s) and signatures are required; publish the signature next to the policy", filepath.Base(sidecarRef(ref).Location))}
		}
		return "", nil
	}
	signer, err = pv.verify(ref, data, bundle)
	if err != nil {
		return "", &SignatureError{Path: ref.Display(), Err: err}
	}
	return signer, nil
}

// readRegularLimited reads a regular file of at most limit bytes.
func readRegularLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // an operator-chosen path, never repository content
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return nil, err //nolint:wrapcheck // contextualized by the caller
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err //nolint:wrapcheck // contextualized by the caller
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("larger than %d bytes", limit)
	}
	return data, nil
}

// fetchSidecar fetches the signature next to a URL policy. (nil, nil) means there
// is none (HTTP 404); another failure is a *networkError.
func fetchSidecar(ctx context.Context, client *http.Client, ref Ref) ([]byte, error) {
	sc := sidecarRef(ref)
	if sc.Location == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sc.Location, nil)
	if err != nil {
		return nil, nil //nolint:nilnil // not a fetchable URL: no signature
	}
	req.Header.Set("User-Agent", "ai-rulez-policy")
	resp, err := client.Do(req)
	if err != nil {
		return nil, &networkError{err: errors.New(scrubURLError(err))}
	}
	defer resp.Body.Close() //nolint:errcheck // read-only
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil
	case resp.StatusCode != http.StatusOK:
		return nil, &networkError{err: &statusError{code: resp.StatusCode}}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, signing.MaxBundleBytes+1))
	if err != nil {
		return nil, &networkError{err: errors.New(scrubURLError(err))}
	}
	if len(data) > signing.MaxBundleBytes {
		return nil, &SignatureError{Path: redactURL(sc.Location), Err: fmt.Errorf("the signature is larger than %d bytes", signing.MaxBundleBytes)}
	}
	return data, nil
}

// readSidecarFile reads the signature next to a policy file; nil when absent.
func readSidecarFile(path string) ([]byte, error) {
	data, err := readRegularLimited(sidecarRef(Ref{Location: path}).Location, signing.MaxBundleBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &SignatureError{Path: path, Err: fmt.Errorf("cannot read the signature: %w", err)}
	}
	return data, nil
}
