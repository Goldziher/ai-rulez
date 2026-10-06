package improve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// runIDPattern is the only shape of run id Apply accepts, so a run id can never
// name a path outside the improve directory.
var runIDPattern = regexp.MustCompile(`^imp-[0-9a-f]{8}(-[0-9]+)?$`)

// ValidRunID reports whether s is a well-formed run id.
func ValidRunID(s string) bool { return runIDPattern.MatchString(s) }

// ApplyOptions configure Apply.
type ApplyOptions struct {
	ConfigDir string
	RunID     string
	// Yes skips the confirmation; otherwise Confirm decides (nil answers no).
	Yes     bool
	Confirm func(question string) bool
	Out     io.Writer
	Counter tokens.Counter
}

// ApplyResult says what Apply wrote.
type ApplyResult struct {
	Skill   string
	Written []string
	Removed []string
}

// LoadReport reads and validates the report of a saved run.
func LoadReport(configDir, runID string) (*Report, string, error) {
	if !ValidRunID(runID) {
		return nil, "", refuse("", "%q is not a run id (expected imp-<8 hex digits>)", runID)
	}
	dir := filepath.Join(configDir, LocalDir, runID)
	data, err := safefs.ReadRegular(filepath.Join(dir, "report.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", refuse("", "no saved run %s under %s", runID, filepath.Join(configDir, LocalDir))
		}
		return nil, "", fmt.Errorf("read report: %w", err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, "", refuse("", "report.json of %s is not valid: %v", runID, err)
	}
	if r.Schema != ReportSchema || r.RunID != runID {
		return nil, "", refuse("", "report.json of %s has schema %q and run id %q: not a report of this run", runID, r.Schema, r.RunID)
	}
	return &r, dir, nil
}

// Apply writes the accepted candidate of a saved run into the authored skill.
// It refuses when the skill changed since the run (AR9J1), re-checks the diff
// policy against the live skill, and never commits.
func Apply(_ context.Context, opts *ApplyOptions) (*ApplyResult, error) {
	report, dir, err := LoadReport(opts.ConfigDir, opts.RunID)
	if err != nil {
		return nil, err
	}
	if !report.Accepted() {
		return nil, refuse("", "run %s has no accepted candidate (%s): nothing to apply", opts.RunID, report.Reason)
	}
	skills, err := evals.FindSkills(opts.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	idx := slices.IndexFunc(skills, func(s evals.Skill) bool { return s.ID == report.Skill })
	if idx < 0 {
		return nil, refuse("", "the skill %q of run %s no longer exists", report.Skill, opts.RunID)
	}
	live := skills[idx]
	if info, err := os.Lstat(live.Dir); err != nil || !info.IsDir() {
		return nil, refuse("", "%s is not a plain directory: improve apply will not write through it", live.Dir)
	}
	if now, err := evals.SkillDigest(live.Dir); err != nil || now != report.OriginalDigest {
		return nil, refuse(CodeRunStale, "%s changed since run %s measured it: run `ai-rulez improve run %s` again", report.Skill, opts.RunID, report.Skill)
	}
	candDir := filepath.Join(dir, "rounds", fmt.Sprint(report.AcceptedRound), "candidate", report.Skill)
	if got, err := evals.SkillDigest(candDir); err != nil || got != report.CandidateDigest {
		return nil, refuse("", "the candidate files of run %s changed after the run (digest mismatch): refusing to apply them", opts.RunID)
	}
	cand, err := ReadTree(candDir)
	if err != nil {
		return nil, err
	}
	orig, err := ReadTree(live.Dir)
	if err != nil {
		return nil, err
	}
	if len(orig.Odd) > 0 {
		return nil, refuse("", "%s contains symlinks, hard links or oversized files: improve apply will not write into it", live.Dir)
	}
	if vs, _ := CheckDiff(&PolicyInput{
		Original: orig, Candidate: cand, Constraints: report.Constraints,
		AllowScripts: slices.Contains(report.Constraints.Editable, "scripts/**"), Counter: opts.Counter,
	}); len(vs) > 0 {
		return nil, refuse(CodePolicyViolation, "the candidate of run %s breaks the diff policy now: %s", opts.RunID, vs[0])
	}
	patch := UnifiedDiff(report.SkillPath, orig, cand)
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "Run %s: %s, round %d accepted\n", report.RunID, report.Skill, report.AcceptedRound)
	if round := acceptedRound(report); round != nil && round.Held != nil {
		fmt.Fprintf(out, "Held-out pass rate %.0f%% -> %.0f%% (%+.1f points), %d win(s), %d loss(es)\n",
			round.Held.Base.PassRate*100, round.Held.Cand.PassRate*100, round.Held.Gain*100, len(round.Held.Wins), len(round.Held.Losses))
		if round.Held.Underpowered {
			fmt.Fprintln(out, "Underpowered: few held-out cases, treat the gain as weak evidence.")
		}
	}
	fmt.Fprintf(out, "\n%s\n", patch)
	if !opts.Yes && (opts.Confirm == nil || !opts.Confirm(fmt.Sprintf("Write this change into %s?", report.SkillPath))) {
		return nil, refuse("", "not confirmed: nothing was written (use --yes to skip the prompt)")
	}
	res := &ApplyResult{Skill: report.Skill}
	for _, rel := range cand.Paths() {
		before, existed := orig.Files[rel]
		now := cand.Files[rel]
		if existed && bytes.Equal(before.Data, now.Data) && before.Exec == now.Exec {
			continue
		}
		if err := writeEntry(filepath.Join(live.Dir, filepath.FromSlash(rel)), now); err != nil {
			return res, err
		}
		res.Written = append(res.Written, rel)
	}
	for _, rel := range orig.Paths() {
		if _, kept := cand.Files[rel]; kept {
			continue
		}
		if err := os.Remove(filepath.Join(live.Dir, filepath.FromSlash(rel))); err != nil {
			return res, fmt.Errorf("remove %s: %w", rel, err)
		}
		res.Removed = append(res.Removed, rel)
	}
	fmt.Fprintf(out, "Wrote %d file(s), removed %d. Nothing was committed. Next:\n  ai-rulez lock\n  ai-rulez eval run %s\n  ai-rulez validate --strict\n", len(res.Written), len(res.Removed), report.Skill)
	return res, nil
}

func acceptedRound(r *Report) *RoundReport {
	for i := range r.Rounds {
		if r.Rounds[i].Round == r.AcceptedRound {
			return &r.Rounds[i]
		}
	}
	return nil
}
