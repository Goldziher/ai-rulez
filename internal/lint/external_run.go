package lint

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
)

// scannerSandbox confines staged scanners. Tests replace it.
var scannerSandbox = sandbox.Default()

// scanScope says how a scanner's reported paths map to project files.
type scanScope struct {
	// root is the project root of an in-root run.
	root string
	// lookup is set for a staged run: it maps a stage-relative path to its
	// source file; ok is false for a path that was not staged for the scanner.
	lookup func(rel string) (abs string, ok bool)
}

// runExternal runs the scanners (the policy preset's members and
// lint.external) and merges their findings. A scanner that fails, times out, is
// missing, or prints nothing trustworthy is reported, so a broken scanner never
// looks like a clean report. The findings of all scanners then pass through the
// scanner baseline.
func (r *runner) runExternal() {
	root := r.rootAbs()
	files := r.scannedFiles()
	list := r.scanners()
	r.opts.Scanner.required = map[string]bool{}
	for _, sc := range list {
		if sc.Required {
			r.opts.Scanner.required[sc.Name] = true
		}
	}
	r.requireConfigured(list)
	var all []scannerFinding
	ran := map[string]bool{}
	for _, sc := range list {
		if len(sc.allProblems()) > 0 || !r.egressAllowed(sc) {
			continue
		}
		if sc.Version != "" && !r.versionSatisfied(sc, root) {
			ran[sc.Name] = false
			continue
		}
		var got []scannerFinding
		var ok bool
		if len(sc.Inputs) > 0 {
			got, ok = r.runStaged(sc, root)
		} else {
			got, ok = r.runInRoot(sc, root, files)
		}
		all = append(all, got...)
		ran[sc.Name] = ok
	}
	r.finishExternal(all, ran)
}

// requireConfigured reports a name in [lint.scanner_policy] required that no
// preset member or entry provides: nothing would ever run it.
func (r *runner) requireConfigured(list []resolvedScanner) {
	have := map[string]bool{}
	for _, sc := range list {
		have[sc.Name] = true
	}
	want := make([]string, 0, len(r.opts.Scanner.required))
	for name := range policyOf(&r.lc).required {
		want = append(want, name)
	}
	sort.Strings(want)
	for _, name := range want {
		if !have[name] {
			r.opts.Scanner.required[name] = true
			r.addRun(CodeScannerUnavailable, name, "is listed in [lint.scanner_policy] required but is not configured: add a [[lint.external]] entry or a preset that provides it")
		}
	}
}

// specFor builds the run spec shared by both modes.
func specFor(ex config.LintExternal, dir string) cmdrun.Spec {
	spec := cmdrun.Spec{Dir: dir, InheritEnv: ex.Egress == nil}
	if ex.Egress != nil {
		spec.Env = cmdrun.ScrubEnv(cmdrun.HostEnv(), ex.EnvPass, nil)
	}
	if ex.Timeout != "" {
		spec.Timeout, _ = time.ParseDuration(ex.Timeout) //nolint:errcheck // validated by externalProblems
	}
	return spec
}

// runInRoot is the legacy mode: run in the project root with the paths of the
// scanned files appended. A long file list is split across runs (each with the
// full timeout) so the command line stays under the OS limit. A failing batch
// stops the rest. It cannot be confined (the scanner needs the project), so
// isolation = "require" refuses it: declare inputs to stage the content.
func (r *runner) runInRoot(sc resolvedScanner, root string, files []string) (all []scannerFinding, ok bool) {
	if policyOf(&r.lc).isolation == sandbox.ModeRequire && !r.opts.Scanner.DryRun {
		r.addRun(CodeScannerRunFailed, sc.Name, "isolation = \"require\" cannot confine a scanner that runs in the project root; set inputs so it runs on a staged copy, or set isolation = \"none\"")
		return nil, false
	}
	spec := specFor(sc.LintExternal, root)
	batches := batchArgs(sc.Command, files, argvBudget)
	if r.opts.Scanner.DryRun {
		r.plan(sc, planInfo{binary: lookExecutable(sc.Command[0], root), argv: batches[0], dir: root, env: spec.Env, inherit: spec.InheritEnv,
			files: relFiles(r, files), note: fmt.Sprintf("in the project root, %d file path(s) appended in %d run(s)", len(files), len(batches))})
		return nil, true
	}
	for _, argv := range batches {
		spec.Argv = argv
		res := cmdrun.Run(context.Background(), spec)
		found, good := r.parseRun(sc, res)
		if !good {
			return all, false
		}
		got := r.convertFindings(sc, scanScope{root: root}, found, 0, "")
		all = append(all, got...)
	}
	return all, true
}

func relFiles(r *runner, abs []string) []string {
	out := make([]string, 0, len(abs))
	for _, a := range abs {
		if rel := r.tree.Rel(a); rel != "" {
			out = append(out, rel)
		}
	}
	return out
}

// runStaged runs a scanner that declared inputs: in a scratch directory that
// holds a read-only copy of exactly those inputs, with HOME and TMPDIR inside
// the scratch directory and a scrubbed environment. The result is cached by the
// staged content (egress = false scanners only) and the run is confined when
// isolation allows it.
func (r *runner) runStaged(sc resolvedScanner, root string) (all []scannerFinding, ok bool) {
	want := map[string]bool{}
	for _, in := range sc.Inputs {
		want[in] = true
	}
	files := r.stageFiles(want, sc.Layout)
	if len(files) == 0 || (slices.Contains(sc.Command, phSkillDirs) && !hasSkillDir(files)) {
		r.log().Debug("Scanner has nothing staged to scan", "scanner", sc.Name, "inputs", sc.Inputs)
		return nil, true
	}
	binary := lookExecutable(sc.Command[0], root)
	digest := digestStage(files)
	key, cache := r.cacheFor(sc, binary, digest)
	if key != "" {
		if hit, found := r.cacheGet(sc, cache, key); found && !r.opts.Scanner.DryRun {
			r.log().Debug("Scanner result served from the cache", "scanner", sc.Name)
			return r.fromCache(sc, files, hit), true
		}
	}
	st, err := r.buildStageFrom(files)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, "could not stage its inputs: "+sanitizeScannerText(err.Error()))
		return nil, false
	}
	defer st.cleanup()
	argv := st.expand(sc.Command)
	if size := argvSize(argv); size > argvBudget {
		r.addRun(CodeScannerRunFailed, sc.Name, fmt.Sprintf("{files} expands to %d bytes of arguments, over the %d byte limit; pass {stage} instead", size, argvBudget))
		return nil, false
	}
	if strings.ContainsAny(argv[0], `/\`) && !filepath.IsAbs(argv[0]) {
		argv[0] = filepath.Join(root, argv[0]) // a relative command is relative to the project, not the stage
	}
	spec := specFor(sc.LintExternal, st.root)
	spec.InheritEnv = false
	spec.Env = st.env(sc.EnvPass, cmdrun.HostEnv())
	if r.opts.Scanner.DryRun {
		r.planStaged(sc, st, argv, spec, files, key != "", r.cacheState(sc, cache, key))
		return nil, true
	}
	norm, outOfScope, firstOut, ok := r.execStaged(sc, st, spec, argv, binary)
	if !ok {
		return nil, false
	}
	if key != "" {
		version, _ := r.probeVersion(binary) //nolint:errcheck // a refused probe only leaves the recorded version empty
		cache.put(key, cachedScan{Findings: toCached(norm), OutOfScope: outOfScope, FirstOut: firstOut, Version: version, Stored: r.clock().Unix()})
	}
	return r.convertFindings(sc, stagedScope(files), norm, outOfScope, firstOut), true
}

// execStaged starts a staged scanner (confined when isolation allows) and
// returns its findings with stage-relative paths.
func (r *runner) execStaged(sc resolvedScanner, st *scannerStage, spec cmdrun.Spec, argv []string, binary string) (norm []externalFinding, outOfScope int, firstOut string, ok bool) {
	usesOut := slices.ContainsFunc(sc.Command, func(a string) bool { return strings.Contains(a, phOut) })
	if usesOut {
		if err := os.WriteFile(st.outFile(), nil, 0o600); err != nil {
			r.addRun(CodeScannerRunFailed, sc.Name, "could not create its output file: "+sanitizeScannerText(err.Error()))
			return nil, 0, "", false
		}
	}
	argv, ok = r.isolate(sc, st, argv, binary)
	if !ok {
		return nil, 0, "", false
	}
	spec.Argv = argv
	r.banner(sc)
	res := cmdrun.Run(context.Background(), spec)
	if usesOut && (res.Status == cmdrun.StatusOK || res.Status == cmdrun.StatusExit) {
		data, rerr := st.readOut()
		if rerr != nil {
			r.addRun(CodeScannerRunFailed, sc.Name, "its output file is not readable: "+sanitizeScannerText(rerr.Error()))
			return nil, 0, "", false
		}
		res.Stdout, res.StdoutTruncated = data, false
	}
	found, good := r.parseRun(sc, res)
	if !good {
		return nil, 0, "", false
	}
	norm, outOfScope, firstOut = st.normalize(found)
	return norm, outOfScope, firstOut, true
}

// stagedScope maps stage-relative paths to the files they were made from.
func stagedScope(files []stagedFile) scanScope {
	source := make(map[string]string, len(files))
	for _, f := range files {
		source[f.rel] = f.source
	}
	return scanScope{lookup: func(rel string) (string, bool) { abs, ok := source[rel]; return abs, ok }}
}

func (r *runner) fromCache(sc resolvedScanner, files []stagedFile, hit cachedScan) []scannerFinding {
	return r.convertFindings(sc, stagedScope(files), fromCached(hit.Findings), hit.OutOfScope, hit.FirstOut)
}

// cacheFor returns the cache and key for a run, or an empty key when the run
// is not cacheable (no cache, egress scanner, scanner not installed).
func (r *runner) cacheFor(sc resolvedScanner, binary, digest string) (string, *ScanCache) {
	cache := r.opts.Scanner.Cache
	if cache == nil || r.opts.Scanner.NoCache || sc.Egress == nil || *sc.Egress || binary == "" {
		return "", nil
	}
	key, ok := scanKeyFor(sc, binary, digest, r.opts.Scanner.ShowSuppressed, r.isolationKey())
	if !ok {
		return "", nil
	}
	return key, cache
}

// cacheGet reads a cached result, treating one that expired (launcher
// scanners) as a miss.
func (r *runner) cacheGet(sc resolvedScanner, c *ScanCache, key string) (cachedScan, bool) {
	hit, ok := c.get(key)
	if !ok || cacheExpired(sc, hit, r.clock()) {
		return cachedScan{}, false
	}
	return hit, true
}

// isolationKey is the part of the cache key that says how a run is confined:
// the policy mode and, when it can confine, the backend. It starts no process
// (the backend is only looked up), so the lock can compute it.
func (r *runner) isolationKey() string {
	mode := policyOf(&r.lc).isolation
	if mode == sandbox.ModeNone {
		return string(mode) + "/"
	}
	return string(mode) + "/" + string(scannerSandbox.Backend())
}

func (r *runner) cacheState(sc resolvedScanner, c *ScanCache, key string) string {
	switch {
	case c == nil || key == "":
		return levelOff
	default:
		if _, ok := r.cacheGet(sc, c, key); ok {
			return "hit"
		}
		return "miss"
	}
}

func hasSkillDir(files []stagedFile) bool {
	for _, f := range files {
		if f.skillOf != "" {
			return true
		}
	}
	return false
}

// probeVersion asks a scanner for its version through the hardened runner,
// confined as the policy's isolation mode asks. The error is set when the
// probe was refused (isolation = "require" with no working backend).
func (r *runner) probeVersion(binary string) (string, error) {
	if binary == "" {
		return "", nil
	}
	return ProbeScannerVersion(context.Background(), ScannerInfo{Path: binary, Isolation: string(policyOf(&r.lc).isolation)})
}

// versionSatisfied checks the entry's version range against `<binary> --version`.
// A scanner that is not installed is left to the run (AR9E2 there); one outside
// the range is not run. A dry run starts nothing, so it skips the check.
func (r *runner) versionSatisfied(sc resolvedScanner, root string) bool {
	if r.opts.Scanner.DryRun {
		return true
	}
	binary := lookExecutable(sc.Command[0], root)
	if binary == "" {
		return true
	}
	c, err := semver.ParseConstraint(normalizeRange(sc.Version))
	if err != nil {
		r.addRun(CodeScannerConfigInvalid, sc.Name, fmt.Sprintf("version %q is not a version range: %v", sc.Version, err))
		return false
	}
	line, err := r.probeVersion(binary)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, "was not run: its version could not be checked: "+sanitizeScannerText(err.Error()))
		return false
	}
	v, ok := firstVersion(line)
	if !ok {
		r.addRun(CodeScannerUnavailable, sc.Name, fmt.Sprintf("was not run: its --version output %q has no version to check against %q", line, sc.Version))
		return false
	}
	if !c.Check(v, true) {
		r.addRun(CodeScannerUnavailable, sc.Name, fmt.Sprintf("is version %s, which does not satisfy %q; it was not run", v, sc.Version))
		return false
	}
	return true
}

// normalizeRange lets a range be written with commas (">=1.0.0, <2"), which the
// constraint grammar spells as spaces.
func normalizeRange(r string) string { return strings.ReplaceAll(r, ",", " ") }

// firstVersion extracts the first x.y or x.y.z version from a line of text.
func firstVersion(line string) (semver.Version, bool) {
	for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' || r == '(' || r == ')' || r == '=' }) {
		field = strings.TrimPrefix(strings.TrimPrefix(field, "v"), "V")
		if field == "" || field[0] < '0' || field[0] > '9' || !strings.Contains(field, ".") {
			continue
		}
		if strings.Count(strings.SplitN(strings.SplitN(field, "-", 2)[0], "+", 2)[0], ".") == 1 {
			field = strings.Replace(field, "-", ".0-", 1)
			if !strings.Contains(field, "-") {
				field = strings.SplitN(field, "+", 2)[0] + ".0"
			}
		}
		if v, err := semver.Parse(field); err == nil {
			return v, true
		}
	}
	return semver.Version{}, false
}

// egressAllowed applies the egress declaration before a scanner runs.
func (r *runner) egressAllowed(sc resolvedScanner) bool {
	if sc.Egress == nil {
		return true
	}
	if *sc.Egress {
		if !slices.Contains(r.opts.AllowEgress, sc.Name) {
			r.addRun(CodeScannerEgressBlocked, sc.Name, "declares egress = true and was not run; pass --allow-egress="+sc.Name+" to allow it for this invocation")
			return false
		}
		if pol := policyOf(&r.lc); pol.allowSet && !slices.Contains(pol.allowEgress, sc.Name) {
			r.addRun(CodeScannerEgressBlocked, sc.Name, "declares egress = true and --allow-egress names it, but [lint.scanner_policy] allow_egress does not list it")
			return false
		}
		return true
	}
	if flag := sc.egressFlag(); flag != "" {
		r.addRun(CodeScannerEgressBlocked, sc.Name, fmt.Sprintf("declares egress = false but its command has %q, which can send content off the machine; remove it or declare egress = true", flag))
		return false
	}
	return true
}

// banner tells the user, before an egress scanner starts, that scanned content
// leaves the machine and what the vendor documents receiving.
func (r *runner) banner(sc resolvedScanner) {
	if sc.Egress == nil || !*sc.Egress {
		return
	}
	sent := "content derived from the scanned files"
	if len(sc.DataSent) > 0 {
		sent = strings.Join(sc.DataSent, "; ")
	}
	r.host.Logger().Warn(fmt.Sprintf("Running %s with egress: it can send data off this machine, and its vendor's terms apply", sc.Name),
		"scanner", sc.Name, "may send", sent, "allowed by", "--allow-egress="+sc.Name)
}

// isolate wraps argv in the process sandbox when the policy asks for it: no
// network (unless the scanner declares egress), no writes outside the scratch
// directory. It reports false when isolation is required and unavailable.
func (r *runner) isolate(sc resolvedScanner, st *scannerStage, argv []string, binary string) ([]string, bool) {
	mode := policyOf(&r.lc).isolation
	on, err := scannerSandbox.Resolve(context.Background(), mode)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, fmt.Sprintf("isolation = \"require\" but %s", sanitizeScannerText(err.Error())))
		return argv, false
	}
	if !on {
		if mode == sandbox.ModeAuto {
			r.noteIsolationDegraded()
		}
		return argv, true
	}
	if binary == "" {
		return argv, true // not installed: the runner reports it
	}
	// The sandbox tool resolves argv[1] itself; give it the absolute path found
	// here, so a relative PATH entry cannot substitute another binary.
	confined := append([]string{binary}, argv[1:]...)
	w, err := scannerSandbox.Wrap(sandbox.Spec{WriteDirs: []string{st.scratch}, AllowNetwork: sc.Egress != nil && *sc.Egress}, confined)
	if err != nil {
		r.addRun(CodeScannerRunFailed, sc.Name, "could not apply isolation: "+sanitizeScannerText(err.Error()))
		return argv, false
	}
	return w.Argv, true
}

// noteIsolationDegraded reports once per run that isolation = "auto" found no
// backend, so staged scanners ran with a scrubbed environment only.
func (r *runner) noteIsolationDegraded() {
	if r.opts.Scanner.degraded {
		return
	}
	r.opts.Scanner.degraded = true
	r.addRun(CodeScannerNoIsolation, "isolation", "no process isolation is available on this system, so staged scanners ran with a scrubbed environment but without network or write confinement; set isolation = \"none\" to accept this, or \"require\" to refuse")
}

// parseRun checks how the run ended and parses its output. It reports false
// when the scanner itself failed, so the caller stops running it.
func (r *runner) parseRun(sc resolvedScanner, res cmdrun.Result) ([]externalFinding, bool) {
	switch res.Status {
	case cmdrun.StatusUnavailable:
		r.addRun(CodeScannerUnavailable, sc.Name, fmt.Sprintf("%q was not found or is not executable, so it was not run (%s)", sc.Command[0], sanitizeScannerText(res.Err.Error())))
		return nil, false
	case cmdrun.StatusTimeout:
		r.addRun(CodeScannerRunFailed, sc.Name, fmt.Sprintf("timed out after %s and was killed", res.Timeout))
		return nil, false
	case cmdrun.StatusError:
		r.addRun(CodeScannerRunFailed, sc.Name, "could not run: "+sanitizeScannerText(res.Err.Error()))
		return nil, false
	case cmdrun.StatusOK, cmdrun.StatusExit:
	}
	if res.StdoutTruncated {
		r.addRun(CodeScannerRunFailed, sc.Name, fmt.Sprintf("printed more than %d bytes; output was not ingested", cmdrun.DefaultMaxOutput))
		return nil, false
	}
	found, err := parseExternalKeep(sc.Format, res.Stdout, res.ExitCode, r.opts.Scanner.ShowSuppressed)
	if err != nil {
		detail := strings.TrimSpace(string(res.Stderr))
		if res.Err != nil {
			detail = res.Err.Error() + " " + detail
		}
		r.addRun(CodeScannerRunFailed, sc.Name, "failed or printed unreadable output: "+sanitizeScannerText(detail)+" ("+sanitizeScannerText(err.Error())+")")
		return nil, false
	}
	return found, true
}

// normalize rewrites the paths of a staged run's findings to stage-relative
// slash paths and drops the ones that are not a staged file, counting them
// (AR9E6). The result no longer depends on where the stage was written, which
// is what the cache stores.
func (st *scannerStage) normalize(found []externalFinding) (out []externalFinding, outOfScope int, firstOut string) {
	for _, f := range found {
		if f.File != "" {
			rel, ok := st.resolveRel(f.File)
			if !ok {
				if outOfScope++; firstOut == "" {
					firstOut = sanitizeScannerText(f.File)
				}
				continue
			}
			f.File = rel
		}
		out = append(out, f)
	}
	return out, outOfScope, firstOut
}

// planInfo is what a dry run prints for one scanner.
type planInfo struct {
	binary  string
	argv    []string
	dir     string
	env     []string
	inherit bool
	files   []string
	note    string
	cache   string
	stage   *scannerStage
}

func (r *runner) planStaged(sc resolvedScanner, st *scannerStage, argv []string, spec cmdrun.Spec, files []stagedFile, _ bool, cache string) {
	rels := make([]string, len(files))
	for i, f := range files {
		rels[i] = f.rel
	}
	r.plan(sc, planInfo{binary: lookExecutable(sc.Command[0], r.rootAbs()), argv: argv, dir: st.root, env: spec.Env, files: rels, cache: cache, stage: st,
		note: "a read-only staged copy of the inputs"})
}

// plan prints one scanner's plan: argv, staged files and environment variable
// names (never values). Nothing is started.
func (r *runner) plan(sc resolvedScanner, p planInfo) {
	var w strings.Builder
	egress := "undeclared"
	if sc.Egress != nil {
		egress = fmt.Sprintf("%t", *sc.Egress)
	}
	format := sc.Format
	if format == "" {
		format = FormatSARIF
	}
	fmt.Fprintf(&w, "scanner %s (egress = %s, format = %s)\n", sc.Name, egress, format)
	binary := p.binary
	if binary == "" {
		binary = "not found (it would be reported as AR9E2)"
	}
	fmt.Fprintf(&w, "  binary:    %s\n", binary)
	fmt.Fprintf(&w, "  command:   %s\n", p.shown(r))
	fmt.Fprintf(&w, "  isolation: %s\n", r.planIsolation(sc, p))
	if p.inherit {
		fmt.Fprintln(&w, "  env:       the full environment (egress is undeclared)")
	} else {
		names := make([]string, 0, len(p.env))
		for _, kv := range p.env {
			if name, _, ok := strings.Cut(kv, "="); ok {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		fmt.Fprintf(&w, "  env:       %s\n", strings.Join(names, ", "))
	}
	if p.note != "" {
		fmt.Fprintf(&w, "  input:     %s\n", p.note)
	}
	if p.cache != "" {
		fmt.Fprintf(&w, "  cache:     %s\n", p.cache)
	}
	fmt.Fprintf(&w, "  files:     %d\n", len(p.files))
	for _, f := range p.files {
		fmt.Fprintf(&w, "    %s\n", f)
	}
	out := r.opts.Scanner.Out
	if out == nil {
		return
	}
	if _, err := io.WriteString(out, w.String()); err != nil {
		r.host.Logger().Warn("could not write the scanner plan", "scanner", sc.Name, "error", err)
	}
}

func (p planInfo) shown(r *runner) string {
	argv := make([]string, len(p.argv))
	for i, a := range p.argv {
		if p.stage != nil {
			a = strings.ReplaceAll(a, p.stage.root, "<stage>")
			a = strings.ReplaceAll(a, p.stage.scratch, "<scratch>")
		}
		argv[i] = a
	}
	if len(argv) > 40 {
		argv = append(argv[:40:40], fmt.Sprintf("... (%d more)", len(p.argv)-40))
	}
	return strings.Join(argv, " ")
}

func (r *runner) planIsolation(sc resolvedScanner, p planInfo) string {
	mode := policyOf(&r.lc).isolation
	if p.stage == nil {
		if mode == sandbox.ModeRequire {
			return "require: refused, a scanner in the project root cannot be confined"
		}
		return "none (a scanner in the project root is not confined)"
	}
	b := scannerSandbox.Backend()
	switch {
	case mode == sandbox.ModeNone:
		return "none (isolation = \"none\")"
	case b == sandbox.BackendNone && mode == sandbox.ModeRequire:
		return "require: refused, no backend is available"
	case b == sandbox.BackendNone:
		return "none available (auto)"
	}
	net := "network denied"
	if sc.Egress != nil && *sc.Egress {
		net = "network allowed (egress = true)"
	}
	return fmt.Sprintf("%s (%s, writes only under the scratch directory; mode %s)", b, net, mode)
}

// convertFindings turns parsed findings into AR011 findings: path mapping,
// severity bands, fingerprints. outOfScope and firstOut carry the results a
// staged run already dropped (AR9E6).
func (r *runner) convertFindings(sc resolvedScanner, scope scanScope, found []externalFinding, outOfScope int, firstOut string) []scannerFinding {
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		return a.Fingerprint < b.Fingerprint
	})
	var out []scannerFinding
	occurrence := map[string]int{}
	for _, f := range found {
		f.Line = min(max(f.Line, 0), maxScannerLine)
		abs, inside := r.locate(&f, scope)
		msg := sanitizeScannerText(f.Message)
		if rule := sanitizeScannerText(f.Rule); rule != "" {
			msg = rule + ": " + msg
		}
		if !inside {
			if scope.lookup != nil {
				if outOfScope++; firstOut == "" {
					firstOut = sanitizeScannerText(f.File)
				}
				continue
			}
			msg += " (the scanner reported a path outside the project: " + sanitizeScannerText(f.File) + ")"
			abs = r.configFilePath()
		}
		if u := evidenceURL(f.HelpURI); u != "" {
			msg += " (" + u + ")"
		}
		band := scannerBand(sc.SeverityMap, f)
		if limit, ok := parseBand(sc.MaxSeverity); ok && band > limit {
			band = limit
		}
		sev := bandSeverity(band)
		if f.Suppressed {
			sev, msg = SeverityInfo, "(suppressed) "+msg
		}
		built, keep := r.externalFinding(sc.Name, abs, f.Line, sev, msg)
		if !keep {
			continue
		}
		built.meta().Fingerprint = r.scannerFingerprint(sc.Name, f, abs, built.Line, occurrence)
		out = append(out, scannerFinding{Finding: built, scanner: sc.Name, rule: sanitizeScannerText(f.Rule), abs: abs})
	}
	if outOfScope > 0 {
		r.addRun(CodeScannerOutOfScope, sc.Name, fmt.Sprintf("reported %d result(s) for files that were not staged for it, first %q; they were dropped", outOfScope, firstOut))
	}
	return out
}

// locate maps a finding's path to a project file. A result with no location
// belongs to the configuration that ran the scanner (line 1). inside is false
// for a path the scope does not contain.
func (r *runner) locate(f *externalFinding, scope scanScope) (abs string, inside bool) {
	switch {
	case f.File == "":
		f.Line = 1
		return r.configFilePath(), true
	case scope.lookup != nil:
		return scope.lookup(f.File)
	}
	return resolveScannerPath(f.File, scope.root)
}
