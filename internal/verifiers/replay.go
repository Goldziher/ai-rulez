package verifiers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// maxReplayFlagged bounds the commits listed per proposal; the count is exact.
const maxReplayFlagged = 5

// Replay is a candidate verifier tried on the last N merged diffs: each commit
// of the first-parent history is evaluated the way a CI run on that change
// would have (its changed files, against the tree as of that commit). A merged
// change passed review, so a verifier that fails on many of them is noisy.
type Replay struct {
	// Diffs is how many merged diffs were evaluated.
	Diffs int `json:"diffs"`
	// Flagged is how many of them the verifier failed on.
	Flagged int `json:"flagged"`
	// FlaggedCommits lists the first few, as "<short sha> <subject>".
	FlaggedCommits []string `json:"flagged_commits,omitempty"`
	// Errors is how many diffs the verifier could not be evaluated on.
	Errors int `json:"errors,omitempty"`
}

// replayProposals runs every usable proposal over the last n merged diffs and
// sets its Replay. A repository without history, or n <= 0, replays nothing and
// the note says why. Each commit's tree is extracted once for all proposals.
func replayProposals(ctx context.Context, cfg *config.Config, props []*Proposal, n int) (note string) {
	if n <= 0 || len(props) == 0 {
		return ""
	}
	root := cfg.BaseDir
	top := gitutil.TopLevel(root)
	if top == "" {
		return "replay skipped: the project is not in a git repository"
	}
	realTop, terr := filepath.EvalSymlinks(top)
	realRoot, rerr := filepath.EvalSymlinks(root)
	if terr != nil || rerr != nil {
		return "replay skipped: the project directory cannot be resolved"
	}
	rel, err := filepath.Rel(realTop, realRoot)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "replay skipped: the project is not inside its repository"
	}
	commits, err := gitutil.FirstParentCommits(root, n)
	if err != nil {
		return "replay skipped: " + oneLineErr(err)
	}
	for _, p := range props {
		p.Replay = &Replay{}
	}
	skipped := 0
	for _, c := range commits {
		if c.Parent == "" {
			continue // a root commit adds the whole tree: not a change to replay
		}
		if err := ctx.Err(); err != nil {
			return "replay stopped: " + err.Error()
		}
		if err := replayCommit(ctx, cfg, top, filepath.ToSlash(rel), c, props); err != nil {
			skipped++
			note = "replay: " + oneLineErr(err)
		}
	}
	if skipped > 0 {
		note = fmt.Sprintf("%d of %d merged diff(s) could not be replayed (%s)", skipped, len(commits), strings.TrimPrefix(note, "replay: "))
	}
	return note
}

func oneLineErr(err error) string { return collapseSpace(err.Error()) }

// replayCommit evaluates every proposal on the change commit c made.
func replayCommit(ctx context.Context, cfg *config.Config, top, rel string, c gitutil.Commit, props []*Proposal) error {
	changes, err := gitutil.ChangesBetween(cfg.BaseDir, c.Parent, c.SHA)
	if err != nil {
		return oops.Wrapf(err, "diff of %s", shortSHA(c.SHA))
	}
	if len(changes) == 0 {
		return nil
	}
	dest, err := os.MkdirTemp("", "ai-rulez-replay-")
	if err != nil {
		return oops.Wrapf(err, "create a scratch directory")
	}
	defer os.RemoveAll(dest) //nolint:errcheck // throwaway directory
	rootAt := dest
	if rel == "." {
		_, err = govview.ExtractRevisionAll(ctx, top, c.SHA, dest)
	} else {
		_, err = govview.ExtractRevision(ctx, top, c.SHA, rel, dest)
		rootAt = filepath.Join(dest, filepath.FromSlash(rel))
	}
	if err != nil {
		return oops.Wrapf(err, "read the tree of %s", shortSHA(c.SHA))
	}
	env := &Env{Cfg: cfg, Root: rootAt}
	tree, err := env.treeFiles(ctx)
	if err != nil {
		return oops.Wrapf(err, "list the tree of %s", shortSHA(c.SHA))
	}
	env.scope = newScope(ModeSince, c.Parent, changes, tree)
	for _, p := range props {
		res := evaluateSpec(ctx, env, &p.Spec)
		switch res.Status {
		case StatusFail:
			p.Replay.Diffs++
			p.Replay.Flagged++
			if len(p.Replay.FlaggedCommits) < maxReplayFlagged {
				p.Replay.FlaggedCommits = append(p.Replay.FlaggedCommits, shortSHA(c.SHA)+" "+shorten(c.Subject, 60))
			}
		case StatusPass, StatusNotApplicable:
			p.Replay.Diffs++
		default:
			p.Replay.Errors++
		}
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
