package mcp

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// skillOrigin is where a remote skill's files live on disk, for the publisher
// signature check.
type skillOrigin struct {
	// Dir is the skill directory.
	Dir string
	// Source names the [[skill_sources]] or [[installed_skills]] entry, which
	// scopes [[signing.trust]] entries.
	Source string
	// Verbatim marks a skill whose served bytes are the files on disk (a skill
	// source), so the signed bytes can be compared with the served ones.
	Verbatim bool
}

// SignatureGate applies [signing] require to served skills. "served" refuses
// every skill while the lock's attestation does not verify (the lock then pins
// each skill, see Admission.Enforce); "skill" refuses a remote skill without a
// publisher attestation from a trusted signer. A nil gate admits everything.
type SignatureGate struct {
	lockProblem *signing.Error
	check       *signing.ArtifactCheck
	prepareErr  *signing.Error
	origins     map[string]skillOrigin
}

// enforcesLock reports whether served skills must be pinned in the lock:
// [lock] enforce, or [signing] require = ["served"], whose signed lock is the
// pin list.
func enforcesLock(cfg *config.Config) bool {
	return cfg.LockEnforced() || cfg.Signing.Requires(config.SigningSubjectServed)
}

// newSignatureGate verifies what the policy requires once, at build time. It
// returns nil when [signing] requires neither "served" nor "skill".
func newSignatureGate(cfg *config.Config, origins map[string]skillOrigin, now time.Time) *SignatureGate {
	if cfg == nil || cfg.Signing == nil {
		return nil
	}
	served, skill := cfg.Signing.Requires(config.SigningSubjectServed), cfg.Signing.Requires(config.SigningSubjectSkill)
	if !served && !skill {
		return nil
	}
	g := &SignatureGate{origins: origins}
	if served {
		g.lockProblem = lockAttestationProblem(cfg, now)
	}
	if skill {
		check, err := signing.PrepareArtifactCheck(cfg, signing.SubjectSkill, signing.VerifyOptions{Now: now})
		if err != nil {
			g.prepareErr = asSigningError(err)
		} else {
			g.check = check
			for _, w := range check.Warnings {
				logger.Warn(w)
			}
		}
	}
	return g
}

// lockAttestationProblem is the first reason the lock attestation does not
// verify, nil when it does.
func lockAttestationProblem(cfg *config.Config, now time.Time) *signing.Error {
	findings, err := signing.CheckLockAttestation(cfg, nil, now)
	if err != nil {
		return asSigningError(err)
	}
	if len(findings) > 0 {
		return findings[0]
	}
	return nil
}

// asSigningError keeps a signing error as it is and reports anything else (a
// lock that cannot be read, no trusted signer) as AR720: nothing verified.
func asSigningError(err error) *signing.Error {
	var se *signing.Error
	if errors.As(err, &se) {
		return se
	}
	return &signing.Error{Code: signing.CodeMissing, Reason: err.Error()}
}

// Check returns the refusal of s, nil when the policy admits it.
func (g *SignatureGate) Check(s *CatalogSkill) *Refusal {
	if g == nil {
		return nil
	}
	if g.lockProblem != nil {
		return &Refusal{Name: s.Name, Code: g.lockProblem.Code, Reason: "[signing] require includes \"served\" and the lock attestation does not verify: " + g.lockProblem.Reason}
	}
	if g.check == nil && g.prepareErr == nil {
		return nil
	}
	origin, known := g.origins[s.Name]
	if !known {
		if s.Imported || s.Ref != "" || s.Commit != "" {
			return &Refusal{Name: s.Name, Code: signing.CodeMissing, Reason: "[signing] require includes \"skill\" and no publisher attestation can be located for a skill from this origin (only skill sources and installed skills carry one)"}
		}
		return nil
	}
	if g.prepareErr != nil {
		return &Refusal{Name: s.Name, Code: g.prepareErr.Code, Reason: "[signing] require includes \"skill\" and the policy cannot be applied: " + g.prepareErr.Reason}
	}
	if err := g.verifyPublisher(s, origin); err != nil {
		return &Refusal{Name: s.Name, Code: err.Code, Reason: "[signing] require includes \"skill\": " + err.Reason}
	}
	return nil
}

// verifyPublisher judges the sidecar attestation in the skill's directory. The
// digest is recomputed from the files on disk and, for a skill source, compared
// with the bytes that will be served, so what is verified is what is returned.
func (g *SignatureGate) verifyPublisher(s *CatalogSkill, o skillOrigin) *signing.Error {
	tree, err := signing.ReadTreeSubject(signing.KindSkillTree, o.Dir)
	if err != nil {
		return &signing.Error{Code: signing.CodeSubjectMismatch, Reason: "cannot compute the skill's digest: " + err.Error()}
	}
	bundles, err := signing.ReadBundleFiles(filepath.Join(o.Dir, signing.SidecarName))
	if err != nil {
		return asSigningError(err)
	}
	if _, err := g.check.VerifyFor(o.Source, bundles, signing.TreeExpectation(signing.SubjectSkill, tree)); err != nil {
		return asSigningError(err)
	}
	if o.Verbatim {
		return servedMatchesSigned(s, tree.Tree)
	}
	return nil
}

// servedMatchesSigned is AR724 when a file about to be served differs from the
// file the signature covered: the skill changed between being read and being
// verified, or carries a file the publisher never signed.
func servedMatchesSigned(s *CatalogSkill, tree *signing.DirTree) *signing.Error {
	for i := range s.Files {
		f := &s.Files[i]
		if signing.IsSignatureFile(f.RelPath) {
			continue
		}
		signed := tree.Content(f.RelPath)
		switch {
		case signed == nil:
			return signing.Errorf(signing.CodeSubjectMismatch, "%s is served but the publisher's signature does not cover it", f.RelPath)
		case f.RelPath == skillMarkdown && sameSkillMarkdown(signed, f.Content):
			continue
		case !bytes.Equal(signed, f.Content):
			return signing.Errorf(signing.CodeSubjectMismatch, "%s changed between being read and being verified", f.RelPath)
		}
	}
	return nil
}

// sameSkillMarkdown reports whether two SKILL.md files differ at most in the
// frontmatter `name:` line: a skill source serves its SKILL.md with the name set
// to the served name (skillsource.rewriteName), which the signed file need not
// carry. The body and every other frontmatter line must be equal.
func sameSkillMarkdown(signed, served []byte) bool {
	return bytes.Equal(withoutNameLine(signed), withoutNameLine(served))
}

func withoutNameLine(content []byte) []byte {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return []byte(text)
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return []byte(text)
	}
	lines := strings.Split(rest[:end], "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !strings.HasPrefix(l, "name:") {
			kept = append(kept, l)
		}
	}
	return []byte("---\n" + strings.Join(kept, "\n") + rest[end:])
}

// skillOrigins locates the directories of the remote skills of a build: skill
// source skills by their served name, installed skills by their configured name.
func skillOrigins(cfg *config.Config, sources []*skillsource.Resolved) map[string]skillOrigin {
	origins := map[string]skillOrigin{}
	for _, res := range sources {
		for _, sk := range res.Skills {
			dir := filepath.Join(res.Dir, sk.Dir)
			if fileInDir(res.Dir, "SKILL.md") {
				dir = res.Dir
			}
			origins[sk.Name] = skillOrigin{Dir: dir, Source: res.Spec.Name, Verbatim: true}
		}
	}
	if cfg == nil || cfg.Content == nil || len(cfg.InstalledSkills) == 0 {
		return origins
	}
	installed := map[string]bool{}
	for i := range cfg.InstalledSkills {
		installed[cfg.InstalledSkills[i].Name] = true
	}
	add := func(files []config.ContentFile) {
		for i := range files {
			f := files[i]
			if installed[f.Name] {
				o := skillOrigin{Dir: filepath.Dir(f.Path), Source: f.Name}
				origins[f.Name] = o
				origins[config.SkillID(f)] = o
			}
		}
	}
	add(cfg.Content.Skills)
	for _, d := range cfg.Content.Domains {
		add(d.Skills)
	}
	return origins
}

func fileInDir(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && info.Mode().IsRegular()
}
