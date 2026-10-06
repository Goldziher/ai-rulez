package approval

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Statuses of a subject.
const (
	StatusNotRequired = "not_required"
	StatusOK          = "ok"
	StatusMissing     = "missing"
	StatusStale       = "stale"
	StatusExpired     = "expired"
	// StatusUnauthorized: every approval of the current digest is by a reviewer outside [governance] approvers.
	StatusUnauthorized = "unauthorized"
	// StatusInsufficient: fewer distinct valid reviewers than min_approvers.
	StatusInsufficient = "insufficient"
	// StatusDenied: the content's digest is on the lock's deny list.
	StatusDenied = "denied"
	// StatusUnverified: every approval of the current digest claims an assurance
	// that could not be verified (a signed approval whose attestation fails).
	StatusUnverified = "unverified"
)

// Strict-validation codes of the statuses (internal/lint registers them).
const (
	CodeMissing      = "AR710"
	CodeStale        = "AR711"
	CodeExpired      = "AR712"
	CodeUnauthorized = "AR713"
	CodeInsufficient = "AR714"
	CodeOrphan       = "AR715"
	CodeDenied       = "AR717"
	CodeUnverified   = "AR718"
	// CodeUnresolved is AR719: approvers_from or a team cannot be resolved, so
	// nobody can be authorized by it.
	CodeUnresolved = "AR719"
)

// CodeOf returns the code of a failing status, "" for ok and not_required.
func CodeOf(status string) string {
	switch status {
	case StatusMissing:
		return CodeMissing
	case StatusStale:
		return CodeStale
	case StatusExpired:
		return CodeExpired
	case StatusUnauthorized:
		return CodeUnauthorized
	case StatusInsufficient:
		return CodeInsufficient
	case StatusDenied:
		return CodeDenied
	case StatusUnverified:
		return CodeUnverified
	}
	return ""
}

// Result is the approval status of one subject.
type Result struct {
	Subject
	Required bool
	Status   string
	// Reviewers are the distinct reviewers whose approval of the current digest applies.
	Reviewers []string
	// Recorded are the distinct reviewers of every record of the current digest,
	// valid or not, so a listing can say who approved an expired or unauthorized item.
	Recorded []string
	// Expires is the earliest expiry among the applying approvals ("" for none).
	Expires string
	// ApprovedDigest is the digest the newest record approved, for a stale subject.
	ApprovedDigest string
	// Assurance is the weakest assurance among the applying approvals ("" for none).
	Assurance string
	// Detail adds to Message: the deny reason, or why an approval did not count.
	Detail string
}

// Failing reports whether the subject needs approval and does not have it.
func (r Result) Failing() bool { return r.Required && r.Status != StatusOK }

// Message is a complete sentence for a finding or a refusal.
func (r Result) Message() string {
	switch r.Status {
	case StatusMissing:
		return fmt.Sprintf("%s requires approval and has none; review it, then run `ai-rulez approve %s`", r.Ref(), r.Ref())
	case StatusStale:
		return fmt.Sprintf("%s was approved at %s but its content is now %s; review the change, then run `ai-rulez approve %s`", r.Ref(), short(r.ApprovedDigest), short(r.Digest), r.Ref())
	case StatusExpired:
		return fmt.Sprintf("the approval of %s has expired; review it again and run `ai-rulez approve %s`", r.Ref(), r.Ref())
	case StatusUnauthorized:
		return fmt.Sprintf("%s is approved only by reviewers outside [governance] approvers", r.Ref())
	case StatusInsufficient:
		if len(r.Reviewers) == 0 && r.Detail != "" {
			return fmt.Sprintf("%s has no approval of the required assurance: %s", r.Ref(), r.Detail)
		}
		return fmt.Sprintf("%s has %d of the required approvers; another reviewer must run `ai-rulez approve %s`", r.Ref(), len(r.Reviewers), r.Ref())
	case StatusDenied:
		return fmt.Sprintf("%s is on the deny list (%s)%s; it can be neither approved nor used", r.Ref(), short(r.Digest), reasonSuffix(r.Detail))
	case StatusUnverified:
		return fmt.Sprintf("%s has approvals whose assurance cannot be verified: %s", r.Ref(), r.Detail)
	}
	return r.Ref() + " is approved"
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + reason
}

func short(digest string) string {
	if len(digest) > 19 {
		return digest[:19] + "…"
	}
	return digest
}

// Today formats now as the date an expiry is compared with (UTC).
func Today(now time.Time) string { return now.UTC().Format(time.DateOnly) }

// ExpiredAt reports whether an approval with this expiry no longer applies on
// now's date. The approval holds through its expiry date. An expiry that is not a
// YYYY-MM-DD date fails closed: it counts as expired.
func ExpiredAt(expires string, now time.Time) bool {
	if expires == "" {
		return false
	}
	if _, err := time.Parse(time.DateOnly, expires); err != nil {
		return true
	}
	return expires < Today(now)
}

// Ceiling is the last date an approval recorded at approvedAt holds when the
// policy sets max_age: approved_at plus max_age. "" when there is no ceiling or
// approvedAt is not an RFC 3339 time (callers treat that as past the ceiling).
func (p Policy) Ceiling(approvedAt string) (date string, ok bool) {
	if p.MaxAge <= 0 {
		return "", false
	}
	t, err := time.Parse(time.RFC3339, approvedAt)
	if err != nil {
		return "", true
	}
	return t.UTC().Add(p.MaxAge).Format(time.DateOnly), true
}

// pastCeiling reports whether max_age ended the approval: [governance] max_age is
// a ceiling, so an `expires` date later than approved_at + max_age (or none) never
// extends it. An approved_at that does not parse fails closed.
func (p Policy) pastCeiling(a lockfile.Approval, now time.Time) bool {
	date, ok := p.Ceiling(a.ApprovedAt)
	return ok && (date == "" || ExpiredAt(date, now))
}

// knownAssurance reports whether a record's assurance is one this version defines.
func knownAssurance(level string) bool { return lockfile.AssuranceRank(level) > 0 }

// counted is an approval that applies, with the reviewer it counts for.
type counted struct {
	reviewer  string
	expires   string
	assurance string
}

// Evaluate decides the status of s from the lock's approval records.
func (p Policy) Evaluate(recs []lockfile.Approval, s Subject, now time.Time) Result {
	res := Result{Subject: s, Required: p.Requires(s)}
	if reason, denied := p.Deny[s.Digest]; denied && s.Digest != "" {
		res.Required, res.Status, res.Detail = true, StatusDenied, reason
		return res
	}
	if !res.Required {
		res.Status = StatusNotRequired
		res.Reviewers = p.currentReviewers(recs, s, now)
		return res
	}
	var forKey, current []lockfile.Approval
	for _, a := range recs {
		// A record claiming an assurance this version does not define must not count.
		if a.ItemKey() != s.Key() || !knownAssurance(a.Assurance) {
			continue
		}
		forKey = append(forKey, a)
		if a.Digest == s.Digest {
			current = append(current, a)
		}
	}
	if len(forKey) == 0 {
		res.Status = StatusMissing
		return res
	}
	if len(current) == 0 {
		res.Status = StatusStale
		res.ApprovedDigest = newest(forKey).Digest
		return res
	}
	res.Recorded, _ = reviewersOf(asCounted(current))
	var valid []counted
	var expired, unauthorized, low bool
	var unverified []string
	for _, a := range current {
		who := a.Reviewer
		if a.Assurance != lockfile.AssuranceAsserted {
			var err error
			if who, err = p.VerifyAssurance(a, s, now); err != nil {
				unverified = append(unverified, fmt.Sprintf("%s by %s: %v", a.Assurance, safeDetail(a.Reviewer), err))
				continue
			}
		}
		switch {
		case ExpiredAt(a.Expires, now) || p.pastCeiling(a, now):
			expired = true
		case !p.AuthorizedFor(who, s):
			unauthorized = true
		case lockfile.AssuranceRank(a.Assurance) < lockfile.AssuranceRank(p.MinAssurance):
			low = true
			res.Detail = fmt.Sprintf("%s approval, [governance] min_assurance is %s", a.Assurance, p.MinAssurance)
		default:
			valid = append(valid, counted{reviewer: who, expires: a.Expires, assurance: a.Assurance})
		}
	}
	res.Reviewers, res.Expires = reviewersOf(valid)
	res.Assurance = weakest(valid)
	switch {
	case len(res.Reviewers) >= p.minApprovers():
		res.Status = StatusOK
	case len(valid) > 0:
		res.Status = StatusInsufficient
	case low:
		res.Status = StatusInsufficient
	case len(unverified) > 0 && !expired && !unauthorized:
		res.Status, res.Detail = StatusUnverified, strings.Join(unverified, "; ")
	case expired:
		res.Status = StatusExpired
	case unauthorized:
		res.Status = StatusUnauthorized
	default:
		res.Status = StatusMissing
	}
	return res
}

// VerifyAssurance checks a record that claims more than "asserted" and returns
// the reviewer it counts for. A signed record is verified against its
// attestation and the signer's identity replaces the reviewer string; a
// review-linked record must carry a ref (the forge is consulted only by
// `verify --approvals --online`).
func (p Policy) VerifyAssurance(a lockfile.Approval, s Subject, now time.Time) (string, error) {
	switch a.Assurance {
	case lockfile.AssuranceReviewLinked:
		if strings.TrimSpace(a.Ref) == "" {
			return "", errors.New("a review-linked approval needs a ref")
		}
		return a.Reviewer, nil
	case lockfile.AssuranceSigned:
		if p.Signed == nil {
			return "", errors.New("no verifier is configured")
		}
		return p.Signed.Verify(a, s, now)
	}
	return "", fmt.Errorf("unknown assurance %q", a.Assurance)
}

func safeDetail(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func asCounted(recs []lockfile.Approval) []counted {
	out := make([]counted, 0, len(recs))
	for i := range recs {
		out = append(out, counted{reviewer: recs[i].Reviewer, expires: recs[i].Expires, assurance: recs[i].Assurance})
	}
	return out
}

func weakest(valid []counted) string {
	level := ""
	for _, c := range valid {
		if level == "" || lockfile.AssuranceRank(c.assurance) < lockfile.AssuranceRank(level) {
			level = c.assurance
		}
	}
	return level
}

// currentReviewers lists who approved the current digest, for a subject that needs no approval.
func (p Policy) currentReviewers(recs []lockfile.Approval, s Subject, now time.Time) []string {
	var valid []counted
	for i := range recs {
		a := recs[i]
		if a.ItemKey() != s.Key() || a.Digest != s.Digest || ExpiredAt(a.Expires, now) || p.pastCeiling(a, now) {
			continue
		}
		who := a.Reviewer
		if a.Assurance != lockfile.AssuranceAsserted {
			var err error
			if who, err = p.VerifyAssurance(a, s, now); err != nil {
				continue
			}
		}
		valid = append(valid, counted{reviewer: who, expires: a.Expires, assurance: a.Assurance})
	}
	reviewers, _ := reviewersOf(valid)
	return reviewers
}

func reviewersOf(valid []counted) (reviewers []string, expires string) {
	seen := map[string]bool{}
	for _, c := range valid {
		who := NormalizeReviewer(c.reviewer)
		key := Identity(c.reviewer)
		if !seen[key] {
			seen[key] = true
			reviewers = append(reviewers, who)
		}
		if c.expires != "" && (expires == "" || c.expires < expires) {
			expires = c.expires
		}
	}
	sort.Strings(reviewers)
	return reviewers, expires
}

func newest(recs []lockfile.Approval) lockfile.Approval {
	best := recs[0]
	for _, a := range recs[1:] {
		if a.ApprovedAt > best.ApprovedAt {
			best = a
		}
	}
	return best
}

// EvaluateAll evaluates every subject, in the order given.
func (p Policy) EvaluateAll(recs []lockfile.Approval, subs []Subject, now time.Time) []Result {
	out := make([]Result, 0, len(subs))
	for _, s := range subs {
		out = append(out, p.Evaluate(recs, s, now))
	}
	return out
}

// Failures keeps the results that need approval and lack it.
func Failures(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if r.Failing() {
			out = append(out, r)
		}
	}
	return out
}

// Orphans returns the records that name no current subject (the item was removed or renamed).
func Orphans(recs []lockfile.Approval, subs []Subject) []lockfile.Approval {
	known := map[string]bool{}
	for _, s := range subs {
		known[s.Key()] = true
	}
	var out []lockfile.Approval
	for _, a := range recs {
		if !known[a.ItemKey()] {
			out = append(out, a)
		}
	}
	return out
}

// Resolve finds the subject a user reference names: the exact reference, else
// kind:id ignoring the domain, else a bare id. An ambiguous reference is an error
// that lists the candidates.
func Resolve(subs []Subject, ref string) (Subject, error) {
	tiers := []func(Subject) bool{
		func(s Subject) bool { return s.Ref() == ref },
		func(s Subject) bool { return s.Kind+":"+s.ID == ref },
		func(s Subject) bool { return s.ID == ref },
	}
	for _, match := range tiers {
		var found []Subject
		for _, s := range subs {
			if match(s) {
				found = append(found, s)
			}
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], nil
		}
		names := make([]string, len(found))
		for i := range found {
			names[i] = found[i].Ref()
		}
		return Subject{}, fmt.Errorf("%q is ambiguous: use one of %v", ref, names)
	}
	return Subject{}, fmt.Errorf("%q is not pinned in %s: only pinned content can be approved (run `ai-rulez lock` first)", ref, lockfile.FileName)
}
