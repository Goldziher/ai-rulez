package lockfile

import (
	"sort"
	"strings"
)

// Scan results recorded in a lock.
const (
	ScanPass = "pass"
	ScanFail = "fail"
)

// Scan records what an external scanner reported over the staged content at lock
// time (docs/strict-validation.md, "External scanners"): which scanner, the
// digest of the content it scanned, and the outcome. It is a reviewer's aid, not
// a pin: the records sit outside the tree digest, and `lock` writes them from the
// scanner result cache without starting any program, so a scan that was never run
// has no record.
type Scan struct {
	Scanner string `toml:"scanner"`
	// Version is the scanner's --version line when the scan ran ("" when unknown).
	Version string `toml:"version,omitempty"`
	// Tree is the "sha256:<hex>" digest of the content staged for the scanner.
	Tree string `toml:"tree"`
	// Findings counts the results the scanner reported (suppressed ones excluded),
	// before the scanner baseline.
	Findings int `toml:"findings"`
	// MaxSeverity is the highest ai-rulez severity among them ("" for none).
	MaxSeverity string `toml:"max_severity,omitempty"`
	// Result is ScanPass or ScanFail: fail when a finding reaches the
	// [lint.scanner_policy] fail_on threshold (error by default).
	Result string `toml:"result"`
}

// SetScan adds s, or replaces the record of the same scanner.
func (f *File) SetScan(s Scan) {
	for i := range f.Scan {
		if f.Scan[i].Scanner == s.Scanner {
			f.Scan[i] = s
			return
		}
	}
	f.Scan = append(f.Scan, s)
}

// FindScan returns the record of scanner, or nil.
func (f *File) FindScan(scanner string) *Scan {
	if f == nil {
		return nil
	}
	for i := range f.Scan {
		if strings.EqualFold(f.Scan[i].Scanner, scanner) {
			return &f.Scan[i]
		}
	}
	return nil
}

func sortedScans(in []Scan) []Scan {
	if len(in) == 0 {
		return nil
	}
	out := append([]Scan(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Scanner < out[j].Scanner })
	return out
}
