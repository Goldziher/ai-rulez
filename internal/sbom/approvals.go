package sbom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// approvalIndex answers the approval status of the pinned subjects.
type approvalIndex struct {
	results map[string]approval.Result
	records []lockfile.Approval
	salt    string
	key     string
	redact  bool
}

// newApprovalIndex evaluates the lock's approval records against the current
// digests. It returns nil when there is nothing to say: no lock, or neither a
// [governance] policy nor an approval record. now fixes the clock the expiry of
// an approval is judged by.
func newApprovalIndex(cfg *config.Config, lock *lockfile.File, items []lockfile.Item, tree string, redact bool, key string, now time.Time) *approvalIndex {
	if lock == nil || (cfg.Governance == nil && len(lock.Approval) == 0) {
		return nil
	}
	policy := approval.PolicyOf(cfg)
	subjects := approval.SubjectsOf(lock, items)
	idx := &approvalIndex{results: map[string]approval.Result{}, records: lock.Approval, salt: tree, key: key, redact: redact}
	for _, r := range policy.EvaluateAll(lock.Approval, subjects, now) {
		idx.results[r.Key()] = r
	}
	return idx
}

// status maps an approval.Result status to the value of ai-rulez:approval.
func statusOf(r approval.Result) string {
	switch r.Status {
	case approval.StatusOK:
		return "approved"
	case approval.StatusNotRequired:
		if len(r.Reviewers) > 0 {
			return "approved"
		}
		return "not-required"
	}
	return r.Status
}

// reviewerName is the reviewer as written into the document: the identity, or a
// salted hash of it (an HMAC under RedactKey when one is set; without a key the
// salt is public, so the token only hides identities nobody can guess). The salt is the lock tree, so the same reviewer is the same
// token within one document and across rebuilds of it, but not across projects.
func (x *approvalIndex) reviewerName(reviewer string) string {
	if !x.redact {
		return reviewer
	}
	msg := []byte(x.salt + "\x00" + approval.NormalizeReviewer(reviewer))
	if x.key != "" {
		mac := hmac.New(sha256.New, []byte(x.key))
		mac.Write(msg)
		return "reviewer-" + hex.EncodeToString(mac.Sum(nil)[:4])
	}
	sum := sha256.Sum256(msg)
	return "reviewer-" + hex.EncodeToString(sum[:4])
}

// annotate adds the approval properties and review records of the subject.
// It does nothing for a subject the lock does not pin.
func (x *approvalIndex) annotate(c *Component, kind, domain, id string) {
	if x == nil {
		return
	}
	s := approval.Subject{Kind: kind, Domain: domain, ID: id}
	res, ok := x.results[s.Key()]
	if !ok {
		return
	}
	props := []Property{prop("approval", statusOf(res))}
	var names []string
	for _, who := range res.Reviewers {
		names = append(names, x.reviewerName(who))
	}
	sort.Strings(names)
	if len(names) > 0 {
		props = append(props, prop("approvers", strings.Join(names, ",")), prop("approval-assurance", res.Assurance))
	}
	if res.Expires != "" {
		props = append(props, prop("approval-expires", res.Expires))
	}
	c.Properties = sortProps(append(c.Properties, props...))
	if len(res.Reviewers) == 0 {
		return
	}
	applying := map[string]bool{}
	for _, who := range res.Reviewers {
		applying[approval.Identity(who)] = true
	}
	if c.info == nil {
		c.info = &info{}
	}
	for _, rec := range x.records {
		if rec.ItemKey() != s.Key() || rec.Digest != res.Digest {
			continue
		}
		if !applying[approval.Identity(rec.Reviewer)] {
			continue
		}
		c.info.reviews = append(c.info.reviews, review{reviewer: x.reviewerName(rec.Reviewer), at: rec.ApprovedAt, digest: rec.Digest, assurance: rec.Assurance})
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
