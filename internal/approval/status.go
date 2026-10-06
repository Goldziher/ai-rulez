package approval

import (
	"fmt"
	"sort"
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
)

// Strict-validation codes of the statuses (internal/lint registers them).
const (
	CodeMissing      = "AR710"
	CodeStale        = "AR711"
	CodeExpired      = "AR712"
	CodeUnauthorized = "AR713"
	CodeInsufficient = "AR714"
	CodeOrphan       = "AR715"
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
	// Expires is the earliest expiry among the applying approvals ("" for none).
	Expires string
	// ApprovedDigest is the digest the newest record approved, for a stale subject.
	ApprovedDigest string
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
		return fmt.Sprintf("%s has %d of the required approvers; another reviewer must run `ai-rulez approve %s`", r.Ref(), len(r.Reviewers), r.Ref())
	}
	return r.Ref() + " is approved"
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

// Evaluate decides the status of s from the lock's approval records.
func (p Policy) Evaluate(recs []lockfile.Approval, s Subject, now time.Time) Result {
	res := Result{Subject: s, Required: p.Requires(s)}
	if !res.Required {
		res.Status = StatusNotRequired
		res.Reviewers = p.currentReviewers(recs, s, now)
		return res
	}
	var forKey, current []lockfile.Approval
	for _, a := range recs {
		if a.ItemKey() != s.Key() {
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
	var valid []lockfile.Approval
	expired, unauthorized := false, false
	for _, a := range current {
		switch {
		case ExpiredAt(a.Expires, now):
			expired = true
		case !p.Authorized(a.Reviewer):
			unauthorized = true
		default:
			valid = append(valid, a)
		}
	}
	res.Reviewers, res.Expires = reviewersOf(valid)
	switch {
	case len(res.Reviewers) >= p.minApprovers():
		res.Status = StatusOK
	case len(valid) > 0:
		res.Status = StatusInsufficient
	case expired:
		res.Status = StatusExpired
	case unauthorized:
		res.Status = StatusUnauthorized
	default:
		res.Status = StatusMissing
	}
	return res
}

// currentReviewers lists who approved the current digest, for a subject that needs no approval.
func (p Policy) currentReviewers(recs []lockfile.Approval, s Subject, now time.Time) []string {
	var valid []lockfile.Approval
	for _, a := range recs {
		if a.ItemKey() == s.Key() && a.Digest == s.Digest && !ExpiredAt(a.Expires, now) {
			valid = append(valid, a)
		}
	}
	reviewers, _ := reviewersOf(valid)
	return reviewers
}

func reviewersOf(valid []lockfile.Approval) (reviewers []string, expires string) {
	seen := map[string]bool{}
	for _, a := range valid {
		who := NormalizeReviewer(a.Reviewer)
		if !seen[who] {
			seen[who] = true
			reviewers = append(reviewers, who)
		}
		if a.Expires != "" && (expires == "" || a.Expires < expires) {
			expires = a.Expires
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
