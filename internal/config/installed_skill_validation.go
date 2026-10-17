package config

import (
	"fmt"
	pathpkg "path"
	"regexp"
	"strings"
	"unicode"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// reasonControlChars is the problem reported for a field with a control character.
const reasonControlChars = "must not contain control characters"

// DecodeTOMLConfig parses the bytes of a config.toml. path is used for error
// context only.
func DecodeTOMLConfig(data []byte, path string) (*Config, error) {
	return decodeConfigTOML(data, path)
}

// transportHelper matches git's `<helper>::<address>` remote syntax (ext::,
// fd::, ...), which can run arbitrary commands.
var transportHelper = regexp.MustCompile(`^[A-Za-z0-9_+.-]+::`)

// ValidateInstalledSkills validates the installed_skills section: required
// fields, unique names, and a source, ref and path that are safe to hand to git.
func ValidateInstalledSkills(skills []InstalledSkillConfig) error {
	seen := make(map[string]bool)
	for i := range skills {
		skill := &skills[i]
		if skill.Name == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].name", i)).
				Hint("Each installed skill must have a non-empty 'name' field").
				Errorf("installed skill at index %d missing required field 'name'", i)
		}
		if skill.Source == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].source", i)).
				With("skill_name", skill.Name).
				Hint("Provide a git URL or local path as the 'source'").
				Errorf("installed skill %q missing required field 'source'", skill.Name)
		}
		if seen[skill.Name] {
			return oops.
				With("field", "installed_skills").
				With("skill_name", skill.Name).
				Hint("Each installed skill must have a unique name").
				Errorf("duplicate installed skill name: %q", skill.Name)
		}
		seen[skill.Name] = true
		if err := ValidateInstalledSkillFields(skill); err != nil {
			return err
		}
	}
	return nil
}

// ValidateInstalledSkillFields rejects values that would be read as a git
// option or transport helper, or that escape the skill repository.
func ValidateInstalledSkillFields(skill *InstalledSkillConfig) error {
	bad := func(field, reason string) error {
		return oops.
			With("field", "installed_skills."+field).
			With("skill_name", skill.Name).
			Errorf("installed skill %q: %s %s", skill.Name, field, reason)
	}
	if reason := skillSourceProblem(skill.Source); reason != "" {
		return bad("source", reason)
	}
	// A ref with ^, ~, * or a space is a version constraint (see IsVersionSugar),
	// checked as one by validateVersionKeys; git refs cannot contain them.
	if reason := skillRefProblem(skill.Ref); reason != "" && !IsVersionConstraintRef(skill.Ref) {
		return bad("ref", reason)
	}
	if reason := skillPathProblem(skill.Path); reason != "" {
		return bad("path", reason)
	}
	switch skill.Trust {
	case "", TrustError, TrustWarn:
	default:
		return bad("trust", fmt.Sprintf("has an invalid trust level %q (use 'error', the default, or 'warn')", skill.Trust))
	}
	for _, globs := range [][]string{skill.Include, skill.Exclude} {
		for _, g := range globs {
			if _, err := pathpkg.Match(g, ""); err != nil {
				return bad("include/exclude", fmt.Sprintf("has an invalid glob %q", g))
			}
		}
	}
	for _, pattern := range skill.Resources {
		if reason := skillResourcePatternProblem(pattern); reason != "" {
			return bad("resources", fmt.Sprintf("has an invalid pattern %q: %s", pattern, reason))
		}
	}
	return nil
}

// ValidateInstalledSkillVersion checks the version-constraint keys of an entry
// (ref as a constraint, version, tag_prefix, include_prerelease,
// min_release_age), the check config load applies.
func ValidateInstalledSkillVersion(skill *InstalledSkillConfig) error {
	return validateVersionKeys("installed_skills", skill.Name, skill.Ref, skill.Version, skill.TagPrefix, skill.IncludePrerelease, skill.MinReleaseAge)
}

// skillResourcePatternProblem says why a resources pattern cannot select files
// inside a skill directory ("" when it can).
func skillResourcePatternProblem(pattern string) string {
	raw := strings.TrimPrefix(pattern, "!")
	switch {
	case strings.TrimSpace(strings.Trim(raw, "/")) == "":
		return "is empty"
	case strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") || (len(raw) >= 2 && raw[1] == ':'):
		return "must be relative to the skill directory"
	case hasControl(raw):
		return reasonControlChars
	case strings.Contains(raw, "\\"):
		return "must use '/' separators; a backslash escapes the next character and the pattern would match nothing"
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return "must not contain '..'"
		}
	}
	if _, err := pathpkg.Match(strings.Trim(raw, "/"), ""); err != nil {
		return "is not a valid glob"
	}
	return ""
}

// skillRefProblem says why a ref cannot be handed to git ("" when it can).
func skillRefProblem(ref string) string {
	switch {
	case ref == "":
		return ""
	case strings.HasPrefix(ref, "-"):
		return "must not start with '-'"
	case hasControl(ref) || strings.ContainsAny(ref, " ~^:?*[\\") || strings.Contains(ref, "..") || strings.Contains(ref, "@{"):
		return "is not a valid git ref"
	}
	return ""
}

// skillPathProblem says why a path cannot address a directory inside the skill repository.
func skillPathProblem(p string) string {
	switch {
	case p == "":
		return ""
	case strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || (len(p) >= 2 && p[1] == ':'):
		return "must be relative to the skill repository"
	case hasControl(p):
		return reasonControlChars
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return "must not contain '..'"
		}
	}
	return ""
}

// skillSourceProblem says why a skill source cannot be handed to git ("" when it can).
func skillSourceProblem(source string) string {
	src := strings.TrimSpace(source)
	switch {
	case strings.HasPrefix(src, "-"):
		return "must not start with '-' (it would be read as a git option)"
	case transportHelper.MatchString(src):
		return "must not use a git transport helper such as ext::"
	case hasControl(source):
		return reasonControlChars
	case strings.HasPrefix(strings.ToLower(src), "http://"):
		return "uses plain http://, which is not accepted since ai-rulez 5: use https:// (or ssh, git@host:path, file://)"
	case strings.HasPrefix(src, githubSourcePrefix) && !lockfile.IsGitHubShorthand(strings.TrimPrefix(src, githubSourcePrefix)):
		return "must be github:owner/repo"
	case strings.HasPrefix(src, "~") && src != "~" && !strings.HasPrefix(src, "~/") && !strings.HasPrefix(src, "~\\"):
		return "uses ~user, which is not supported: write ~ or ~/path for your own home directory"
	case lockfile.IsGitSource(src):
		return gitAuthorityProblem(src)
	}
	return ""
}

// gitAuthorityProblem refuses a git source whose host a reader could take for another one: a "?", "#" or
// backslash in the authority, whitespace anywhere, or a user in a web or git:// URL. The host policy
// (sources.allowed_hosts) and git must read the same host.
func gitAuthorityProblem(src string) string {
	if strings.IndexFunc(src, unicode.IsSpace) >= 0 || strings.Contains(src, "\\") {
		return "must not contain whitespace or a backslash"
	}
	lower := strings.ToLower(strings.TrimPrefix(strings.ToLower(src), "git+"))
	scheme, rest, hasScheme := strings.Cut(lower, "://")
	if !hasScheme {
		host, _, _ := strings.Cut(src, ":")
		if strings.ContainsAny(host, "?#") || strings.Count(host, "@") > 1 {
			return "has an invalid host: '?', '#' and a second '@' are not allowed before the ':'"
		}
		return ""
	}
	if scheme == "file" {
		return ""
	}
	end := strings.IndexAny(rest, "/?#")
	if end >= 0 && rest[end] != '/' {
		return "has an invalid host: '?' and '#' are not allowed in the authority"
	}
	authority := rest
	if end >= 0 {
		authority = rest[:end]
	}
	if at := strings.Index(authority, "@"); at >= 0 && (scheme != "ssh" || strings.Count(authority, "@") > 1) {
		return "must not embed credentials in the URL (use the git credential helper or AI_RULEZ_GIT_TOKEN)"
	}
	return ""
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
