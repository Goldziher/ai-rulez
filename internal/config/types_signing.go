package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/samber/oops"
)

// Signing subjects and TLog modes of the [signing] table (docs/signing.md).
const (
	// SigningSubjectLock is the lock attestation, the only subject so far.
	SigningSubjectLock = "lock"
	// SigningTLogRequired, SigningTLogOptional and SigningTLogOff are the tlog modes.
	SigningTLogRequired = "required"
	SigningTLogOptional = "optional"
	SigningTLogOff      = "off"
	// SigningAttestationFile is the default sidecar of the lock, next to it.
	SigningAttestationFile = "ai-rulez.lock.sigstore.json"
)

// SigningConfig is the [signing] table: who may sign the lock and how fresh the
// signature must be. Nothing is enforced unless require names a subject; signing
// itself is always opt-in.
type SigningConfig struct {
	// Require lists the subjects that must carry a valid attestation: "lock".
	// `lock --check` and `generate --locked` fail without one.
	Require []string `yaml:"require,omitempty" json:"require,omitempty" toml:"require,omitempty"`
	// TLog is "required", "optional" or "off". Default: required when a trusted
	// signer is a certificate identity, off when only keys are trusted.
	TLog string `yaml:"tlog,omitempty" json:"tlog,omitempty" toml:"tlog,omitempty"`
	// MaxAge is the oldest acceptable signature ("180d", "720h"); empty is no limit.
	MaxAge string `yaml:"max_age,omitempty" json:"max_age,omitempty" toml:"max_age,omitempty"` //nolint:tagliatelle
	// TrustedRoot is a Sigstore trusted root file inside the project (keyless
	// verification). Default: the root `ai-rulez trust update` cached for the user.
	TrustedRoot string `yaml:"trusted_root,omitempty" json:"trusted_root,omitempty" toml:"trusted_root,omitempty"` //nolint:tagliatelle
	// MinHashVersion rejects an attestation of an older hash scheme.
	MinHashVersion int `yaml:"min_hash_version,omitempty" json:"min_hash_version,omitempty" toml:"min_hash_version,omitempty"` //nolint:tagliatelle
	// Attestation is the lock bundle path relative to the config directory.
	// Default ai-rulez.lock.sigstore.json.
	Attestation string `yaml:"attestation,omitempty" json:"attestation,omitempty" toml:"attestation,omitempty"`

	// Identity and Issuer are the shorthand for one trusted keyless signer.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" toml:"identity,omitempty"`
	Issuer   string `yaml:"issuer,omitempty" json:"issuer,omitempty" toml:"issuer,omitempty"`
	// KeyFile is the shorthand for one trusted public key (PEM, inside the project).
	KeyFile string `yaml:"key_file,omitempty" json:"key_file,omitempty" toml:"key_file,omitempty"` //nolint:tagliatelle

	// Trust is the full form: scoped, repeatable trust entries.
	Trust []SigningTrust `yaml:"trust,omitempty" json:"trust,omitempty" toml:"trust,omitempty"`
}

// SigningTrust is one [[signing.trust]] entry: a certificate identity (Identity
// or IdentityRegexp, with Issuer) or a public key (KeyFile).
type SigningTrust struct {
	// Subject is what the signer may vouch for; default "lock".
	Subject string `yaml:"subject,omitempty" json:"subject,omitempty" toml:"subject,omitempty"`
	// Identity is matched exactly against the certificate subject alternative name.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" toml:"identity,omitempty"`
	// IdentityRegexp is matched against the whole identity and must be anchored (^...$).
	IdentityRegexp string `yaml:"identity_regexp,omitempty" json:"identity_regexp,omitempty" toml:"identity_regexp,omitempty"` //nolint:tagliatelle
	// Issuer is matched exactly against the OIDC issuer extension.
	Issuer string `yaml:"issuer,omitempty" json:"issuer,omitempty" toml:"issuer,omitempty"`
	// KeyFile is a PEM public key inside the project.
	KeyFile string `yaml:"key_file,omitempty" json:"key_file,omitempty" toml:"key_file,omitempty"` //nolint:tagliatelle
	// ValidFrom and ValidUntil ("2026-01-01" or RFC 3339) bound the signing time
	// the entry accepts; an expiry forces a reviewed renewal.
	ValidFrom  string `yaml:"valid_from,omitempty" json:"valid_from,omitempty" toml:"valid_from,omitempty"`    //nolint:tagliatelle
	ValidUntil string `yaml:"valid_until,omitempty" json:"valid_until,omitempty" toml:"valid_until,omitempty"` //nolint:tagliatelle
}

// ValidateIdentityRegexp checks that expr compiles and is anchored at both ends:
// an unanchored pattern accepts any identity that merely contains it (AR722).
func ValidateIdentityRegexp(expr string) error {
	if !strings.HasPrefix(expr, "^") || !endsWithUnescapedDollar(expr) {
		return fmt.Errorf("identity_regexp %q must be anchored: start it with ^ and end it with $", expr)
	}
	if _, err := regexp.Compile(expr); err != nil {
		return fmt.Errorf("identity_regexp %q: %w", expr, err)
	}
	return nil
}

func endsWithUnescapedDollar(expr string) bool {
	if !strings.HasSuffix(expr, "$") {
		return false
	}
	slashes := 0
	for i := len(expr) - 2; i >= 0 && expr[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 0
}

// ParseSigningTime parses a trust entry bound: a date ("2026-01-01", UTC) or an
// RFC 3339 time. endOfDay makes a date bound inclusive of that whole day.
func ParseSigningTime(s string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond), nil
		}
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q (use 2026-01-01 or an RFC 3339 time)", s)
	}
	return t, nil
}

// SigningTLog returns the effective tlog mode: the configured one, else
// "required" when any trusted signer is a certificate identity, else "off".
func (s *SigningConfig) SigningTLog() string {
	if s == nil {
		return SigningTLogRequired
	}
	if s.TLog != "" {
		return s.TLog
	}
	if s.Identity != "" {
		return SigningTLogRequired
	}
	for _, t := range s.Trust {
		if t.KeyFile == "" {
			return SigningTLogRequired
		}
	}
	if s.KeyFile != "" || len(s.Trust) > 0 {
		return SigningTLogOff
	}
	return SigningTLogRequired
}

// Requires reports whether subject must carry a valid attestation.
func (s *SigningConfig) Requires(subject string) bool {
	return s != nil && slices.Contains(s.Require, subject)
}

// SigningTrustEntries returns the shorthand as a trust entry followed by the
// [[signing.trust]] entries, with the default subject filled in.
func (s *SigningConfig) SigningTrustEntries() []SigningTrust {
	if s == nil {
		return nil
	}
	var out []SigningTrust
	if s.Identity != "" || s.KeyFile != "" {
		out = append(out, SigningTrust{Subject: SigningSubjectLock, Identity: s.Identity, Issuer: s.Issuer, KeyFile: s.KeyFile})
	}
	for _, t := range s.Trust {
		if t.Subject == "" {
			t.Subject = SigningSubjectLock
		}
		out = append(out, t)
	}
	return out
}

func (c *Config) validateSigning() error {
	s := c.Signing
	if s == nil {
		return nil
	}
	fail := func(field, format string, args ...any) error {
		return oops.With("field", "signing."+field).Errorf(format, args...)
	}
	for _, r := range s.Require {
		if r != SigningSubjectLock {
			return fail("require", "invalid subject %q (only %q is supported)", r, SigningSubjectLock)
		}
	}
	if !slices.Contains([]string{"", SigningTLogRequired, SigningTLogOptional, SigningTLogOff}, s.TLog) {
		return fail("tlog", "invalid tlog %q (use required, optional or off)", s.TLog)
	}
	if s.MaxAge != "" {
		if _, err := ParseApprovalMaxAge(s.MaxAge); err != nil {
			return fail("max_age", "%v", err)
		}
	}
	if s.MinHashVersion < 0 {
		return fail("min_hash_version", "min_hash_version must not be negative")
	}
	for field, p := range map[string]string{"trusted_root": s.TrustedRoot, "attestation": s.Attestation, "key_file": s.KeyFile} {
		if err := validateSigningPath(p); err != nil {
			return fail(field, "%v", err)
		}
	}
	if (s.Identity == "") != (s.Issuer == "") {
		return fail("identity", "identity and issuer go together")
	}
	if s.Identity != "" && s.KeyFile != "" {
		return fail("identity", "use identity and issuer, or key_file, not both; add [[signing.trust]] entries for several signers")
	}
	for i, t := range s.Trust {
		if err := validateSigningTrust(t, s.SigningTLog()); err != nil {
			return oops.With("field", fmt.Sprintf("signing.trust[%d]", i)).Wrap(err)
		}
	}
	if len(s.Require) > 0 && len(s.SigningTrustEntries()) == 0 {
		return fail("require", "require needs a trusted signer: set identity and issuer, key_file or [[signing.trust]]")
	}
	if s.TLog == SigningTLogOff {
		for _, t := range s.SigningTrustEntries() {
			if t.KeyFile == "" {
				return fail("tlog", "tlog = %q only works with keys: a certificate identity needs a transparency log", SigningTLogOff)
			}
		}
	}
	return nil
}

func validateSigningTrust(t SigningTrust, _ string) error {
	if t.Subject != "" && t.Subject != SigningSubjectLock {
		return fmt.Errorf("invalid subject %q (only %q is supported)", t.Subject, SigningSubjectLock)
	}
	byIdentity := t.Identity != "" || t.IdentityRegexp != ""
	switch {
	case byIdentity && t.KeyFile != "":
		return fmt.Errorf("an entry trusts an identity or a key_file, not both")
	case !byIdentity && t.KeyFile == "":
		return fmt.Errorf("an entry needs identity, identity_regexp or key_file")
	case t.Identity != "" && t.IdentityRegexp != "":
		return fmt.Errorf("use identity or identity_regexp, not both")
	case byIdentity && t.Issuer == "":
		return fmt.Errorf("an identity entry needs an issuer")
	case t.KeyFile != "" && t.Issuer != "":
		return fmt.Errorf("a key_file entry has no issuer")
	}
	if t.IdentityRegexp != "" {
		if err := ValidateIdentityRegexp(t.IdentityRegexp); err != nil {
			return fmt.Errorf("AR722: %w", err)
		}
	}
	if err := validateSigningPath(t.KeyFile); err != nil {
		return fmt.Errorf("key_file: %w", err)
	}
	var from, until time.Time
	var err error
	if t.ValidFrom != "" {
		if from, err = ParseSigningTime(t.ValidFrom, false); err != nil {
			return fmt.Errorf("valid_from: %w", err)
		}
	}
	if t.ValidUntil != "" {
		if until, err = ParseSigningTime(t.ValidUntil, true); err != nil {
			return fmt.Errorf("valid_until: %w", err)
		}
	}
	if !from.IsZero() && !until.IsZero() && until.Before(from) {
		return fmt.Errorf("valid_until is before valid_from")
	}
	return nil
}

// validateSigningPath requires a project-relative path that stays inside the
// project: a committed config cannot point at files elsewhere on the machine.
func validateSigningPath(p string) error {
	if p == "" {
		return nil
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path %q must be relative and stay inside the project", p)
	}
	return nil
}
