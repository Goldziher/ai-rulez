package config

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
	"github.com/samber/oops"
)

var verifierNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateVerifiers checks every [[verifiers]] entry: unique safe names, a
// known type, the fields that type needs, and patterns that compile.
func (c *Config) validateVerifiers() error {
	seen := map[string]bool{}
	for i := range c.Verifiers {
		v := &c.Verifiers[i]
		field := fmt.Sprintf("verifiers[%d]", i)
		if err := validateVerifierName(v, field, seen); err != nil {
			return err
		}
		if err := c.validateVerifierBody(v, field); err != nil {
			return oops.With("field", field).With("verifier", v.Name).Wrap(err)
		}
	}
	return nil
}

func validateVerifierName(v *VerifierConfig, field string, seen map[string]bool) error {
	if !verifierNameRe.MatchString(v.Name) {
		return oops.With("field", field+".name").
			Hint("Use letters, digits, '.', '_' and '-'.").
			Errorf("invalid verifier name %q at %s.name", v.Name, field)
	}
	if seen[v.Name] {
		return oops.With("field", field+".name").Errorf("duplicate verifier name %q", v.Name)
	}
	seen[v.Name] = true
	return nil
}

func (c *Config) validateVerifierBody(v *VerifierConfig, field string) error {
	known := false
	for _, t := range VerifierTypes {
		known = known || t == v.Type
	}
	if !known {
		return oops.Hint("Use one of: "+strings.Join(VerifierTypes, ", ")).
			Errorf("unknown verifier type %q at %s.type", v.Type, field)
	}
	switch v.Severity {
	case "", "error", "warning", "info":
	default:
		return oops.Hint("Use one of: error, warning, info.").
			Errorf("invalid verifier severity %q at %s.severity", v.Severity, field)
	}
	if err := validateVerifierFields(v, field); err != nil {
		return err
	}
	switch v.Type {
	case VerifierFileExists, VerifierFileAbsent:
		return validateVerifierPath(v, field)
	case VerifierGlobCount:
		return validateVerifierGlobCount(v, field)
	case VerifierRegex, VerifierForbid:
		return validateVerifierRegex(v, field)
	case VerifierKeyEquals:
		if err := validateVerifierPath(v, field); err != nil {
			return err
		}
		if v.Key == "" {
			return oops.Errorf("verifier type key_equals needs %s.key", field)
		}
		if v.Equals == nil {
			return oops.Errorf("verifier type key_equals needs %s.equals", field)
		}
		return validateVerifierKey(v, field)
	case VerifierGeneratedInSync:
		return c.validateVerifierProfile(v, field)
	}
	return nil
}

// verifierFields maps each type to the type-specific fields it uses; setting
// any other is rejected so a misplaced field is not silently ignored.
var verifierFields = map[string][]string{
	VerifierFileExists:      {"path"},
	VerifierFileAbsent:      {"path"},
	VerifierGlobCount:       {"glob", "exclude", "min", "max"},
	VerifierRegex:           {"glob", "exclude", "pattern"},
	VerifierForbid:          {"glob", "exclude", "pattern"},
	VerifierKeyEquals:       {"path", "key", "equals"},
	VerifierGeneratedInSync: {"profile"},
}

func validateVerifierFields(v *VerifierConfig, field string) error {
	set := map[string]bool{
		"path": v.Path != "", "glob": v.Glob != "", "exclude": len(v.Exclude) > 0, "pattern": v.Pattern != "",
		"min": v.Min != nil, "max": v.Max != nil, "key": v.Key != "", "equals": v.Equals != nil, "profile": v.Profile != "",
	}
	for _, f := range verifierFields[v.Type] {
		delete(set, f)
	}
	var extra []string
	for _, f := range []string{"path", "glob", "exclude", "pattern", "min", "max", "key", "equals", "profile"} {
		if set[f] {
			extra = append(extra, f)
		}
	}
	if len(extra) > 0 {
		return oops.Hint(fmt.Sprintf("Type %s uses: %s.", v.Type, strings.Join(verifierFields[v.Type], ", "))).
			Errorf("verifier %s sets %s, which does not apply to type %s", field, strings.Join(extra, ", "), v.Type)
	}
	return nil
}

func validateVerifierKey(v *VerifierConfig, field string) error {
	switch strings.ToLower(filepath.Ext(v.Path)) {
	case ".json", ".yaml", ".yml", ".toml":
	default:
		return oops.Hint("Use a .json, .yaml, .yml or .toml file.").
			Errorf("verifier %s.path %q has an unsupported format for key_equals", field, v.Path)
	}
	if _, err := vspec.ParseKey(v.Key); err != nil {
		return oops.Hint(`Separate segments with '.', escape a literal dot as '\.' or write ["a.b"].`).
			Wrapf(err, "invalid verifier %s.key", field)
	}
	return nil
}

func (c *Config) validateVerifierProfile(v *VerifierConfig, field string) error {
	if v.Profile == "" {
		return nil
	}
	for _, name := range SplitProfileNames(v.Profile) {
		if _, ok := c.Profiles[name]; !ok && name != "default" {
			return oops.Hint("Define the profile under [profiles] or use an existing one.").
				Errorf("verifier %s.profile %q is not a defined profile", field, name)
		}
	}
	return nil
}

func validateVerifierGlobs(v *VerifierConfig, field string) error {
	if _, err := vspec.CompileGlob(v.Glob); err != nil {
		return oops.Wrapf(err, "invalid verifier %s.glob %q", field, v.Glob)
	}
	for i, e := range v.Exclude {
		if _, err := vspec.CompileGlob(e); err != nil {
			return oops.Wrapf(err, "invalid verifier %s.exclude[%d] %q", field, i, e)
		}
	}
	return nil
}

func validateVerifierPath(v *VerifierConfig, field string) error {
	if v.Path == "" {
		return oops.Errorf("verifier type %s needs %s.path", v.Type, field)
	}
	clean := path.Clean(strings.ReplaceAll(v.Path, "\\", "/"))
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return oops.Hint("Use a path relative to the project root that stays inside it.").
			Errorf("verifier %s.path %q must be relative and inside the project", field, v.Path)
	}
	return nil
}

func validateVerifierGlobCount(v *VerifierConfig, field string) error {
	if v.Glob == "" {
		return oops.Errorf("verifier type glob_count needs %s.glob", field)
	}
	if v.Min == nil && v.Max == nil {
		return oops.Errorf("verifier type glob_count needs a min or max at %s", field)
	}
	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		return oops.Errorf("verifier %s.min (%d) is greater than max (%d)", field, *v.Min, *v.Max)
	}
	if (v.Min != nil && *v.Min < 0) || (v.Max != nil && *v.Max < 0) {
		return oops.Errorf("verifier %s.min and max must not be negative", field)
	}
	return validateVerifierGlobs(v, field)
}

func validateVerifierRegex(v *VerifierConfig, field string) error {
	if v.Glob == "" {
		return oops.Errorf("verifier type %s needs %s.glob", v.Type, field)
	}
	if v.Pattern == "" {
		return oops.Errorf("verifier type %s needs %s.pattern", v.Type, field)
	}
	if _, err := regexp.Compile(v.Pattern); err != nil {
		return oops.Wrapf(err, "invalid verifier %s.pattern", field)
	}
	return validateVerifierGlobs(v, field)
}
