package improve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// prWorktreeDir is where `improve pr` makes its linked worktree, inside the run directory.
const prWorktreeDir = "pr-worktree"

// CleanOptions configure Clean.
type CleanOptions struct {
	ConfigDir string
	// RunID selects one run; All selects every run. Exactly one must be set.
	RunID string
	All   bool
	// DryRun lists what would be removed and removes nothing.
	DryRun bool
	// RepoDir and Git let Clean unregister the linked worktree `improve pr` made, so git is left
	// with no stale worktree entry. A zero Git runs real git.
	RepoDir string
	Git     gitutil.Git
}

// CleanResult lists the runs Clean removed (or would remove).
type CleanResult struct {
	Removed []string `json:"removed"`
	DryRun  bool     `json:"dry_run,omitempty"`
}

// Clean deletes saved runs under .ai-rulez/local/improve. It touches nothing outside that
// directory: a run id is validated, and a run directory that is a symlink is unlinked, never followed.
func Clean(ctx context.Context, opts *CleanOptions) (*CleanResult, error) {
	switch {
	case opts.All == (opts.RunID != ""):
		return nil, refuse("", "name a run id or pass --all")
	case opts.RunID != "" && !ValidRunID(opts.RunID):
		return nil, refuse("", "%q is not a run id (expected imp-<8 hex digits>, optionally followed by -<n>)", opts.RunID)
	}
	base := filepath.Join(opts.ConfigDir, LocalDir)
	if err := requireRealDirs(opts.ConfigDir, base); err != nil {
		return nil, err
	}
	ids := []string{opts.RunID}
	if opts.All {
		entries, err := os.ReadDir(base)
		if err != nil {
			if os.IsNotExist(err) {
				return &CleanResult{Removed: []string{}, DryRun: opts.DryRun}, nil
			}
			return nil, fmt.Errorf("read %s: %w", base, err)
		}
		ids = ids[:0]
		for _, e := range entries {
			if ValidRunID(e.Name()) {
				ids = append(ids, e.Name())
			}
		}
		sort.Strings(ids)
	}
	res := &CleanResult{Removed: []string{}, DryRun: opts.DryRun}
	for _, id := range ids {
		dir := filepath.Join(base, id)
		if _, err := os.Lstat(dir); err != nil {
			if os.IsNotExist(err) && !opts.All {
				return nil, refuse("", "no saved run %s under %s", id, base)
			}
			continue
		}
		if !opts.DryRun {
			if err := removeRun(ctx, opts, dir); err != nil {
				return res, err
			}
		}
		res.Removed = append(res.Removed, id)
	}
	return res, nil
}

// requireRealDirs refuses to clean through a symlinked local/ or local/improve: a link there would
// make the deletion reach somewhere else.
func requireRealDirs(configDir, base string) error {
	for _, d := range []string{filepath.Join(configDir, "local"), base} {
		info, err := os.Lstat(d)
		switch {
		case os.IsNotExist(err):
			return nil
		case err != nil:
			return fmt.Errorf("check %s: %w", d, err)
		case !info.IsDir():
			return refuse("", "%s is not a plain directory: improve clean will not delete through it", d)
		}
	}
	return nil
}

func removeRun(ctx context.Context, opts *CleanOptions, dir string) error {
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("remove link %s: %w", dir, err)
		}
		return nil
	}
	if wt := filepath.Join(dir, prWorktreeDir); opts.RepoDir != "" {
		if _, err := os.Lstat(wt); err == nil {
			// Best effort: RemoveAll below deletes the directory and prune drops a stale entry.
			_ = opts.Git.WorktreeRemove(ctx, opts.RepoDir, wt) //nolint:errcheck // see above
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	if opts.RepoDir != "" {
		_ = opts.Git.WorktreePrune(ctx, opts.RepoDir) //nolint:errcheck // best effort: a stale worktree entry is harmless
	}
	return nil
}
