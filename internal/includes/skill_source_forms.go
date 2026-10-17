package includes

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// homeRelative reports whether source is "~" or "~/path" and returns the part
// after the "~/".
func homeRelative(source string) (rest string, ok bool) {
	switch {
	case source == "~":
		return "", true
	case strings.HasPrefix(source, "~/"), strings.HasPrefix(source, `~\`):
		return source[2:], true
	}
	return "", false
}

// SkillSourceRoot classifies an installed_skills source. A "~" or "~/path"
// source and a file:// URL that names a directory which is not a git repository
// are local, and the returned string is the directory: the home directory is
// expanded, a file:// URL is turned into its path. Any other source comes back
// as written, classified like an include source. GitHub shorthand has already
// been expanded to an https URL when the config was loaded.
func SkillSourceRoot(cfg *config.Config, source string) (SourceType, string, error) {
	if rest, ok := homeRelative(source); ok {
		var env ambient.Env
		if cfg != nil {
			env = cfg.Host.Env
		}
		home, err := ambient.OrOS(env).UserHomeDir()
		if err != nil || home == "" {
			return "", "", oops.Hint("Set HOME, or write the path without '~'").
				Errorf("cannot expand %q: no home directory", source)
		}
		return SourceTypeLocal, filepath.Join(home, rest), nil
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		if path, ok := lockfile.FileURLPath(source); ok && !isGitCheckout(path) {
			return SourceTypeLocal, path, nil
		}
	}
	return DetectSourceType(source), source, nil
}

// isGitCheckout reports whether dir is a git working tree or a bare repository,
// i.e. something git can clone. A file:// URL to anything else is read as a
// plain directory of skills.
func isGitCheckout(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		_, objErr := os.Stat(filepath.Join(dir, "objects"))
		return objErr == nil
	}
	return false
}
