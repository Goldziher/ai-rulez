package approval

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Identity is the form a reviewer, an owner or a team member is compared in:
// trimmed and lower-cased, with a "github:" or "@" prefix dropped, so
// "github:Alice", "@alice" and "alice" are one person. An email is kept whole.
func Identity(s string) string {
	s = NormalizeReviewer(s)
	s = strings.TrimPrefix(s, "github:")
	if !strings.Contains(s, "@") || strings.HasPrefix(s, "@") {
		s = strings.TrimPrefix(s, "@")
	}
	return s
}

// SameReviewer reports whether a and b name one reviewer.
func SameReviewer(a, b string) bool { return Identity(a) == Identity(b) }

// IsTeam reports whether an owner or approver entry names a team ("@org/team").
func IsTeam(entry string) bool {
	e := strings.TrimSpace(entry)
	return strings.HasPrefix(e, "@") && strings.Contains(e, "/")
}

// teamKey is the form teams are keyed by: lower-cased, with the "@".
func teamKey(team string) string { return NormalizeReviewer(team) }

// OwnerSet resolves who may approve an item from a CODEOWNERS file and the
// team map. A CODEOWNERS file that cannot be read, a path with no owner and a
// team that cannot be expanded all fail closed: nobody is authorized by them.
type OwnerSet struct {
	// Codeowners is the parsed file; nil when it could not be read (Problem says why).
	Codeowners *Codeowners
	// Source is the file that was read, relative to the repository root.
	Source string
	// Prefix is the configuration directory relative to the CODEOWNERS root
	// ("" when they are one), the base of a subject's Path.
	Prefix string
	// LockPath is the repository-relative path of ai-rulez.lock, the owner path
	// of content that has no source file (remote entries, role outputs).
	LockPath string
	// Problem is why Codeowners is nil.
	Problem string
}

// codeownersLocations are where the forge looks, in order.
var codeownersLocations = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// LoadOwnerSet reads the CODEOWNERS file approvers_from names. The root is the
// git top level of the configuration directory, else baseDir. It never returns
// nil: a file that cannot be read is an OwnerSet with a Problem.
func LoadOwnerSet(baseDir, configDir, from string) *OwnerSet {
	root := baseDir
	if top := (gitutil.Git{}).TopLevel(configDir); top != "" {
		root = top
	}
	set := &OwnerSet{}
	if rel, err := filepath.Rel(root, configDir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		set.Prefix = filepath.ToSlash(rel)
	}
	set.LockPath = path.Join(set.Prefix, lockfile.FileName)
	candidates := codeownersLocations
	if from != "" && from != config.ApproversFromCodeowners {
		candidates = []string{filepath.ToSlash(from)}
	}
	for _, rel := range candidates {
		data, err := readOwnersFile(root, rel)
		if err != nil {
			continue
		}
		set.Codeowners, set.Source = ParseCodeowners(data), rel
		return set
	}
	set.Problem = "approvers_from = " + `"` + from + `"` + ": no CODEOWNERS file found (looked at " + strings.Join(candidates, ", ") + ")"
	return set
}

func readOwnersFile(root, rel string) ([]byte, error) {
	data, _, err := safefs.ReadRegularKeepMode(filepath.Join(root, filepath.FromSlash(rel)))
	return data, err //nolint:wrapcheck // only "unreadable" matters to the caller
}

// PathOf is the repository-relative path owners are looked up by: the item's
// source, else the lock file.
func (s *OwnerSet) PathOf(sub Subject) string {
	if sub.Path == "" {
		return s.LockPath
	}
	return path.Join(s.Prefix, sub.Path)
}

// OwnersOf lists the owner entries of a subject's path and whether CODEOWNERS
// covers it.
func (s *OwnerSet) OwnersOf(sub Subject) (owners []string, covered bool) {
	if s == nil || s.Codeowners == nil {
		return nil, false
	}
	owners, covered = s.Codeowners.OwnersOf(s.PathOf(sub))
	return owners, covered && len(owners) > 0
}

// Teams expands team entries: the explicit [governance.teams] map, and members
// read from the forge (--resolve-teams). Both are keyed by lower-cased "@org/team".
type Teams struct {
	Explicit map[string][]string
	Resolved map[string][]string
}

// NewTeams builds the team map from [governance.teams].
func NewTeams(explicit map[string][]string) Teams {
	t := Teams{Explicit: map[string][]string{}}
	for k, v := range explicit {
		t.Explicit[teamKey(k)] = v
	}
	return t
}

// Members returns the reviewers a team stands for, and whether it could be expanded.
func (t Teams) Members(team string) ([]string, bool) {
	k := teamKey(team)
	explicit, okA := t.Explicit[k]
	resolved, okB := t.Resolved[k]
	return append(append([]string(nil), explicit...), resolved...), okA || okB
}

// Unresolved lists the teams among entries that cannot be expanded.
func (t Teams) Unresolved(entries []string) []string {
	var out []string
	for _, e := range entries {
		if IsTeam(e) {
			if _, ok := t.Members(e); !ok {
				out = append(out, teamKey(e))
			}
		}
	}
	sort.Strings(out)
	return out
}

// Matches reports whether reviewer is one of entries: a user entry by identity,
// a team entry when the reviewer is among its members.
func (t Teams) Matches(entries []string, reviewer string) bool {
	for _, e := range entries {
		if IsTeam(e) {
			members, _ := t.Members(e)
			for _, m := range members {
				if SameReviewer(m, reviewer) {
					return true
				}
			}
			continue
		}
		if SameReviewer(e, reviewer) {
			return true
		}
	}
	return false
}
