package contentlock

import (
	"fmt"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// KindRoleOutput labels the aggregate digest of a role's rendered outputs and the
// changes that report it.
const KindRoleOutput = "role"

// computeRoleOutputs adds one aggregate pin per rendered role to snap.Outputs
// (Role set, no Path) and keeps the per-file digests in snap.RoleFiles. The
// aggregate is the tree digest of every output file of the role, so it moves when
// any file's bytes, set or mode does. Unlike the default outputs it does not
// depend on [lock] include_outputs: pinning a role is its own opt-in.
func computeRoleOutputs(snap *Snapshot, roles map[string][]Output, requested map[string]bool) error {
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	snap.RoleFiles = map[string][]lockfile.OutputPin{}
	for _, name := range names {
		var leaves []Leaf
		var files []lockfile.OutputPin
		for _, out := range roles[name] {
			leaf := Leaf{Path: out.Path, Mode: ModeFor(out.Mode), Data: out.Data}
			leaves = append(leaves, leaf)
			digest, err := TreeDigest("output", []Leaf{leaf})
			if err != nil {
				return err
			}
			files = append(files, lockfile.OutputPin{Path: out.Path, Digest: digest})
		}
		aggregate, err := TreeDigest("role-output", leaves)
		if err != nil {
			return err
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		snap.RoleFiles[name] = files
		snap.Outputs = append(snap.Outputs, lockfile.OutputPin{Role: name, Digest: aggregate, Requested: requested[name]})
	}
	return nil
}

// compareRoleOutputs compares the role pins of the lock with the roles rendered
// for snap. It does nothing unless snap.Options.CheckRoles is set, so a caller
// that rendered no role never reports one as removed. When Options.OnlyRoles is
// set, the lock's pins of other roles are left alone.
func (d *Diff) compareRoleOutputs(lock *lockfile.File, snap *Snapshot) {
	if !snap.Options.CheckRoles {
		return
	}
	only := map[string]bool{}
	for _, r := range snap.Options.OnlyRoles {
		only[r] = true
	}
	locked := map[string]string{}
	for _, o := range lock.RoleOutputs() {
		locked[o.Role] = o.Digest
	}
	rendered := map[string]bool{}
	for _, cur := range snap.Outputs {
		if cur.Role == "" {
			continue
		}
		rendered[cur.Role] = true
		prev, ok := locked[cur.Role]
		switch {
		case !ok:
			d.Changes = append(d.Changes, d.roleChange(snap, Added, cur, ""))
		case prev != cur.Digest:
			d.Changes = append(d.Changes, d.roleChange(snap, Changed, cur, prev))
		}
	}
	for role, digest := range locked {
		if rendered[role] || (len(only) > 0 && !only[role]) {
			continue
		}
		d.Changes = append(d.Changes, Change{Scope: ScopeOutput, Change: Removed, Kind: KindRoleOutput, ID: role, Old: digest,
			Detail: "the role is no longer pinned or no longer exists"})
	}
}

func (d *Diff) roleChange(snap *Snapshot, change string, cur lockfile.OutputPin, prev string) Change {
	c := Change{Scope: ScopeOutput, Change: change, Kind: KindRoleOutput, ID: cur.Role, Old: prev, New: cur.Digest}
	if change == Added {
		c.Detail = fmt.Sprintf("role %q is not pinned in the lock yet; run `ai-rulez lock`", cur.Role)
	}
	if snap.Options.RoleFiles {
		c.Files = map[string]string{}
		for _, f := range snap.RoleFiles[cur.Role] {
			c.Files[f.Path] = f.Digest
		}
	}
	return c
}

// CompareRoles compares the pins of the named roles in lock with their rendered
// outputs, and nothing else. It is the check behind `generate --locked --role`,
// which must not pay for the full snapshot.
func CompareRoles(lock *lockfile.File, rendered map[string][]Output, only []string) ([]Change, error) {
	snap := &Snapshot{Options: Options{CheckRoles: true, OnlyRoles: only}}
	if err := computeRoleOutputs(snap, rendered, nil); err != nil {
		return nil, err
	}
	d := &Diff{Changes: []Change{}}
	d.compareRoleOutputs(lock, snap)
	SortChanges(d.Changes)
	return d.Changes, nil
}
