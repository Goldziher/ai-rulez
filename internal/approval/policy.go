// Package approval decides whether pinned content has the reviewer approvals the
// [governance] table asks for. An approval is a record in ai-rulez.lock bound to
// one content digest (docs/approvals.md): when the digest changes the record stops
// applying, so a new version of an approved item needs a new review.
package approval

import (
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Remote entry kinds an approval can name, besides the item kinds. The installed
// skill is "installed-skill" (as in the lock's tree digest) so it cannot collide
// with an authored skill item.
const (
	KindInclude        = lockfile.KindInclude
	KindInstalledSkill = "installed-skill"
	KindSource         = lockfile.KindSource
	KindServed         = lockfile.KindServed
)

// Classes group subjects for the require_approval selectors.
const (
	ClassRemote = "remote"
	ClassLocal  = "local"
	// ClassServedLocal is a served skill authored in the project: it is already an
	// item, so only `kind:served` selects it.
	ClassServedLocal = "served-local"
	// ClassRoleOutput is the pinned rendering of a role: selected only by
	// `kind:role-output`, never by remote, local or all, so pinning a role does
	// not silently widen an existing policy.
	ClassRoleOutput = "role-output"
)

// KindRoleOutput is the subject kind of a role's pinned outputs ([roles] pin).
const KindRoleOutput = "role-output"

// Subject is one piece of pinned content that may need approval.
type Subject struct {
	Kind   string
	Domain string
	ID     string
	Digest string
	// Path is the item's source relative to the configuration directory ("" for remote entries).
	Path  string
	Class string
}

// Ref is the user-facing reference: kind:id, or kind:domain/id.
func (s Subject) Ref() string {
	if s.Domain != "" {
		return s.Kind + ":" + s.Domain + "/" + s.ID
	}
	return s.Kind + ":" + s.ID
}

// Key is the key records are matched by.
func (s Subject) Key() string { return s.Kind + "\x00" + s.Domain + "\x00" + s.ID }

// SubjectsOf lists the content that can be approved: the given items (the lock's
// pins, or a fresh snapshot of the working tree) and the remote entries of lock.
// The result is sorted by reference.
func SubjectsOf(lock *lockfile.File, items []lockfile.Item) []Subject {
	var out []Subject
	for _, it := range items {
		out = append(out, Subject{Kind: it.Kind, Domain: it.Domain, ID: it.ID, Digest: it.Digest, Path: it.Path, Class: ClassLocal})
	}
	if lock != nil {
		for _, group := range []struct {
			kind    string
			entries []lockfile.Entry
		}{{KindInclude, lock.Include}, {KindInstalledSkill, lock.Skill}, {KindSource, lock.Source}} {
			for i := range group.entries {
				e := &group.entries[i]
				out = append(out, Subject{Kind: group.kind, ID: e.Name, Digest: e.Digest, Class: ClassRemote})
			}
		}
		for _, o := range lock.RoleOutputs() {
			out = append(out, Subject{Kind: KindRoleOutput, ID: o.Role, Digest: o.Digest, Class: ClassRoleOutput})
		}
		for i := range lock.Served {
			e := &lock.Served[i]
			out = append(out, Subject{Kind: KindServed, Domain: e.View, ID: e.Name, Digest: e.Digest, Class: ServedClass(e.Source, e.Ref, e.Commit)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref() < out[j].Ref() })
	return out
}

// ServedClass is the class of a served skill, the one rule the lock listing
// (SubjectsOf) and the skills server share: remote when RemoteServed, else
// ClassServedLocal.
func ServedClass(source, ref, commit string) string {
	if RemoteServed(source, ref, commit) {
		return ClassRemote
	}
	return ClassServedLocal
}

// RemoteServed reports whether a served skill comes from outside the project: it
// names a ref or a commit, or its source is a git URL. A skill authored in the
// project records its own file path as the source and neither of the others.
func RemoteServed(source, ref, commit string) bool {
	return ref != "" || commit != "" || lockfile.IsGitSource(source)
}

// Policy is the [governance] table in evaluable form.
type Policy struct {
	// Floor holds the selectors an organization policy imposes; Exempt never
	// narrows them.
	Floor        []string
	Selectors    []string
	Exempt       []string
	MinApprovers int
	Approvers    []string
	MaxAge       time.Duration
	Enforce      bool
	// MinAssurance is the weakest assurance level that counts ("" is asserted).
	MinAssurance string
	// ForbidSelf rejects approvals by authors of the content (checked against git
	// history by the commands, not by Evaluate).
	ForbidSelf bool
	// Teams expands @org/team entries of Approvers and CODEOWNERS.
	Teams Teams
	// Owners restricts who may approve an item to the owners of its path
	// (approvers_from); nil when the policy sets no such restriction.
	Owners *OwnerSet
	// Deny maps a denied digest to its reason (the lock's [[deny]] entries).
	Deny map[string]string
	// DenyProblem is why the deny list could not be read ("" when it could). A
	// policy that cannot read its deny list refuses every digest: it must not
	// fail open (AR717).
	DenyProblem string
	// Origin reports the forge repository of the project (its origin remote); nil
	// or false when unknown. A review-linked approval must name a review of it.
	Origin func() (forge.Repo, bool)
	// Signed verifies signed approvals; nil means none can be verified.
	Signed SignedVerifier
}

// PolicyOf reads the policy of cfg; a project without [governance] gets a
// policy that requires nothing. It also loads what the policy needs from disk:
// the lock's deny list, the CODEOWNERS file approvers_from names, and the
// verifier of signed approvals (built on first use).
func PolicyOf(cfg *config.Config) Policy {
	if cfg == nil {
		return Policy{}
	}
	p := Policy{}
	if g := cfg.Governance; g != nil {
		p = Policy{
			Selectors: g.RequireApproval, Exempt: g.Exempt, MinApprovers: g.MinApprovers,
			Enforce: g.Enforce, Floor: g.PolicyFloor, MinAssurance: g.MinAssurance, ForbidSelf: g.ForbidSelfApproval,
			Teams: NewTeams(g.Teams),
		}
		for _, a := range g.Approvers {
			p.Approvers = append(p.Approvers, NormalizeReviewer(a))
		}
		if g.MaxAge != "" {
			p.MaxAge, _ = config.ParseApprovalMaxAge(g.MaxAge) //nolint:errcheck // validated on load
		}
		if g.ApproversFrom != "" {
			p.Owners = LoadOwnerSet(cfg.BaseDir, cfg.ConfigDir, g.ApproversFrom)
		}
	}
	if cfg.ConfigDir != "" {
		if lock, err := lockfile.Load(cfg.ConfigDir); err == nil {
			p.Deny = lock.DenySet()
		} else {
			p.DenyProblem = "the deny list cannot be read: " + err.Error()
		}
		p.Signed = newConfigVerifier(cfg)
		p.Origin = originOf(cfg.BaseDir)
	}
	return p
}

// WithLock returns the policy with the deny list of lock, for a caller that has
// the lock in memory and has changed it.
func (p Policy) WithLock(lock *lockfile.File) Policy {
	p.Deny, p.DenyProblem = lock.DenySet(), ""
	return p
}

// WithResolvedTeams returns the policy with team members read from the forge.
func (p Policy) WithResolvedTeams(resolved map[string][]string) Policy {
	p.Teams.Resolved = resolved
	return p
}

// Active reports whether the policy requires approval of anything.
func (p Policy) Active() bool { return len(p.Selectors) > 0 || len(p.Floor) > 0 }

// LockProblem is the AR710 message for a policy that is active and enforced but
// has no lock, or a lock that pins no content, to approve against; "" when there
// is nothing to refuse. Enforcement never fails open: a deleted or stripped lock
// must not switch the approvals off.
func (p Policy) LockProblem(lock *lockfile.File) string {
	if !p.Active() || !p.Enforce {
		return ""
	}
	switch {
	case lock == nil:
		return CodeMissing + " approval-missing: [governance] enforce is on and there is no " + lockfile.FileName + " to hold approvals; run `ai-rulez lock`, then `ai-rulez approve`"
	case !lock.HasContentPins():
		return CodeMissing + " approval-missing: [governance] enforce is on and " + lockfile.FileName + " pins no content, so no approval can apply; run `ai-rulez lock`"
	}
	return ""
}

// NormalizeReviewer is the identity a reviewer is compared by: trimmed and
// lower-cased, so "Alice@Example.org" and "alice@example.org" are one person. It
// is applied when a record is written and when records and allowlists are compared.
func NormalizeReviewer(r string) string { return strings.ToLower(strings.TrimSpace(r)) }

func (p Policy) minApprovers() int { return max(p.MinApprovers, 1) }

// Requires reports whether s needs approval under the policy: the organization
// floor always applies; the repository's own selectors apply unless exempt.
func (p Policy) Requires(s Subject) bool {
	if selects(p.Floor, s) {
		return true
	}
	for _, ex := range p.Exempt {
		if globMatch(ex, s.Ref()) {
			return false
		}
	}
	return selects(p.Selectors, s)
}

func selects(selectors []string, s Subject) bool {
	for _, sel := range selectors {
		switch sel {
		case config.ApprovalSelectorAll:
			if s.Class != ClassServedLocal && s.Class != ClassRoleOutput {
				return true
			}
		case config.ApprovalSelectorRemote:
			if s.Class == ClassRemote {
				return true
			}
		case config.ApprovalSelectorLocal:
			if s.Class == ClassLocal {
				return true
			}
		default:
			if kind, ok := strings.CutPrefix(sel, "kind:"); ok && kindMatches(kind, s) {
				return true
			}
		}
	}
	return false
}

// kindMatches reports whether a kind selector selects s. "mcp_server" selects
// the pinned MCP server declarations, which the lock keeps as the settings item
// "mcp-servers".
func kindMatches(kind string, s Subject) bool {
	return kind == s.Kind || (kind == "mcp_server" && s.Kind == "settings" && s.ID == "mcp-servers")
}

// globMatch matches name against a pattern where * matches any run of characters
// (slashes and colons included) and ? matches one.
func globMatch(pattern, name string) bool {
	px, nx, starP, starN := 0, 0, -1, 0
	for nx < len(name) {
		switch {
		case px < len(pattern) && pattern[px] == '*':
			starP, starN = px, nx
			px++
		case px < len(pattern) && (pattern[px] == '?' || pattern[px] == name[nx]):
			px++
			nx++
		case starP >= 0:
			starN++
			px, nx = starP+1, starN
		default:
			return false
		}
	}
	for px < len(pattern) && pattern[px] == '*' {
		px++
	}
	return px == len(pattern)
}

// NobodyMayApprove is the single "approver" that an organization policy with an
// empty approvers list leaves: Authorized never accepts anyone then, not even a
// reviewer string equal to it.
const NobodyMayApprove = "\x00"

// Authorized reports whether reviewer is on the allowlist: any reviewer when no
// allowlist is set. A team entry matches its members (Teams).
func (p Policy) Authorized(reviewer string) bool {
	if len(p.Approvers) == 0 {
		return true
	}
	if len(p.Approvers) == 1 && p.Approvers[0] == NobodyMayApprove {
		return false
	}
	return p.Teams.Matches(p.Approvers, reviewer)
}

// Names reports whether the policy explicitly names reviewer for s: the reviewer
// is in [governance] approvers (a user, or a member of a listed team), or owns
// the path of s according to CODEOWNERS. A policy that names nobody names no one;
// unlike Authorized, an empty allowlist does not stand for everybody.
func (p Policy) Names(reviewer string, s Subject) bool {
	if len(p.Approvers) > 0 && (len(p.Approvers) != 1 || p.Approvers[0] != NobodyMayApprove) && p.Teams.Matches(p.Approvers, reviewer) {
		return true
	}
	if p.Owners == nil {
		return false
	}
	owners, covered := p.Owners.OwnersOf(s)
	return covered && p.Teams.Matches(owners, reviewer)
}

// IdentityOf returns the person a signing key belongs to when the reviewer is
// "key:<fingerprint>" and its [[signing.trust]] entry names one (reviewer = "..."),
// else reviewer itself. forbid_self_approval matches commit authors with it.
func (p Policy) IdentityOf(reviewer string) string {
	if m, ok := p.Signed.(IdentityMapper); ok && strings.HasPrefix(reviewer, "key:") {
		if who := m.IdentityOf(reviewer); who != "" {
			return who
		}
	}
	return reviewer
}

// AuthorizedFor is Authorized and, when approvers_from is set, the requirement
// that reviewer owns the path of s according to CODEOWNERS. A CODEOWNERS file
// that cannot be read, a path no line covers and a team that cannot be expanded
// authorize nobody.
func (p Policy) AuthorizedFor(reviewer string, s Subject) bool {
	if !p.Authorized(reviewer) {
		return false
	}
	if p.Owners == nil {
		return true
	}
	owners, covered := p.Owners.OwnersOf(s)
	return covered && p.Teams.Matches(owners, reviewer)
}

// OwnersProblem says why approvers_from cannot authorize anyone ("" when it
// can): the CODEOWNERS file is missing.
func (p Policy) OwnersProblem() string {
	if p.Owners == nil {
		return ""
	}
	return p.Owners.Problem
}

// UnresolvedTeams lists the teams of the allowlist and of the subjects' owners
// that cannot be expanded: add them to [governance.teams] or resolve them with
// --resolve-teams.
func (p Policy) UnresolvedTeams(subs []Subject) []string {
	entries := append([]string(nil), p.Approvers...)
	if p.Owners != nil {
		for _, s := range subs {
			o, _ := p.Owners.OwnersOf(s)
			entries = append(entries, o...)
		}
	}
	return p.Teams.Unresolved(entries)
}
