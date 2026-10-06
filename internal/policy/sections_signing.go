package policy

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Signing governs [signing]: whether the lock must carry a verified attestation
// and who may vouch for it.
type Signing struct {
	// RequireVerified lists the subjects that must carry a valid attestation
	// ("lock"); the repository's own list is added to it.
	RequireVerified []string
	// TLog is the weakest transparency-log mode the repository may use: "optional"
	// or "required", "" for no constraint.
	TLog string
	// MaxAge is the oldest signature the repository may accept; 0 for no bound.
	MaxAge time.Duration
	// MinHashVersion is the lowest attestation hash scheme the repository may accept.
	MinHashVersion int
	// Trust, when set, is the only list of signers the repository may trust, as
	// canonical entries (see trustKey). An empty list trusts nobody.
	Trust List
}

type fileSigning struct {
	RequireVerified     []string       `toml:"require_verified"`
	AllowRepoIdentities *bool          `toml:"allow_repo_identities"`
	TLog                string         `toml:"tlog"`
	MaxAge              string         `toml:"max_age"`
	MinHashVersion      *int           `toml:"min_hash_version"`
	Trust               []fileTrustRow `toml:"trust"`
}

type fileTrustRow struct {
	Subject        string `toml:"subject"`
	Identity       string `toml:"identity"`
	IdentityRegexp string `toml:"identity_regexp"`
	Issuer         string `toml:"issuer"`
}

// tlogRank orders the modes a policy may hold; "off" is not a floor.
var tlogRank = map[string]int{"": 0, config.SigningTLogOff: 1, config.SigningTLogOptional: 2, config.SigningTLogRequired: 3}

func (s *Signing) fromDoc(d *fileSigning) error {
	if d == nil {
		return nil
	}
	for _, subject := range d.RequireVerified {
		if subject != config.SigningSubjectLock {
			return fmt.Errorf("signing.require_verified: %q is not a subject this ai-rulez attests (use %q; a newer policy needs a newer ai-rulez)", subject, config.SigningSubjectLock)
		}
	}
	s.RequireVerified = sortedUnique(d.RequireVerified)
	switch mode := strings.ToLower(strings.TrimSpace(d.TLog)); mode {
	case "":
	case config.SigningTLogOptional, config.SigningTLogRequired:
		s.TLog = mode
	default:
		return fmt.Errorf("signing.tlog: %q is not allowed in a policy (use optional or required; off is no floor)", d.TLog)
	}
	if d.MaxAge != "" {
		age, err := config.ParseApprovalMaxAge(d.MaxAge)
		if err != nil {
			return fmt.Errorf("signing.max_age: %w", err)
		}
		s.MaxAge = age
	}
	if d.MinHashVersion != nil {
		if *d.MinHashVersion < 0 {
			return fmt.Errorf("signing.min_hash_version: %d must not be negative", *d.MinHashVersion)
		}
		s.MinHashVersion = *d.MinHashVersion
	}
	if d.AllowRepoIdentities != nil && *d.AllowRepoIdentities && len(d.Trust) > 0 {
		return fmt.Errorf("signing.allow_repo_identities = true contradicts [[signing.trust]]: a listed trust set already bounds the repository's signers")
	}
	if len(d.Trust) > 0 || (d.AllowRepoIdentities != nil && !*d.AllowRepoIdentities) {
		keys := make([]string, 0, len(d.Trust))
		for i, row := range d.Trust {
			key, err := trustRowKey(row)
			if err != nil {
				return fmt.Errorf("signing.trust[%d]: %w", i, err)
			}
			keys = append(keys, key)
		}
		s.Trust = List{Set: true, Items: sortedUnique(keys)}
	}
	return nil
}

// trustRowKey validates a policy trust row and returns its canonical form.
func trustRowKey(r fileTrustRow) (string, error) {
	t := config.SigningTrust{Subject: r.Subject, Identity: r.Identity, IdentityRegexp: r.IdentityRegexp, Issuer: r.Issuer}
	switch {
	case t.Identity == "" && t.IdentityRegexp == "":
		return "", fmt.Errorf("an entry needs identity or identity_regexp (a policy trusts certificate identities, not key files)")
	case t.Identity != "" && t.IdentityRegexp != "":
		return "", fmt.Errorf("use identity or identity_regexp, not both")
	case t.Issuer == "":
		return "", fmt.Errorf("an identity entry needs an issuer")
	}
	if t.Subject != "" && t.Subject != config.SigningSubjectLock {
		return "", fmt.Errorf("invalid subject %q (only %q is supported)", t.Subject, config.SigningSubjectLock)
	}
	if t.IdentityRegexp != "" {
		if err := config.ValidateIdentityRegexp(t.IdentityRegexp); err != nil {
			return "", fmt.Errorf("AR722: %w", err)
		}
	}
	for _, v := range []string{t.Identity, t.IdentityRegexp, t.Issuer} {
		if strings.ContainsAny(v, " \t\r\n\x00") {
			return "", fmt.Errorf("%q must not contain whitespace", v)
		}
	}
	return trustKey(t), nil
}

// trustKey is the canonical text of a trust entry: space-separated key=value
// pairs in a fixed order, with the default subject filled in. Two entries are the
// same signer exactly when their keys are equal. A key file has no place in a
// policy, so a repository entry that names one never matches.
func trustKey(t config.SigningTrust) string {
	if t.Subject == "" {
		t.Subject = config.SigningSubjectLock
	}
	parts := []string{"subject=" + t.Subject}
	for _, kv := range [][2]string{{"identity", t.Identity}, {"identity_regexp", t.IdentityRegexp}, {"issuer", t.Issuer}, {"key_file", t.KeyFile}} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	return strings.Join(parts, " ")
}

// trustFromKey is the inverse of trustKey for a key built from a policy row.
func trustFromKey(key string) config.SigningTrust {
	var t config.SigningTrust
	for _, part := range strings.Fields(key) {
		name, value, _ := strings.Cut(part, "=")
		switch name {
		case "subject":
			t.Subject = value
		case "identity":
			t.Identity = value
		case "identity_regexp":
			t.IdentityRegexp = value
		case "issuer":
			t.Issuer = value
		}
	}
	return t
}

func mergeSigning(a, b Signing) Signing {
	return Signing{
		RequireVerified: union(a.RequireVerified, b.RequireVerified),
		TLog:            stricterTLog(a.TLog, b.TLog),
		MaxAge:          lowerDuration(a.MaxAge, b.MaxAge),
		MinHashVersion:  max(a.MinHashVersion, b.MinHashVersion),
		Trust:           intersectExact(a.Trust, b.Trust),
	}
}

func stricterTLog(a, b string) string {
	if tlogRank[b] > tlogRank[a] {
		return b
	}
	return a
}

// lowerDuration is the meet of two upper bounds; 0 is no bound.
func lowerDuration(a, b time.Duration) time.Duration {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return min(a, b)
}

// formatAge writes a duration as the shortest age the config parsers read back.
func formatAge(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	return d.String()
}

func (s Signing) addTo(table func(path ...string) map[string]any) {
	if len(s.RequireVerified) > 0 {
		table("signing")["require_verified"] = s.RequireVerified
	}
	if s.TLog != "" {
		table("signing")["tlog"] = s.TLog
	}
	if s.MaxAge > 0 {
		table("signing")["max_age"] = formatAge(s.MaxAge)
	}
	if s.MinHashVersion > 0 {
		table("signing")["min_hash_version"] = s.MinHashVersion
	}
	if s.Trust.Set {
		table("signing")["trust"] = nonNil(s.Trust.Items)
	}
}

// signing clamps [signing] to the policy: the policy's subjects are always
// required, the log mode, age bound and hash version only tighten, and the
// repository's trusted signers are cut to the policy list. A repository that
// states a weaker value is reported; one that leaves a key alone is not.
func (a *applier) signing() {
	pol := a.res.Policy.Signing
	if len(pol.RequireVerified) == 0 && pol.TLog == "" && pol.MaxAge == 0 && pol.MinHashVersion == 0 && !pol.Trust.Set {
		return
	}
	if a.cfg.Signing == nil {
		a.cfg.Signing = &config.SigningConfig{}
	}
	s := a.cfg.Signing
	for _, subject := range pol.RequireVerified {
		if !slices.Contains(s.Require, subject) {
			s.Require = append(s.Require, subject)
		}
	}
	a.signingTLog(s, pol)
	a.signingMaxAge(s, pol)
	if pol.MinHashVersion > s.MinHashVersion {
		if s.MinHashVersion > 0 {
			a.violate(lint.CodePolicyLoosened, "signing.min_hash_version", "min_hash_version",
				"[signing] min_hash_version = %d is below the policy minimum %d (origin: %s); %d is enforced", s.MinHashVersion, pol.MinHashVersion, a.origin("signing.min_hash_version"), pol.MinHashVersion)
		}
		s.MinHashVersion = pol.MinHashVersion
	}
	if pol.Trust.Set {
		a.signingTrust(s, pol.Trust)
	}
}

func (a *applier) signingTLog(s *config.SigningConfig, pol Signing) {
	if pol.TLog == "" || tlogRank[s.SigningTLog()] >= tlogRank[pol.TLog] {
		return
	}
	if s.TLog != "" {
		a.violate(lint.CodePolicyLoosened, "signing.tlog", "tlog",
			"[signing] tlog = %q is weaker than the policy level %q (origin: %s); %q is enforced", s.TLog, pol.TLog, a.origin("signing.tlog"), pol.TLog)
	}
	s.TLog = pol.TLog
}

func (a *applier) signingMaxAge(s *config.SigningConfig, pol Signing) {
	if pol.MaxAge == 0 {
		return
	}
	have, err := config.ParseApprovalMaxAge(s.MaxAge)
	switch {
	case s.MaxAge == "" || err != nil:
		s.MaxAge = formatAge(pol.MaxAge)
	case have > pol.MaxAge:
		a.violate(lint.CodePolicyLoosened, "signing.max_age", "max_age",
			"[signing] max_age = %q accepts older signatures than the policy limit %s (origin: %s); %s is enforced", s.MaxAge, formatAge(pol.MaxAge), a.origin("signing.max_age"), formatAge(pol.MaxAge))
		s.MaxAge = formatAge(pol.MaxAge)
	case have < pol.MaxAge:
		a.accept = append(a.accept, fmt.Sprintf("signing.max_age (lowered to %s)", s.MaxAge))
	}
}

// signingTrust keeps the repository's trusted signers that the policy list also
// names; a repository with none left (or none) gets the policy list. An empty
// policy list trusts nobody, so verification has no signer and fails closed.
func (a *applier) signingTrust(s *config.SigningConfig, pol List) {
	var kept []config.SigningTrust
	for _, t := range s.SigningTrustEntries() {
		if slices.Contains(pol.Items, trustKey(t)) {
			kept = append(kept, t)
			continue
		}
		needle := t.Identity
		if needle == "" {
			needle = t.IdentityRegexp
		}
		if needle == "" {
			needle = t.KeyFile
		}
		a.violate(lint.CodePolicyLoosened, "signing.trust", needle,
			"[signing] trusts the signer %q, which is not in the policy list %s (origin: %s); it is dropped", trustKey(t), quoteList(pol.Items), a.origin("signing.trust"))
	}
	s.Identity, s.Issuer, s.KeyFile = "", "", ""
	switch {
	case len(kept) > 0:
		if len(kept) < len(pol.Items) {
			a.accept = append(a.accept, fmt.Sprintf("signing.trust (narrowed to %d of %d signers)", len(kept), len(pol.Items)))
		}
		s.Trust = kept
	default:
		s.Trust = nil
		for _, key := range pol.Items {
			s.Trust = append(s.Trust, trustFromKey(key))
		}
	}
}
