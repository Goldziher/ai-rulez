package signing

import (
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// SkillFinding is one installed skill whose publisher attestation does not
// satisfy [signing] require.
type SkillFinding struct {
	// Name is the installed skill's configured name.
	Name string
	// Dir is the skill directory that was checked.
	Dir string
	Err *Error
}

// CheckInstalledSkills judges the publisher attestation of every installed skill
// that is delivered statically, when [signing] require names "skill". Served
// skills are gated by `mcp --serve-skills` when they are served. It returns
// nothing when the policy does not require skill signatures. An error means the
// policy could not be applied at all (no trusted signer, no trusted root).
func CheckInstalledSkills(cfg *config.Config, now time.Time) ([]SkillFinding, error) {
	if cfg == nil || cfg.Signing == nil || !cfg.Signing.Requires(config.SigningSubjectSkill) ||
		len(cfg.InstalledSkills) == 0 || cfg.Content == nil {
		return nil, nil
	}
	skills := staticInstalledSkills(cfg)
	if len(skills) == 0 {
		return nil, nil
	}
	check, err := PrepareArtifactCheck(cfg, SubjectSkill, VerifyOptions{Now: now})
	if err != nil {
		return nil, err
	}
	var findings []SkillFinding
	for _, s := range skills {
		if err := verifyInstalledDir(check, s.name, s.dir); err != nil {
			findings = append(findings, SkillFinding{Name: s.name, Dir: s.dir, Err: err})
		}
	}
	return findings, nil
}

type installedSkill struct{ name, dir string }

// staticInstalledSkills lists the installed skills that are written into harness
// trees, in name order.
func staticInstalledSkills(cfg *config.Config) []installedSkill {
	installed := map[string]bool{}
	for i := range cfg.InstalledSkills {
		installed[cfg.InstalledSkills[i].Name] = true
	}
	seen := map[string]bool{}
	var out []installedSkill
	add := func(domain string, files []config.ContentFile) {
		for i := range files {
			f := files[i]
			if !installed[f.Name] || seen[f.Name] || cfg.EffectiveDelivery(f, domain, nil) == config.DeliveryServed {
				continue
			}
			seen[f.Name] = true
			out = append(out, installedSkill{name: f.Name, dir: filepath.Dir(f.Path)})
		}
	}
	add("", cfg.Content.Skills)
	for name, d := range cfg.Content.Domains {
		if d != nil {
			add(name, d.Skills)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// verifyInstalledDir judges the sidecar attestation in dir against the digest of
// the files on disk, so an edited or extended skill fails AR724.
func verifyInstalledDir(check *ArtifactCheck, name, dir string) *Error {
	tree, err := ReadTreeSubject(KindSkillTree, dir)
	if err != nil {
		return &Error{Code: CodeSubjectMismatch, Reason: "cannot compute the skill's digest: " + err.Error()}
	}
	bundles, err := ReadBundleFiles(filepath.Join(dir, SidecarName))
	if err != nil {
		return asError(err)
	}
	if _, err := check.VerifyFor(name, bundles, TreeExpectation(SubjectSkill, tree)); err != nil {
		return asError(err)
	}
	return nil
}

// asError keeps a signing error and reports anything else as AR720: nothing verified.
func asError(err error) *Error {
	var se *Error
	if errors.As(err, &se) {
		return se
	}
	return &Error{Code: CodeMissing, Reason: err.Error()}
}
