package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/samber/oops"
)

// ApproveListSchemaVersion versions the JSON of `approve --list --format json`.
const ApproveListSchemaVersion = 1

type approveListDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	Policy        approveListPolicy   `json:"policy"`
	Items         []approveListItem   `json:"items"`
	Orphans       []approveListOrphan `json:"orphans"`
	Summary       map[string]int      `json:"summary"`
}

type approveListPolicy struct {
	RequireApproval []string `json:"require_approval"`
	Exempt          []string `json:"exempt"`
	MinApprovers    int      `json:"min_approvers"`
	Approvers       []string `json:"approvers"`
	Enforce         bool     `json:"enforce"`
	MinAssurance    string   `json:"min_assurance,omitempty"`
	ApproversFrom   string   `json:"approvers_from,omitempty"`
	ForbidSelf      bool     `json:"forbid_self_approval,omitempty"`
}

type approveListItem struct {
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

type approveListOrphan struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Domain   string `json:"domain,omitempty"`
	Digest   string `json:"digest"`
	Reviewer string `json:"reviewer"`
}

func emptyIfNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (e *approveEnv) listDoc() *approveListDoc {
	p := e.policy
	doc := &approveListDoc{
		SchemaVersion: ApproveListSchemaVersion,
		Policy: approveListPolicy{RequireApproval: emptyIfNil(p.Selectors), Exempt: emptyIfNil(p.Exempt), MinApprovers: max(p.MinApprovers, 1),
			Approvers: emptyIfNil(p.Approvers), Enforce: p.Enforce, MinAssurance: p.MinAssurance, ApproversFrom: e.approversFrom(), ForbidSelf: p.ForbidSelf},
		Items: []approveListItem{}, Orphans: []approveListOrphan{}, Summary: map[string]int{},
	}
	hasRecord := map[string]bool{}
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		hasRecord[a.ItemKey()] = true
	}
	results := e.policy.EvaluateAll(e.lock.Approval, e.subjects, e.now)
	for i := range results {
		r := &results[i]
		if !r.Required && !approveAll && !hasRecord[r.Key()] {
			continue
		}
		who := r.Reviewers
		if len(who) == 0 {
			who = r.Recorded // an expired or unauthorized row still says who approved it
		}
		doc.Items = append(doc.Items, approveListItem{
			Ref: r.Ref(), Kind: r.Kind, ID: r.ID, Domain: r.Domain, Digest: r.Digest, Required: r.Required, Status: r.Status,
			Code: approval.CodeOf(r.Status), Reviewers: emptyIfNil(who), Assurance: r.Assurance, Expires: r.Expires, ApprovedDigest: r.ApprovedDigest,
			Detail: safeText(r.Detail),
		})
		if r.Required {
			doc.Summary["required"]++
			doc.Summary[r.Status]++
		}
	}
	orphans := approval.Orphans(e.lock.Approval, e.subjects)
	for i := range orphans {
		a := &orphans[i]
		doc.Orphans = append(doc.Orphans, approveListOrphan{Kind: a.Kind, ID: a.ID, Domain: a.Domain, Digest: a.Digest, Reviewer: a.Reviewer})
	}
	return doc
}

func shortDigest(d string) string {
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) > 12 {
		return hex[:12] + "…"
	}
	return hex
}

func (e *approveEnv) approversFrom() string {
	if e.cfg.Governance == nil {
		return ""
	}
	return e.cfg.Governance.ApproversFrom
}

// approveListRow is the tab-separated text row of one item of `approve --list`.
func approveListRow(it *approveListItem) string {
	reviewer, expires := "-", "-"
	if len(it.Reviewers) > 0 {
		reviewer = safeText(strings.Join(it.Reviewers, ","))
	}
	if it.Expires != "" {
		expires = it.Expires
	}
	status := it.Status
	if !it.Required && status == approval.StatusNotRequired {
		status = "-"
	}
	id := it.ID
	if it.Domain != "" {
		id = it.Domain + "/" + id
	}
	assurance := "-"
	if it.Assurance != "" {
		assurance = it.Assurance
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", it.Kind, safeText(id), shortDigest(it.Digest), status, reviewer, expires, assurance)
}

func (e *approveEnv) list(out io.Writer) error {
	if err := e.resolveTeams(cmdContext(), e.subjects); err != nil {
		return err
	}
	doc := e.listDoc()
	if approveFormat == formatJSON {
		return writeRawJSON(out, doc)
	}
	if len(doc.Items) == 0 && len(doc.Orphans) == 0 {
		if !e.policy.Active() {
			_, err := fmt.Fprintln(out, "nothing needs approval: [governance] require_approval is not set (see docs/approvals.md)")
			return err
		}
		_, err := fmt.Fprintln(out, "no pinned content needs approval")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tID\tDIGEST\tSTATUS\tREVIEWER\tEXPIRES\tASSURANCE") //nolint:errcheck // flushed below
	for i := range doc.Items {
		fmt.Fprint(tw, approveListRow(&doc.Items[i])) //nolint:errcheck // flushed below
	}
	for _, o := range doc.Orphans {
		id := o.ID
		if o.Domain != "" {
			id = o.Domain + "/" + id
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\torphan\t%s\t-\t-\n", o.Kind, safeText(id), shortDigest(o.Digest), safeText(o.Reviewer)) //nolint:errcheck // flushed below
	}
	if err := tw.Flush(); err != nil {
		return oops.Wrapf(err, "write the list")
	}
	s := doc.Summary
	_, err := fmt.Fprintf(out, "%d require approval: %d ok, %d stale, %d missing, %d expired, %d unauthorized, %d insufficient",
		s["required"], s[approval.StatusOK], s[approval.StatusStale], s[approval.StatusMissing], s[approval.StatusExpired],
		s[approval.StatusUnauthorized], s[approval.StatusInsufficient])
	if err != nil {
		return err //nolint:wrapcheck // write error
	}
	if s[approval.StatusDenied] > 0 || s[approval.StatusUnverified] > 0 {
		_, err = fmt.Fprintf(out, ", %d denied, %d unverified", s[approval.StatusDenied], s[approval.StatusUnverified])
		if err != nil {
			return err //nolint:wrapcheck // write error
		}
	}
	_, err = fmt.Fprintln(out)
	return err
}
