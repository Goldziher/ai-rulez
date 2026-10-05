package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/oops"
)

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
	if reason := skillRefProblem(skill.Ref); reason != "" {
		return bad("ref", reason)
	}
	if reason := skillPathProblem(skill.Path); reason != "" {
		return bad("path", reason)
	}
	return nil
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
		return "must not contain control characters"
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
		return "must not contain control characters"
	case strings.HasPrefix(strings.ToLower(src), "http://"):
		return "uses plain http://, which is not accepted since ai-rulez 5: use https:// (or ssh, git@host:path, file://)"
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
