package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// A baseline records findings a team has accepted. Entries are keyed by the
// finding fingerprint (code, path and normalized line text, never the line
// number), so an accepted finding stays accepted when the file is edited around
// it. Only findings absent from the baseline count toward the exit code; entries
// that match nothing are stale and can be made to fail with --strict-baseline,
// which turns the baseline into a ratchet.

// BaselineFile is the default baseline, relative to the configuration directory.
const BaselineFile = "lint-baseline.json"

const (
	baselineVersion = 1
	dateLayout      = "2006-01-02"
)

// BaselineEntry accepts one finding.
type BaselineEntry struct {
	Fingerprint string `json:"fingerprint"`
	// Code, File and Message describe the finding for a reviewer of the
	// baseline; only Fingerprint is matched.
	Code    string `json:"code"`
	File    string `json:"file"`
	Message string `json:"message,omitempty"`
	// Reason says why the finding is accepted.
	Reason string `json:"reason,omitempty"`
	// Expires is a YYYY-MM-DD date after which the entry no longer accepts the
	// finding (inclusive: it still applies on that day).
	Expires string `json:"expires,omitempty"`
	// Scanner and Rule name the external scanner and its rule id; only the
	// scanner baseline sets them.
	Scanner string `json:"scanner,omitempty"`
	Rule    string `json:"rule,omitempty"`
}

// Baseline is the committed set of accepted findings.
type Baseline struct {
	Version int             `json:"version"`
	Entries []BaselineEntry `json:"entries"`
}

// LoadBaseline reads a baseline. A missing file is (nil, nil).
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the user's own baseline file
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // absent is a normal state
	}
	if err != nil {
		return nil, fmt.Errorf("read baseline %s: %w", path, err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse baseline %s: %w", path, err)
	}
	if b.Version != baselineVersion {
		return nil, fmt.Errorf("baseline %s: unsupported version %d (want %d)", path, b.Version, baselineVersion)
	}
	for i := range b.Entries {
		e := &b.Entries[i]
		if e.Fingerprint == "" {
			return nil, fmt.Errorf("baseline %s: entry %d has no fingerprint", path, i)
		}
		if e.Expires != "" {
			if _, perr := time.Parse(dateLayout, e.Expires); perr != nil {
				return nil, fmt.Errorf("baseline %s: entry %s: expires %q is not a YYYY-MM-DD date", path, e.Fingerprint, e.Expires)
			}
		}
	}
	return &b, nil
}

// Save writes the baseline atomically with stable ordering and a trailing newline.
func (b *Baseline) Save(path string) error {
	sort.SliceStable(b.Entries, func(i, j int) bool {
		x, y := b.Entries[i], b.Entries[j]
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Code != y.Code {
			return x.Code < y.Code
		}
		return x.Fingerprint < y.Fingerprint
	})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if b.Entries == nil {
		b.Entries = []BaselineEntry{}
	}
	if err := enc.Encode(b); err != nil {
		return fmt.Errorf("encode baseline: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create baseline directory: %w", err)
	}
	if err := gitutil.WriteFileAtomic(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

// BaselineResult is what applying a baseline to one report found.
type BaselineResult struct {
	// Path is the baseline file that was applied.
	Path string `json:"path"`
	// Accepted counts findings the baseline accepted.
	Accepted int `json:"accepted"`
	// Stale lists entries that match no finding any more.
	Stale []BaselineEntry `json:"stale,omitempty"`
	// Expired lists entries past their date that match a finding; that finding
	// counts as new again.
	Expired []BaselineEntry `json:"expired,omitempty"`
}

// analyzerRan reports whether the analyzer of code ran under a selection (nil
// selection: all). An entry of an analyzer that did not run cannot match
// anything, so it is not stale.
func analyzerRan(selection []string, code string) bool {
	return selection == nil || AnalyzerSelected(selection, AnalyzerFor(code).Name)
}

// ApplyBaseline marks the findings of r that b accepts. today is a YYYY-MM-DD
// date supplied by the caller, so the result is deterministic.
func ApplyBaseline(r *Report, b *Baseline, path, today string) BaselineResult {
	res := BaselineResult{Path: path}
	if b == nil {
		return res
	}
	matched := map[string]bool{}
	refused := map[string]bool{}
	byFP := map[string]BaselineEntry{}
	for _, e := range b.Entries {
		byFP[e.Fingerprint] = e
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		e, ok := byFP[f.Fingerprint()]
		if !ok {
			continue
		}
		matched[e.Fingerprint] = true
		if e.Expires != "" && today > e.Expires {
			res.Expired = append(res.Expired, e)
			continue
		}
		if r.Protected[f.Code] {
			refused[f.Code] = true // policy wins: the finding stays reported
			continue
		}
		m := f.meta()
		m.Accepted, m.AcceptReason = true, e.Reason
		res.Accepted++
	}
	for _, e := range b.Entries {
		if !matched[e.Fingerprint] && analyzerRan(r.Analyzers, e.Code) {
			res.Stale = append(res.Stale, e)
		}
	}
	r.refuse(routeBaseline, sortedSet(refused))
	return res
}

// UpdateBaseline builds the baseline that accepts every finding of r. Entries
// that still match keep their reason and expiry; stale entries are dropped,
// except the entries of an analyzer that did not run (r.Analyzers), which are
// kept untouched. A
// new entry gets reason. It returns an error when an entry for a security rule
// (AR0xx) would be added without a reason: accepting a security finding must be
// explained.
func UpdateBaseline(r *Report, prev *Baseline, reason string) (*Baseline, error) {
	old := map[string]BaselineEntry{}
	if prev != nil {
		for _, e := range prev.Entries {
			old[e.Fingerprint] = e
		}
	}
	out := &Baseline{Version: baselineVersion, Entries: []BaselineEntry{}}
	seen := map[string]bool{}
	var unexplained []string
	if prev != nil {
		for _, e := range prev.Entries {
			if !analyzerRan(r.Analyzers, e.Code) {
				out.Entries = append(out.Entries, e) // its analyzer did not run: keep the entry as it is
				seen[e.Fingerprint] = true
			}
		}
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		fp := f.Fingerprint()
		if fp == "" || seen[fp] {
			continue
		}
		seen[fp] = true
		if e, ok := old[fp]; ok {
			e.Code, e.File, e.Message = f.Code, f.RepoPath(), f.Message
			out.Entries = append(out.Entries, e)
			continue
		}
		if isSecurityCode(f.Code) && strings.TrimSpace(reason) == "" {
			unexplained = append(unexplained, fmt.Sprintf("%s:%d %s", f.RepoPath(), f.Line, f.Code))
			continue
		}
		out.Entries = append(out.Entries, BaselineEntry{
			Fingerprint: fp, Code: f.Code, File: f.RepoPath(), Message: f.Message, Reason: strings.TrimSpace(reason),
		})
	}
	if len(unexplained) > 0 {
		return nil, fmt.Errorf("accepting security findings needs a reason (--baseline-reason): %s", strings.Join(unexplained, ", "))
	}
	return out, nil
}

// Budgets is the [lint.tolerate] table: it caps how many findings of a rule are tolerated: up to max findings
// of a code that are not accepted by a baseline do not count toward the exit
// code; one more and every finding of that code counts again. Lower the number
// over time to ratchet a rule down.
type Budgets map[string]int

// ResolveBudgets maps codes or names to canonical codes.
func ResolveBudgets(raw map[string]int) Budgets {
	out := Budgets{}
	for key, limit := range raw {
		if rule, ok := lookupRule(key); ok {
			out[rule.Code] = limit
		}
	}
	return out
}

// Without drops the budgets of the protected codes, which a policy never lets a
// repository tolerate, and lists the codes it dropped.
func (b Budgets) Without(protected map[string]bool) (Budgets, []string) {
	if len(protected) == 0 {
		return b, nil
	}
	out := Budgets{}
	dropped := map[string]bool{}
	for code, limit := range b {
		if protected[code] {
			dropped[code] = true
			continue
		}
		out[code] = limit
	}
	return out, sortedSet(dropped)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BudgetExcess describes a rule over its tolerated finding count.
type BudgetExcess struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
	Max   int    `json:"max"`
}

// Excess lists the budgeted rules whose unaccepted findings exceed the cap.
func (b Budgets) Excess(findings []Finding) []BudgetExcess {
	counts := map[string]int{}
	for i := range findings {
		if !findings[i].IsAccepted() {
			counts[findings[i].Code]++
		}
	}
	var out []BudgetExcess
	for code, limit := range b {
		if counts[code] > limit {
			out = append(out, BudgetExcess{Code: code, Count: counts[code], Max: limit})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// FailedWith is Failed with the baseline and budgets applied: accepted
// findings never fail, and a rule within its budget is tolerated.
func FailedWith(findings []Finding, failOn string, budgets Budgets) bool {
	return FailedWithExcess(findings, failOn, budgets, budgets.Excess(findings))
}

// FailedWithExcess is FailedWith with the over-budget rules computed by the
// caller, for a run that narrows findings (changed-only) after judging budgets
// against the full set.
func FailedWithExcess(findings []Finding, failOn string, budgets Budgets, excess []BudgetExcess) bool {
	over := map[string]bool{}
	for _, e := range excess {
		over[e.Code] = true
	}
	var counting []Finding
	for i := range findings {
		f := findings[i]
		if _, budgeted := budgets[f.Code]; budgeted && !over[f.Code] {
			continue
		}
		counting = append(counting, f)
	}
	return Failed(counting, failOn)
}
