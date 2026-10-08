package commands

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
)

// approveReview is what the reviewer is shown for one item.
type approveReview struct {
	subject  approval.Subject
	files    []approvedFile
	note     string
	findings []lint.Finding
	previous []lockfile.Approval
}

func (e *approveEnv) review(s approval.Subject) approveReview {
	rv := approveReview{subject: s}
	rv.files, rv.note = subjectFiles(e.cfg, s)
	rv.findings = scanApproved(e.cfg, s.ID, rv.files)
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if a.ItemKey() == s.Key() {
			rv.previous = append(rv.previous, *a)
		}
	}
	return rv
}

func (e *approveEnv) printReview(out io.Writer, rv *approveReview) {
	s := rv.subject
	fmt.Fprintf(out, "%s  %s\n", safeText(s.Ref()), s.Digest) //nolint:errcheck // terminal output
	for i := range rv.previous {
		a := &rv.previous[i]
		state := "approved"
		if a.Digest != s.Digest {
			state = "previously approved at " + shortDigest(a.Digest) + " (now changed)"
		}
		fmt.Fprintf(out, "  %s by %s at %s\n", state, safeText(a.Reviewer), a.ApprovedAt) //nolint:errcheck // terminal output
	}
	for _, f := range rv.files {
		extra := ""
		switch {
		case f.Executable:
			extra = " (executable)"
		case f.Data == nil:
			extra = " (too large to scan)"
		}
		fmt.Fprintf(out, "  %s  %d bytes%s\n", safeText(f.Path), f.Size, extra) //nolint:errcheck // terminal output
	}
	if rv.note != "" {
		fmt.Fprintf(out, "  note: %s\n", safeText(rv.note)) //nolint:errcheck // terminal output
	}
	if len(rv.findings) == 0 {
		fmt.Fprintln(out, "  scan: no findings") //nolint:errcheck // terminal output
		return
	}
	fmt.Fprintf(out, "  scan: %d finding(s)\n", len(rv.findings)) //nolint:errcheck // terminal output
	for _, f := range rv.findings {
		fmt.Fprintf(out, "    %s %s %s: %s\n", f.Code, f.Severity, safeText(f.File), safeText(f.Message)) //nolint:errcheck // terminal output
	}
}

func (e *approveEnv) resolveAll(refs []string) ([]approval.Subject, error) {
	var subs []approval.Subject
	for _, ref := range refs {
		s, err := approval.Resolve(e.subjects, ref)
		if err != nil {
			return nil, oops.Wrap(err)
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func (e *approveEnv) diff(out io.Writer, refs []string) error {
	subs, err := e.resolveAll(refs)
	if err != nil {
		return err
	}
	for _, s := range subs {
		rv := e.review(s)
		e.printReview(out, &rv)
	}
	return nil
}

// showAndScan prints what each item is, scans it, and returns the finding codes
// the reviewer accepted per item. An error-level finding that is not accepted
// refuses the whole approval.
func (e *approveEnv) showAndScan(out io.Writer, subs []approval.Subject) ([][]string, error) {
	accepted := map[string]bool{}
	for _, c := range approveAccept {
		accepted[strings.ToUpper(c)] = true
	}
	taken := make([][]string, len(subs))
	var blocked []string
	for i, s := range subs {
		rv := e.review(s)
		e.printReview(out, &rv)
		for _, f := range rv.findings {
			switch {
			case accepted[f.Code]:
				taken[i] = appendUnique(taken[i], f.Code)
			case f.Severity == lint.SeverityError:
				blocked = appendUnique(blocked, s.Ref()+" "+f.Code)
			}
		}
	}
	if len(blocked) > 0 {
		return nil, oops.Hint("read the findings above; pass --accept <code> only for a finding you reviewed and accept").
			Errorf("refusing to approve content with error-level scan findings: %s", strings.Join(blocked, ", "))
	}
	return taken, nil
}

// refuseBeforeReview stops what no review can fix: a denied digest, role outputs
// whose pin is stale, and teams that cannot be read.
func (e *approveEnv) refuseBeforeReview(ctx context.Context, subs []approval.Subject) error {
	if err := e.refuseDenied(subs); err != nil {
		return err
	}
	if err := e.checkRoleOutputs(ctx, subs); err != nil {
		return err
	}
	return e.resolveTeams(ctx, subs)
}
