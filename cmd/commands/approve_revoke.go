package commands

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

func (e *approveEnv) save() error {
	return lockfile.Save(e.cfg.ConfigDir, e.lock)
}

func (e *approveEnv) revoke(out io.Writer, refs []string) error {
	if err := e.checkText("--reason", approveReason); err != nil {
		return err
	}
	var dropped []lockfile.Approval
	// Orphaned records can be revoked by name too: they name content that is gone.
	known := append([]approval.Subject(nil), e.subjects...)
	orphans := approval.Orphans(e.lock.Approval, e.subjects)
	for i := range orphans {
		a := &orphans[i]
		known = append(known, approval.Subject{Kind: a.Kind, Domain: a.Domain, ID: a.ID, Digest: a.Digest})
	}
	removed := 0
	for _, ref := range refs {
		s, err := approval.Resolve(known, ref)
		if err != nil {
			return oops.Wrap(err)
		}
		kept := e.lock.Approval[:0:0]
		n := 0
		for i := range e.lock.Approval {
			a := &e.lock.Approval[i]
			if a.ItemKey() == s.Key() && (approveReviewer == "" || approval.SameReviewer(a.Reviewer, approveReviewer)) {
				n++
				dropped = append(dropped, *a)
				continue
			}
			kept = append(kept, *a)
		}
		if n == 0 {
			return oops.Errorf("%s has no approval to revoke", s.Ref())
		}
		e.lock.Approval = kept
		removed += n
		if _, err := fmt.Fprintf(out, "revoked %d approval(s) of %s\n", n, safeText(s.Ref())); err != nil {
			return oops.Wrapf(err, "write output")
		}
		if approveDeny {
			e.lock.SetDeny(lockfile.Deny{Digest: s.Digest, Reason: approveReason})
			if _, err := fmt.Fprintf(out, "denied %s %s\n", safeText(s.Ref()), s.Digest); err != nil {
				return oops.Wrapf(err, "write output")
			}
		}
	}
	if removed == 0 {
		return nil
	}
	if err := e.save(); err != nil {
		return err
	}
	e.removeUnusedBundles(dropped)
	return nil
}

// removeUnusedBundles deletes the attestation bundles of removed approvals that
// no remaining record names, so a revoked or pruned approval leaves no signature
// of it behind. Only files under the attestations directory are touched, and a
// failure to delete is a warning: the lock is already written.
func (e *approveEnv) removeUnusedBundles(dropped []lockfile.Approval) {
	inUse := map[string]bool{}
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		inUse[a.Attestation] = true
	}
	for i := range dropped {
		a := &dropped[i]
		if a.Attestation == "" || inUse[a.Attestation] {
			continue
		}
		inUse[a.Attestation] = true
		path, err := approval.AttestationFile(e.cfg.ConfigDir, a.Attestation)
		if err != nil {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("Could not remove the attestation bundle of a removed approval", "path", path, "error", err)
		}
	}
}

func (e *approveEnv) prune(out io.Writer) error {
	current := map[string]string{}
	for _, s := range e.subjects {
		current[s.Key()] = s.Digest
	}
	kept := e.lock.Approval[:0:0]
	var dropped []lockfile.Approval
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if d, ok := current[a.ItemKey()]; ok && d == a.Digest {
			kept = append(kept, *a)
		} else {
			dropped = append(dropped, *a)
		}
	}
	n := len(e.lock.Approval) - len(kept)
	if n == 0 {
		_, err := fmt.Fprintln(out, "nothing to prune")
		return err
	}
	e.lock.Approval = kept
	if err := e.save(); err != nil {
		return err
	}
	e.removeUnusedBundles(dropped)
	_, err := fmt.Fprintf(out, "pruned %d stale or orphaned approval(s)\n", n)
	return err
}

// supersede removes the reviewer's earlier records for the same item at other
// digests: the new record replaces them, so the lock does not collect one stale
// record per version. Other reviewers' records stay until they approve again.
func (e *approveEnv) supersede(rec *lockfile.Approval) {
	kept := e.lock.Approval[:0:0]
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if a.ItemKey() == rec.ItemKey() && approval.SameReviewer(a.Reviewer, rec.Reviewer) && a.Digest != rec.Digest {
			continue
		}
		kept = append(kept, *a)
	}
	e.lock.Approval = kept
}
