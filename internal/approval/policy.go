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
)

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
			for _, e := range group.entries {
				out = append(out, Subject{Kind: group.kind, ID: e.Name, Digest: e.Digest, Class: ClassRemote})
			}
		}
		for _, e := range lock.Served {
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
}

// PolicyOf reads the policy of cfg; a project without [governance] gets the zero
// policy, which requires nothing.
func PolicyOf(cfg *config.Config) Policy {
	if cfg == nil || cfg.Governance == nil {
		return Policy{}
	}
	g := cfg.Governance
	p := Policy{
		Selectors: g.RequireApproval, Exempt: g.Exempt, MinApprovers: g.MinApprovers,
		Enforce: g.Enforce, Floor: g.PolicyFloor,
	}
	for _, a := range g.Approvers {
		p.Approvers = append(p.Approvers, NormalizeReviewer(a))
	}
	if g.MaxAge != "" {
		p.MaxAge, _ = config.ParseApprovalMaxAge(g.MaxAge) //nolint:errcheck // validated on load
	}
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
			if s.Class != ClassServedLocal {
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

// Authorized reports whether reviewer may approve: any reviewer when no allowlist is set.
func (p Policy) Authorized(reviewer string) bool {
	if len(p.Approvers) == 0 {
		return true
	}
	if len(p.Approvers) == 1 && p.Approvers[0] == NobodyMayApprove {
		return false
	}
	reviewer = NormalizeReviewer(reviewer)
	for _, a := range p.Approvers {
		if NormalizeReviewer(a) == reviewer {
			return true
		}
	}
	return false
}
