package config

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// OKFSpecVersion is the only OKF spec version ai-rulez writes.
const OKFSpecVersion = "0.2"

// DefaultOKFDir is where the okf preset writes its bundle, relative to the
// project root.
const DefaultOKFDir = "docs/okf"

// PresetOKF is the opt-in preset that keeps an OKF bundle in sync.
const PresetOKF = "okf"

// okfKinds are the values accepted in okf.include.
var okfKinds = []string{rulesDir, contextDir, skillsDir, agentsDir, commandsDir, "checks"}

// OKFConfig configures the Open Knowledge Format bundle written by the okf
// preset and linted by `validate --strict`.
type OKFConfig struct {
	// Dir is the bundle directory, relative to the project root. Default docs/okf.
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty" toml:"dir,omitempty"`
	// Include limits the content kinds exported (rules, context, skills, agents,
	// commands, checks). Empty means all.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	// Spec pins the OKF spec version. Only "0.2" is implemented.
	Spec string `yaml:"spec,omitempty" json:"spec,omitempty" toml:"spec,omitempty"`
	// IndexStyle selects the index.md scheme: "body" (default, the OKF 0.2
	// listing in the body) or "frontmatter" (title, version and entries in the
	// frontmatter).
	IndexStyle string `yaml:"index_style,omitempty" json:"index_style,omitempty" toml:"index_style,omitempty"`
}

// OKF index styles accepted in okf.index_style.
const (
	OKFIndexStyleBody        = "body"
	OKFIndexStyleFrontmatter = "frontmatter"
)

// OKFIndexStyle returns the configured index.md style; the default is "body".
func (c *Config) OKFIndexStyle() string {
	if c != nil && c.OKF != nil && c.OKF.IndexStyle != "" {
		return c.OKF.IndexStyle
	}
	return OKFIndexStyleBody
}

// OKFEnabled reports whether the okf preset is configured.
func (c *Config) OKFEnabled() bool {
	return c.HasBuiltInPreset(PresetOKF)
}

// OKFDir returns the bundle directory, relative to the project root.
func (c *Config) OKFDir() string {
	if c != nil && c.OKF != nil && strings.TrimSpace(c.OKF.Dir) != "" {
		return filepath.ToSlash(filepath.Clean(c.OKF.Dir))
	}
	return DefaultOKFDir
}

// OKFInclude returns the configured kinds (empty for all).
func (c *Config) OKFInclude() []string {
	if c == nil || c.OKF == nil {
		return nil
	}
	return c.OKF.Include
}

func (c *Config) validateOKF() error {
	for i := range c.Includes {
		if f := c.Includes[i].Format; f != "" && f != IncludeFormatOKF {
			return oops.
				With("field", "includes.format").
				With("include", c.Includes[i].Name).
				Hint(`The only supported format is "okf".`).
				Errorf("unknown format %q for include %q", f, c.Includes[i].Name)
		}
	}
	if c.OKF == nil {
		return nil
	}
	if c.OKF.Spec != "" && c.OKF.Spec != OKFSpecVersion {
		return oops.
			With("field", "okf.spec").
			With("actual_value", c.OKF.Spec).
			Hint("Only OKF spec "+OKFSpecVersion+" is implemented.").
			Errorf("unsupported okf.spec %q", c.OKF.Spec)
	}
	if st := c.OKF.IndexStyle; st != "" && st != OKFIndexStyleBody && st != OKFIndexStyleFrontmatter {
		return oops.
			With("field", "okf.index_style").
			With("actual_value", st).
			With("valid_values", []string{OKFIndexStyleBody, OKFIndexStyleFrontmatter}).
			Hint(`Use "body" (the OKF 0.2 listing) or "frontmatter".`).
			Errorf("unknown okf.index_style %q", st)
	}
	if dir := strings.TrimSpace(c.OKF.Dir); dir != "" {
		if err := ValidateScopePath(dir); err != nil {
			return oops.
				With("field", "okf.dir").
				Hint("Use a relative directory inside the project, such as docs/okf.").
				Wrapf(err, "invalid okf.dir %q", dir)
		}
	}
	if top := strings.SplitN(filepath.ToSlash(filepath.Clean(c.OKF.Dir)), "/", 2)[0]; top == ".git" || strings.HasPrefix(top, ".ai-rulez") {
		return oops.
			With("field", "okf.dir").
			Hint("Choose a directory outside the configuration directory and .git, such as docs/okf.").
			Errorf("okf.dir %q must not be inside %s", c.OKF.Dir, top)
	}
	for _, kind := range c.OKF.Include {
		if !slices.Contains(okfKinds, strings.ToLower(strings.TrimSpace(kind))) {
			return oops.
				With("field", "okf.include").
				With("actual_value", kind).
				With("valid_values", okfKinds).
				Errorf("unknown kind %q in okf.include", kind)
		}
	}
	return nil
}
