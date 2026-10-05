package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Refusal records why a skill is not served.
type Refusal struct {
	Name string
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
	// DefaultTrust is the scan level for a skill that names none: "warn" for
	// skills authored in the project, "error" for everything that came from a
	// remote. nil means "warn".
	DefaultTrust func(*CatalogSkill) string
}

// Admit returns a catalog holding only the admitted skills. Refused skills are
// kept by name so load_skill can say why, and are logged on stderr. The input
// catalog is not modified.
func (c *Catalog) Admit(a Admission) *Catalog {
	out := &Catalog{
		Profile: c.Profile, Preset: c.Preset,
		byName: map[string]*CatalogSkill{}, byURI: map[string]*CatalogSkill{}, byFile: map[string]*CatalogFile{},
		refused: map[string]Refusal{},
	}
	for name, r := range c.refused {
		out.refused[name] = r
	}
	for _, skill := range c.skills {
		cp := *skill
		if r := a.check(&cp); r != nil {
			out.refused[cp.Name] = *r
			logger.Warn("Refusing to serve a skill", "skill", cp.Name, "code", r.Code, "reason", r.Reason)
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
	if r := a.scan(s); r != nil {
		return r
	}
	entry := a.Lock.Find(lockfile.KindServed, s.Name)
	s.Locked = entry != nil && entry.Digest == s.LockDigest
	if !a.Enforce {
		return nil
	}
	switch {
	case entry == nil:
		return &Refusal{s.Name, CodeServedLockMismatch, fmt.Sprintf("[lock] enforce is on and %s does not pin this skill; review it, then run `ai-rulez lock`", lockfile.FileName)}
	case !s.Locked:
		return &Refusal{s.Name, CodeServedLockMismatch, fmt.Sprintf("digest %s differs from the lock's %s; the skill changed since it was locked (run `ai-rulez lock` only after reviewing the change)", s.LockDigest, entry.Digest)}
	}
	return nil
}

func (a Admission) scan(s *CatalogSkill) *Refusal {
	level := s.Trust
	if level == "" {
		level = config.TrustWarn
		if a.DefaultTrust != nil {
			level = a.DefaultTrust(s)
		}
	}
	files := make([]lint.ServedFile, 0, len(s.Files))
	for i := range s.Files {
		files = append(files, lint.ServedFile{Path: s.Files[i].RelPath, Content: s.Files[i].Content})
	}
	findings := lint.ScanServed(a.Config, s.Name, files, level)
	var blocking []lint.Finding
	for _, f := range findings {
		if f.Severity == lint.SeverityError {
			blocking = append(blocking, f)
		}
	}
	s.ScanFindings = len(findings) - len(blocking)
	if len(blocking) == 0 {
		for _, f := range findings {
			logger.Warn("Security scan finding in a served skill", "skill", s.Name, "code", f.Code, "where", fmt.Sprintf("%s:%d", f.File, f.Line), "message", f.Message)
		}
		return nil
	}
	parts := make([]string, 0, len(blocking))
	for i, f := range blocking {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(blocking)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %s at %s:%d", f.Code, f.Name, f.File, f.Line))
	}
	return &Refusal{s.Name, blocking[0].Code, "security scan (trust=" + level + "): " + strings.Join(parts, "; ")}
}
