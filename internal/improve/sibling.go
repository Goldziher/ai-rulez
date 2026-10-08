package improve

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// SiblingSurface is the activation surface the sibling guard uses: the offline
// find_skill ranker. It calls no model, so the guard costs nothing and is the
// same on every machine.
const SiblingSurface = evals.SurfaceRetrieval

// Bounds on what the guard copies into its scratch tree.
const (
	siblingMaxFiles     = 5000
	siblingMaxFileBytes = 1 << 20
)

// SiblingResult is one sibling skill measured with the baseline and with the candidate.
type SiblingResult struct {
	Skill      string   `json:"skill"`
	Positives  int      `json:"positives"`
	BaseRecall *float64 `json:"baseline_recall"`
	CandRecall *float64 `json:"candidate_recall"`
	// Stolen lists the sibling's positive prompts the baseline routed to it and the candidate does not.
	Stolen    []string `json:"stolen,omitempty"`
	Regressed bool     `json:"regressed"`
}

// SiblingReport is the outcome of the sibling trigger guard for one round.
type SiblingReport struct {
	Surface string `json:"surface"`
	// Runs and CostUSD describe a native measurement: how often each trigger prompt repeated and what the
	// guard spent (the retrieval surface is free).
	Runs    int     `json:"runs,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
	// Skipped says why the guard measured nothing (the candidate did not change what the
	// ranker reads, or no sibling has trigger cases).
	Skipped string          `json:"skipped,omitempty"`
	Results []SiblingResult `json:"results,omitempty"`
	// Unmeasured lists the skills the guard left out because they cannot be copied safely (a
	// symlinked or oversized SKILL.md, too many eval files). The guard measures the others and
	// says so, instead of failing every round of a project with one such skill.
	Unmeasured []SiblingSkip `json:"unmeasured,omitempty"`
}

// SiblingSkip is one skill the sibling guard could not measure and why.
type SiblingSkip struct {
	Skill  string `json:"skill"`
	Reason string `json:"reason"`
}

// Regressions lists the siblings whose trigger recall dropped.
func (r *SiblingReport) Regressions() []SiblingResult {
	var out []SiblingResult
	if r == nil {
		return nil
	}
	for _, s := range r.Results {
		if s.Regressed {
			out = append(out, s)
		}
	}
	return out
}

// Reasons renders the regressions as gate reasons (AR9J4).
func (r *SiblingReport) Reasons() []string {
	var out []string
	for _, s := range r.Regressions() {
		out = append(out, fmt.Sprintf("regression: %s sibling %s lost trigger recall %.0f%% -> %.0f%% (stolen prompt(s): %s)",
			CodeSiblingRegression, Sanitize(s.Skill, 80), pct(s.BaseRecall), pct(s.CandRecall), Sanitize(strings.Join(s.Stolen, ", "), 200)))
	}
	return out
}

func pct(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v * 100
}

// rankerFields are the frontmatter keys the find_skill ranker reads.
var rankerFields = []string{frontmatterName, "description", "triggers", "keywords"}

// rankerFieldsChanged reports whether the candidate's SKILL.md differs from the
// original's in anything the ranker searches, so a body-only edit skips the guard.
func rankerFieldsChanged(orig, cand *Tree) bool {
	before, after := orig.Files[skillFile], cand.Files[skillFile]
	oldFM, _, oldOK := splitFrontmatter(before.Data)
	newFM, _, newOK := splitFrontmatter(after.Data)
	if oldOK != newOK {
		return true
	}
	for _, key := range rankerFields {
		if !sameValue(oldFM[key], newFM[key]) {
			return true
		}
	}
	return false
}

// checkSiblings re-runs the trigger cases of every other skill of the project with
// the original and with the candidate in the target's place, and reports each
// sibling whose trigger recall dropped.
func (p *Plan) checkSiblings(ctx context.Context, cand *Tree) (*SiblingReport, error) {
	return p.guardSiblings(ctx, cand, nil, 0)
}

// checkSiblingsNative is the same guard on the native surface: the harness's model decides which
// skill loads, over repeated runs, so it costs money (budget bounds it) and a baseline measured once is reused
// by every round. Without Options.SiblingNative it measures nothing and returns nil.
func (p *Plan) checkSiblingsNative(ctx context.Context, cand *Tree, budget float64) (*SiblingReport, error) {
	if p.Opts.SiblingNative == nil {
		return nil, nil
	}
	return p.guardSiblings(ctx, cand, p.Opts.SiblingNative, budget)
}

// guardSiblings measures the siblings on the retrieval surface (native nil) or the native one.
func (p *Plan) guardSiblings(ctx context.Context, cand *Tree, native *SiblingNative, budget float64) (*SiblingReport, error) {
	rep := &SiblingReport{Surface: SiblingSurface}
	if native != nil {
		rep.Surface = evals.SurfaceNative
		rep.Runs = native.runs()
	}
	if !rankerFieldsChanged(p.orig, cand) {
		rep.Skipped = "the candidate changed nothing the ranker reads (name, description, triggers, keywords)"
		return rep, nil
	}
	all, err := evals.FindSkills(p.Opts.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	siblings := p.measurableSiblings(all, rep)
	if len(siblings) == 0 {
		rep.Skipped = "no other skill has trigger cases"
		return rep, nil
	}
	scratch, err := os.MkdirTemp("", "ai-rulez-improve-siblings-")
	if err != nil {
		return nil, fmt.Errorf("create the sibling scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch) //nolint:errcheck // a temp directory
	base := p.nativeBase
	if native == nil || base == nil {
		var cost float64
		base, cost, err = p.siblingActivation(ctx, filepath.Join(scratch, "base"), all, p.orig, siblings, native, budget)
		rep.CostUSD += cost
		if err != nil {
			return rep, err
		}
		if native != nil {
			p.nativeBase = base
			budget -= cost
		}
	}
	with, cost, err := p.siblingActivation(ctx, filepath.Join(scratch, "cand"), all, cand, siblings, native, budget)
	rep.CostUSD = roundUSD(rep.CostUSD + cost)
	if err != nil {
		return rep, err
	}
	rep.Results = compareSiblings(base, with)
	if len(rep.Results) == 0 {
		rep.Skipped = "no sibling has a positive trigger case"
	}
	return rep, nil
}

// measurableSiblings lists the other skills that have trigger cases and can be copied into the guard's scratch
// tree; the ones that cannot are recorded in rep.Unmeasured.
func (p *Plan) measurableSiblings(all []evals.Skill, rep *SiblingReport) []string {
	var siblings []string
	for i := range all {
		if all[i].ID == p.Skill.ID {
			continue
		}
		if why := siblingProblem(&all[i]); why != "" {
			rep.Unmeasured = append(rep.Unmeasured, SiblingSkip{Skill: all[i].ID, Reason: why})
			continue
		}
		if len(all[i].EvalDirs) > 0 {
			siblings = append(siblings, all[i].ID)
		}
	}
	return siblings
}

// siblingActivation builds a scratch config directory holding the siblings (SKILL.md and
// eval cases) and the target skill (SKILL.md of tree only, never its eval cases), and
// runs the retrieval activation on the siblings.
func (p *Plan) siblingActivation(ctx context.Context, dir string, all []evals.Skill, tree *Tree, siblings []string, native *SiblingNative, budget float64) (skills map[string]evals.ActivationSkill, costUSD float64, err error) {
	configDir := p.Opts.ConfigDir
	for i := range all {
		s := &all[i]
		dst := filepath.Join(dir, "skills", s.ID)
		if s.Domain != "" {
			dst = filepath.Join(dir, "domains", s.Domain, "skills", s.ID)
		}
		if s.ID == p.Skill.ID {
			if err := writeSiblingFile(filepath.Join(dst, skillFile), tree.Files[skillFile].Data); err != nil {
				return nil, 0, err
			}
			continue
		}
		if siblingProblem(s) != "" {
			continue // reported by checkSiblings; the same skills are left out of both arms
		}
		if err := copySibling(s, dir, dst, configDir); err != nil {
			return nil, 0, err
		}
	}
	report, err := p.runSiblingActivation(ctx, dir, siblings, native, budget)
	if err != nil {
		return nil, 0, fmt.Errorf("sibling activation: %w", err)
	}
	out := map[string]evals.ActivationSkill{}
	for i := range report.Skills {
		if report.Skills[i].Status == evals.RunRan {
			out[report.Skills[i].ID] = report.Skills[i]
		}
	}
	return out, report.Cost.ActualUSD, nil
}

// copySibling copies a sibling's SKILL.md to dst and its eval cases below dir (next to the skill, or under the
// project evals directory when that is where they live).
func copySibling(s *evals.Skill, dir, dst, configDir string) error {
	if err := copyRegular(filepath.Join(s.Dir, skillFile), filepath.Join(dst, skillFile)); err != nil {
		return fmt.Errorf("copy %s: %w", s.ID, err)
	}
	for _, ev := range s.EvalDirs {
		target := filepath.Join(dst, "evals")
		if rel, err := filepath.Rel(s.Dir, ev); err == nil && !safefs.RelEscapes(rel) {
			target = filepath.Join(dst, rel)
		} else if rel, err := filepath.Rel(filepath.Join(configDir, evals.ProjectEvalsDir), ev); err == nil && !safefs.RelEscapes(rel) {
			target = filepath.Join(dir, evals.ProjectEvalsDir, rel)
		}
		if err := copyRegularTree(ev, target); err != nil {
			return fmt.Errorf("copy the eval cases of %s: %w", s.ID, err)
		}
	}
	return nil
}

// runSiblingActivation measures the siblings on the offline ranker, or, with native set, on the
// harness's model, bounded by budget.
func (p *Plan) runSiblingActivation(ctx context.Context, dir string, siblings []string, native *SiblingNative, budget float64) (*evals.ActivationReport, error) {
	opts := &evals.ActivationOptions{ConfigDir: dir, Skills: siblings, Scope: evals.ScopeDomain}
	if native == nil {
		return evals.RunActivationRetrieval(ctx, opts)
	}
	if budget <= 0 {
		return nil, errors.New("no budget is left for the native sibling guard")
	}
	o := &p.Opts
	opts.Surface, opts.Runner, opts.Harness, opts.Model, opts.Runs = evals.SurfaceNative, native.Runner, o.Harness, o.Model, native.runs()
	opts.Date, opts.ToolVersion, opts.Price, opts.Counter, opts.MaxCostUSD = o.Date, o.ToolVersion, o.Price, o.Counter, budget
	return evals.RunActivation(ctx, opts) //nolint:wrapcheck // the caller names the guard
}

// compareSiblings pairs the two measurements. A sibling that stopped being measurable
// with the candidate counts as regressed.
func compareSiblings(base, cand map[string]evals.ActivationSkill) []SiblingResult {
	ids := make([]string, 0, len(base))
	for id := range base {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []SiblingResult
	for _, id := range ids {
		b := base[id]
		if b.Recall == nil {
			continue // no positive prompt: nothing to steal
		}
		res := SiblingResult{Skill: id, Positives: b.Recall.N, BaseRecall: &b.Recall.Value}
		c, ok := cand[id]
		if !ok || c.Recall == nil {
			res.Regressed = true
			out = append(out, res)
			continue
		}
		res.CandRecall = &c.Recall.Value
		res.Stolen = lostPrompts(&b, &c)
		res.Regressed = c.Recall.Value+epsilon < b.Recall.Value
		out = append(out, res)
	}
	return out
}

// lostPrompts lists the positive prompts the baseline won for the sibling and the candidate does not.
func lostPrompts(base, cand *evals.ActivationSkill) []string {
	won := map[string]bool{}
	for i := range cand.Prompts {
		if p := &cand.Prompts[i]; p.Expect && p.Winner == cand.ID {
			won[p.Case] = true
		}
	}
	var lost []string
	for i := range base.Prompts {
		if p := &base.Prompts[i]; p.Expect && p.Winner == base.ID && !won[p.Case] {
			lost = append(lost, p.Case)
		}
	}
	return lost
}

// siblingProblem says why a skill cannot be copied into the guard's scratch tree: its SKILL.md is not a
// regular file within the size bound, or its eval directories hold too many files or cannot be walked.
func siblingProblem(s *evals.Skill) string {
	info, err := os.Lstat(filepath.Join(s.Dir, skillFile))
	switch {
	case err != nil:
		return "SKILL.md is unreadable: " + Sanitize(err.Error(), 120)
	case !info.Mode().IsRegular():
		return "SKILL.md is not a regular file (a symlink?)"
	case info.Size() > siblingMaxFileBytes:
		return fmt.Sprintf("SKILL.md is larger than %d bytes", siblingMaxFileBytes)
	}
	files := 0
	for _, ev := range s.EvalDirs {
		err := filepath.WalkDir(ev, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr //nolint:wrapcheck // reported as the reason
			}
			if d.Type().IsRegular() {
				files++
				if files > siblingMaxFiles {
					return fmt.Errorf("more than %d eval files", siblingMaxFiles)
				}
			}
			return nil
		})
		if err != nil {
			return "its eval cases cannot be copied: " + Sanitize(err.Error(), 120)
		}
	}
	return ""
}

func writeSiblingFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// copyRegular copies one regular file (never a symlink, bounded in size).
func copyRegular(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err //nolint:wrapcheck // the caller names the skill
	}
	if !info.Mode().IsRegular() || info.Size() > siblingMaxFileBytes {
		return fmt.Errorf("%s is not a regular file of at most %d bytes", src, siblingMaxFileBytes)
	}
	data, err := os.ReadFile(src) //nolint:gosec // a file of the user's own project, size-checked above
	if err != nil {
		return err //nolint:wrapcheck // the caller names the skill
	}
	return writeSiblingFile(dst, data)
}

// copyRegularTree copies the regular files below src, skipping hidden entries, results
// and node_modules (as case discovery does), symlinks, and anything past the bounds.
func copyRegularTree(src, dst string) error {
	files := 0
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err //nolint:wrapcheck // named by the caller
		}
		name := d.Name()
		if path != src && (strings.HasPrefix(name, ".") || (d.IsDir() && (name == "results" || name == "node_modules"))) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		files++
		if files > siblingMaxFiles {
			return fmt.Errorf("more than %d files below %s", siblingMaxFiles, src)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err //nolint:wrapcheck // named by the caller
		}
		if info, ierr := d.Info(); ierr != nil || info.Size() > siblingMaxFileBytes {
			return nil //nolint:nilerr // an unreadable or oversized file is left out of the copy, not an error
		}
		return copyRegular(path, filepath.Join(dst, rel))
	})
}
