package mcp

import (
	"path"
	"path/filepath"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// approvalNow is the clock approval expiry is judged by: always the wall clock.
// SOURCE_DATE_EPOCH only stamps generated files; letting it decide expiry would
// let an environment variable revive an expired approval.
func approvalNow() time.Time { return ambient.Clock(nil).Now() }

// now is the time the admission judges approvals and signatures at: Now, or
// the wall clock.
func (a Admission) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return approvalNow()
}

// admitApproval applies [governance] to one served skill. A skill the policy
// requires approval for gets Approved and Approvers set from the lock's records;
// under [governance] enforce a skill without a valid approval of its served digest
// is refused with the AR71x code of the reason. Under enforce a missing lock
// refuses every skill the policy selects: with no lock there is no record to
// approve against, and that must not read as "approved".
//
// A skill authored in the project is also the item skill:<name>: a deny of its
// authored digest refuses it, and a selector that selects the item (all, local,
// kind:skill) gates serving it as it gates generating it.
func (a Admission) admitApproval(s *CatalogSkill) *Refusal {
	policy := approval.PolicyOf(a.Config)
	if a.Lock != nil {
		policy = policy.WithLock(a.Lock)
	}
	served := approval.Subject{
		Kind: approval.KindServed, Domain: a.View, ID: s.Name, Digest: s.LockDigest,
		Class: approval.ServedClass(s.Source, s.Ref, s.Commit),
	}
	subjects := []approval.Subject{served}
	if item, ok := a.authoredItem(s); ok {
		subjects = append(subjects, approval.Subject{
			Kind: item.Kind, Domain: item.Domain, ID: item.ID, Digest: item.Digest, Path: item.Path, Class: approval.ClassLocal,
		})
	}
	for _, subject := range subjects {
		if reason, denied := policy.Deny[subject.Digest]; denied && subject.Digest != "" {
			// A denied digest is never served, whether or not [governance] selects the skill.
			return &Refusal{Name: s.Name, Code: approval.CodeDenied, Reason: approval.Result{Subject: subject, Status: approval.StatusDenied, Detail: reason}.Message()}
		}
	}
	if !policy.Active() {
		return nil
	}
	for _, subject := range subjects {
		if r := a.gate(policy, s, subject); r != nil {
			return r
		}
	}
	return nil
}

// gate evaluates one subject of a served skill against the policy.
func (a Admission) gate(policy approval.Policy, s *CatalogSkill, subject approval.Subject) *Refusal {
	if a.Lock == nil {
		if policy.Enforce && !a.Pinning && policy.Requires(subject) {
			return &Refusal{Name: s.Name, Code: approval.CodeMissing, Reason: "[governance] enforce is on and there is no " + lockfile.FileName + " to hold approvals: " + approval.Result{Subject: subject, Status: approval.StatusMissing}.Message()}
		}
		return nil
	}
	res := policy.Evaluate(a.Lock.Approval, subject, a.now())
	if !res.Required {
		return nil
	}
	s.Approved, s.Approvers = res.Status == approval.StatusOK, res.Reviewers
	if s.Approved || !policy.Enforce {
		return nil
	}
	return &Refusal{Name: s.Name, Code: approval.CodeOf(res.Status), Reason: "[governance] enforce is on: " + res.Message()}
}

// authoredKey keys an authored skill by its domain and directory name.
func authoredKey(domain, dir string) string { return domain + "\x00" + dir }

// authoredSkillItems digests the skills authored in the project as the lock
// pins them (the skill:<name> items), keyed by domain and directory.
func authoredSkillItems(cfg *config.Config) (map[string]lockfile.Item, error) {
	if cfg == nil || cfg.Content == nil || cfg.ConfigDir == "" {
		return nil, nil
	}
	snap, err := contentlock.Compute(cfg, contentlock.Options{Scope: config.LockScopeSkills, SourcesOnly: true})
	if err != nil {
		return nil, oops.Wrapf(err, "digest the authored skills")
	}
	out := map[string]lockfile.Item{}
	for i := range snap.Items {
		if it := snap.Items[i]; it.Kind == contentlock.KindSkill {
			out[authoredKey(it.Domain, path.Base(it.Path))] = it
		}
	}
	return out, nil
}

// authoredItem is the skill:<name> item of a served skill authored in the
// project; false for a skill from an include, an installed skill or a source.
func (a Admission) authoredItem(s *CatalogSkill) (lockfile.Item, bool) {
	if a.Authored == nil || s.Imported || approval.RemoteServed(s.Source, s.Ref, s.Commit) {
		return lockfile.Item{}, false
	}
	dir := path.Base(path.Dir(filepath.ToSlash(s.Source)))
	it, ok := a.Authored[authoredKey(s.Domain, dir)]
	return it, ok
}
