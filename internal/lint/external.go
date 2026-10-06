package lint

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// knownEgressFlags are flags that make a scanner send scanned content, or call a
// network service with a credential. Matched by name, with or without =value.
var knownEgressFlags = map[string]bool{
	"--use-llm": true, "--use-virustotal": true, "--vt-upload-files": true, "--use-aidefense": true,
	"--use-osv": true, "--system-one-endpoint": true, "--dangerously-run-mcp-servers": true,
}

// externalProblems lists what is wrong with one [[lint.external]] entry's
// hardening keys. Entries with a problem are reported (AR9E0) and not run.
func externalProblems(ex config.LintExternal) []string {
	var problems []string
	if ex.Timeout != "" {
		d, err := time.ParseDuration(ex.Timeout)
		switch {
		case err != nil || d <= 0:
			problems = append(problems, fmt.Sprintf("timeout %q is not a positive duration such as \"90s\"", ex.Timeout))
		case d > cmdrun.MaxTimeout:
			problems = append(problems, fmt.Sprintf("timeout %s exceeds the maximum of %s", d, cmdrun.MaxTimeout))
		}
	}
	for _, name := range ex.EnvPass {
		switch {
		case !cmdrun.ValidEnvName(name):
			problems = append(problems, fmt.Sprintf("env_pass %q is not an environment variable name", name))
		case ex.Egress != nil && !*ex.Egress && cmdrun.Sensitive(name):
			problems = append(problems, fmt.Sprintf("env_pass %q is a proxy or credential variable, which an egress = false scanner must not receive", name))
		}
	}
	problems = append(problems, inputProblems(ex)...)
	return append(problems, severityMapProblems(ex.SeverityMap, ex.MaxSeverity)...)
}

// egressFlagViolation returns the first argument that turns on network egress,
// or "". It is a heuristic over argv, not a sandbox: it catches a flag added to
// a scanner declared egress = false, not a scanner that ignores its flags.
// Single-dash spellings (-use-llm) count as the same flag, and an explicit
// false value (--use-llm=false) switches a flag off.
func egressFlagViolation(argv []string) string {
	for i, arg := range argv {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.ToLower(strings.TrimLeft(arg, "-")), "=")
		if name == "" {
			continue
		}
		if hasValue && falseValue(value) {
			continue
		}
		if knownEgressFlags["--"+name] || strings.HasPrefix(name, "llm-") {
			return arg
		}
		if !strings.Contains(name, "endpoint") && !strings.Contains(name, "url") {
			continue
		}
		if !hasValue && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			value = strings.ToLower(argv[i+1])
		}
		if value != "" && !loopbackValue(value) {
			return arg
		}
	}
	return ""
}

func falseValue(v string) bool {
	switch v {
	case "false", "0", "no", "off":
		return true
	}
	return false
}

// loopbackValue reports whether v (a URL, host or host:port, with an optional
// bracketed IPv6 address) names this machine.
func loopbackValue(v string) bool {
	host := v
	if u, err := url.Parse(v); err == nil && u.Host != "" {
		host = u.Hostname()
	} else if h, _, err := net.SplitHostPort(v); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == hostLocalhost {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkExternalConfig reports hardening problems in the configured scanners
// (AR9E0) and scanners that declare no egress (AR9E1). It runs with every
// strict validation, not only with --external, because it reads configuration.
func (r *runner) checkExternalConfig() {
	for _, ex := range r.lc.External {
		if strings.TrimSpace(ex.Name) == "" || len(ex.Command) == 0 {
			continue
		}
		for _, p := range externalProblems(ex) {
			r.addRun(CodeScannerConfigInvalid, ex.Name, p)
		}
		if ex.Egress == nil {
			why := "so it runs with the full environment"
			if len(ex.Inputs) > 0 {
				why = "so its network flags are not checked"
			}
			r.addRun(CodeScannerEgressUndeclared, ex.Name,
				"declares no egress, "+why+"; set egress = false (scrubbed environment) or egress = true (needs --allow-egress)")
		}
	}
}

// scanScope says where a scanner's reported paths are resolved.
type scanScope struct {
	// root is the project root (legacy runs) or the stage root.
	root string
	// stage is set for a staged run; a path outside it is out of scope (AR9E6).
	stage *scannerStage
}

// runExternal runs the scanners from lint.external and merges their findings.
// A scanner that fails, times out, is missing, or prints nothing trustworthy is
// reported, so a broken scanner never looks like a clean report. The findings
// of all scanners then pass through the scanner baseline.
func (r *runner) runExternal() {
	root := r.rootAbs()
	files := r.scannedFiles()
	var all []scannerFinding
	ran := map[string]bool{}
	for _, ex := range r.lc.External {
		if strings.TrimSpace(ex.Name) == "" || len(ex.Command) == 0 || len(externalProblems(ex)) > 0 {
			continue
		}
		if !r.egressAllowed(ex) {
			continue
		}
		var got []scannerFinding
		var ok bool
		if len(ex.Inputs) > 0 {
			got, ok = r.runStaged(ex, root)
		} else {
			got, ok = r.runInRoot(ex, root, files)
		}
		all = append(all, got...)
		ran[ex.Name] = ok
	}
	r.finishExternal(all, ran)
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
// stops the rest.
func (r *runner) runInRoot(ex config.LintExternal, root string, files []string) (all []scannerFinding, ok bool) {
	spec := specFor(ex, root)
	for _, argv := range batchArgs(ex.Command, files, argvBudget) {
		spec.Argv = argv
		res := cmdrun.Run(context.Background(), spec)
		got, good := r.ingestExternal(ex, scanScope{root: root}, res)
		all = append(all, got...)
		if !good {
			return all, false
		}
	}
	return all, true
}

// runStaged runs a scanner that declared inputs: in a scratch directory that
// holds a read-only copy of exactly those inputs, with HOME and TMPDIR inside
// the scratch directory and a scrubbed environment.
func (r *runner) runStaged(ex config.LintExternal, root string) (all []scannerFinding, ok bool) {
	st, err := r.buildStage(ex.Inputs)
	if err != nil {
		r.addRun(CodeScannerRunFailed, ex.Name, "could not stage its inputs: "+sanitizeScannerText(err.Error()))
		return nil, false
	}
	defer st.cleanup()
	usesSkillDirs := slices.Contains(ex.Command, phSkillDirs)
	if len(st.files) == 0 || (usesSkillDirs && len(st.skillDirs) == 0) {
		logger.Debug("Scanner has nothing staged to scan", "scanner", ex.Name, "inputs", ex.Inputs)
		return nil, true
	}
	argv := st.expand(ex.Command)
	if size := argvSize(argv); size > argvBudget {
		r.addRun(CodeScannerRunFailed, ex.Name, fmt.Sprintf("{files} expands to %d bytes of arguments, over the %d byte limit; pass {stage} instead", size, argvBudget))
		return nil, false
	}
	if strings.ContainsAny(argv[0], `/\`) && !filepath.IsAbs(argv[0]) {
		argv[0] = filepath.Join(root, argv[0]) // a relative command is relative to the project, not the stage
	}
	usesOut := slices.ContainsFunc(ex.Command, func(a string) bool { return strings.Contains(a, phOut) })
	if usesOut {
		if err := os.WriteFile(st.outFile(), nil, 0o600); err != nil {
			r.addRun(CodeScannerRunFailed, ex.Name, "could not create its output file: "+sanitizeScannerText(err.Error()))
			return nil, false
		}
	}
	spec := specFor(ex, st.root)
	spec.Argv = argv
	spec.InheritEnv = false
	spec.Env = st.env(ex.EnvPass, cmdrun.HostEnv())
	res := cmdrun.Run(context.Background(), spec)
	if usesOut && (res.Status == cmdrun.StatusOK || res.Status == cmdrun.StatusExit) {
		data, rerr := st.readOut()
		if rerr != nil {
			r.addRun(CodeScannerRunFailed, ex.Name, "its output file is not readable: "+sanitizeScannerText(rerr.Error()))
			return nil, false
		}
		res.Stdout, res.StdoutTruncated = data, false
	}
	return r.ingestExternal(ex, scanScope{root: st.root, stage: st}, res)
}

func argvSize(argv []string) int {
	n := 0
	for _, a := range argv {
		n += len(a) + 1
	}
	return n
}

// readOut reads the {out} file the scanner wrote, through the scratch root so a
// link the scanner planted cannot point it at another file.
func (st *scannerStage) readOut() ([]byte, error) {
	root, err := os.OpenRoot(st.scratch)
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	defer root.Close() //nolint:errcheck // read only
	f, err := root.Open("out.sarif")
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	defer f.Close() //nolint:errcheck // read only
	data, err := io.ReadAll(io.LimitReader(f, cmdrun.DefaultMaxOutput+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(data)) > cmdrun.DefaultMaxOutput {
		return nil, fmt.Errorf("more than %d bytes", cmdrun.DefaultMaxOutput)
	}
	return data, nil
}

// argvBudget is how many bytes of file arguments one scanner run may carry. It
// stays well under ARG_MAX (about 1 MiB on macOS, 2 MiB on Linux, shared with
// the environment) and the 32 KiB command line of Windows.
var argvBudget = defaultArgvBudget()

func defaultArgvBudget() int {
	if runtime.GOOS == "windows" {
		return 24 << 10
	}
	return 128 << 10
}

// batchArgs appends files to base in as few argument lists as fit in budget
// bytes (each argument counted with its terminator). With no files it returns
// base alone, and a file too long for any batch gets a batch of its own.
func batchArgs(base, files []string, budget int) [][]string {
	baseCost := 0
	for _, a := range base {
		baseCost += len(a) + 1
	}
	newBatch := func() []string { return append(make([]string, 0, len(base)+len(files)), base...) }
	batches := [][]string{}
	cur, cost := newBatch(), baseCost
	added := 0
	for _, f := range files {
		if added > 0 && cost+len(f)+1 > budget {
			batches = append(batches, cur)
			cur, cost, added = newBatch(), baseCost, 0
		}
		cur = append(cur, f)
		cost += len(f) + 1
		added++
	}
	return append(batches, cur)
}

// egressAllowed applies the egress declaration before a scanner runs.
func (r *runner) egressAllowed(ex config.LintExternal) bool {
	if ex.Egress == nil {
		return true
	}
	if *ex.Egress {
		if slices.Contains(r.opts.AllowEgress, ex.Name) {
			return true
		}
		r.addRun(CodeScannerEgressBlocked, ex.Name, "declares egress = true and was not run; pass --allow-egress="+ex.Name+" to allow it for this invocation")
		return false
	}
	if flag := egressFlagViolation(ex.Command); flag != "" {
		r.addRun(CodeScannerEgressBlocked, ex.Name, fmt.Sprintf("declares egress = false but its command has %q, which can send content off the machine; remove it or declare egress = true", flag))
		return false
	}
	return true
}

// scannerFinding is a finding of one scanner, kept with its scanner for deduplication and the baseline.
type scannerFinding struct {
	Finding
	scanner string
	rule    string
	abs     string
}

// ingestExternal turns one run into findings.
// It reports false when the scanner itself failed, so the caller stops running it.
func (r *runner) ingestExternal(ex config.LintExternal, scope scanScope, res cmdrun.Result) ([]scannerFinding, bool) {
	switch res.Status {
	case cmdrun.StatusUnavailable:
		r.addRun(CodeScannerUnavailable, ex.Name, fmt.Sprintf("%q was not found or is not executable, so it was not run (%s)", ex.Command[0], sanitizeScannerText(res.Err.Error())))
		return nil, false
	case cmdrun.StatusTimeout:
		r.addRun(CodeScannerRunFailed, ex.Name, fmt.Sprintf("timed out after %s and was killed", res.Timeout))
		return nil, false
	case cmdrun.StatusError:
		r.addRun(CodeScannerRunFailed, ex.Name, "could not run: "+sanitizeScannerText(res.Err.Error()))
		return nil, false
	case cmdrun.StatusOK, cmdrun.StatusExit:
	}
	if res.StdoutTruncated {
		r.addRun(CodeScannerRunFailed, ex.Name, fmt.Sprintf("printed more than %d bytes; output was not ingested", cmdrun.DefaultMaxOutput))
		return nil, false
	}
	found, err := parseExternalKeep(ex.Format, res.Stdout, res.ExitCode, r.opts.Scanner.ShowSuppressed)
	if err != nil {
		detail := strings.TrimSpace(string(res.Stderr))
		if res.Err != nil {
			detail = res.Err.Error() + " " + detail
		}
		r.addRun(CodeScannerRunFailed, ex.Name, "failed or printed unreadable output: "+sanitizeScannerText(detail)+" ("+sanitizeScannerText(err.Error())+")")
		return nil, false
	}
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
	outOfScope, firstOut := 0, ""
	for _, f := range found {
		abs, inside := "", true
		f.Line = min(max(f.Line, 0), maxScannerLine)
		if f.File == "" {
			// A result with no location belongs to the configuration that ran the scanner.
			abs, f.Line = r.configFilePath(), 1
		} else {
			if scope.stage != nil {
				abs, inside = scope.stage.resolve(f.File)
			} else {
				abs, inside = resolveScannerPath(f.File, scope.root)
			}
		}
		msg := sanitizeScannerText(f.Message)
		if rule := sanitizeScannerText(f.Rule); rule != "" {
			msg = rule + ": " + msg
		}
		if !inside {
			if scope.stage != nil {
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
		band := scannerBand(ex.SeverityMap, f)
		if limit, ok := parseBand(ex.MaxSeverity); ok && band > limit {
			band = limit
		}
		sev := bandSeverity(band)
		if f.Suppressed {
			sev, msg = SeverityInfo, "(suppressed) "+msg
		}
		fnd, keep := r.externalFinding(ex.Name, abs, f.Line, sev, msg)
		if !keep {
			continue
		}
		fnd.meta().Fingerprint = r.scannerFingerprint(ex.Name, f, abs, fnd.Line, occurrence)
		out = append(out, scannerFinding{Finding: fnd, scanner: ex.Name, rule: sanitizeScannerText(f.Rule), abs: abs})
	}
	if outOfScope > 0 {
		r.addRun(CodeScannerOutOfScope, ex.Name, fmt.Sprintf("reported %d result(s) for files that were not staged for it, first %q; they were dropped", outOfScope, firstOut))
	}
	return out, true
}

// evidenceURL returns a rule's helpUri when it is a plain https URL.
func evidenceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "https://") || len(raw) > 200 || strings.ContainsAny(raw, " \"<>") ||
		strings.IndexFunc(raw, func(r rune) bool { return r < 0x21 || r == 0x7f }) >= 0 {
		return ""
	}
	return sanitizeScannerText(raw)
}

// resolve maps a path a scanner printed to the source file of a staged file.
// ok is false for anything that is not a staged file. A relative path that is
// not under the stage root is tried against the staged skill directories, which
// is where a scanner that was handed one prints its paths from; it must match
// exactly one.
func (st *scannerStage) resolve(raw string) (string, bool) {
	if src, ok := st.lookup(raw, st.root); ok {
		return src, true
	}
	if filepath.IsAbs(filepath.FromSlash(strings.TrimPrefix(raw, "file://"))) || strings.Contains(raw, "://") {
		return "", false
	}
	hit, n := "", 0
	for _, dir := range st.skillDirs {
		if src, ok := st.lookup(raw, dir); ok {
			hit, n = src, n+1
		}
	}
	return hit, n == 1
}

// lookup resolves raw against base and maps the result through the stage.
func (st *scannerStage) lookup(raw, base string) (string, bool) {
	p, ok := resolveScannerPath(raw, base)
	if !ok {
		return "", false
	}
	for _, root := range scannerRoots(st.root) {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		src, found := st.source[filepath.ToSlash(rel)]
		return src, found
	}
	return "", false
}

// externalFinding builds one AR011 finding; keep is false when the rule is off,
// ignored, or the file is ignored by configuration.
func (r *runner) externalFinding(scanner, abs string, line int, sev Severity, msg string) (Finding, bool) {
	if r.sev[CodeExternalFinding] == SeverityOff || r.ignore[CodeExternalFinding] || (abs != "" && r.pathSuppresses(CodeExternalFinding, abs)) {
		return Finding{}, false
	}
	rule, _ := lookupRule(CodeExternalFinding) //nolint:errcheck // registered
	file := ""
	if abs != "" {
		file = r.display(abs)
	}
	f := Finding{
		Code: CodeExternalFinding, Name: rule.Name, Severity: sev, File: file, Line: max(line, 1),
		Message: "[" + scanner + "] " + msg, Root: r.display(r.rootAbs()),
	}
	return f, true
}

// addRun records a finding about a scanner itself (not one of its results),
// attributed to the config file.
func (r *runner) addRun(code, scanner, msg string) {
	sev := r.sev[code]
	if sev == SeverityOff || r.ignore[code] {
		return
	}
	rule, _ := lookupRule(code) //nolint:errcheck // registered
	file := ""
	if abs := r.configFilePath(); abs != "" {
		if r.pathSuppresses(code, abs) {
			return
		}
		file = r.display(abs)
	}
	r.findings = append(r.findings, Finding{
		Code: code, Name: rule.Name, Severity: sev, File: file, Line: 1,
		Message: "[" + scanner + "] " + msg, Root: r.display(r.rootAbs()),
	})
}

// scannedFiles lists the files the built-in scan read, for the external scanners.
func (r *runner) scannedFiles() []string {
	set := map[string]bool{}
	for abs := range r.docs {
		if filepath.IsAbs(abs) {
			set[abs] = true
		}
	}
	files := make([]string, 0, len(set))
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// hostLocalhost is the loopback host name the URL checks treat as local.
const hostLocalhost = "localhost"
