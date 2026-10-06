package contentlock

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// SkillDigestScheme is the scheme name recorded beside a skill digest in the
// usage log: the domain-separation label of the skill item tree.
const SkillDigestScheme = "ai-rulez/skill/v1"

// SkillDigest returns the lock's item digest of one skill: the digest `lock`
// pins for it, and therefore the one identity usage logs, eval results and the
// lock share. It covers SKILL.md and the loaded resources (references/, scripts/,
// assets/). A top-level evals/ directory is never loaded as a resource, so
// editing a case does not change it.
//
// The SKILL.md bytes come from disk, with skill.Content as the fallback for
// content that only exists in memory (a builtin). root is the directory whose
// git index decides file modes on Windows; any directory inside the project does.
func SkillDigest(skill *config.ContentFile, root string) (string, error) {
	primary, err := os.ReadFile(skill.Path)
	if err != nil {
		primary = []byte(skill.Content)
	}
	modes := newModeResolver()
	mode := ModeRegular
	if info, statErr := os.Stat(skill.Path); statErr == nil {
		mode = modes.mode(root, skill.Path, info)
	}
	leaves := contentLeaves(modes, root, skill, primary, mode)
	digest, err := TreeDigest(KindSkill, leaves)
	if err != nil {
		return "", oops.With("path", skill.Path).Wrap(err)
	}
	return digest, nil
}

// SkillDirDigest is SkillDigest of the skill in dir, loaded with the default
// bundle rules.
func SkillDirDigest(dir string) (string, error) {
	resources, err := config.LoadSkillResources(dir)
	if err != nil {
		return "", oops.With("dir", dir).Wrapf(err, "load skill resources")
	}
	skill := &config.ContentFile{Path: filepath.Join(dir, "SKILL.md"), Resources: resources}
	if _, err := os.Stat(skill.Path); err != nil {
		return "", oops.With("path", skill.Path).Wrapf(err, "read skill")
	}
	return SkillDigest(skill, dir)
}
