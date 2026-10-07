package signing

import (
	"strings"
	"time"

	"github.com/samber/oops"
)

// ArtifactPolicy is how the attestations of one non-lock subject (a plugin
// bundle, a published skill, an SBOM) are judged.
type ArtifactPolicy struct {
	// Subject is the trust subject: SubjectBundle, SubjectSkill or SubjectSBOM.
	Subject string
	// Source scopes the trust entries (a skill's origin); "" for none.
	Source   string
	Verifier Verifier
	Trust    TrustSet
	// MaxAge is the oldest acceptable signature; 0 disables freshness.
	MaxAge time.Duration
	// Threshold is the number of distinct trusted signers required; below 2 one.
	Threshold int
	// State, when set, is consulted for rollback (AR727) per signer and artifact.
	State *State
	// ScopeRel and ScopeAbs locate the project the rollback mark belongs to (see LockPolicy).
	ScopeRel, ScopeAbs string
	// Now is the clock; required.
	Now time.Time
}

// Expectation is what the verifier recomputed from the artifact itself: a valid
// signature says who signed, never that the content matches.
type Expectation struct {
	// PredicateType is the predicate type the statement must carry.
	PredicateType string
	// Name is the statement subject name the digest is looked up under; empty
	// accepts any name (the digest decides).
	Name string
	// DigestHex is the sha256 of the artifact, lower-case hex, without a prefix.
	DigestHex string
}

// artifactClaim is the part of every ai-rulez predicate the verifier reads: the
// signer's own statement of when and for which repository it signed.
type artifactClaim struct {
	IssuedAt   time.Time `json:"issued_at"`
	Repository string    `json:"repository"`
}

// ArtifactReport is a verified attestation of one artifact.
type ArtifactReport struct {
	Result *Result
	// Statement is the verified statement (its predicate is the signer's claim).
	Statement *Statement
	// SigningTime is SignedAt, or the claimed issue time when Weak.
	SigningTime time.Time
	// Age is how long ago the signature was made.
	Age time.Duration
	// StateKeys are the rollback marks this report is checked and committed under.
	StateKeys []string
	// Cosigners are the reports of the other distinct trusted signers accepted
	// beside this one.
	Cosigners []*ArtifactReport
}

// Signers lists the distinct trusted signers that were accepted, this report's first.
func (r *ArtifactReport) Signers() []SignerInfo {
	out := []SignerInfo{r.Result.Signer}
	for _, c := range r.Cosigners {
		out = append(out, c.Result.Signer)
	}
	return out
}

// Commit records the signing times of an accepted report in the rollback state.
func (r *ArtifactReport) Commit(s *State) error {
	if s == nil {
		return nil
	}
	for _, c := range r.Cosigners {
		if err := c.Commit(s); err != nil {
			return err
		}
	}
	if r.SigningTime.IsZero() {
		return nil
	}
	return s.advanceAll(r.StateKeys, r.SigningTime)
}

// VerifyArtifact verifies the attestation files of one artifact against what the
// caller recomputed (exp). It runs, per bundle: the cryptographic check (AR721,
// AR725, AR726), the statement's predicate type and subject (AR724), the signer
// (AR722), a future signing time and freshness (AR723) and rollback (AR727); then
// it counts distinct trusted signers against the threshold (AR728). It never
// writes the rollback state: call Commit after accepting the result.
func VerifyArtifact(bundles [][]byte, exp Expectation, p ArtifactPolicy) (*ArtifactReport, error) {
	if p.Now.IsZero() {
		return nil, oops.Errorf("ArtifactPolicy.Now is required: the caller supplies the clock")
	}
	if len(bundles) == 0 {
		return nil, Errorf(CodeMissing, "no attestation to verify")
	}
	var reports []*ArtifactReport
	errs := make([]error, 0, len(bundles))
	for _, data := range bundles {
		rep, err := verifyArtifactBundle(data, exp, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		reports = append(reports, rep)
	}
	idOf := func(r *ArtifactReport) string { return p.Trust.signerKey(r.Result.Signer, p.Subject, p.Source) }
	kept, err := pickSigners(reports, errs, idOf, p.Threshold)
	if err != nil {
		return nil, err
	}
	first := *kept[0]
	first.Cosigners = kept[1:]
	return &first, nil
}

func verifyArtifactBundle(data []byte, exp Expectation, p ArtifactPolicy) (*ArtifactReport, error) {
	res, err := p.Verifier.Verify(data)
	if err != nil {
		return nil, err
	}
	st := res.Statement
	if st == nil {
		return nil, Errorf(CodeSubjectMismatch, "the signed payload is not an in-toto statement")
	}
	if st.PredicateType != exp.PredicateType {
		return nil, Errorf(CodeSubjectMismatch, "the signed statement is a %q attestation, not %q", st.PredicateType, exp.PredicateType)
	}
	if err := requireSubject(st, exp); err != nil {
		return nil, err
	}
	if err := p.Trust.CheckFor(res, p.Subject, p.Source, p.Now); err != nil {
		return nil, err
	}
	claim, err := decodeClaim(st)
	if err != nil {
		return nil, err
	}
	at := res.SignedAt
	if at.IsZero() {
		at = claim.IssuedAt
	}
	if err := checkNotFuture(at, p.Now); err != nil {
		return nil, err
	}
	if err := CheckFresh(res, claim.IssuedAt, p.MaxAge, p.Now); err != nil {
		return nil, err
	}
	keys := suffixed(scopedStateKeys(res.Signer, claim.Repository, p.ScopeRel, p.ScopeAbs), "|"+p.Subject+":"+p.Source+"/"+subjectName(st, exp)+"@"+st.PredicateType)
	if p.State != nil && !at.IsZero() {
		if err := p.State.checkAll(keys, at); err != nil {
			return nil, err
		}
	}
	return &ArtifactReport{Result: res, Statement: st, SigningTime: at, Age: p.Now.Sub(at), StateKeys: keys}, nil
}

// requireSubject finds the statement subject carrying the expected digest (and
// name, when one is expected). The name is part of the match so a signature of
// one skill cannot vouch for another skill with identical bytes.
func requireSubject(st *Statement, exp Expectation) error {
	for _, sub := range st.Subject {
		if exp.Name != "" && sub.Name != exp.Name {
			continue
		}
		if exp.DigestHex != "" && strings.EqualFold(sub.Digest["sha256"], exp.DigestHex) {
			return nil
		}
	}
	return Errorf(CodeSubjectMismatch, "the signed statement does not cover %s sha256:%s: the artifact changed after it was signed, or the attestation belongs to another one", nameOrArtifact(exp.Name), exp.DigestHex)
}

func nameOrArtifact(name string) string {
	if name == "" {
		return "the artifact"
	}
	return name
}

func subjectName(st *Statement, exp Expectation) string {
	if exp.Name != "" {
		return exp.Name
	}
	if len(st.Subject) > 0 {
		return st.Subject[0].Name
	}
	return ""
}
