package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/samber/oops"
)

// ApproveListSchemaVersion versions the JSON of `approve --list --format json`.
const ApproveListSchemaVersion = govview.ApprovalListSchemaVersion

type (
	approveListDoc    = govview.ApprovalListDoc
	approveListItem   = govview.ApprovalListItem
	approveListOrphan = govview.ApprovalListOrphan
)

// listDoc is the document the approvals_status tool returns as well.
func (e *approveEnv) listDoc() *approveListDoc {
	return govview.BuildApprovalList(govview.ApprovalListInput{
		Policy: e.policy, Lock: e.lock, Subjects: e.subjects, Now: e.now,
		ApproversFrom: e.approversFrom(), All: approveAll, Safe: safeText,
	})
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
