package config

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// PresetLLMsTxt is the opt-in preset that renders the content tree as llms.txt.
const PresetLLMsTxt = "llms-txt"

// llmsTxtKinds are the values accepted in llms_txt.include.
var llmsTxtKinds = []string{rulesDir, contextDir, skillsDir, agentsDir, commandsDir}

// LLMsTxtConfig configures the llms-txt preset.
type LLMsTxtConfig struct {
	// Dir is the directory llms.txt is written to, relative to the project root.
	// Empty means the project root.
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty" toml:"dir,omitempty"`
	// Title is the H1 of the file. Empty means the project name.
	Title string `yaml:"title,omitempty" json:"title,omitempty" toml:"title,omitempty"`
	// Summary is the blockquote under the title. Empty means the project description.
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty" toml:"summary,omitempty"`
	// Full also writes llms-full.txt, which holds the full text of every listed item.
	Full bool `yaml:"full,omitempty" json:"full,omitempty" toml:"full,omitempty"`
	// Include limits the content kinds listed (rules, context, skills, agents,
	// commands). Empty means rules, context and skills. Agents and commands are
	// listed in the Optional section.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
}

// LLMsTxtEnabled reports whether the llms-txt preset is configured.
func (c *Config) LLMsTxtEnabled() bool {
	return c.HasBuiltInPreset(PresetLLMsTxt)
}

// LLMsTxtSettings returns the [llms_txt] table, or an empty one.
func (c *Config) LLMsTxtSettings() LLMsTxtConfig {
	if c == nil || c.LLMsTxt == nil {
		return LLMsTxtConfig{}
	}
	return *c.LLMsTxt
}

// LLMsTxtDir returns the output directory, relative to the project root ("." for the root).
func (c *Config) LLMsTxtDir() string {
	if dir := strings.TrimSpace(c.LLMsTxtSettings().Dir); dir != "" {
		return filepath.ToSlash(filepath.Clean(dir))
	}
	return "."
}

func (c *Config) validateLLMsTxt() error {
	if c.LLMsTxt == nil {
		return nil
	}
	l := c.LLMsTxt
	if dir := strings.TrimSpace(l.Dir); dir != "" {
		if err := ValidateScopePath(dir); err != nil {
			return oops.With("field", "llms_txt.dir").
				Hint("Use a relative directory inside the project, such as docs.").
				Wrapf(err, "invalid llms_txt.dir %q", dir)
		}
		if top := strings.SplitN(filepath.ToSlash(filepath.Clean(dir)), "/", 2)[0]; top == ".git" || strings.HasPrefix(top, ".ai-rulez") {
			return oops.With("field", "llms_txt.dir").
				Hint("Choose a directory outside the configuration directory and .git.").
				Errorf("llms_txt.dir %q must not be inside %s", dir, top)
		}
	}
	for _, kind := range l.Include {
		if !slices.Contains(llmsTxtKinds, strings.ToLower(strings.TrimSpace(kind))) {
			return oops.With("field", "llms_txt.include").
				With("actual_value", kind).
				With("valid_values", llmsTxtKinds).
				Errorf("unknown kind %q in llms_txt.include", kind)
		}
	}
	return nil
}
