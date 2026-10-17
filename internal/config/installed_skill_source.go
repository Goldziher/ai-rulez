package config

import (
	"os"
	"path/filepath"
	"strings"

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
