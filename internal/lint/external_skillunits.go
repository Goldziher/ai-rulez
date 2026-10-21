package lint

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// A scanner whose command passes {skill_dirs} and nothing wider ({stage},
// {root} and {files} would show it other skills) judges each skill directory on
// its own. Its result is therefore cached per skill: the key is the tree hash of
// that skill, the command line without the directories, the scanner's identity
// and its environment. A run hands the scanner only the skills that have no
// cached result (and, under --changed or --since, only those with a changed
// file), so one edited skill costs one skill's scan.

// perSkillCommand reports whether a command passes skill directories and
// nothing that shows the scanner more than them.
func perSkillCommand(cmd []string) bool {
	hasDirs := false
	for _, a := range cmd {
		switch {
		case a == phSkillDirs:
			hasDirs = true
		case a == phFiles || strings.Contains(a, phStage) || strings.Contains(a, phRoot):
			return false
		}
	}
	return hasDirs
}

// skillUnit is one skill directory of a staged run.
type skillUnit struct {
	dir   string // stage-relative slash path
	files []stagedFile
	key   string
	cache *ScanCache
	hit   *cachedScan
	// skip is set when --changed leaves the skill out: no file of it changed.
	skip bool
}

// skillUnits groups the staged files by skill directory, in directory order.
// Files outside a skill directory belong to no unit.
func skillUnits(files []stagedFile) []skillUnit {
	var units []skillUnit
	index := map[string]int{}
	for _, f := range files {
		if f.skillOf == "" {
			continue
		}
		i, ok := index[f.skillOf]
		if !ok {
			i = len(units)
			index[f.skillOf] = i
			units = append(units, skillUnit{dir: f.skillOf})
		}
		units[i].files = append(units[i].files, f)
	}
	slices.SortFunc(units, func(a, b skillUnit) int { return strings.Compare(a.dir, b.dir) })
	return units
}

// markUnchanged sets skip on every unit none of whose files changed, when the
// run was narrowed to changed files.
func (r *runner) markUnchanged(units []skillUnit) {
	changed := r.opts.Scanner.ChangedFiles
	if changed == nil || r.opts.Scanner.WriteBaseline {
		return
	}
	set := make(map[string]bool, len(changed))
	for _, c := range changed {
		set[filepath.ToSlash(c)] = true
	}
	for i := range units {
		units[i].skip = !slices.ContainsFunc(units[i].files, func(f stagedFile) bool { return set[r.tree.Rel(f.source)] })
	}
}

// runSkillUnits runs a per-skill scanner: cached skills are read from the
// cache, the others are handed to the scanner in batches and cached each.
func (r *runner) runSkillUnits(sc resolvedScanner, root string, files []stagedFile) (all []scannerFinding, ok bool) {
	units := skillUnits(files)
	binary := lookExecutable(sc.Command[0], root)
	extra := keyFiles(sc, root)
	r.markUnchanged(units)
	var norm []externalFinding
	outOfScope, firstOut := 0, ""
	var misses []int
	served := 0
	for i := range units {
		u := &units[i]
		if u.skip {
			continue
		}
		u.key, u.cache = r.cacheFor(sc, binary, digestStage(u.files), extra)
		if u.key != "" {
			if hit, found := r.cacheGet(sc, u.cache, u.key); found {
				u.hit = &hit
				served++
				norm = append(norm, fromCached(hit.Findings)...)
				outOfScope, firstOut = outOfScope+hit.OutOfScope, firstNonEmpty(firstOut, hit.FirstOut)
				continue
			}
		}
		misses = append(misses, i)
	}
	if r.opts.Scanner.DryRun {
		r.planSkillUnits(sc, root, binary, units, misses)
		return nil, true
	}
	ok = true
	if len(misses) > 0 {
		fresh, freshOut, freshFirst, good := r.scanSkillUnits(sc, root, binary, units, misses)
		norm = append(norm, fresh...)
		outOfScope, firstOut, ok = outOfScope+freshOut, firstNonEmpty(firstOut, freshFirst), good
	}
	if served > 0 {
		r.log().Debug("Scanner results served from the cache", "scanner", sc.Name, "skills", served)
	}
	return r.convertFindings(sc, stagedScope(files), norm, outOfScope, firstOut), ok
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// unitBatch is the skills handed to the scanner in one run.
type unitBatch struct {
	idx   int
	units []int // indexes into the run's units
}

// splitBatches divides the skills to scan into runs: at most size skills each
// (0: no limit) and no run whose arguments pass argvBudget.
func splitBatches(units []skillUnit, misses []int, size, baseCost, budget, rootLen int) []unitBatch {
	var batches []unitBatch
	cur := unitBatch{}
	cost := baseCost
	for _, mi := range misses {
		add := rootLen + len(units[mi].dir) + 2 // the absolute path and its terminator
		full := (size > 0 && len(cur.units) >= size) || (len(cur.units) > 0 && cost+add > budget)
		if full {
			batches = append(batches, cur)
			cur, cost = unitBatch{idx: len(batches)}, baseCost
		}
		cur.units = append(cur.units, mi)
		cost += add
	}
	if len(cur.units) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// scanSkillUnits stages the skills to scan and runs them in batches, caching the
// result of every skill whose findings can be told apart. It returns the
// findings with stage-relative paths.
func (r *runner) scanSkillUnits(sc resolvedScanner, root, binary string, units []skillUnit, misses []int) (norm []externalFinding, outOfScope int, firstOut string, ok bool) {
	var files []stagedFile
	for _, mi := range misses {
		files = append(files, units[mi].files...)
	}
	st, err := r.buildStageFrom(files)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, "could not stage its inputs: "+sanitizeScannerText(err.Error()))
		return nil, 0, "", false
	}
	defer st.cleanup()
	batches := splitBatches(units, misses, sc.BatchSize, argvSize(st.expandFor(sc.Command, nil, 0)), argvBudget, len(st.root))
	r.banner(sc)
	outcomes := r.runBatches(sc, root, binary, st, units, batches, r.startProgress(sc, len(batches)))
	version, versionKnown := "", false
	ok = true
	for i, out := range outcomes {
		if !out.ran {
			continue
		}
		if !out.ok {
			ok = false
			continue
		}
		norm = append(norm, out.norm...)
		outOfScope, firstOut = outOfScope+out.outOfScope, firstNonEmpty(firstOut, out.firstOut)
		if !versionKnown {
			version, _ = r.probeVersion(sc, binary, root) //nolint:errcheck // a refused probe only leaves the recorded version empty
			versionKnown = true
		}
		r.cacheBatch(sc, units, batches[i], out.norm, out.outOfScope, out.firstOut, version)
	}
	return norm, outOfScope, firstOut, ok
}

// batchOutcome is what one batch produced. ran is false for a batch that was
// never started because an earlier one failed.
type batchOutcome struct {
	norm       []externalFinding
	outOfScope int
	firstOut   string
	ok, ran    bool
}

// preparedBatch is a batch ready to start.
type preparedBatch struct {
	spec    cmdrun.Spec
	usesOut bool
	// dirs are the absolute stage paths of the skills handed to the batch.
	dirs []string
}

// runBatches runs the batches one after the other, or up to the scanner's
// parallel at a time. A failed batch stops the ones not yet started; the
// outcomes are in batch order whatever order the runs finished in.
func (r *runner) runBatches(sc resolvedScanner, root, binary string, st *scannerStage, units []skillUnit, batches []unitBatch, progress *batchProgress) []batchOutcome {
	outcomes := make([]batchOutcome, len(batches))
	workers := min(max(sc.Parallel, 1), maxScannerParallel)
	if workers == 1 || len(batches) == 1 {
		for i, b := range batches {
			outcomes[i] = batchOutcome{ran: true}
			prep, ok := r.prepareUnitBatch(sc, root, binary, st, units, b)
			if !ok {
				break
			}
			outcomes[i] = r.runPrepared(sc, st, prep, b.idx)
			progress.finished()
			if !outcomes[i].ok {
				break
			}
		}
		return outcomes
	}
	// Everything that touches shared state is prepared first, one batch at a time.
	prepared := make([]preparedBatch, len(batches))
	for i, b := range batches {
		prep, ok := r.prepareUnitBatch(sc, root, binary, st, units, b)
		if !ok {
			outcomes[i] = batchOutcome{ran: true}
			return outcomes
		}
		prepared[i] = prep
	}
	var wg sync.WaitGroup
	var failed atomic.Bool
	slots := make(chan struct{}, workers)
	for i := range batches {
		if failed.Load() {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-slots }()
			outcomes[i] = r.runPrepared(sc, st, prepared[i], batches[i].idx)
			progress.finished()
			if !outcomes[i].ok {
				failed.Store(true)
			}
		}(i)
	}
	wg.Wait()
	return outcomes
}

// batchProgress reports a scanner's runs on stderr through the logger: one line
// when the scanner starts and, with several runs, one as each finishes. A
// finished run is counted under a lock, so the lines stay in order and none is
// repeated when parallel runs end together. The lines are information level, so
// --quiet drops them, and they never reach the report on stdout.
type batchProgress struct {
	mu    sync.Mutex
	log   logger.Logger
	name  string
	total int
	done  int
}

// startProgress announces a scanner and its number of runs, and returns the
// counter its runs report to.
func (r *runner) startProgress(sc resolvedScanner, total int) *batchProgress {
	noun := "batches"
	if total == 1 {
		noun = "batch"
	}
	r.log().Info(fmt.Sprintf("%s: %d %s to run", sc.Name, total, noun))
	return &batchProgress{log: r.log(), name: sc.Name, total: total}
}

// finished records one finished run. It is safe to call from several goroutines.
func (p *batchProgress) finished() {
	if p == nil || p.total < 2 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	p.log.Info(fmt.Sprintf("%s: batch %d/%d", p.name, p.done, p.total))
}

// batchDirs are the absolute stage paths of the skill directories of a batch.
func (r *runner) batchDirs(st *scannerStage, units []skillUnit, b unitBatch) []string {
	dirs := make([]string, len(b.units))
	for i, ui := range b.units {
		dirs[i] = filepath.Join(st.root, filepath.FromSlash(units[ui].dir))
	}
	return dirs
}

// batchArgv is the argv of one batch: the command with the batch's skill
// directories in place of {skill_dirs}.
func (r *runner) batchArgv(sc resolvedScanner, root string, st *scannerStage, units []skillUnit, b unitBatch) []string {
	argv := st.expandFor(sc.Command, r.batchDirs(st, units, b), b.idx)
	if strings.ContainsAny(argv[0], `/\`) && !filepath.IsAbs(argv[0]) {
		argv[0] = filepath.Join(root, argv[0]) // a relative command is relative to the project, not the stage
	}
	return argv
}

// prepareUnitBatch builds the argv, environment and sandbox wrapping of a batch.
func (r *runner) prepareUnitBatch(sc resolvedScanner, root, binary string, st *scannerStage, units []skillUnit, b unitBatch) (preparedBatch, bool) {
	argv := r.batchArgv(sc, root, st, units, b)
	if size := argvSize(argv); size > argvBudget {
		r.addRun(CodeScannerRunFailed, sc.Name, fmt.Sprintf("{skill_dirs} expands to %d bytes of arguments, over the %d byte limit", size, argvBudget))
		return preparedBatch{}, false
	}
	spec := specFor(sc, st.root)
	spec.InheritEnv = false
	spec.Env = st.envFor(sc.EnvPass, cmdrun.HostEnv(), b.idx)
	spec, usesOut, ok := r.prepareBatch(sc, st, spec, argv, binary, b.idx)
	return preparedBatch{spec: spec, usesOut: usesOut, dirs: r.batchDirs(st, units, b)}, ok
}

// runPrepared starts a prepared batch and reads its report. It is safe to call
// from several goroutines for different batches of one stage.
func (r *runner) runPrepared(sc resolvedScanner, st *scannerStage, p preparedBatch, idx int) batchOutcome {
	norm, outOfScope, firstOut, ok := r.finishBatch(sc, st, p.dirs, true, p.usesOut, idx, cmdrun.Run(scanContext(), p.spec))
	return batchOutcome{norm: norm, outOfScope: outOfScope, firstOut: firstOut, ok: ok, ran: true}
}

// ownerDir returns the skill directory of dirs that holds the stage-relative
// path file ("" when none does).
func ownerDir(file string, dirs []string) string {
	owner := ""
	for _, d := range dirs {
		if strings.HasPrefix(file, d+"/") && len(d) > len(owner) {
			owner = d
		}
	}
	return owner
}

// cacheBatch stores the result of every skill of a finished batch. With several
// skills in the batch a result is kept only when each finding names a file of
// one skill and nothing was dropped as out of scope: a finding that belongs to
// no skill could not be replayed from one skill's entry.
func (r *runner) cacheBatch(sc resolvedScanner, units []skillUnit, b unitBatch, norm []externalFinding, outOfScope int, firstOut, version string) {
	dirs := make([]string, len(b.units))
	for i, ui := range b.units {
		dirs[i] = units[ui].dir
	}
	per := map[string][]externalFinding{}
	if len(dirs) == 1 {
		per[dirs[0]] = norm
	} else {
		if outOfScope > 0 {
			return
		}
		for i := range norm {
			owner := ownerDir(norm[i].File, dirs)
			if owner == "" {
				return
			}
			per[owner] = append(per[owner], norm[i])
		}
	}
	for _, ui := range b.units {
		u := &units[ui]
		if u.key == "" {
			continue
		}
		entry := cachedScan{Findings: toCached(per[u.dir]), Version: version, Stored: r.clock().Unix()}
		if len(dirs) == 1 {
			entry.OutOfScope, entry.FirstOut = outOfScope, firstOut
		}
		u.cache.put(u.key, entry)
	}
}

// planSkillUnits prints the plan of a per-skill scanner: the cache state of each
// skill, the runs the scanner would get and the files it would see.
func (r *runner) planSkillUnits(sc resolvedScanner, root, binary string, units []skillUnit, misses []int) {
	var files []stagedFile
	var lines []string
	hits := 0
	missing := map[int]bool{}
	for _, mi := range misses {
		missing[mi] = true
	}
	for i := range units {
		u := &units[i]
		state := cacheStateHit
		switch {
		case u.skip:
			state = "skipped (no changed file)"
		case u.key == "":
			state = "not cached"
		case missing[i]:
			state = "miss"
		}
		if state == cacheStateHit {
			hits++
		}
		if !u.skip {
			files = append(files, u.files...)
		}
		lines = append(lines, u.dir+": "+state)
	}
	st, err := r.buildStageFrom(files)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, "could not stage its inputs: "+sanitizeScannerText(err.Error()))
		return
	}
	defer st.cleanup()
	batches := splitBatches(units, misses, sc.BatchSize, argvSize(st.expandFor(sc.Command, nil, 0)), argvBudget, len(st.root))
	var argv []string
	if len(batches) > 0 {
		argv = r.batchArgv(sc, root, st, units, batches[0])
	} else {
		argv = r.batchArgv(sc, root, st, units, unitBatch{})
	}
	spec := specFor(sc, st.root)
	spec.Env = st.envFor(sc.EnvPass, cmdrun.HostEnv(), 0)
	rels := make([]string, len(files))
	for i, f := range files {
		rels[i] = f.rel
	}
	note := fmt.Sprintf("a read-only staged copy of the inputs; %d skill(s) to scan in %d run(s)", len(misses), len(batches))
	r.plan(sc, planInfo{binary: binary, argv: argv, dir: st.root, env: spec.Env, files: rels, stage: st, note: note,
		cache: fmt.Sprintf("%d hit, %d miss", hits, len(misses)), units: lines})
}
