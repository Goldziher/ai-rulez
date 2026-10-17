package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// githubSourcePrefix marks an installed skill source as a GitHub repository:
// "github:owner/repo".
const githubSourcePrefix = "github:"

// NormalizeInstalledSkillSources rewrites GitHub shorthand sources to their https
// URL, so every consumer (resolver, lock, version pins) sees one form and a lock
// key for a shorthand matches the one for the full URL. "github:owner/repo" is
// always a repository. A bare "owner/repo" is one only when no such path exists
// under baseDir, so an existing relative local source keeps meaning a directory.
// Any other source is left as written.
func NormalizeInstalledSkillSources(baseDir string, skills []InstalledSkillConfig) {
	for i := range skills {
		skills[i].Source = normalizeSkillSource(baseDir, skills[i].Source)
	}
}

func normalizeSkillSource(baseDir, source string) string {
	src := strings.TrimSpace(source)
	if rest, ok := strings.CutPrefix(src, githubSourcePrefix); ok {
		if lockfile.IsGitHubShorthand(rest) {
			return lockfile.GitHubURL(rest)
		}
		return source
	}
	if !lockfile.IsGitHubShorthand(src) || lockfile.IsGitSource(src) {
		return source
	}
	if _, err := os.Stat(filepath.Join(baseDir, filepath.FromSlash(src))); err == nil {
		return source
	}
	return lockfile.GitHubURL(src)
}

// IsHomeSource reports whether source is "~" or "~/path".
func IsHomeSource(source string) bool {
	_, ok := homeRelative(source)
	return ok
}

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

// LocalSkillSource reports whether an installed_skills source is a local
// directory and returns it, not yet joined to the project: a "~" or "~/path"
// source (the home directory comes from env), and a file:// URL naming a
// directory that is not a git repository. Any other source is returned as
// written with local false; GitHub shorthand has already been expanded.
func LocalSkillSource(env ambient.Env, source string) (dir string, local bool, err error) {
	if rest, ok := homeRelative(source); ok {
		home, homeErr := ambient.OrOS(env).UserHomeDir()
		if homeErr != nil || home == "" {
			return "", false, oops.Hint("Set HOME, or write the path without '~'").
				Errorf("cannot expand %q: no home directory", source)
		}
		return filepath.Join(home, rest), true, nil
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		if path, ok := lockfile.FileURLPath(source); ok && !isGitCheckout(path) {
			return path, true, nil
		}
	}
	return source, false, nil
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
