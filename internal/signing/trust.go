package signing

import (
	"crypto"
	"fmt"
	"regexp"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TrustEntry names one signer a project accepts for a subject ([[signing.trust]]).
// An entry is either a certificate identity (Identity or IdentityRegexp, with
// Issuer) or a key.
type TrustEntry struct {
	// Subject is what the signer may vouch for ("lock"); empty means any subject.
	Subject string
	// Identity is matched exactly against the certificate SAN.
	Identity string
	// IdentityRegexp is matched against the whole SAN; it must be anchored.
	IdentityRegexp string
	// Issuer is matched exactly against the OIDC issuer extension.
	Issuer string
	// Key trusts signatures by this public key instead of an identity.
	Key crypto.PublicKey
	// ValidFrom and ValidUntil bound the signing time the entry accepts; a zero
	// bound is open. ValidUntil is inclusive.
	ValidFrom, ValidUntil time.Time
}

// ValidateIdentityRegexp checks that expr compiles and is anchored at both ends
// ("^" first, "$" last): an unanchored pattern matches any identity containing
// it, which is how look-alike identities get trusted (AR722).
func ValidateIdentityRegexp(expr string) error { return config.ValidateIdentityRegexp(expr) }

// TrustSet is the signers a project accepts.
type TrustSet struct {
	Entries []TrustEntry
}

func (e TrustEntry) appliesTo(subject string) bool { return e.Subject == "" || e.Subject == subject }

// Keys returns the public keys trusted for subject.
func (t TrustSet) Keys(subject string) []crypto.PublicKey {
	var out []crypto.PublicKey
	for _, e := range t.Entries {
		if e.Key != nil && e.appliesTo(subject) {
			out = append(out, e.Key)
		}
	}
	return out
}

// HasIdentities reports whether any entry for subject trusts a certificate
// identity (so keyless verification and a log are in play).
func (t TrustSet) HasIdentities(subject string) bool {
	for _, e := range t.Entries {
		if e.Key == nil && e.appliesTo(subject) {
			return true
		}
	}
	return false
}

func (e TrustEntry) matches(s SignerInfo) bool {
	if e.Key != nil {
		fp, err := Fingerprint(e.Key)
		return err == nil && s.Kind == KindKey && s.KeyID == fp
	}
	if s.Kind != KindKeyless || e.Issuer == "" || s.Issuer != e.Issuer {
		return false
	}
	if e.Identity != "" {
		return s.Identity == e.Identity
	}
	if e.IdentityRegexp == "" || ValidateIdentityRegexp(e.IdentityRegexp) != nil {
		return false
	}
	// Wrapped so an alternation ("^a|b$") cannot match a substring.
	re, err := regexp.Compile(`^(?:` + e.IdentityRegexp + `)$`)
	return err == nil && re.MatchString(s.Identity)
}

func (e TrustEntry) validAt(t time.Time) bool {
	return (e.ValidFrom.IsZero() || !t.Before(e.ValidFrom)) && (e.ValidUntil.IsZero() || !t.After(e.ValidUntil))
}

// Check reports AR722 unless a trust entry for subject accepts the signer of res
// at the time it signed (the log time, else now).
func (t TrustSet) Check(res *Result, subject string, now time.Time) error {
	at := res.SignedAt
	if at.IsZero() {
		at = now
	}
	matched := false
	for _, e := range t.Entries {
		if !e.appliesTo(subject) || !e.matches(res.Signer) {
			continue
		}
		if e.validAt(at) {
			return nil
		}
		matched = true
	}
	if matched {
		return Errorf(CodeSignerNotTrusted, "%s signed outside the validity window of its trust entry (signed %s)", describe(res.Signer), at.UTC().Format(time.RFC3339))
	}
	return Errorf(CodeSignerNotTrusted, "%s is not a trusted signer for %s", describe(res.Signer), subject)
}

func describe(s SignerInfo) string {
	if s.Kind == KindKey {
		return "key " + s.KeyID
	}
	return fmt.Sprintf("%s (issuer %s)", s.Identity, s.Issuer)
}

// CheckFresh reports AR723 when the signature is older than maxAge (0 disables
// the check). The age is measured from the log or timestamp time; without one
// (Weak) it falls back to claimedAt, the issue time the signer wrote into the
// statement, which the signer controls: such a result proves nothing about
// time.
func CheckFresh(res *Result, claimedAt time.Time, maxAge time.Duration, now time.Time) error {
	if maxAge <= 0 {
		return nil
	}
	at := res.SignedAt
	if at.IsZero() {
		at = claimedAt
	}
	if at.IsZero() {
		return Errorf(CodeStale, "the signature has no time and [signing] max_age is set")
	}
	if err := checkNotFuture(at, now); err != nil {
		return err
	}
	if age := now.Sub(at); age > maxAge {
		return Errorf(CodeStale, "signed %s ago (%s), older than max_age %s", age.Round(time.Hour), at.UTC().Format(time.RFC3339), maxAge)
	}
	return nil
}

// clockSkew is how far ahead of the local clock a signing time may be before it
// is rejected.
const clockSkew = 5 * time.Minute

// checkNotFuture reports AR723 when at is later than now plus the allowed skew.
// A signature from the future has a negative age, which would pass any max_age,
// and a weak (claimed) time that far ahead would pin the rollback mark there.
func checkNotFuture(at, now time.Time) error {
	if !at.IsZero() && at.After(now.Add(clockSkew)) {
		return Errorf(CodeStale, "the signing time %s is in the future (now %s); check the clock of the signing machine", at.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	return nil
}
