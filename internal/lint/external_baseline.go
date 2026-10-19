package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// ScannerBaselineFile is the default scanner baseline, relative to the
// configuration directory. It has the format of the lint baseline (see
// Baseline); each entry also names its scanner and rule.
const ScannerBaselineFile = "scanner-baseline.json"

// scannerFingerprintVersion prefixes a scanner finding's fingerprint.
const scannerFingerprintVersion = "sc1"

// TodayEnv pins the date baseline entries expire against, like --today.
const TodayEnv = "AI_RULEZ_TODAY"

// ScannerOptions are the external-scanner settings of a run.
type ScannerOptions struct {
	// BaselinePath is the scanner baseline; empty means <config dir>/scanner-baseline.json.
	BaselinePath string
	// WriteBaseline rewrites the baseline to accept every current finding of the
	// scanners that ran, keeping the entries of the ones that did not. Reason is
	// required for a new entry.
	WriteBaseline bool
	Reason        string
	// Filter narrows what --write-baseline records; findings it leaves out stay
	// reported and keep any entry they already have. ReasonFor maps a rule (id,
	// `scanner:rule`, AR code or glob) to the reason of its new entries; Reason,
	// which may contain {rule}, {severity} and {scanner}, covers the rest.
	Filter    BaselineFilter
	ReasonFor map[string]string
	// Today is the YYYY-MM-DD date entries expire against; empty reads
	// AI_RULEZ_TODAY, then the clock.
	Today string
	// ShowSuppressed keeps results the scanner marked suppressed, as info.
	ShowSuppressed bool
	// Cache is the result cache of staged egress = false scanners; nil turns it
	// off. NoCache bypasses it for this run (--no-scan-cache).
	Cache   *ScanCache
	NoCache bool
	// ChangedFiles narrows a scanner handed {skill_dirs} to the skill directories
	// that hold one of these files (repository-relative slash paths): the files
	// `validate --changed` or `--since` found changed. nil means no narrowing; an
	// empty slice means nothing changed. Ignored by --write-baseline, which must
	// see every finding.
	ChangedFiles []string
	// DryRun prints every scanner's plan (command, staged files, environment
	// names, isolation, cache state) to Out and starts nothing.
	DryRun bool
	Out    io.Writer

	// required names the scanners whose absence is an error; degraded is set
	// once AR9E7 was reported. Both are run state.
	required map[string]bool
	degraded bool
}

func (o ScannerOptions) today(host ambient.Host) string {
	log := host.Logger()
	for _, candidate := range []struct{ source, value string }{{"--today", o.Today}, {TodayEnv, host.GetEnv(TodayEnv)}} {
		if candidate.value == "" {
			continue
		}
		if _, err := time.Parse(dateLayout, candidate.value); err != nil {
			log.Warn("Ignoring a malformed baseline date; expecting YYYY-MM-DD", "source", candidate.source, "value", candidate.value)
			continue
		}
		return candidate.value
	}
	return host.Now().UTC().Format(dateLayout)
}

func (r *runner) scannerBaselinePath() string {
	if p := r.opts.Scanner.BaselinePath; p != "" {
		return p
	}
	if abs, problem := r.policyBaselinePath(); problem == "" && abs != "" {
		return abs
	}
	if r.cfg.ConfigDir == "" {
		return ""
	}
	return filepath.Join(r.cfg.ConfigDir, ScannerBaselineFile)
}

// policyBaselinePath resolves [lint.scanner_policy] baseline against the
// project root. A committed config must not point outside the project, so an
// absolute path, a ".." escape or a symlink that leaves the project is a problem.
func (r *runner) policyBaselinePath() (abs, problem string) {
	rel := policyOf(&r.lc).baseline
	if rel == "" {
		return "", ""
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || safefs.RelEscapes(clean) {
		return "", fmt.Sprintf("lint.scanner_policy.baseline %q must be a path inside the project", rel)
	}
	abs = filepath.Join(r.rootAbs(), clean)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && r.tree.Rel(resolved) == "" {
		return "", fmt.Sprintf("lint.scanner_policy.baseline %q resolves outside the project", rel)
	}
	return abs, ""
}

// checkScannerBaselineLocation reports an unusable baseline location (AR9E0).
func (r *runner) checkScannerBaselineLocation() {
	if _, problem := r.policyBaselinePath(); problem != "" {
		r.addRun(CodeScannerConfigInvalid, "scanner_policy", problem)
	}
}

// scannerFingerprint is the identity of one scanner result. It is the scanner's
// own SARIF fingerprint when it gave one (made unique across rules, files and
// repeated occurrences by adding them), else a hash of the scanner, rule, repository path, normalized
// text of the flagged line and an occurrence index. The line number is never part
// of it, so moving or reformatting code around a finding keeps its baseline entry.
func (r *runner) scannerFingerprint(scanner string, f externalFinding, abs string, line int, occurrence map[string]int) string {
	rel := ""
	if abs != "" {
		if rel = r.tree.Rel(abs); rel == "" {
			rel = filepath.ToSlash(abs)
		}
	}
	h := sha256.New()
	put := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	if f.Fingerprint != "" {
		// Two results may carry one scanner fingerprint (the same line text in two
		// places), so the occurrence index keeps them apart, as in the fallback below.
		key := scanner + "\x00own\x00" + f.Rule + "\x00" + rel + "\x00" + f.Fingerprint
		put(scannerFingerprintVersion, scanner, "own", f.Rule, rel, f.Fingerprint)
		// The first occurrence hashes as before, so existing baselines keep matching.
		if n := occurrence[key]; n > 0 {
			h.Write([]byte(strconv.Itoa(n)))
		}
		occurrence[key]++
		return scannerFingerprintVersion + ":" + hex.EncodeToString(h.Sum(nil))[:24]
	}
	text := f.Message // no readable line: the message is the identity
	if abs != "" {
		if lines := r.fileLines(abs); line >= 1 && line <= len(lines) {
			text = lines[line-1]
		}
	}
	key := scanner + "\x00" + f.Rule + "\x00" + rel + "\x00" + normalizeText(text)
	put(scannerFingerprintVersion, scanner, f.Rule, rel, normalizeText(text))
	h.Write([]byte(strconv.Itoa(occurrence[key])))
	occurrence[key]++
	return scannerFingerprintVersion + ":" + hex.EncodeToString(h.Sum(nil))[:24]
}

// finishExternal merges the findings of every scanner that ran: duplicates of
// one (scanner, fingerprint) collapse, the scanner baseline is written
// (--write-baseline) and applied, and an expired entry resurfaces its finding
// with AR9E5. ran maps each scanner that was run to whether it completed.
func (r *runner) finishExternal(all []scannerFinding, ran map[string]bool) {
	all = dedupeScannerFindings(all)
	findings := make([]Finding, len(all))
	failOn := r.scannerFailOns()
	for i := range all {
		findings[i] = all[i].Finding
		if t := failOn[all[i].scanner]; t != "" {
			findings[i].meta().ScannerFailOn = t
		}
		if p := r.tree.Rel(all[i].abs); all[i].abs != "" && p != "" {
			findings[i].meta().Path = p
		}
	}
	path := r.scannerBaselinePath()
	if r.opts.Scanner.WriteBaseline && path != "" {
		r.writeScannerBaseline(path, all, findings, ran)
	}
	rep := &Report{Findings: findings, Protected: r.protected, ConfigFile: r.display(r.configFilePath()), PolicyWarn: r.policyWarn(), Root: r.display(r.rootAbs())}
	var res BaselineResult
	if path != "" {
		b, err := LoadBaseline(path)
		if err != nil {
			r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		} else if b != nil {
			res = ApplyBaseline(rep, b, path, r.opts.Scanner.today(r.host))
		}
	}
	r.findings = append(r.findings, rep.Findings[len(findings):]...) // policy refusals of the baseline
	for i := range res.Expired {
		e := &res.Expired[i]
		r.addAt(CodeScannerBaselineExpired, path, fmt.Sprintf("[%s] the baseline entry for %s in %s expired on %s; the finding is reported again (remove the entry or renew it with --write-baseline)",
			e.Scanner, e.Rule, e.File, e.Expires))
	}
	for i := range findings {
		if findings[i].IsAccepted() {
			continue
		}
		r.findings = append(r.findings, findings[i])
	}
	if res.Accepted > 0 {
		r.log().Info(fmt.Sprintf("%d scanner finding(s) accepted by %s", res.Accepted, path))
	}
}

// dedupeScannerFindings keeps the first of each (scanner, fingerprint).
func dedupeScannerFindings(all []scannerFinding) []scannerFinding {
	seen := map[string]bool{}
	out := all[:0:0]
	for i := range all {
		f := all[i]
		key := f.scanner + "\x00" + f.Fingerprint()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

// writeScannerBaseline records every finding of the scanners that completed,
// through UpdateBaseline (so it shares the lint baseline's rules: an entry for
// a security rule, which AR011 is, needs a reason). Entries of a scanner that
// did not run or failed are kept as they are, so a missing tool does not erase
// its accepted findings.
func (r *runner) writeScannerBaseline(path string, all []scannerFinding, findings []Finding, ran map[string]bool) {
	prev, err := LoadBaseline(path)
	if err != nil {
		r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		return
	}
	opts := r.opts.Scanner
	recorded, skipped := opts.Filter.split(all, findings)
	// External marks the report as having looked for scanner findings, so
	// UpdateBaseline judges (and drops) the entries that match none of them;
	// entries of scanners that did not run are restored below.
	next, err := UpdateBaseline(&Report{Findings: recorded, External: true}, prev, opts.Reason)
	if err != nil {
		r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		return
	}
	scanners := map[string]scannerFinding{}
	for i := range all {
		f := all[i]
		scanners[f.Fingerprint()] = f
	}
	opts.describeEntries(next, baselineEntryIndex(prev), scanners)
	have := map[string]bool{}
	for i := range next.Entries {
		e := next.Entries[i]
		have[e.Fingerprint] = true
	}
	if prev != nil {
		for i := range prev.Entries {
			e := prev.Entries[i]
			if !have[e.Fingerprint] && (!ran[e.Scanner] || skipped[e.Fingerprint]) {
				next.Entries = append(next.Entries, e)
			}
		}
	}
	if err := next.Save(path); err != nil {
		r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		return
	}
	r.log().Info(fmt.Sprintf("scanner baseline %s: %d entries", path, len(next.Entries)))
}

// addAt records a finding attributed to a file of the project.
func (r *runner) addAt(code, abs, msg string) {
	if r.sev[code] == SeverityOff || r.ignore[code] {
		return
	}
	rule, _ := lookupRule(code) //nolint:errcheck // registered
	file := ""
	if abs != "" {
		file = r.display(abs)
	}
	r.findings = append(r.findings, Finding{
		Code: code, Name: rule.Name, Severity: r.sev[code], File: file, Line: 1,
		Message: strings.TrimSpace(msg), Root: r.display(r.rootAbs()),
	})
}

// scannerThreshold is the fail_on threshold a scanner finding was stamped with
// (its entry's fail_on, else [lint.scanner_policy] fail_on); ok is false for any
// other finding and for a scanner with no threshold of its own.
func (f *Finding) scannerThreshold() (Severity, bool) {
	if f.Meta == nil || f.Meta.ScannerFailOn == "" {
		return SeverityOff, false
	}
	threshold, ok := ParseSeverity(f.Meta.ScannerFailOn)
	return threshold, ok && threshold != SeverityOff
}

// failsAt reports whether f fails a run whose threshold is run. A scanner
// finding with a threshold of its own is judged by that alone, so --strict (run
// = warning) does not fail on a scanner warning when fail_on = "error".
func (f *Finding) failsAt(run Severity) bool {
	if t, ok := f.scannerThreshold(); ok {
		run = t
	}
	return f.Severity.AtLeast(run)
}

// scannerFailOns maps each scanner to its failure threshold ("" for the run's).
func (r *runner) scannerFailOns() map[string]string {
	pol := policyOf(&r.lc)
	out := map[string]string{}
	list := r.scanners()
	for i := range list {
		out[list[i].Name] = list[i].failOnOf(pol)
	}
	return out
}
