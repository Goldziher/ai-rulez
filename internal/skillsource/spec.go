// Package skillsource resolves [[skill_sources]] and `--source` arguments: a git
// repository (pinned to the commit a tag or SHA names) or a local directory that
// holds skill directories. Resolution records what it found in ai-rulez.lock's
// existing structure, never touches the network when told not to, and keeps
// fetched trees in a local cache keyed by commit.
package skillsource

import (
	"path"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
)

// Spec describes one skill source.
type Spec struct {
	Name string
	URL  string
	Ref  string
	// Version is a semver constraint resolved against the repository's tags; it
	// excludes Ref. TagPrefix and IncludePrerelease refine it.
	Version           string
	TagPrefix         string
	IncludePrerelease bool
	Path              string
	Include           []string
	Exclude           []string
	NamePrefix        string
	Trust             string
	// MaxSkills and MaxBytes bound what the source loads; 0 selects the defaults.
	MaxSkills int
	MaxBytes  int
	// MaxCloneBytes bounds what a git source may download and check out; 0
	// selects the global limit or the default (256 MiB).
	MaxCloneBytes int64
	// MaxCloneFiles bounds the number of entries of a git clone; 0 selects
	// AI_RULEZ_MAX_CLONE_FILES or the default (20000).
	MaxCloneFiles int
	// MinReleaseAge holds back tags younger than this ("7d"); "" defers to [lock].
	MinReleaseAge string
	// AllowOutside lets a local source resolve outside the project: set for
	// `--source` arguments and for sources of the user's own config, never for a
	// source declared in a committed project config.
	AllowOutside bool
}

// FromConfig converts a [[skill_sources]] entry.
func FromConfig(c *config.SkillSourceConfig) Spec {
	return Spec{Name: c.Name, URL: c.URL, Ref: c.Ref, Version: c.Version, TagPrefix: c.TagPrefix, IncludePrerelease: c.IncludePrerelease, MinReleaseAge: c.MinReleaseAge, Path: c.Path, Include: c.Include, Exclude: c.Exclude, NamePrefix: c.NamePrefix, Trust: c.Trust, MaxSkills: c.MaxSkills, MaxBytes: c.MaxBytes, MaxCloneBytes: c.MaxCloneBytes, MaxCloneFiles: c.MaxCloneFiles}
}

// TrustLevel is the scan level, defaulting to the strict one.
func (s Spec) TrustLevel() string {
	if s.Trust == config.TrustWarn {
		return config.TrustWarn
	}
	return config.TrustError
}

// IsGit reports whether the source is a git repository rather than a directory.
func (s Spec) IsGit() bool { return includes.IsGitURL(gitURL(s.URL)) }

// gitURL strips the optional `git+` scheme prefix.
func gitURL(u string) string { return strings.TrimPrefix(u, "git+") }

// Redacted is the URL with credentials removed; the lock is committed.
func (s Spec) Redacted() string { return includes.RedactURL(gitURL(s.URL)) }

var nameCleanRe = regexp.MustCompile(`[^a-z0-9._-]+`)

// ParseArg parses `--source` as `[git+]<url>[@<ref>][#<path>]` or a local
// directory, optionally with `#<path>`. The ref separator is the last `@` after
// the final `/` of the URL, so `git@host:org/repo` and `https://user@host/x`
// keep their user info. The name is derived from the last URL segment.
func ParseArg(arg string) (Spec, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Spec{}, oops.Errorf("empty --source")
	}
	spec := Spec{}
	if i := strings.LastIndex(arg, "#"); i >= 0 {
		spec.Path, arg = strings.Trim(arg[i+1:], "/"), arg[:i]
	}
	url := arg
	if includes.IsGitURL(gitURL(arg)) {
		if i := strings.LastIndex(arg, "@"); i > strings.LastIndex(arg, "/") && i >= 0 {
			spec.Ref, url = arg[i+1:], arg[:i]
		}
	}
	spec.URL = url
	_, isFile := lockfile.FileURLPath(url)
	spec.AllowOutside = !includes.IsGitURL(gitURL(url)) || isFile // a path or file:// URL typed on the command line is the user's own choice
	if err := checkRemote(url, spec.Ref); err != nil {
		return Spec{}, err
	}
	if spec.Path != "" {
		if clean := path.Clean(spec.Path); clean == ".." || strings.HasPrefix(clean, "../") {
			return Spec{}, oops.Errorf("--source path %q escapes the repository", spec.Path)
		}
	}
	base := path.Base(strings.TrimRight(gitURL(url), "/"))
	base = strings.TrimSuffix(base, ".git")
	spec.Name = "cli-" + strings.Trim(nameCleanRe.ReplaceAllString(strings.ToLower(base), "-"), "-.")
	if spec.Name == "cli-" {
		spec.Name = "cli-source"
	}
	return spec, nil
}
