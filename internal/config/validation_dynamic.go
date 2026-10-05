package config

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/samber/oops"
)

var (
	sourceNameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	namePrefixRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	errDeliveryHint = "Use one of: static, served, both"
)

// validateDynamicSkills checks [skills], [domains.<x>] and [[skill_sources]].
func (c *Config) validateDynamicSkills() error {
	if c.Skills != nil && c.Skills.Delivery != "" {
		if _, ok := ParseDelivery(c.Skills.Delivery); !ok {
			return oops.With("field", "skills.delivery").Hint(errDeliveryHint).
				Errorf("invalid skills.delivery %q", c.Skills.Delivery)
		}
	}
	domains := make([]string, 0, len(c.DomainSettings))
	for name := range c.DomainSettings {
		domains = append(domains, name)
	}
	sort.Strings(domains)
	for _, name := range domains {
		if v := c.DomainSettings[name].Delivery; v != "" {
			if _, ok := ParseDelivery(v); !ok {
				return oops.With("field", "domains."+name+".delivery").Hint(errDeliveryHint).
					Errorf("invalid delivery %q for domain %q", v, name)
			}
		}
	}
	return c.validateSkillSources()
}

func (c *Config) validateSkillSources() error {
	seen := map[string]bool{}
	for i := range c.SkillSources {
		if err := c.SkillSources[i].Validate(i); err != nil {
			return err
		}
		name := c.SkillSources[i].Name
		if seen[name] {
			return oops.With("field", "skill_sources").Hint("Each skill source needs a unique name").
				Errorf("duplicate skill source name: %q", name)
		}
		seen[name] = true
	}
	return nil
}

// Validate checks one [[skill_sources]] entry; index is used in messages only.
func (s *SkillSourceConfig) Validate(index int) error {
	field := func(f string) string { return fmt.Sprintf("skill_sources[%d].%s", index, f) }
	if !sourceNameRe.MatchString(s.Name) {
		return oops.With("field", field("name")).Hint("Use lowercase letters, digits, '.', '_' and '-'").
			Errorf("skill source at index %d has an invalid name %q", index, s.Name)
	}
	if strings.TrimSpace(s.URL) == "" {
		return oops.With("field", field("url")).Hint("Provide a git URL or a local directory").
			Errorf("skill source %q missing required field 'url'", s.Name)
	}
	if err := gitutil.CheckArg("url", strings.TrimPrefix(s.URL, "git+")); err != nil {
		return oops.With("field", field("url")).Hint("A url or ref that starts with '-' would be read by git as an option").
			Errorf("skill source %q: %s", s.Name, err.Error())
	}
	if err := gitutil.CheckArg("ref", s.Ref); err != nil {
		return oops.With("field", field("ref")).Hint("A url or ref that starts with '-' would be read by git as an option").
			Errorf("skill source %q: %s", s.Name, err.Error())
	}
	if s.NamePrefix != "" && !namePrefixRe.MatchString(s.NamePrefix) {
		return oops.With("field", field("name_prefix")).Hint("The prefix becomes part of a skill:// path segment").
			Errorf("skill source %q has an invalid name_prefix %q", s.Name, s.NamePrefix)
	}
	switch s.Trust {
	case "", TrustError, TrustWarn:
	default:
		return oops.With("field", field("trust")).Hint("Use 'error' (default) or 'warn'").
			Errorf("skill source %q has an invalid trust level %q", s.Name, s.Trust)
	}
	if clean := path.Clean(strings.Trim(s.Path, "/")); s.Path != "" && (clean == ".." || strings.HasPrefix(clean, "../")) {
		return oops.With("field", field("path")).Errorf("skill source %q path %q escapes the repository", s.Name, s.Path)
	}
	for _, list := range [][]string{s.Include, s.Exclude} {
		for _, g := range list {
			if _, err := path.Match(g, ""); err != nil {
				return oops.With("field", field("include/exclude")).Errorf("skill source %q has an invalid glob %q", s.Name, g)
			}
		}
	}
	return nil
}
