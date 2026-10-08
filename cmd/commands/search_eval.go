package commands

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch/fromevals"
	"github.com/samber/oops"
)

// evalModes parses --mode of an evaluation: a comma-separated list of lexical,
// vector and hybrid, or all. The first is the primary mode the gates check.
func evalModes(spec, configured string) ([]string, error) {
	if strings.TrimSpace(spec) == "" {
		return []string{configured}, nil
	}
	if spec == "all" {
		return []string{skillsearch.ModeLexical, skillsearch.ModeVector, skillsearch.ModeHybrid}, nil
	}
	var modes []string
	for _, m := range strings.Split(spec, ",") {
		m = strings.TrimSpace(m)
		if err := checkQueryMode(m); err != nil {
			return nil, oops.Errorf("--mode %q: each mode must be lexical, hybrid or vector, or use all", spec)
		}
		if !slices.Contains(modes, m) {
			modes = append(modes, m)
		}
	}
	return modes, nil
}

// loadEvalCases reads the cases file (when given) and the cases derived from
// the skills' eval-runner cases (when asked), dropping derived cases whose skill
// is not in the served catalog.
func loadEvalCases(env *searchEnv) (*skillsearch.CaseFile, string, error) {
	cases := &skillsearch.CaseFile{Version: 1}
	if searchFlags.eval != "" {
		loaded, err := skillsearch.LoadCases(searchFlags.eval)
		if err != nil {
			return nil, "", err //nolint:wrapcheck // already contextual
		}
		cases = loaded
	}
	note := ""
	if searchFlags.fromEvals {
		derived, err := fromevals.Derive(env.cfg.ConfigDir)
		if err != nil {
			return nil, "", oops.Wrapf(err, "derive cases from the eval-runner cases")
		}
		known := map[string]bool{}
		for i := range env.items {
			known[env.items[i].ID] = true
		}
		have := map[string]bool{}
		for i := range cases.Cases {
			have[cases.Cases[i].ID] = true
		}
		skipped := 0
		for _, c := range derived.Cases {
			ids := append(c.ExpectIDs(), c.Avoid...)
			if have[c.ID] || slices.ContainsFunc(ids, func(id string) bool { return !known[id] }) {
				skipped++
				continue
			}
			cases.Cases = append(cases.Cases, c)
		}
		note = fmt.Sprintf("from-evals: %d cases derived, %d skipped (skill not served or id taken)", len(derived.Cases)-skipped, skipped)
		if len(derived.Problems) > 0 {
			note += fmt.Sprintf("; %d eval-file problems ignored (run 'ai-rulez evals validate')", len(derived.Problems))
		}
		if len(cases.Cases) == 0 {
			return nil, "", oops.Hint("Add eval cases under skills/<name>/evals/*.eval.yaml, or pass --eval").Errorf("no eval-runner case maps to a served skill")
		}
	}
	return cases, note, nil
}

func runSearchEval(ctx context.Context, out, errOut io.Writer, env *searchEnv) int {
	res, err := measureSearchEval(ctx, errOut, env)
	if err != nil {
		renderStderr(err)
		return 1
	}
	return reportSearchEval(out, errOut, res)
}

// measureSearchEval loads the cases, ranks them in every requested mode and applies the gates.
func measureSearchEval(ctx context.Context, errOut io.Writer, env *searchEnv) (*skillsearch.Result, error) {
	if err := skillsearch.CheckK(searchFlags.k); err != nil {
		return nil, err
	}
	mins, err := skillsearch.ParseMinimums(searchFlags.min)
	if err != nil {
		return nil, err
	}
	modes, err := evalModes(searchFlags.mode, env.resolved.Search.Mode)
	if err != nil {
		return nil, err
	}
	cases, note, err := loadEvalCases(env)
	if err != nil {
		return nil, err
	}
	if note != "" {
		reportWriter{errOut}.printf("%s\n", note)
	}
	if err := cases.CheckSkills(skillsearch.IDs(env.items)); err != nil {
		return nil, err
	}
	wantVectors := slices.ContainsFunc(modes, func(m string) bool { return m != skillsearch.ModeLexical })
	ranker, release, err := env.ranker(wantVectors)
	defer release()
	if err != nil {
		return nil, err
	}
	if wantVectors && ranker.Index == nil {
		return nil, oops.Hint("Run 'ai-rulez search index' first").Errorf("%s evaluation needs an embedding index, and none is usable", strings.Join(modes, "/"))
	}
	evalEnv := &skillsearch.EvalEnv{Items: env.items, Cfg: env.resolved.Search, Index: ranker.Index, Embedder: ranker.Embedder, Scope: roleScope(env)}
	res, err := skillsearch.Eval(ctx, evalEnv, cases, cases.EffectiveK(searchFlags.k), modes)
	if err != nil {
		return nil, err
	}
	maxFlips := -1
	if searchFlags.baseline != "" {
		base, err := skillsearch.LoadBaseline(searchFlags.baseline)
		if err != nil {
			return nil, err
		}
		if err := res.CompareBaseline(base); err != nil {
			return nil, err
		}
		maxFlips = searchFlags.maxFlips
	}
	res.GateFailures = res.Gate(mins, maxFlips)
	return res, nil
}

// reportSearchEval prints the result and returns the exit code.
func reportSearchEval(out, errOut io.Writer, res *skillsearch.Result) int {
	fail := func(err error) int {
		renderStderr(err)
		return 1
	}
	degraded := degradedEvalMessage(res)
	if searchFlags.out != "" && degraded == "" {
		// A run that fell back to lexical does not measure the requested ranking: never a baseline.
		if err := writeJSONFile(searchFlags.out, res); err != nil {
			return fail(err)
		}
	}
	if searchFlags.format == formatJSON {
		if err := writeJSON(out, res); err != nil {
			return fail(err)
		}
	} else {
		printEvalText(out, res)
	}
	if degraded != "" {
		// The numbers would not measure the ranker asked for: never a pass.
		reportWriter{errOut}.printf("%s\nCheck 'ai-rulez search status' and the [llm] budget and network settings\n", degraded)
		return 1
	}
	for _, msg := range res.GateFailures {
		reportWriter{errOut}.printf("%s\n", msg)
	}
	if len(res.GateFailures) > 0 {
		return exitSearchGateFailed
	}
	return 0
}

// degradedEvalMessage says when a vector mode fell back to lexical for a case.
func degradedEvalMessage(r *skillsearch.Result) string {
	var parts []string
	for _, mode := range slices.Sorted(maps.Keys(r.ByMode)) {
		mr := r.ByMode[mode]
		if mr.DegradedCases == 0 {
			continue
		}
		reason := ""
		for i := range mr.Cases {
			if reason = mr.Cases[i].Degraded; reason != "" {
				break
			}
		}
		for i := 0; reason == "" && i < len(mr.Negatives); i++ {
			reason = mr.Negatives[i].Degraded
		}
		parts = append(parts, fmt.Sprintf("%s: %d cases fell back to lexical (%s)", mode, mr.DegradedCases, reason))
	}
	if len(parts) == 0 {
		return ""
	}
	return "the embeddings were unavailable, so the result does not measure the requested ranking: " + strings.Join(parts, "; ")
}

// roleScope resolves a case's role against the project's [[roles]]: the filter
// keeps the catalog skills the role serves, as `search --role` does.
func roleScope(env *searchEnv) func(role string) (func(int) bool, error) {
	return func(role string) (func(int) bool, error) {
		if role == "" {
			return nil, nil //nolint:nilnil // no scope: everything is admitted
		}
		scope, ok := env.srv.RoleScope(role)
		if !ok {
			return nil, oops.Errorf("unknown role %q", role)
		}
		return func(i int) bool { return scope.Includes(env.cat.SkillAt(i)) }, nil
	}
}

func printEvalText(out io.Writer, r *skillsearch.Result) {
	p := reportWriter{out}
	modes := evalModeOrder(r)
	p.printf("cases: %d (+%d negative), k=%d, mode: %s\n", r.N, r.NNegative, r.K, strings.Join(modes, ", "))
	if r.Index != nil {
		p.printf("index: %s, %d dims, %s, %d rows (%d stale, %d missing)\n", r.Index.Model, r.Index.Dims, r.Index.DType, r.Index.Rows, r.Index.Stale, r.Index.Missing)
	}
	if len(modes) > 1 {
		printModeTable(p, r, modes)
	} else {
		printPrimaryMetrics(p, r)
	}
	if len(r.Tags) > 0 {
		printTags(p, r, modes)
	}
	if len(r.Negatives) > 0 {
		printNegatives(p, r, modes)
	}
	if c := r.Calibration; c != nil {
		p.printf("\ncalibration (%s): vector_min_sim = %.3f answers %d of %d positive cases and abstains on %d of %d negative cases\n",
			c.Mode, c.MinSim, c.PositivesKept, c.Positives, c.NegativesAbstained, c.Negatives)
	}
	if len(r.Misses) > 0 {
		p.printf("%s\n", "\nmisses:")
		for i := range r.Misses {
			c := &r.Misses[i]
			rank := "not retrieved"
			if c.Rank != nil {
				rank = fmt.Sprintf("rank %d", *c.Rank)
			}
			p.printf("  %s: expected %s, got [%s] (%s)\n", c.ID, strings.Join(c.Expected, ","), strings.Join(c.Got, ","), rank)
		}
	}
	if r.Flips != nil {
		p.printf("\nvs baseline: %d regressed, %d fixed\n", len(r.Flips.Regressed), len(r.Flips.Fixed))
		for _, id := range r.Flips.Regressed {
			p.printf("  regressed: %s\n", id)
		}
	}
	if len(r.GateFailures) > 0 {
		p.printf("%s\n", "\ngate failed")
	}
}

// evalModeOrder lists the evaluated modes, the primary first.
func evalModeOrder(r *skillsearch.Result) []string {
	modes := []string{r.Mode}
	for _, m := range []string{skillsearch.ModeLexical, skillsearch.ModeVector, skillsearch.ModeHybrid} {
		if _, ok := r.Modes[m]; ok && m != r.Mode {
			modes = append(modes, m)
		}
	}
	if r.Mode == "" {
		modes = modes[1:]
	}
	return modes
}

func printPrimaryMetrics(p reportWriter, r *skillsearch.Result) {
	m := r.Modes[r.Mode]
	width := len(fmt.Sprintf("recall@%d", r.K)) + 2
	row := func(label string, v float64, ci *skillsearch.Interval) {
		p.printf("%-*s%.4f", width, label, v)
		if ci != nil {
			p.printf("  (95%% CI %.4f-%.4f)", ci.Low, ci.High)
		}
		p.printf("\n")
	}
	ci := func(key string) *skillsearch.Interval {
		v, ok := r.CI95[key]
		if !ok {
			return nil
		}
		return &v
	}
	row("top1", m.Top1, ci("top1"))
	row(fmt.Sprintf("recall@%d", r.K), m.RecallAt, ci("recall_at_k"))
	row(fmt.Sprintf("hit@%d", r.K), m.HitAt, nil)
	row("mrr", m.MRR, ci("mrr"))
	if m.NDCG != nil {
		row(fmt.Sprintf("ndcg@%d", r.K), *m.NDCG, ci("ndcg_at_k"))
	}
	if m.AvoidTop != nil {
		row("avoid@1", *m.AvoidTop, nil)
	}
	if m.AbstainRate != nil {
		row("abstain", *m.AbstainRate, nil)
	}
}

// searchModeMetric is one row of the per-mode table: key names its interval in CI95.
type searchModeMetric struct {
	label, key string
	get        func(skillsearch.Metrics) (float64, bool)
}

// searchModeMetrics lists the rows of the per-mode table.
func searchModeMetrics(k int) []searchModeMetric {
	return []searchModeMetric{
		{"top1", "top1", func(m skillsearch.Metrics) (float64, bool) { return m.Top1, true }},
		{fmt.Sprintf("recall@%d", k), "recall_at_k", func(m skillsearch.Metrics) (float64, bool) { return m.RecallAt, true }},
		{fmt.Sprintf("hit@%d", k), "", func(m skillsearch.Metrics) (float64, bool) { return m.HitAt, true }},
		{"mrr", "mrr", func(m skillsearch.Metrics) (float64, bool) { return m.MRR, true }},
		{fmt.Sprintf("ndcg@%d", k), "ndcg_at_k", func(m skillsearch.Metrics) (float64, bool) {
			if m.NDCG == nil {
				return 0, false
			}
			return *m.NDCG, true
		}},
		{"avoid@1", "", func(m skillsearch.Metrics) (float64, bool) {
			if m.AvoidTop == nil {
				return 0, false
			}
			return *m.AvoidTop, true
		}},
		{"abstain", "", func(m skillsearch.Metrics) (float64, bool) {
			if m.AbstainRate == nil {
				return 0, false
			}
			return *m.AbstainRate, true
		}},
	}
}

// printMetricRows prints each metric that at least one mode reports.
func printMetricRows(p reportWriter, r *skillsearch.Result, modes []string) {
	for _, mt := range searchModeMetrics(r.K) {
		shown := false
		for _, mode := range modes {
			_, ok := mt.get(r.Modes[mode])
			shown = shown || ok
		}
		if !shown {
			continue
		}
		p.printf("%-14s", mt.label)
		for _, mode := range modes {
			v, ok := mt.get(r.Modes[mode])
			if !ok {
				p.printf("%-24s", "-")
				continue
			}
			cell := fmt.Sprintf("%.4f", v)
			if ci, ok := r.ByMode[mode].CI95[mt.key]; ok && mt.key != "" {
				cell += fmt.Sprintf(" (%.2f-%.2f)", ci.Low, ci.High)
			}
			p.printf("%-24s", cell)
		}
		p.printf("\n")
	}
}

// printPairedDifferences prints the paired difference of each mode against lexical.
func printPairedDifferences(p reportWriter, r *skillsearch.Result) {
	if len(r.Paired) > 0 {
		p.printf("%s\n", "\npaired difference vs lexical (95% bootstrap interval; an interval excluding 0 is a real change):")
		for _, mode := range slices.Sorted(maps.Keys(r.Paired)) {
			for _, key := range slices.Sorted(maps.Keys(r.Paired[mode])) {
				d := r.Paired[mode][key]
				p.printf("  %-8s %-12s %+.4f  (%+.4f to %+.4f)\n", mode, key, d.Diff, d.Low, d.High)
			}
		}
	}
}

// printModeTable prints each metric per mode with its interval, then the paired
// difference of each mode against lexical.
func printModeTable(p reportWriter, r *skillsearch.Result, modes []string) {
	p.printf("\n%-14s", "")
	for _, m := range modes {
		p.printf("%-24s", m)
	}
	p.printf("\n")
	printMetricRows(p, r, modes)
	printPairedDifferences(p, r)
	for _, mode := range modes {
		if mr := r.ByMode[mode]; mr != nil && mr.DegradedCases > 0 {
			p.printf("\nwarning: %s: %d cases fell back to lexical\n", mode, mr.DegradedCases)
		}
	}
}

// printTags prints the metrics per tag, for every mode when several were evaluated.
func printTags(p reportWriter, r *skillsearch.Result, modes []string) {
	p.printf("%s\n", "\nby tag:")
	for _, tag := range slices.Sorted(maps.Keys(r.Tags)) {
		for _, mode := range modes {
			mr := r.ByMode[mode]
			if mr == nil {
				continue
			}
			tm, ok := mr.Tags[tag]
			if !ok {
				continue
			}
			label := ""
			if len(modes) > 1 {
				label = fmt.Sprintf(" [%s]", mode)
			}
			p.printf("  %s (%d)%s: top1 %.4f  recall@%d %.4f  hit@%d %.4f  mrr %.4f\n", tag, tm.N, label, tm.Top1, r.K, tm.RecallAt, r.K, tm.HitAt, tm.MRR)
		}
	}
}

// printNegatives lists what ranked first for each negative case, per mode.
func printNegatives(p reportWriter, r *skillsearch.Result, modes []string) {
	p.printf("%s\n", "\nnegatives (nothing should match; shown is what ranked first, with the ranker's own score):")
	for i := range r.Negatives {
		p.printf("  %s:", r.Negatives[i].ID)
		for _, mode := range modes {
			mr := r.ByMode[mode]
			if mr == nil || i >= len(mr.Negatives) {
				continue
			}
			n := &mr.Negatives[i]
			top := "nothing"
			if n.Top != "" {
				top = fmt.Sprintf("%s (%.4f)", n.Top, n.TopScore)
			}
			if len(modes) > 1 {
				top = mode + " " + top
			}
			if n.Violated {
				top += " [avoided skill first]"
			}
			p.printf(" %s;", top)
		}
		p.printf("\n")
	}
}
