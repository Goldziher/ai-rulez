package govview

import (
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// ApprovalListSchemaVersion versions the JSON of `approve --list --format json`
// and of the approvals_status tool, which return the same document.
const ApprovalListSchemaVersion = 1

// ApprovalListDoc is the document `approve --list --format json` prints.
type ApprovalListDoc struct {
	SchemaVersion int                  `json:"schema_version"`
	Policy        ApprovalListPolicy   `json:"policy"`
	Items         []ApprovalListItem   `json:"items"`
	Orphans       []ApprovalListOrphan `json:"orphans"`
	Summary       map[string]int       `json:"summary"`
}

// ApprovalListPolicy is the governance policy the list was judged against.
type ApprovalListPolicy struct {
	RequireApproval []string `json:"require_approval"`
	Exempt          []string `json:"exempt"`
	MinApprovers    int      `json:"min_approvers"`
	Approvers       []string `json:"approvers"`
	Enforce         bool     `json:"enforce"`
	MinAssurance    string   `json:"min_assurance,omitempty"`
	ApproversFrom   string   `json:"approvers_from,omitempty"`
	ForbidSelf      bool     `json:"forbid_self_approval,omitempty"`
}

// ApprovalListItem is one pinned item and its approval status.
type ApprovalListItem struct {
	Ref            string   `json:"ref"`
	Kind           string   `json:"kind"`
	ID             string   `json:"id"`
	Domain         string   `json:"domain,omitempty"`
	Digest         string   `json:"digest"`
	Required       bool     `json:"required"`
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	Reviewers      []string `json:"reviewers"`
	Assurance      string   `json:"assurance,omitempty"`
	Expires        string   `json:"expires,omitempty"`
	ApprovedDigest string   `json:"approved_digest,omitempty"`
	Detail         string   `json:"detail,omitempty"`
}

// ApprovalListOrphan is an approval whose content no longer exists.
type ApprovalListOrphan struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Domain   string `json:"domain,omitempty"`
	Digest   string `json:"digest"`
	Reviewer string `json:"reviewer"`
}

// ApprovalListInput is what the list is built from.
type ApprovalListInput struct {
	Policy        approval.Policy
	Lock          *lockfile.File
	Subjects      []approval.Subject
	Now           time.Time
	ApproversFrom string
	// All lists every pinned item, not only those that need approval or have a record.
	All bool
	// Safe makes free text printable (control and invisible characters escaped).
	Safe func(string) string
}

func emptyIfNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// BuildApprovalList builds the `approve --list` document: the one both the
// command and the approvals_status tool print.
func BuildApprovalList(in ApprovalListInput) *ApprovalListDoc {
	safe := in.Safe
	if safe == nil {
		safe = func(s string) string { return s }
	}
	p := in.Policy
	doc := &ApprovalListDoc{
		SchemaVersion: ApprovalListSchemaVersion,
		Policy: ApprovalListPolicy{
			RequireApproval: emptyIfNil(p.Selectors), Exempt: emptyIfNil(p.Exempt), MinApprovers: max(p.MinApprovers, 1),
			Approvers: emptyIfNil(p.Approvers), Enforce: p.Enforce, MinAssurance: p.MinAssurance,
			ApproversFrom: in.ApproversFrom, ForbidSelf: p.ForbidSelf,
		},
		Items: []ApprovalListItem{}, Orphans: []ApprovalListOrphan{}, Summary: map[string]int{},
	}
	hasRecord := map[string]bool{}
	for i := range in.Lock.Approval {
		hasRecord[in.Lock.Approval[i].ItemKey()] = true
	}
	results := p.EvaluateAll(in.Lock.Approval, in.Subjects, in.Now)
	for i := range results {
		r := &results[i]
		if !r.Required && !in.All && !hasRecord[r.Key()] {
			continue
		}
		who := r.Reviewers
		if len(who) == 0 {
			who = r.Recorded // an expired or unauthorized row still says who approved it
		}
		doc.Items = append(doc.Items, ApprovalListItem{
			Ref: r.Ref(), Kind: r.Kind, ID: r.ID, Domain: r.Domain, Digest: r.Digest, Required: r.Required, Status: r.Status,
			Code: approval.CodeOf(r.Status), Reviewers: emptyIfNil(who), Assurance: r.Assurance, Expires: r.Expires,
			ApprovedDigest: r.ApprovedDigest, Detail: safe(r.Detail),
		})
		if r.Required {
			doc.Summary["required"]++
			doc.Summary[r.Status]++
		}
	}
	orphans := approval.Orphans(in.Lock.Approval, in.Subjects)
	for i := range orphans {
		a := &orphans[i]
		doc.Orphans = append(doc.Orphans, ApprovalListOrphan{Kind: a.Kind, ID: a.ID, Domain: a.Domain, Digest: a.Digest, Reviewer: a.Reviewer})
	}
	return doc
}
