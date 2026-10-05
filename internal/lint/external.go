package lint

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
	return problems
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
			r.addRun(CodeScannerEgressUndeclared, ex.Name,
				"declares no egress, so it runs with the full environment; set egress = false (scrubbed environment) or egress = true (needs --allow-egress)")
		}
	}
}

// runExternal runs the scanners from lint.external and merges their findings.
// A scanner that fails, times out, is missing, or prints nothing trustworthy is
// reported, so a broken scanner never looks like a clean report.
func (r *runner) runExternal() {
	root := r.rootAbs()
	files := r.scannedFiles()
	for _, ex := range r.lc.External {
		if strings.TrimSpace(ex.Name) == "" || len(ex.Command) == 0 || len(externalProblems(ex)) > 0 {
			continue
		}
		if !r.egressAllowed(ex) {
			continue
		}
		spec := cmdrun.Spec{Dir: root, InheritEnv: ex.Egress == nil}
		if ex.Egress != nil {
			spec.Env = cmdrun.ScrubEnv(cmdrun.HostEnv(), ex.EnvPass, nil)
		}
		if ex.Timeout != "" {
			spec.Timeout, _ = time.ParseDuration(ex.Timeout) //nolint:errcheck // validated by externalProblems
		}
		// A long file list is split across runs (each with the full timeout) so
		// the command line stays under the OS limit. A failing batch stops the rest.
		for _, argv := range batchArgs(ex.Command, files, argvBudget) {
			spec.Argv = argv
			res := cmdrun.Run(context.Background(), spec)
			if !r.ingestExternal(ex, root, res) {
				break
			}
		}
	}
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

// ingestExternal turns one run into findings.
// It reports false when the scanner itself failed, so the caller stops running it.
func (r *runner) ingestExternal(ex config.LintExternal, root string, res cmdrun.Result) bool {
	switch res.Status {
	case cmdrun.StatusUnavailable:
		r.addRun(CodeScannerUnavailable, ex.Name, fmt.Sprintf("%q was not found or is not executable, so it was not run (%s)", ex.Command[0], sanitizeScannerText(res.Err.Error())))
		return false
	case cmdrun.StatusTimeout:
		r.addRun(CodeScannerRunFailed, ex.Name, fmt.Sprintf("timed out after %s and was killed", res.Timeout))
		return false
	case cmdrun.StatusError:
		r.addRun(CodeScannerRunFailed, ex.Name, "could not run: "+sanitizeScannerText(res.Err.Error()))
		return false
	case cmdrun.StatusOK, cmdrun.StatusExit:
	}
	if res.StdoutTruncated {
		r.addRun(CodeScannerRunFailed, ex.Name, fmt.Sprintf("printed more than %d bytes; output was not ingested", cmdrun.DefaultMaxOutput))
		return false
	}
	found, err := parseExternal(ex.Format, res.Stdout, res.ExitCode)
	if err != nil {
		detail := strings.TrimSpace(string(res.Stderr))
		if res.Err != nil {
			detail = res.Err.Error() + " " + detail
		}
		r.addRun(CodeScannerRunFailed, ex.Name, "failed or printed unreadable output: "+sanitizeScannerText(detail)+" ("+sanitizeScannerText(err.Error())+")")
		return false
	}
	for _, f := range found {
		msg := sanitizeScannerText(f.Message)
		if rule := sanitizeScannerText(f.Rule); rule != "" {
			msg = rule + ": " + msg
		}
		abs, inside := resolveScannerPath(f.File, root)
		if !inside {
			msg += " (the scanner reported a path outside the project: " + sanitizeScannerText(f.File) + ")"
			abs = r.configFilePath()
		}
		r.addExternal(ex.Name, abs, f.Line, externalSeverity(f.Severity), msg)
	}
	return true
}

func externalSeverity(level string) Severity {
	switch strings.ToLower(level) {
	case string(SeverityError), "high", "critical":
		return SeverityError
	case "note", "none", "info", "low":
		return SeverityInfo
	}
	return SeverityWarning
}

func (r *runner) addExternal(scanner, abs string, line int, sev Severity, msg string) {
	if r.sev[CodeExternalFinding] == SeverityOff || r.ignore[CodeExternalFinding] || (abs != "" && r.pathIgnored(abs)) {
		return
	}
	rule, _ := lookupRule(CodeExternalFinding) //nolint:errcheck // registered
	file := ""
	if abs != "" {
		file = r.display(abs)
	}
	r.findings = append(r.findings, Finding{
		Code: CodeExternalFinding, Name: rule.Name, Severity: sev, File: file, Line: max(line, 1),
		Message: "[" + scanner + "] " + msg, Root: r.display(r.rootAbs()),
	})
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
		if r.pathIgnored(abs) {
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
