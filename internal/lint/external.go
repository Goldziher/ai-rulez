package lint

import (
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
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/samber/oops"
)

// knownEgressFlags are flags that make a scanner send scanned content, or call a
// network service with a credential. Matched by name, with or without =value.
var knownEgressFlags = map[string]bool{
	"--use-llm": true, "--use-virustotal": true, "--vt-upload-files": true, "--use-aidefense": true,
	"--use-osv": true, "--system-one-endpoint": true, "--dangerously-run-mcp-servers": true,
}

// externalProblems lists what is wrong with one [[lint.external]] entry's
// hardening keys. Entries with a problem are reported (AR9E0) and not run.
func externalProblems(ex config.LintExternal, ceiling time.Duration) []string {
	problems := timeoutProblems(ex.Timeout, ceiling)
	problems = append(problems, batchProblems(ex)...)
	for _, name := range ex.EnvPass {
		switch {
		case !cmdrun.ValidEnvName(name):
			problems = append(problems, fmt.Sprintf("env_pass %q is not an environment variable name", name))
		case ex.Egress != nil && !*ex.Egress && cmdrun.Sensitive(name):
			problems = append(problems, fmt.Sprintf("env_pass %q is a proxy or credential variable, which an egress = false scanner must not receive", name))
		}
	}
	if ex.Version != "" {
		if _, err := semver.ParseConstraint(normalizeRange(ex.Version)); err != nil {
			problems = append(problems, fmt.Sprintf("version %q is not a version range such as \">=1.0.0, <2\": %v", ex.Version, err))
		}
	}
	if !validMaxMessage(ex.MaxMessage) {
		problems = append(problems, fmt.Sprintf("max_message %d is not between 1 and %d", ex.MaxMessage, maxScannerMessageLimit))
	}
	problems = append(problems, messagePropertiesProblems(ex.MessageProperties)...)
	problems = append(problems, cacheFilesProblems(ex.CacheFiles)...)
	problems = append(problems, stageGlobProblems(ex)...)
	problems = append(problems, versionCommandProblems(ex)...)
	problems = append(problems, versionTimeoutProblems(ex, ceiling)...)
	if !validScannerFailOn(ex.FailOn) {
		problems = append(problems, fmt.Sprintf("fail_on %q is not error, warning or info", ex.FailOn))
	}
	problems = append(problems, inputProblems(ex)...)
	return append(problems, severityMapProblems(ex.SeverityMap, ex.MaxSeverity)...)
}

// timeoutProblems lists what is wrong with a timeout against the longest one
// the policy allows (the runner's default maximum when ceiling is 0).
func timeoutProblems(timeout string, ceiling time.Duration) []string {
	if timeout == "" {
		return nil
	}
	if ceiling <= 0 {
		ceiling = cmdrun.MaxTimeout
	}
	d, err := time.ParseDuration(timeout)
	switch {
	case err != nil || d <= 0:
		return []string{fmt.Sprintf("timeout %q is not a positive duration such as \"90s\"", timeout)}
	case d > ceiling:
		return []string{fmt.Sprintf("timeout %s exceeds the maximum of %s (raise it with [lint.scanner_policy] max_timeout, at most %s)", d, ceiling, cmdrun.HardMaxTimeout)}
	}
	return nil
}

// maxScannerParallel bounds parallel.
const maxScannerParallel = 16

// batchProblems lists what is wrong with batch_size and parallel.
func batchProblems(ex config.LintExternal) []string {
	var problems []string
	if ex.BatchSize < 0 {
		problems = append(problems, fmt.Sprintf("batch_size %d is negative", ex.BatchSize))
	}
	if ex.Parallel < 0 || ex.Parallel > maxScannerParallel {
		problems = append(problems, fmt.Sprintf("parallel %d is not between 1 and %d", ex.Parallel, maxScannerParallel))
	}
	if (ex.BatchSize > 0 || ex.Parallel > 0) && !perSkillCommand(ex.Command) {
		problems = append(problems, "batch_size and parallel need {skill_dirs} in command, without {stage}, {root} or {files}")
	}
	return problems
}

// versionCommandProblems lists what is wrong with version_command.
func versionCommandProblems(ex config.LintExternal) []string {
	switch {
	case len(ex.VersionCommand) == 0:
		return nil
	case ex.Version == "":
		return []string{"version_command needs version (the range its output is checked against)"}
	case strings.TrimSpace(ex.VersionCommand[0]) == "":
		return []string{"version_command starts with an empty program name"}
	}
	return nil
}

// versionTimeoutProblems lists what is wrong with version_timeout.
func versionTimeoutProblems(ex config.LintExternal, ceiling time.Duration) []string {
	if ex.VersionTimeout == "" {
		return nil
	}
	if ex.Version == "" {
		return []string{"version_timeout needs version (the probe it bounds)"}
	}
	if ceiling <= 0 {
		ceiling = cmdrun.MaxTimeout
	}
	d, err := time.ParseDuration(ex.VersionTimeout)
	switch {
	case err != nil || d <= 0:
		return []string{fmt.Sprintf("version_timeout %q is not a positive duration such as \"30s\"", ex.VersionTimeout)}
	case d > ceiling:
		return []string{fmt.Sprintf("version_timeout %s exceeds the maximum of %s", d, ceiling)}
	}
	return nil
}

// egressFlagViolation returns the first argument that turns on network egress,
// or "". It is a heuristic over argv, not a sandbox: it catches a flag added to
// a scanner declared egress = false, not a scanner that ignores its flags.
// Single-dash spellings (-use-llm) count as the same flag, and an explicit
// false value (--use-llm=false) switches a flag off.
func egressFlagViolation(argv []string) string { return egressFlagViolationWith(argv, nil) }

// egressFlagViolationWith is egressFlagViolation that also rejects the flags in
// extra (a profile's deny-list, spelled with their dashes).
func egressFlagViolationWith(argv, extra []string) string {
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
		if knownEgressFlags["--"+name] || strings.HasPrefix(name, "llm-") || slices.Contains(extra, "--"+name) {
			return arg
		}
		if remoteEndpointFlag(argv, i, name, value, hasValue) {
			return arg
		}
	}
	return ""
}

// remoteEndpointFlag reports whether argv[i] is an endpoint or URL flag whose
// value (inline, or the next argument) names a host other than this machine.
func remoteEndpointFlag(argv []string, i int, name, value string, hasValue bool) bool {
	if !strings.Contains(name, "endpoint") && !strings.Contains(name, "url") {
		return false
	}
	if !hasValue && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
		value = strings.ToLower(argv[i+1])
	}
	return value != "" && !loopbackValue(value)
}

func falseValue(v string) bool {
	switch v {
	case boolFalse, "0", "no", levelOff:
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
	for _, p := range validateScannerPolicy(&r.lc) {
		r.addRun(CodeScannerConfigInvalid, "scanner_policy", p)
	}
	r.checkScannerBaselineLocation()
	scanners := r.scanners()
	for i := range scanners {
		sc := &scanners[i]
		for _, p := range sc.allProblems() {
			r.addRun(CodeScannerConfigInvalid, sc.Name, p)
		}
		if sc.Egress == nil {
			why := "so it runs with the full environment"
			if len(sc.Inputs) > 0 {
				why = "so its network flags are not checked"
			}
			r.addRun(CodeScannerEgressUndeclared, sc.Name,
				"declares no egress, "+why+"; set egress = false (scrubbed environment) or egress = true (needs --allow-egress)")
		}
	}
}

// scanners resolves the configured scanners: preset members, then entries.
func (r *runner) scanners() []resolvedScanner {
	return resolveScanners(&r.lc, r.cfg.Plugin != nil || r.cfg.Marketplace != nil || r.cfg.HasPluginSet())
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
func (st *scannerStage) readOut(idx int) ([]byte, error) {
	root, err := os.OpenRoot(st.scratch)
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	defer root.Close() //nolint:errcheck // read only
	f, err := root.Open(outName(idx))
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	defer f.Close() //nolint:errcheck // read only
	data, err := io.ReadAll(io.LimitReader(f, cmdrun.DefaultMaxOutput+1))
	if err != nil {
		return nil, oops.Wrapf(err, "read")
	}
	if int64(len(data)) > cmdrun.DefaultMaxOutput {
		return nil, oops.Errorf("more than %d bytes", cmdrun.DefaultMaxOutput)
	}
	return data, nil
}

// argvBudget is how many bytes of file arguments one scanner run may carry. It
// stays well under ARG_MAX (about 1 MiB on macOS, 2 MiB on Linux, shared with
// the environment) and the 32 KiB command line of Windows.
var argvBudget = defaultArgvBudget()

func defaultArgvBudget() int {
	if runtime.GOOS == osWindows {
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

// scannerFinding is a finding of one scanner, kept with its scanner for deduplication and the baseline.
type scannerFinding struct {
	Finding
	scanner string
	rule    string
	abs     string
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

// resolveRel maps a path a scanner printed to the stage-relative slash path of
// a staged file. A relative path that is not under the stage root is tried
// against the staged skill directories, which is where a scanner that was
// handed one prints its paths from; it must match exactly one. dirs are the
// skill directories to try: the ones the producing run was handed, so a
// batch of one skill resolves "SKILL.md" to that skill.
func (st *scannerStage) resolveRel(raw string, dirs []string) (string, bool) {
	if rel, ok := st.lookup(raw, st.root); ok {
		return rel, true
	}
	if filepath.IsAbs(filepath.FromSlash(strings.TrimPrefix(raw, "file://"))) || strings.Contains(raw, "://") {
		return "", false
	}
	hit, n := "", 0
	for _, dir := range dirs {
		if rel, ok := st.lookup(raw, dir); ok {
			hit, n = rel, n+1
		}
	}
	return hit, n == 1
}

// lookup resolves raw against base and returns the stage-relative path of the
// staged file it names.
func (st *scannerStage) lookup(raw, base string) (string, bool) {
	p, ok := resolveScannerPath(raw, base)
	if !ok {
		return "", false
	}
	for _, root := range scannerRoots(st.root) {
		rel, err := filepath.Rel(root, p)
		if err != nil || safefs.RelEscapes(rel) {
			continue
		}
		rel = filepath.ToSlash(rel)
		_, found := st.source[rel]
		return rel, found
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
	r.addRunAs(code, scanner, "", msg)
}

// addRunAs is addRun with an escalated severity: raise replaces the rule's
// severity unless the rule is switched off or its severity was changed from the
// default (by the user or a profile). "" keeps the rule's.
func (r *runner) addRunAs(code, scanner string, raise Severity, msg string) {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	sev := r.sev[code]
	if code == CodeScannerUnavailable && r.opts.Scanner.required[scanner] {
		sev = SeverityError // a required scanner that did not run is a failure, whatever the rule's severity
	}
	if raise != "" && sev != SeverityOff && r.escalatable(code, sev) {
		sev = raise
	}
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

// escalatable reports whether a rule still has its default severity, so an
// escalation does not override a severity the user set (even to the default) or a profile.
func (r *runner) escalatable(code string, sev Severity) bool {
	rule, ok := lookupRule(code)
	if !ok || sev != rule.Default {
		return false
	}
	for key := range r.lc.Severity {
		if set, found := lookupRule(key); found && set.Code == code {
			return false
		}
	}
	return true
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
