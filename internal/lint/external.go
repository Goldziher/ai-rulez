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
	if ex.Version != "" {
		if _, err := semver.ParseConstraint(normalizeRange(ex.Version)); err != nil {
			problems = append(problems, fmt.Sprintf("version %q is not a version range such as \">=1.0.0, <2\": %v", ex.Version, err))
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
	return resolveScanners(&r.lc, r.cfg.Plugin != nil || r.cfg.Marketplace != nil)
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
// handed one prints its paths from; it must match exactly one.
func (st *scannerStage) resolveRel(raw string) (string, bool) {
	if rel, ok := st.lookup(raw, st.root); ok {
		return rel, true
	}
	if filepath.IsAbs(filepath.FromSlash(strings.TrimPrefix(raw, "file://"))) || strings.Contains(raw, "://") {
		return "", false
	}
	hit, n := "", 0
	for _, dir := range st.skillDirs {
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
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
	sev := r.sev[code]
	if code == CodeScannerUnavailable && r.opts.Scanner.required[scanner] {
		sev = SeverityError // a required scanner that did not run is a failure, whatever the rule's severity
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
