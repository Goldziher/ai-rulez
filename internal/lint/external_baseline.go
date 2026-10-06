package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
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
	// Today is the YYYY-MM-DD date entries expire against; empty reads
	// AI_RULEZ_TODAY, then the clock.
	Today string
	// ShowSuppressed keeps results the scanner marked suppressed, as info.
	ShowSuppressed bool
}

func (o ScannerOptions) today(host ambient.Host) string {
	for _, candidate := range []struct{ source, value string }{{"--today", o.Today}, {TodayEnv, host.GetEnv(TodayEnv)}} {
		if candidate.value == "" {
			continue
		}
		if _, err := time.Parse(dateLayout, candidate.value); err != nil {
			logger.Warn("Ignoring a malformed baseline date; expecting YYYY-MM-DD", "source", candidate.source, "value", candidate.value)
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
	if r.cfg.ConfigDir == "" {
		return ""
	}
	return filepath.Join(r.cfg.ConfigDir, ScannerBaselineFile)
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
			h.Write([]byte(fmt.Sprint(n)))
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
	h.Write([]byte(fmt.Sprint(occurrence[key])))
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
	for i := range all {
		findings[i] = all[i].Finding
		if p := r.tree.Rel(all[i].abs); all[i].abs != "" && p != "" {
			findings[i].meta().Path = p
		}
	}
	path := r.scannerBaselinePath()
	if r.opts.Scanner.WriteBaseline && path != "" {
		r.writeScannerBaseline(path, all, findings, ran)
	}
	rep := &Report{Findings: findings}
	var res BaselineResult
	if path != "" {
		b, err := LoadBaseline(path)
		if err != nil {
			r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		} else if b != nil {
			res = ApplyBaseline(rep, b, path, r.opts.Scanner.today(r.host))
		}
	}
	for _, e := range res.Expired {
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
		logger.Info(fmt.Sprintf("%d scanner finding(s) accepted by %s", res.Accepted, path))
	}
}

// dedupeScannerFindings keeps the first of each (scanner, fingerprint).
func dedupeScannerFindings(all []scannerFinding) []scannerFinding {
	seen := map[string]bool{}
	out := all[:0:0]
	for _, f := range all {
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
	next, err := UpdateBaseline(&Report{Findings: findings}, prev, r.opts.Scanner.Reason)
	if err != nil {
		r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		return
	}
	scanners := map[string]scannerFinding{}
	for _, f := range all {
		scanners[f.Fingerprint()] = f
	}
	for i := range next.Entries {
		if sf, ok := scanners[next.Entries[i].Fingerprint]; ok {
			next.Entries[i].Scanner, next.Entries[i].Rule = sf.scanner, sf.rule
		}
	}
	have := map[string]bool{}
	for _, e := range next.Entries {
		have[e.Fingerprint] = true
	}
	if prev != nil {
		for _, e := range prev.Entries {
			if !have[e.Fingerprint] && !ran[e.Scanner] {
				next.Entries = append(next.Entries, e)
			}
		}
	}
	if err := next.Save(path); err != nil {
		r.addRun(CodeScannerConfigInvalid, "baseline", sanitizeScannerText(err.Error()))
		return
	}
	logger.Info(fmt.Sprintf("scanner baseline %s: %d entries", path, len(next.Entries)))
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
