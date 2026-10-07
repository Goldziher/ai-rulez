package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// CheckSeverities lists the values a check's `severity` frontmatter may take.
var CheckSeverities = []string{"low", "medium", "high", "critical"}

var checkNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// IsValidCheckName reports whether name is usable as a check identity: it is a
// file stem restricted to [A-Za-z0-9._-], so it can be used in output paths and
// section markers without escaping, and it cannot be a path traversal segment.
func IsValidCheckName(name string) bool {
	return checkNamePattern.MatchString(name) && name != "." && name != ".."
}

// CheckSeverity returns the declared severity of a check, lowercased, or "".
func CheckSeverity(f *ContentFile) string {
	return strings.ToLower(CheckMetaString(f, "severity"))
}

// CheckDescription returns the declared description of a check, or "".
func CheckDescription(f *ContentFile) string {
	return CheckMetaString(f, "description")
}

// CheckMetaString reads a string frontmatter key of a content file.
func CheckMetaString(f *ContentFile, key string) string {
	if f == nil || f.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(f.Metadata.Extra[key])
}

func (c *Config) validateCheckSlice(checks []ContentFile, scope string) error {
	for i := range checks {
		check := &checks[i]
		c.warnInvalidTargets(*check)
		if !IsValidCheckName(check.Name) {
			return oops.
				With("check", check.Name).
				With("path", check.Path).
				Hint("Rename the file so its name uses only letters, digits, '.', '_' and '-'.").
				Errorf("invalid check name %q in %s: only [A-Za-z0-9._-] is allowed", check.Name, scope)
		}
		severity := CheckSeverity(check)
		if severity == "" {
			continue
		}
		valid := false
		for _, v := range CheckSeverities {
			if severity == v {
				valid = true
				break
			}
		}
		if !valid {
			return oops.
				With("field", fmt.Sprintf("%s check[%s].severity", scope, check.Name)).
				With("actual_value", severity).
				With("valid_values", CheckSeverities).
				Hint("Use one of: low, medium, high, critical.").
				Errorf("invalid severity %q for check %q in %s", severity, check.Name, scope)
		}
	}
	return nil
}

// validateChecks validates check names and severities in every scope.
func (c *Config) validateChecks() error {
	if c.Content == nil {
		return nil
	}
	if err := c.validateCheckSlice(c.Content.Checks, "root"); err != nil {
		return err
	}
	names := make([]string, 0, len(c.Content.Domains))
	for name := range c.Content.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		domain := c.Content.Domains[name]
		if domain == nil {
			continue
		}
		if err := c.validateCheckSlice(domain.Checks, "domain "+name); err != nil {
			return err
		}
	}
	return nil
}
