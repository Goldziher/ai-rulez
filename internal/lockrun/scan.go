package lockrun

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// Bounds of what one scan reads of a tree: a tree larger than this is not shown
// or scanned in full, and the reviewer is told so.
const (
	MaxFiles    = 2000
	MaxFileSize = lint.MaxServedScanBytes
)

// maxScanFindingsListed bounds the findings kept per source; the counts are exact.
const maxScanFindingsListed = 20

// File is one file of a tree being scanned or approved.
type File struct {
	Path       string
	Size       int64
	Executable bool
	Data       []byte // nil when the file was too large or unreadable to scan
}

// WalkFiles lists the regular files below dir (only the named ones when only is
// given), skipping symlinks and VCS and cache bookkeeping. note explains what
// could not be listed.
func WalkFiles(dir string, only ...string) (files []File, note string) {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil || rel == "." {
			return nil //nolint:nilerr // the root itself
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || rel == ".cache_meta.json" || (len(only) > 0 && rel != only[0]) {
			return nil
		}
		if len(files) >= MaxFiles {
			note = fmt.Sprintf("more than %d files: the rest are not listed or scanned", MaxFiles)
			return filepath.SkipAll
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // listed as unreadable below
		}
		f := File{Path: filepath.ToSlash(rel), Size: info.Size(), Executable: info.Mode().Perm()&0o100 != 0}
		if info.Size() <= MaxFileSize {
			if data, readErr := safefs.ReadRegular(path); readErr == nil {
				f.Data = data
			}
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		note = "the tree cannot be read completely: " + err.Error()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, note
}

// ScanFiles runs the security scan over the files and returns the findings,
// errors first. A file with no Data (over the size limit) is not scanned; the
// caller lists it. A binary file is reported by the scan itself.
func ScanFiles(cfg *config.Config, name string, files []File) []lint.Finding {
	var served []lint.ServedFile
	for _, f := range files {
		if f.Data != nil {
			served = append(served, lint.ServedFile{Path: f.Path, Content: f.Data})
		}
	}
	found := lint.ScanServed(cfg, name, served, "")
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].Severity == lint.SeverityError) != (found[j].Severity == lint.SeverityError) {
			return found[i].Severity == lint.SeverityError
		}
		return found[i].Code < found[j].Code
	})
	return found
}

// ScanSummary is the security scan (AR001-AR009) of a tree about to be pinned.
type ScanSummary struct {
	Errors   int           `json:"errors"`
	Warnings int           `json:"warnings"`
	Findings []ScanFinding `json:"findings,omitempty"`
	// Refused is true when the error findings block the pin.
	Refused bool `json:"refused,omitempty"`
	// Accepted is true when --accept-findings let the pin through.
	Accepted bool `json:"accepted,omitempty"`
	// Note says what could not be scanned (a tree over the limits).
	Note string `json:"note,omitempty"`
}

// ScanFinding is one finding of a ScanSummary.
type ScanFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// ScanTree scans the cached tree in dir ("" = not cached, not scanned). Error
// findings refuse the pin unless accept is set.
func ScanTree(cfg *config.Config, name, dir string, accept bool) *ScanSummary {
	sum := &ScanSummary{}
	if dir == "" {
		sum.Note = "the new tree is not in the local cache: it was not scanned"
		return sum
	}
	files, note := WalkFiles(dir)
	sum.Note = note
	for _, f := range ScanFiles(cfg, name, files) {
		switch f.Severity {
		case lint.SeverityError:
			sum.Errors++
		case lint.SeverityWarning:
			sum.Warnings++
		}
		if len(sum.Findings) < maxScanFindingsListed && (f.Severity == lint.SeverityError || f.Severity == lint.SeverityWarning) {
			sum.Findings = append(sum.Findings, ScanFinding{Code: f.Code, Severity: string(f.Severity), File: f.File, Line: f.Line, Message: f.Message})
		}
	}
	if sum.Errors > 0 {
		sum.Refused, sum.Accepted = !accept, accept
	}
	return sum
}

// SafeText makes untrusted text safe to print: control characters (other than
// tab), bidirectional controls, zero-width and tag characters are replaced by
// their \u escape, so content cannot hide text from the reviewer.
func SafeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r), r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E,
			r >= 0x2060 && r <= 0x2069, r == 0xFEFF, r >= 0xE0000 && r <= 0xE007F:
			fmt.Fprintf(&b, "\\u{%X}", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PinScan is the security scan of one remote tree `lock` was about to pin.
type PinScan struct {
	Kind, Name string
	Summary    *ScanSummary
}

// scanNewPins runs the security scan (AR001-AR009, as `update` does) over the
// tree of every remote entry that `lock` is about to pin to something new: an
// entry with no pin yet, or a different commit or digest. An unchanged pin was
// scanned when it was pinned. It returns the scans with error findings (refused,
// or accepted with accept) in order, and prints each to report.
func scanNewPins(cfg *config.Config, current, next *lockfile.File, accept bool, report io.Writer) (scans []PinScan, refused int) {
	wants := map[string]lockfile.Want{}
	lockable := includes.Lockable(cfg)
	for i := range lockable {
		w := &lockable[i]
		wants[w.Kind+"\x00"+w.Name] = *w
	}
	specs := map[string]skillsource.Spec{}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		specs[spec.Name] = spec
	}
	groups := []struct {
		kind    string
		entries []lockfile.Entry
	}{{lockfile.KindInclude, next.Include}, {lockfile.KindSkill, next.Skill}, {lockfile.KindSource, next.Source}}
	for _, g := range groups {
		for i := range g.entries {
			e := &g.entries[i]
			if unchangedPin(current, g.kind, e) {
				continue
			}
			dir := ""
			if g.kind == lockfile.KindSource {
				if spec, ok := specs[e.Name]; ok {
					dir = skillsource.CachedTreeDir(spec, e.Commit, "")
				}
			} else if w, ok := wants[g.kind+"\x00"+e.Name]; ok {
				dir = includes.CachedTreeDir(cfg, w)
			}
			if dir == "" {
				continue // not in the local cache (a local source, or nothing fetched): nothing to scan
			}
			sum := ScanTree(cfg, e.Name, dir, accept)
			if sum.Errors == 0 {
				continue
			}
			printPinScan(report, g.kind, e.Name, sum)
			scans = append(scans, PinScan{Kind: g.kind, Name: e.Name, Summary: sum})
			if sum.Refused {
				refused++
			}
		}
	}
	return scans, refused
}

func unchangedPin(current *lockfile.File, kind string, e *lockfile.Entry) bool {
	if current == nil {
		return false
	}
	old := current.Find(kind, e.Name)
	return old != nil && old.Commit == e.Commit && old.Digest == e.Digest
}

// printPinScan reports the findings that stop (or, accepted, ride along with) a pin.
func printPinScan(w io.Writer, kind, name string, sum *ScanSummary) {
	verdict := "accepted with --accept-findings"
	if sum.Refused {
		verdict = "refused"
	}
	fmt.Fprintf(w, "%s %s: the security scan found %d error(s), %d warning(s): %s\n", kind, name, sum.Errors, sum.Warnings, verdict) //nolint:errcheck // a report to the terminal
	for _, f := range sum.Findings {
		fmt.Fprintf(w, "  %s %s %s:%d %s\n", f.Code, f.Severity, SafeText(f.File), f.Line, SafeText(f.Message)) //nolint:errcheck // a report to the terminal
	}
}
