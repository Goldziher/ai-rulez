package config

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/samber/oops"
)

// VersionSpec is what a git source asks for when it uses a version constraint
// instead of a ref.
type VersionSpec struct {
	// Constraint is the npm-style range ("" when the source uses a plain ref).
	Constraint        string
	TagPrefix         string
	IncludePrerelease bool
	// MinReleaseAge is the source's own minimum release age ("" defers to [lock]).
	MinReleaseAge string
}

// Active reports whether the source asks for a version range.
func (v VersionSpec) Active() bool { return v.Constraint != "" }

// IsVersionSugar reports whether a ref is really a version constraint. Git
// forbids ^ ~ * and spaces in ref names (git check-ref-format), so a ref that
// contains one cannot be a ref; "1.2" or ">=1" stay plain ref names because
// those characters are legal in branch names.
func IsVersionSugar(ref string) bool { return strings.ContainsAny(ref, "^~* ") }

// IsVersionConstraintRef reports whether ref is the shorthand for a constraint
// that parses (see IsVersionSugar).
func IsVersionConstraintRef(ref string) bool {
	if !IsVersionSugar(ref) {
		return false
	}
	_, err := semver.ParseConstraint(ref)
	return err == nil
}

// resolveVersionSpec merges the version key and the ref shorthand of one source.
func resolveVersionSpec(ref, version, prefix string, prerelease bool, minAge string) VersionSpec {
	constraint := strings.TrimSpace(version)
	if constraint == "" && IsVersionSugar(ref) {
		constraint = strings.TrimSpace(ref)
	}
	if constraint == "" {
		return VersionSpec{}
	}
	return VersionSpec{Constraint: constraint, TagPrefix: prefix, IncludePrerelease: prerelease, MinReleaseAge: strings.TrimSpace(minAge)}
}

// VersionSpec returns the version constraint of the include.
func (c *IncludeConfig) VersionSpec() VersionSpec {
	return resolveVersionSpec(c.Ref, c.Version, c.TagPrefix, c.IncludePrerelease, c.MinReleaseAge)
}

// RequestedRef is what the lock records as the requested ref: the constraint
// when there is one, else the ref.
func (c *IncludeConfig) RequestedRef() string { return requestedRef(c.Ref, c.VersionSpec()) }

// VersionSpec returns the version constraint of the installed skill.
func (s *InstalledSkillConfig) VersionSpec() VersionSpec {
	return resolveVersionSpec(s.Ref, s.Version, s.TagPrefix, s.IncludePrerelease, s.MinReleaseAge)
}

// RequestedRef is what the lock records as the requested ref.
func (s *InstalledSkillConfig) RequestedRef() string { return requestedRef(s.Ref, s.VersionSpec()) }

// VersionSpec returns the version constraint of the skill source.
func (s *SkillSourceConfig) VersionSpec() VersionSpec {
	return resolveVersionSpec(s.Ref, s.Version, s.TagPrefix, s.IncludePrerelease, s.MinReleaseAge)
}

// RequestedRef is what the lock records as the requested ref.
func (s *SkillSourceConfig) RequestedRef() string { return requestedRef(s.Ref, s.VersionSpec()) }

func requestedRef(ref string, v VersionSpec) string {
	if v.Active() {
		return v.Constraint
	}
	return ref
}

// codeConstraintInvalid is rule AR731 (docs/strict-validation.md).
const codeConstraintInvalid = "AR731"

// validateVersionKeys checks the version, tag_prefix and include_prerelease keys
// of one source: ref and version are exclusive, the constraint must parse, and
// the refinements need a constraint.
func validateVersionKeys(kind, name, ref, version, prefix string, prerelease bool, minAge string) error {
	field := func(key string) string { return fmt.Sprintf("%s.%s", kind, key) }
	bad := func(key, format string, args ...any) error {
		return oops.With("field", field(key)).With("name", name).
			Errorf("%s %s %q: %s %s", codeConstraintInvalid, strings.TrimSuffix(strings.ReplaceAll(kind, "_", " "), "s"), name, key, fmt.Sprintf(format, args...))
	}
	version = strings.TrimSpace(version)
	switch {
	case version != "" && ref != "":
		return bad("version", "cannot be combined with ref: set one of them (ref names a git ref, version a range)")
	case version == "" && !IsVersionSugar(ref):
		if prefix != "" {
			return bad("tag_prefix", "needs version")
		}
		if prerelease {
			return bad("include_prerelease", "needs version")
		}
		if strings.TrimSpace(minAge) != "" {
			return bad("min_release_age", "needs version")
		}
		return nil
	}
	constraint := version
	key := "version"
	if constraint == "" {
		constraint, key = strings.TrimSpace(ref), "ref"
	}
	if _, err := semver.ParseConstraint(constraint); err != nil {
		return bad(key, "is not a valid version constraint: %s", err.Error())
	}
	if strings.ContainsAny(prefix, " \t\n") || strings.HasPrefix(prefix, "-") || hasControl(prefix) {
		return bad("tag_prefix", "must not contain whitespace or start with '-'")
	}
	if _, err := semver.ParseAge(minAge); err != nil {
		return bad("min_release_age", "is invalid: %s", err.Error())
	}
	return nil
}

// validateVersionKeys checks every include, installed skill and skill source.
func (c *Config) validateVersionKeys() error {
	for i := range c.Includes {
		inc := &c.Includes[i]
		if err := validateVersionKeys("includes", inc.Name, inc.Ref, inc.Version, inc.TagPrefix, inc.IncludePrerelease, inc.MinReleaseAge); err != nil {
			return err
		}
	}
	for i := range c.InstalledSkills {
		sk := &c.InstalledSkills[i]
		if err := validateVersionKeys("installed_skills", sk.Name, sk.Ref, sk.Version, sk.TagPrefix, sk.IncludePrerelease, sk.MinReleaseAge); err != nil {
			return err
		}
	}
	for i := range c.SkillSources {
		src := &c.SkillSources[i]
		if err := validateVersionKeys("skill_sources", src.Name, src.Ref, src.Version, src.TagPrefix, src.IncludePrerelease, src.MinReleaseAge); err != nil {
			return err
		}
	}
	return nil
}
