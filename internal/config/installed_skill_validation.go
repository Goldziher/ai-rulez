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
	src := strings.TrimSpace(skill.Source)
	switch {
	case strings.HasPrefix(src, "-"):
		return bad("source", "must not start with '-' (it would be read as a git option)")
	case transportHelper.MatchString(src):
		return bad("source", "must not use a git transport helper such as ext::")
	case hasControl(skill.Source):
		return bad("source", "must not contain control characters")
	}
	if skill.Ref != "" {
		switch {
		case strings.HasPrefix(skill.Ref, "-"):
			return bad("ref", "must not start with '-'")
		case hasControl(skill.Ref) || strings.ContainsAny(skill.Ref, " ~^:?*[\\") || strings.Contains(skill.Ref, "..") || strings.Contains(skill.Ref, "@{"):
			return bad("ref", "is not a valid git ref")
		}
	}
	if skill.Path != "" {
		p := skill.Path
		switch {
		case strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || (len(p) >= 2 && p[1] == ':'):
			return bad("path", "must be relative to the skill repository")
		case hasControl(p):
			return bad("path", "must not contain control characters")
		}
		for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
			if seg == ".." {
				return bad("path", "must not contain '..'")
			}
		}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
