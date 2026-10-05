package lint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// Fixes are mechanical corrections a finding can carry. They exist only for
// cases that are deterministic and local: a frontmatter key spelled with the
// wrong separator, a skill name that is not lowercase-hyphen, a script or hook
// without the executable bit. Applying them is idempotent (an edit states the
// line it replaces and is skipped when the line changed), touches authored
// sources only (never a generated output), and never applies to security
// findings.

// FixConfidence says how safe it is to apply a fix unattended.
type FixConfidence string

// Fix confidences. A safe fix cannot change what an item means; an unsafe one
// can (renaming a skill changes the name it is invoked by).
const (
	FixSafe   FixConfidence = "safe"
	FixUnsafe FixConfidence = "unsafe"
)

// Edit replaces one whole line. Old is the line as the lint run saw it (without
// its line ending); the edit is skipped when the file no longer has it there.
type Edit struct {
	File string // absolute
	Line int    // 1-based
	Old  string
	New  string
}

// Chmod adds the executable bit (where the read bit is set) to a file.
type Chmod struct {
	File string // absolute
}

// Fix is the correction attached to a finding.
type Fix struct {
	Description string
	Confidence  FixConfidence
	Edits       []Edit
	Chmods      []Chmod
}

// addFix records a finding like add and attaches fix to it when it was recorded
// (not disabled or suppressed).
func (r *runner) addFix(fix *Fix, code, abs string, line int, format string, args ...any) {
	before := len(r.findings)
	r.add(code, abs, line, format, args...)
	if len(r.findings) > before {
		r.findings[len(r.findings)-1].meta().Fix = fix
	}
}

// Fixable reports whether the finding carries a fix.
func (f *Finding) Fixable() bool { return f.Meta != nil && f.Meta.Fix != nil }

// FixOptions controls ApplyFixes.
type FixOptions struct {
	// Unsafe also applies FixUnsafe fixes.
	Unsafe bool
	// DryRun computes the result and diff without touching any file.
	DryRun bool
	// EditRoot, when set, limits text edits to files below it (the authored
	// source directory); a chmod is not limited.
	EditRoot string
	// Refuse returns a reason when the file must not be modified (a generated
	// output, a file outside the authored sources), or "".
	Refuse func(abs string) string
}

// FixApplied describes one fix that was (or, in a dry run, would be) applied.
type FixApplied struct {
	Fingerprint string
	Code        string
	File        string
	Line        int
	Description string
	Confidence  FixConfidence
}

// FixSkipped describes a finding whose fix was not applied, and why.
type FixSkipped struct {
	Code   string
	File   string
	Line   int
	Reason string
}

// FixResult is the outcome of ApplyFixes.
type FixResult struct {
	Applied []FixApplied
	Skipped []FixSkipped
	// Files lists the files that were (or would be) changed, sorted.
	Files []string
	// Diff is a unified diff of the text edits plus a `chmod` line per mode change.
	Diff string
}

// ApplyFixes applies the fixes carried by findings. Accepted findings are left
// alone, security findings are never fixed, and unsafe fixes need o.Unsafe.
func ApplyFixes(findings []Finding, o FixOptions) (FixResult, error) {
	var res FixResult
	plan := planFixes(findings, o, &res)

	files := make([]string, 0, len(plan.edits))
	for f := range plan.edits {
		files = append(files, f)
	}
	sort.Strings(files)
	changed := map[string]bool{}
	failed := map[string]string{} // file -> reason, for every fix that touches it
	var diff strings.Builder
	for _, file := range files {
		diffText, err := applyEdits(file, plan.edits[file], o.DryRun)
		if err != nil {
			failed[file] = err.Error()
			continue
		}
		if diffText != "" {
			changed[file] = true
			diff.WriteString(diffText)
		}
	}
	chmods := make([]string, 0, len(plan.chmods))
	for f := range plan.chmods {
		chmods = append(chmods, f)
	}
	sort.Strings(chmods)
	for _, file := range chmods {
		line, did, err := applyChmod(file, o.DryRun)
		if err != nil {
			failed[file] = err.Error()
			continue
		}
		if did {
			changed[file] = true
			diff.WriteString(line)
		}
	}

	for i := range plan.candidates {
		p := &plan.candidates[i]
		if reason := failedReason(p, failed); reason != "" {
			res.Skipped = append(res.Skipped, FixSkipped{Code: p.code, File: p.file, Line: p.line, Reason: reason})
			continue
		}
		if anyChanged(p.targets, changed) {
			res.Applied = append(res.Applied, p.applied)
		}
	}
	for f := range changed {
		res.Files = append(res.Files, f)
	}
	sort.Strings(res.Files)
	res.Diff = diff.String()
	return res, nil
}

type plannedFix struct {
	applied FixApplied
	code    string
	file    string
	line    int
	targets []string
}

type fixPlan struct {
	edits      map[string][]Edit
	chmods     map[string]bool
	candidates []plannedFix
}

func planFixes(findings []Finding, o FixOptions, res *FixResult) fixPlan {
	plan := fixPlan{edits: map[string][]Edit{}, chmods: map[string]bool{}}
	taken := map[string]Edit{} // "file:line" -> the edit already planned there
	for i := range findings {
		f := &findings[i]
		if !f.Fixable() || f.IsAccepted() {
			continue
		}
		skip := func(reason string) {
			res.Skipped = append(res.Skipped, FixSkipped{Code: f.Code, File: f.RepoPath(), Line: f.Line, Reason: reason})
		}
		fix := f.Meta.Fix
		switch {
		case isSecurityCode(f.Code):
			skip("security findings are never fixed automatically")
			continue
		case fix.Confidence == FixUnsafe && !o.Unsafe:
			skip("unsafe fix; use --fix-unsafe")
			continue
		}
		if reason := refuseFix(fix, o); reason != "" {
			skip(reason)
			continue
		}
		p := plannedFix{code: f.Code, file: f.RepoPath(), line: f.Line, applied: FixApplied{
			Fingerprint: f.Fingerprint(), Code: f.Code, File: f.RepoPath(), Line: f.Line,
			Description: fix.Description, Confidence: fix.Confidence,
		}}
		conflict := ""
		for _, e := range fix.Edits {
			key := fmt.Sprintf("%s:%d", e.File, e.Line)
			if prev, dup := taken[key]; dup && prev != e {
				conflict = fmt.Sprintf("another fix already rewrites line %d", e.Line)
				break
			}
		}
		if conflict != "" {
			skip(conflict)
			continue
		}
		for _, e := range fix.Edits {
			key := fmt.Sprintf("%s:%d", e.File, e.Line)
			if _, dup := taken[key]; !dup {
				taken[key] = e
				plan.edits[e.File] = append(plan.edits[e.File], e)
			}
			p.targets = append(p.targets, e.File)
		}
		for _, c := range fix.Chmods {
			plan.chmods[c.File] = true
			p.targets = append(p.targets, c.File)
		}
		plan.candidates = append(plan.candidates, p)
	}
	return plan
}

func refuseFix(fix *Fix, o FixOptions) string {
	for _, e := range fix.Edits {
		if o.EditRoot != "" && !underDir(gitutil.Resolve(e.File), o.EditRoot) {
			return "the file is not an authored source under " + filepath.ToSlash(o.EditRoot)
		}
		if o.Refuse != nil {
			if r := o.Refuse(e.File); r != "" {
				return r
			}
		}
	}
	for _, c := range fix.Chmods {
		if o.Refuse != nil {
			if r := o.Refuse(c.File); r != "" {
				return r
			}
		}
	}
	return ""
}

func anyChanged(targets []string, changed map[string]bool) bool {
	for _, t := range targets {
		if changed[t] {
			return true
		}
	}
	return false
}

func failedReason(p *plannedFix, failed map[string]string) string {
	for _, t := range p.targets {
		if reason, bad := failed[t]; bad {
			return reason
		}
	}
	return ""
}

var errStale = errors.New("the file changed since the lint run; nothing was rewritten")

// applyEdits rewrites the lines of one file. It returns the unified diff of the
// change, or "" when no edit applied (already fixed).
func applyEdits(file string, edits []Edit, dry bool) (string, error) {
	data, err := os.ReadFile(file) //nolint:gosec // a source file the lint run read
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	info, err := os.Stat(file)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", file, err)
	}
	oldLines := strings.Split(string(data), "\n")
	newLines := append([]string(nil), oldLines...)
	sort.Slice(edits, func(i, j int) bool { return edits[i].Line < edits[j].Line })
	applied := 0
	for _, e := range edits {
		if e.Line < 1 || e.Line > len(newLines) {
			return "", errStale
		}
		cur := newLines[e.Line-1]
		cr := ""
		if strings.HasSuffix(cur, "\r") {
			cur, cr = strings.TrimSuffix(cur, "\r"), "\r"
		}
		switch cur {
		case e.New:
			continue // already fixed
		case e.Old:
			newLines[e.Line-1] = e.New + cr
			applied++
		default:
			return "", errStale
		}
	}
	if applied == 0 {
		return "", nil
	}
	if !dry {
		if err := gitutil.WriteFileAtomic(file, []byte(strings.Join(newLines, "\n")), info.Mode().Perm()); err != nil {
			return "", fmt.Errorf("write %s: %w", file, err)
		}
	}
	return unifiedDiff(file, oldLines, newLines), nil
}

// applyChmod adds the executable bit. did is false when it was already set.
func applyChmod(file string, dry bool) (line string, did bool, err error) {
	info, err := os.Stat(file)
	if err != nil {
		return "", false, fmt.Errorf("stat %s: %w", file, err)
	}
	mode := info.Mode().Perm()
	want := mode | ((mode & 0o444) >> 2)
	var sb strings.Builder
	if want != mode {
		if !dry {
			if err := os.Chmod(file, want); err != nil {
				return "", false, fmt.Errorf("chmod %s: %w", file, err)
			}
		}
		fmt.Fprintf(&sb, "chmod %04o -> %04o %s\n", mode, want, filepath.ToSlash(file))
		did = true
	}
	// The mode lint reads for a tracked file is the index's, so stage the bit.
	if !dry {
		staged, serr := gitutil.StageExecutable(file)
		if serr != nil {
			return "", false, fmt.Errorf("stage %s: %w", file, serr)
		}
		if staged && !did {
			fmt.Fprintf(&sb, "git update-index --chmod=+x %s\n", filepath.ToSlash(file))
			did = true
		}
	}
	return sb.String(), did, nil
}

// unifiedDiff renders a diff of two equally long line slices (every fix rewrites
// lines in place) with three lines of context.
func unifiedDiff(name string, oldLines, newLines []string) string {
	const context = 3
	var changed []int
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", filepath.ToSlash(name), filepath.ToSlash(name))
	for i := 0; i < len(changed); {
		start := max(changed[i]-context, 0)
		end := min(changed[i]+context, len(oldLines)-1)
		j := i + 1
		for j < len(changed) && changed[j]-context <= end+1 {
			end = min(changed[j]+context, len(oldLines)-1)
			j++
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", start+1, end-start+1, start+1, end-start+1)
		for l := start; l <= end; l++ {
			if oldLines[l] == newLines[l] {
				fmt.Fprintf(&sb, " %s\n", oldLines[l])
				continue
			}
			fmt.Fprintf(&sb, "-%s\n+%s\n", oldLines[l], newLines[l])
		}
		i = j
	}
	return sb.String()
}

// FixedFingerprints returns the fingerprints of the applied fixes.
func (r FixResult) FixedFingerprints() map[string]bool {
	out := map[string]bool{}
	for _, a := range r.Applied {
		out[a.Fingerprint] = true
	}
	return out
}

// DropFixed removes from rep the findings whose fix was applied. A dry run
// changes nothing, so nothing is dropped.
func (r FixResult) DropFixed(rep *Report, dry bool) {
	if dry {
		return
	}
	fixed := r.FixedFingerprints()
	kept := rep.Findings[:0:0]
	for i := range rep.Findings {
		if !fixed[rep.Findings[i].Fingerprint()] {
			kept = append(kept, rep.Findings[i])
		}
	}
	rep.Findings = kept
}

var fmKeyLineRe = regexp.MustCompile(`^(\s*)(["']?)([^:"'\s]+)(["']?)(\s*:.*)$`)

// renameKeyLine returns line with its frontmatter key replaced, or false when
// the line does not start with that key.
func renameKeyLine(line, from, to string) (string, bool) {
	m := fmKeyLineRe.FindStringSubmatch(line)
	if len(m) == 0 || m[3] != from {
		return "", false
	}
	return m[1] + m[2] + to + m[4] + m[5], true
}

var fmNameLineRe = regexp.MustCompile(`^(\s*name\s*:\s*)(["']?)([^"'#]*?)(["']?)(\s*(?:#.*)?)$`)

// renameNameLine returns a frontmatter `name:` line with the value replaced.
func renameNameLine(line, to string) (string, bool) {
	m := fmNameLineRe.FindStringSubmatch(line)
	if len(m) == 0 || m[2] != m[4] {
		return "", false
	}
	return m[1] + m[2] + to + m[4] + m[5], true
}

var nonNameRe = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeSkillName converts a name to lowercase letters, digits and single
// hyphens, at most 64 characters; "" when nothing usable remains.
func normalizeSkillName(name string) string {
	n := strings.Trim(nonNameRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(n) > maxSkillNameLen {
		n = strings.Trim(n[:maxSkillNameLen], "-")
	}
	return n
}

// chmodFix is the safe fix for a missing executable bit.
func chmodFix(abs string) *Fix {
	return &Fix{Description: "make the file executable (chmod +x)", Confidence: FixSafe, Chmods: []Chmod{{File: abs}}}
}

// renameSkillFix is the unsafe fix for AR804: rewrite the frontmatter name. It
// is unsafe because the name is how the skill is invoked and referenced.
func (r *runner) renameSkillFix(it *item, d doc, line int, to, from string) *Fix {
	if to == "" || to == from || !skillNameRe.MatchString(to) || line < 1 || line > len(d.lines) {
		return nil
	}
	newLine, ok := renameNameLine(d.lines[line-1], to)
	if !ok || !strings.Contains(d.lines[line-1], from) {
		return nil
	}
	return &Fix{
		Description: fmt.Sprintf("rename skill %q to %q", from, to),
		Confidence:  FixUnsafe,
		Edits:       []Edit{{File: it.abs, Line: line, Old: d.lines[line-1], New: newLine}},
	}
}
