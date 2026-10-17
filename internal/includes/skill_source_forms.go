package includes

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// SkillSourceRoot classifies an installed_skills source. A "~" or "~/path"
// source and a file:// URL that names a directory which is not a git repository
// are local, and the returned string is the directory: the home directory is
// expanded, a file:// URL is turned into its path. Any other source comes back
// as written, classified like an include source. GitHub shorthand has already
// been expanded to an https URL when the config was loaded.
func SkillSourceRoot(cfg *config.Config, source string) (SourceType, string, error) {
	dir, local, err := config.LocalSkillSource(cfg.Host.Env, source)
	if err != nil {
		return "", "", err //nolint:wrapcheck // already contextual
	}
	if local {
		return SourceTypeLocal, dir, nil
	}
	return DetectSourceType(source), source, nil
}
