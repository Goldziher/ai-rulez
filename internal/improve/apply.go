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
	"strings"

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
	// AllowScripts and AllowFrontmatter widen the diff policy re-checked on
	// apply. The policy is never taken from the report, which a local edit
	// could widen.
	AllowScripts     bool
	AllowFrontmatter bool
}

// ApplyResult says what Apply wrote.
type ApplyResult struct {
	Skill   string   `json:"skill"`
	Written []string `json:"written"`
	Removed []string `json:"removed"`
}

// LoadReport reads and validates the report of a saved run.
func LoadReport(configDir, runID string) (*Report, string, error) {
	r, dir, _, err := loadReport(configDir, runID)
	return r, dir, err
}

func loadReport(configDir, runID string) (*Report, string, []byte, error) {
	if !ValidRunID(runID) {
		return nil, "", nil, refuse("", "%q is not a run id (expected imp-<8 hex digits>, optionally followed by -<n>, e.g. imp-bfc748ff or imp-bfc748ff-2)", runID)
	}
	dir := filepath.Join(configDir, LocalDir, runID)
	data, err := safefs.ReadRegular(filepath.Join(dir, "report.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil, refuse("", "no saved run %s under %s", runID, filepath.Join(configDir, LocalDir))
		}
		return nil, "", nil, fmt.Errorf("read report: %w", err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, "", nil, refuse("", "report.json of %s is not valid: %v", runID, err)
	}
	if r.Schema != ReportSchema || r.RunID != runID {
		return nil, "", nil, refuse("", "report.json of %s has schema %q and run id %q: not a report of this run", runID, r.Schema, r.RunID)
	}
	return &r, dir, data, nil
}

// Apply writes the accepted candidate of a saved run into the authored skill.
// It refuses when the skill changed since the run (AR9J1), re-checks the diff
// policy against the live skill, and never commits.
func Apply(_ context.Context, opts *ApplyOptions) (*ApplyResult, error) {
	report, dir, raw, err := loadReport(opts.ConfigDir, opts.RunID)
	if err != nil {
		return nil, err
	}
	if !verifyReport(dir, opts.RunID, raw) {
		return nil, refuse("", "run %s is not signed by this machine's user key (missing, edited or foreign report): refusing to apply it; run `ai-rulez improve run` again", opts.RunID)
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
	counter := opts.Counter
	if counter == nil {
		if counter, err = tokens.New(""); err != nil {
			return nil, fmt.Errorf("token counter: %w", err)
		}
	}
	skillMD := orig.Files[skillFile]
	constraints := ConstraintsFor(counter.Count(string(skillMD.Data)), report.Gate.MaxSkillGrowth, opts.AllowFrontmatter, opts.AllowScripts)
	if vs, _ := CheckDiff(&PolicyInput{
		Original: orig, Candidate: cand, Constraints: constraints, AllowScripts: opts.AllowScripts, Counter: counter,
	}); len(vs) > 0 {
		return nil, refuse(CodePolicyViolation, "the candidate of run %s breaks the diff policy now: %s", opts.RunID, vs[0])
	}
	patch := UnifiedDiff(report.SkillPath, orig, cand)
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "Run %s: %s, round %d accepted\n", report.RunID, Sanitize(report.Skill, 120), report.AcceptedRound)
	if round := acceptedRound(report); round != nil && round.Held != nil {
		fmt.Fprintf(out, "Held-out pass rate %.0f%% -> %.0f%% (%+.1f points), %d win(s), %d loss(es)\n",
			round.Held.Base.PassRate*100, round.Held.Cand.PassRate*100, round.Held.Gain*100, len(round.Held.Wins), len(round.Held.Losses))
		if n := heldEvaluations(report); n > 1 {
			fmt.Fprintf(out, "Selected among %d held-out evaluations: the gain is optimistic.\n", n)
		}
		if round.Held.Underpowered {
			fmt.Fprintln(out, "Underpowered: few held-out cases, treat the gain as weak evidence.")
		}
	}
	fmt.Fprintf(out, "\n%s\n", SanitizeMultiline(patch)) // the candidate is untrusted text: no terminal escapes
	if !opts.Yes && (opts.Confirm == nil || !opts.Confirm(fmt.Sprintf("Write this change into %s?", Sanitize(report.SkillPath, 200)))) {
		return nil, refuse("", "not confirmed: nothing was written (use --yes to skip the prompt)")
	}
	res := &ApplyResult{Skill: report.Skill}
	written, removed, err := writeTree(live.Dir, orig, cand)
	res.Written, res.Removed = written, removed
	if err != nil {
		return res, err
	}
	fmt.Fprintf(out, "Wrote %d file(s), removed %d. Nothing was committed. Next:\n  ai-rulez lock\n  ai-rulez eval run %s\n  ai-rulez validate --strict\n", len(res.Written), len(res.Removed), Sanitize(report.Skill, 120))
	return res, nil
}

// heldEvaluations counts the rounds that scored a candidate on the held-out set: best-of-N selection over
// them makes the accepted gain optimistic.
func heldEvaluations(r *Report) int {
	n := 0
	for i := range r.Rounds {
		if r.Rounds[i].Held != nil {
			n++
		}
	}
	return n
}

func acceptedRound(r *Report) *RoundReport {
	for i := range r.Rounds {
		if r.Rounds[i].Round == r.AcceptedRound {
			return &r.Rounds[i]
		}
	}
	return nil
}

// writeTree makes dir hold cand, which differs from orig (what dir holds now).
// It is all or nothing: on the first failure every file already changed is put
// back from orig, so a failed apply leaves the skill as it was.
func writeTree(dir string, orig, cand *Tree) (written, removed []string, err error) {
	var undo []func() error
	rollback := func(cause error) error {
		for i := len(undo) - 1; i >= 0; i-- {
			if uerr := undo[i](); uerr != nil {
				cause = fmt.Errorf("%w (rollback also failed: %v)", cause, uerr) //nolint:errorlint // the cause stays the wrapped error
			}
		}
		return cause
	}
	for _, rel := range cand.Paths() {
		before, existed := orig.Files[rel]
		now := cand.Files[rel]
		if existed && bytes.Equal(before.Data, now.Data) && before.Exec == now.Exec {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if existed {
			undo = append(undo, func() error { return writeEntry(path, before) })
		} else {
			undo = append(undo, func() error { return removeAndEmptyParents(dir, path) })
		}
		if err := writeEntry(path, now); err != nil {
			return nil, nil, rollback(err)
		}
		written = append(written, rel)
	}
	for _, rel := range orig.Paths() {
		if _, kept := cand.Files[rel]; kept {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		before := orig.Files[rel]
		if err := os.Remove(path); err != nil {
			return nil, nil, rollback(fmt.Errorf("remove %s: %w", rel, err))
		}
		undo = append(undo, func() error {
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return fmt.Errorf("restore %s: %w", rel, err)
			}
			return writeEntry(path, before)
		})
		removed = append(removed, rel)
	}
	return written, removed, nil
}

// removeAndEmptyParents deletes a file the failed apply created and the
// directories it created above it, never dir itself.
func removeAndEmptyParents(dir, path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	for p := filepath.Dir(path); p != dir && strings.HasPrefix(p, dir); p = filepath.Dir(p) {
		if os.Remove(p) != nil { // not empty (or not ours): stop
			break
		}
	}
	return nil
}
