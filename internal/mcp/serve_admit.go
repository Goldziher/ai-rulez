package mcp

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Refusal records why a skill is not served.
type Refusal struct {
	Name string
	// View is the serve view the skill was refused in ("" for the default view).
	View string
	// Code is the strict-validation code of the reason (AR001.. from the security
	// scan, AR995 for a lock mismatch).
	Code   string
	Reason string
}

// CodeServedLockMismatch is the strict-validation code of a served skill whose
// digest differs from (or is missing in) ai-rulez.lock under [lock] enforce.
const CodeServedLockMismatch = "AR995"

// Admission decides which skills of a catalog may be served: each skill is
// scanned with the security rules, and under lock enforcement its digest must
// equal the one in the lock.
type Admission struct {
	// Config supplies the [lint] settings of the scan; may be nil.
	Config *config.Config
	// Lock is ai-rulez.lock (nil when the project has none).
	Lock *lockfile.File
	// Enforce refuses a skill that the lock does not pin with exactly its digest.
	Enforce bool
	// Pinning marks a build that computes the lock itself (lock, lock --check):
	// without a lock there is nothing to approve against yet, and the approval
	// check of those commands runs separately (govview.ApprovalChanges).
	Pinning bool
	// View is the serve view whose pins the lock is read at (see ServeSetup.ViewKey).
	View string
	// Signatures applies [signing] require = ["served", "skill"] (serve_signing.go);
	// nil admits every skill.
	Signatures *SignatureGate
	// DefaultTrust is the scan level for a skill that names none: "warn" for
	// skills authored in the project, "error" for everything that came from a
	// remote. nil means "warn".
	DefaultTrust func(*CatalogSkill) string
	// Log receives the refusals and scan findings; nil is the CLI's logger.
	Log logger.Logger
	// Now is the clock approvals are judged by; nil is the wall clock.
	Now func() time.Time
	// PolicyDenied maps a digest the organization policy denies
	// (sources.deny_digests) to the policy's message; a skill with that digest
	// is refused with AR747.
	PolicyDenied map[string]string
	// Authored holds the skill:<name> items of the skills authored in the
	// project (authoredSkillItems); nil skips the authored-item gate.
	Authored map[string]lockfile.Item
	// Verdicts remembers which content the security scan found nothing in, so
	// a later build does not scan it again; nil scans every skill. Only a scan
	// without any finding is recorded, and a verdict stands for exactly the
	// bytes and the trust level it was recorded for.
	Verdicts VerdictCache
	// policy memoises the [governance] policy across the skills of one Admit.
	policy *admissionPolicy
}

// VerdictCache holds the scan verdicts of Admission. Implementations
// authenticate what they return: a verdict is trusted without a new scan.
type VerdictCache interface {
	// Clean reports whether the scan found nothing for the verdict key.
	Clean(key string) bool
	// MarkClean records that the scan found nothing for the verdict key.
	MarkClean(key string)
}

// Admit returns a catalog holding only the admitted skills. Refused skills are
// kept by name so load_skill can say why, and are logged on stderr. The input
// catalog is not modified.
func (c *Catalog) Admit(a Admission) *Catalog {
	out := &Catalog{
		Profile: c.Profile, Preset: c.Preset,
		byName: map[string]*CatalogSkill{}, byURI: map[string]*CatalogSkill{}, byFile: map[string]*CatalogFile{},
		refused: map[string]Refusal{}, reports: append([]ScanReport(nil), c.reports...),
		discovery: c.discovery, repo: c.repo,
	}
	for name, r := range c.refused {
		out.refused[name] = r
	}
	a.policy = &admissionPolicy{}
	for _, skill := range c.skills {
		cp := *skill
		r := a.check(&cp)
		if len(cp.unscannable) > 0 {
			out.reports = append(out.reports, ScanReport{
				Skill: cp.Name, View: a.View, Level: cp.scanLevel, Findings: cp.unscannable, Unserved: cp.Unscanned,
			})
		}
		if r != nil {
			r.View = a.View
			out.refused[cp.Name] = *r
			logger.Or(a.Log).Warn("Refusing to serve a skill", "skill", cp.Name, "code", r.Code, "reason", r.Reason)
			continue
		}
		out.skills = append(out.skills, &cp)
		out.byName[cp.Name] = &cp
		out.byURI[cp.URI] = &cp
		for j := range cp.Files {
			out.byFile[cp.Files[j].URI] = &cp.Files[j]
		}
	}
	return out
}

// ScanReport lists the files of one skill the security scan could not read
// (AR989), with the trust level the skill was scanned at and the files the
// server leaves out because of it.
type ScanReport struct {
	Skill string
	// View is the serve view the skill was scanned in ("" for the default view).
	View string
	// Level is the scan level, config.TrustWarn or config.TrustError.
	Level    string
	Findings []lint.Finding
	// Unserved are the files not served (non-empty only at trust=error).
	Unserved []string
}

// ScanReports lists the unscannable-file findings of every skill that was
// scanned, served or refused.
func (c *Catalog) ScanReports() []ScanReport { return c.reports }

// Refusal reports why a skill was refused, if it was.
func (c *Catalog) Refusal(name string) (Refusal, bool) {
	r, ok := c.refused[name]
	return r, ok
}

// Refusals lists every refused skill by name.
func (c *Catalog) Refusals() []Refusal {
	out := make([]Refusal, 0, len(c.refused))
	for _, r := range c.refused {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (a Admission) check(s *CatalogSkill) *Refusal {
	if reason, denied := a.PolicyDenied[s.LockDigest]; denied && s.LockDigest != "" {
		return &Refusal{Name: s.Name, Code: lint.CodeDigestDenied, Reason: reason}
	}
	if r := a.scan(s); r != nil {
		return r
	}
	if r := a.Signatures.Check(s); r != nil {
		return r
	}
	entry := servedPin(a.Lock, a.View, s.Name)
	s.Locked = entry != nil && entry.Digest == s.LockDigest
	if !a.Enforce {
		return a.admitApproval(s)
	}
	switch {
	case entry == nil:
		return &Refusal{Name: s.Name, Code: CodeServedLockMismatch, Reason: fmt.Sprintf("[lock] enforce is on and %s does not pin this skill; review it, then run `ai-rulez lock`", lockfile.FileName)}
	case !s.Locked:
		return &Refusal{Name: s.Name, Code: CodeServedLockMismatch, Reason: fmt.Sprintf("digest %s differs from the lock's %s; the skill changed since it was locked (run `ai-rulez lock` only after reviewing the change)", s.LockDigest, entry.Digest)}
	}
	return a.admitApproval(s)
}

// servedPin finds the lock's pin of a served skill in a view. An entry written
// before views existed has no view and applies to every view, so it is the
// fallback; its digest is still compared, so it can only agree with what is served.
func servedPin(lock *lockfile.File, view, name string) *lockfile.Entry {
	if e := lock.FindView(lockfile.KindServed, name, view); e != nil || view == "" {
		return e
	}
	return lock.FindView(lockfile.KindServed, name, "")
}

// dropUnscannable removes the files the security scan could not read from what
// the skill serves (trust=error: content from a remote must be scanned to be
// served). The skill's digest still covers them, so the lock sees them change.
func (s *CatalogSkill) dropUnscannable() {
	kept := make([]CatalogFile, 0, len(s.Files))
	for i := range s.Files {
		if lint.UnscannableReason(s.Files[i].Content) != "" {
			s.Unscanned = append(s.Unscanned, s.Files[i].RelPath)
			continue
		}
		kept = append(kept, s.Files[i])
	}
	sort.Strings(s.Unscanned)
	s.Files = kept
}

// scanLevelOf is the trust level a skill is scanned at: its own, else the
// admission's default for it.
func (a Admission) scanLevelOf(s *CatalogSkill) string {
	if s.Trust != "" {
		return s.Trust
	}
	if a.DefaultTrust != nil {
		return a.DefaultTrust(s)
	}
	return config.TrustWarn
}

// verdictKey is the key of the scan verdict of a skill at a level.
func verdictKey(level string, files []CatalogFile) string {
	sums := make([]indexcache.FileSum, 0, len(files))
	for i := range files {
		sums = append(sums, indexcache.SumOfContent(files[i].RelPath, files[i].Content))
	}
	return indexcache.VerdictKey(level, sums)
}

// knownClean asks the verdict cache whether the scan found nothing in the skill
// at this level before. key is the verdict key, "" without a cache.
func (a Admission) knownClean(s *CatalogSkill, level string) (key string, known bool) {
	if a.Verdicts == nil {
		return "", false
	}
	key = verdictKey(level, s.Files)
	if !a.Verdicts.Clean(key) {
		return key, false
	}
	s.scanLevel = level
	return key, true
}

func (a Admission) scan(s *CatalogSkill) *Refusal { return a.scanAs(s, skillSubject()) }

// The log attribute that names what a scan finding is about.
const (
	labelSkill   = "skill"
	labelContext = "context"
)

// scanSubject says how the scan names what it scanned: a served skill, or an
// item of repository context.
type scanSubject struct {
	// where renders the location of a finding.
	where func(file string, line int) string
	// noun and label name the thing in a log line ("a served skill").
	noun, label string
	// reasonPrefix starts the reason of a refusal.
	reasonPrefix func(level string) string
}

// skillSubject names a served skill, as the scan does.
func skillSubject() scanSubject {
	return scanSubject{
		where: func(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) },
		noun:  "a served skill", label: labelSkill,
		reasonPrefix: func(level string) string { return "security scan (trust=" + level + "): " },
	}
}

// contextSubject names an item of repository context by its context:// URI. The
// trust level plays no part in a refusal: an error finding withholds the file
// whatever the level.
func contextSubject(uri string) scanSubject {
	return scanSubject{
		where: func(_ string, line int) string { return fmt.Sprintf("%s:%d", uri, line) },
		noun:  "repository context", label: labelContext,
		reasonPrefix: func(string) string { return "security scan: " },
	}
}

func (a Admission) scanAs(s *CatalogSkill, subject scanSubject) *Refusal {
	level := a.scanLevelOf(s)
	key, known := a.knownClean(s, level)
	if known {
		return nil
	}
	files := make([]lint.ServedFile, 0, len(s.Files))
	for i := range s.Files {
		files = append(files, lint.ServedFile{Path: s.Files[i].RelPath, Content: s.Files[i].Content})
	}
	findings := lint.ScanServed(a.Config, s.Name, files, level)
	if key != "" && len(findings) == 0 {
		a.Verdicts.MarkClean(key)
	}
	var blocking []lint.Finding
	for _, f := range findings {
		if f.Severity == lint.SeverityError {
			blocking = append(blocking, f)
		}
	}
	s.scanLevel = level
	for _, f := range findings {
		if f.Code == lint.CodeServedUnscannable {
			s.unscannable = append(s.unscannable, f)
		}
	}
	s.ScanFindings = len(findings) - len(blocking)
	if len(blocking) == 0 && level == config.TrustError {
		s.dropUnscannable()
	}
	if len(blocking) == 0 {
		for _, f := range findings {
			logger.Or(a.Log).Warn("Security scan finding in "+subject.noun, subject.label, s.Name, "code", f.Code, "where", subject.where(f.File, f.Line), "message", f.Message)
		}
		return nil
	}
	parts := make([]string, 0, len(blocking))
	for i, f := range blocking {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(blocking)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %s at %s", f.Code, f.Name, subject.where(f.File, f.Line)))
	}
	return &Refusal{Name: s.Name, Code: blocking[0].Code, Reason: subject.reasonPrefix(level) + strings.Join(parts, "; ")}
}
