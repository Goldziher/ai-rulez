package signing

import (
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

// PredicateApproval is the predicate type of a signed approval (docs/approvals.md).
const PredicateApproval = "https://github.com/Goldziher/ai-rulez/attestations/approval/v1"

// SubjectApproval is the trust subject of a signed approval: a
// [[signing.trust]] entry with subject = "approval" says who may approve.
const SubjectApproval = config.SigningSubjectApproval

// ApprovalSubject names the content an approval statement vouches for.
type ApprovalSubject struct {
	Kind, Domain, ID string
	// Digest is the "sha256:<hex>" digest of the content that was approved.
	Digest string
}

func (s ApprovalSubject) name() string {
	if s.Domain != "" {
		return s.Kind + ":" + s.Domain + "/" + s.ID
	}
	return s.Kind + ":" + s.ID
}

// ApprovalPredicate is the predicate of an approval attestation. Everything in
// it is checked against the lock record on verification, so a record cannot be
// edited after signing: the signature covers the item, the digest, the expiry
// and the findings the reviewer accepted.
type ApprovalPredicate struct {
	Kind             string   `json:"kind"`
	ID               string   `json:"id"`
	Domain           string   `json:"domain,omitempty"`
	Digest           string   `json:"digest"`
	AcceptedFindings []string `json:"accepted_findings,omitempty"`
	Expires          string   `json:"expires,omitempty"`
	ApprovedAt       string   `json:"approved_at"`
	Repository       string   `json:"repository,omitempty"`
	AIRulezVersion   string   `json:"ai_rulez_version,omitempty"`
}

// ApprovalStatement builds the in-toto statement that attests one approval.
func ApprovalStatement(sub ApprovalSubject, pred ApprovalPredicate) (*Statement, error) {
	pred.Kind, pred.ID, pred.Domain, pred.Digest = sub.Kind, sub.ID, sub.Domain, sub.Digest
	hex := hexDigest(sub.Digest)
	if !strings.HasPrefix(sub.Digest, "sha256:") || !isHex(hex) {
		return nil, oops.Errorf("approval digest %q is not a sha256 digest", sub.Digest)
	}
	return NewStatement(PredicateApproval, []Subject{{Name: sub.name(), Digest: map[string]string{contentlock.Algorithm: hex}}}, pred)
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// ApprovalReviewer is the reviewer identity a verified signer stands for: the
// certificate identity, or "key:<id>" for a key signature.
func ApprovalReviewer(s SignerInfo) string {
	if s.Kind == KindKeyless {
		return s.Identity
	}
	return "key:" + s.KeyID
}

// ApprovalCheck verifies approval attestations offline against the
// [[signing.trust]] entries whose subject is "approval".
type ApprovalCheck struct {
	Verifier Verifier
	Trust    TrustSet
}

// ApprovalResult is a verified approval attestation.
type ApprovalResult struct {
	Signer SignerInfo
	// Reviewer is ApprovalReviewer(Signer).
	Reviewer  string
	Predicate ApprovalPredicate
}

// PrepareApprovalCheck builds the verification from the [signing] policy. It
// fails when no [[signing.trust]] entry has subject = "approval": a verification
// without a trust set would accept anyone.
func PrepareApprovalCheck(cfg *config.Config, env ambient.Env) (*ApprovalCheck, error) {
	all, err := buildTrust(cfg, VerifyOptions{Env: env})
	if err != nil {
		return nil, err
	}
	var trust TrustSet
	for i := range all.Entries {
		if all.Entries[i].Subject == SubjectApproval {
			trust.Entries = append(trust.Entries, all.Entries[i])
		}
	}
	if len(trust.Entries) == 0 {
		return nil, oops.Hint(`add a [[signing.trust]] entry with subject = "approval"`).
			Errorf("no trusted approval signer is configured: a valid signature alone does not say who may approve")
	}
	tlog := TLogOff
	if trust.HasIdentities(SubjectApproval) {
		tlog = TLogRequired
	}
	if cfg.Signing != nil && cfg.Signing.TLog != "" {
		tlog = TLogMode(cfg.Signing.TLog)
	}
	check := &ApprovalCheck{Trust: trust, Verifier: Verifier{Keys: trust.Keys(SubjectApproval), TLog: tlog}}
	if check.Verifier.TrustedRoot, err = loadTrustedRoot(cfg, VerifyOptions{Env: env}); err != nil {
		return nil, err
	}
	return check, nil
}

// KeyReviewers maps the reviewer string of each trusted approval key
// ("key:<fingerprint>") to the person its [[signing.trust]] entry names.
func (c *ApprovalCheck) KeyReviewers() map[string]string {
	out := map[string]string{}
	for i := range c.Trust.Entries {
		e := &c.Trust.Entries[i]
		if e.Key == nil || e.Reviewer == "" {
			continue
		}
		if fp, err := Fingerprint(e.Key); err == nil {
			out["key:"+fp] = e.Reviewer
		}
	}
	return out
}

// Verify checks a bundle: the signature, the signer's trust for approvals, that
// the statement is an approval attestation of exactly sub, and returns the
// decoded predicate. Failures are *Error values (AR721 to AR726).
func (c *ApprovalCheck) Verify(data []byte, sub ApprovalSubject, now time.Time) (*ApprovalResult, error) {
	res, err := c.Verifier.Verify(data)
	if err != nil {
		return nil, err
	}
	if res.Statement == nil || res.Statement.PredicateType != PredicateApproval {
		return nil, Errorf(CodeSubjectMismatch, "the signed statement is not an approval attestation")
	}
	if err := c.Trust.Check(res, SubjectApproval, now); err != nil {
		return nil, err
	}
	if err := res.Statement.RequireSubject(contentlock.Algorithm, hexDigest(sub.Digest)); err != nil {
		return nil, Errorf(CodeSubjectMismatch, "the attestation does not cover %s at %s", sub.name(), sub.Digest)
	}
	var pred ApprovalPredicate
	if err := res.Statement.DecodePredicate(&pred); err != nil {
		return nil, err
	}
	if pred.Kind != sub.Kind || pred.ID != sub.ID || pred.Domain != sub.Domain || !strings.EqualFold(pred.Digest, sub.Digest) {
		return nil, Errorf(CodeSubjectMismatch, "the attestation approves %s:%s at %s, not %s", pred.Kind, pred.ID, pred.Digest, sub.name())
	}
	return &ApprovalResult{Signer: res.Signer, Reviewer: ApprovalReviewer(res.Signer), Predicate: pred}, nil
}
