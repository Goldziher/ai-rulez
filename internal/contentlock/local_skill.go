package contentlock

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// localSkillSource is the directory an installed_skills entry with a local
// source reads its skills from.
type localSkillSource struct {
	// written is the source as the config spells it, the machine-independent
	// prefix of a recorded path.
	written string
	// dir is the directory, absolute.
	dir string
}

// localSkillSources lists the local sources of the installed_skills entries. A
// source that cannot be expanded (no home directory) is skipped: resolving it
// failed too, so no skill came from it.
func (c *collector) localSkillSources() []localSkillSource {
	if c.skillSources != nil {
		return *c.skillSources
	}
	sources := []localSkillSource{}
	for i := range c.cfg.InstalledSkills {
		sk := &c.cfg.InstalledSkills[i]
		if sk.LocalOverride != "" {
			continue // a development shortcut, not the pinned source
		}
		dir, local, err := config.LocalSkillSource(c.cfg.Host.Env, sk.Source)
		if err != nil {
			continue
		}
		if !local {
			if lockfile.IsGitSource(sk.Source) {
				continue // pinned by commit and digest
			}
			dir = sk.Source
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(c.cfg.BaseDir, dir)
		}
		sources = append(sources, localSkillSource{written: sk.Source, dir: filepath.Clean(dir)})
	}
	c.skillSources = &sources
	return sources
}

// addLocalInstalledSkill pins a skill that an installed_skills entry read from a
// local directory. Such a file lies outside the configuration directory, so it
// is not authored content, yet editing it changes the generated output; a
// remote skill is pinned by commit and digest, a local one by its tree digest,
// like a local include. A file that no local entry reads is not pinned here.
func (c *collector) addLocalInstalledSkill(domain string, cf *config.ContentFile) error {
	if cf.Path == "" || !filepath.IsAbs(cf.Path) {
		return nil
	}
	for _, src := range c.localSkillSources() {
		rel, err := filepath.Rel(src.dir, cf.Path)
		if err != nil || safefs.RelEscapes(rel) || filepath.IsAbs(rel) {
			continue
		}
		recorded := strings.TrimRight(filepath.ToSlash(src.written), "/") + "/" + filepath.ToSlash(rel)
		if rel == "." {
			recorded = filepath.ToSlash(src.written)
		}
		return c.pinFile(KindSkill, KindLocalSkill, cf.Name, domain, cf, recorded)
	}
	return nil
}
