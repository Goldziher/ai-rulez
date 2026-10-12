package config

import (
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// DeliveryConfig is the [delivery] table: how served skills are presented to
// the agent.
type DeliveryConfig struct {
	// Stub customises the generated dynamic-skills stub skill.
	Stub *StubConfig `yaml:"stub,omitempty" json:"stub,omitempty" toml:"stub,omitempty"`
}

// StubConfig is [delivery.stub]: the name and text of the stub skill that tells
// an MCP-capable agent to call find_skill and load_skill. Every field is
// optional; an unset one keeps the built-in text.
type StubConfig struct {
	// Description replaces the frontmatter description, which every session
	// lists, so a shorter one saves tokens.
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Body replaces the stub's text. {{server}} is the MCP server that serves
	// the skills. It must still tell the agent to call find_skill and load_skill.
	Body string `yaml:"body,omitempty" json:"body,omitempty" toml:"body,omitempty"`
	// BodyFile is Body read from a file, relative to the config directory.
	// Body and BodyFile are mutually exclusive.
	BodyFile string `yaml:"body_file,omitempty" json:"body_file,omitempty" toml:"body_file,omitempty"`
}

// The tools a stub body must tell the agent to call.
const stubTool1, stubTool2 = "find_skill", "load_skill"

// stubSettings returns [delivery.stub], never nil.
func (c *Config) stubSettings() *StubConfig {
	if c == nil || c.Delivery == nil || c.Delivery.Stub == nil {
		return &StubConfig{}
	}
	return c.Delivery.Stub
}

// stubBody returns the configured stub text (inline, or read from body_file at
// load), or "" when the built-in text applies.
func (c *Config) stubBody() string {
	if s := c.stubSettings(); s.Body != "" {
		return s.Body
	}
	return c.stubBodyFileText
}

// dynamicSkillsStub builds the stub skill for this project: the built-in text
// and description, each replaced by [delivery.stub], with {{server}} resolved.
func (c *Config) dynamicSkillsStub() ContentFile {
	body, description := dynamicSkillsBody, dynamicSkillsDescription
	if custom := c.stubBody(); strings.TrimSpace(custom) != "" {
		body = custom
	}
	if custom := strings.TrimSpace(c.stubSettings().Description); custom != "" {
		description = custom
	}
	return ContentFile{
		Name:     DynamicSkillsName,
		Path:     dynamicSkillsPath,
		Content:  expandTemplateVars(body, map[string]string{varServer: c.skillsServerName()}),
		Metadata: &Metadata{Extra: map[string]string{keyFrontmatterDescription: description}},
	}
}

// loadStubBodyFile reads [delivery.stub] body_file (relative to the config
// directory, never outside it) so rendering needs no I/O and a bad path is a
// load error rather than a silently built-in stub.
func (c *Config) loadStubBodyFile(v workspace.View) error {
	s := c.stubSettings()
	if s.BodyFile == "" || s.Body != "" {
		return nil // an inline body is reported by validation as a conflict
	}
	rel := filepath.FromSlash(s.BodyFile)
	if filepath.IsAbs(rel) || safefs.RelEscapes(filepath.Clean(rel)) {
		return oops.With("field", "delivery.stub.body_file").
			Hint("Use a path relative to the config directory that stays inside it").
			Errorf("delivery.stub.body_file %q must stay inside the config directory", s.BodyFile)
	}
	data, err := v.ReadFile(filepath.Join(c.ConfigDir, rel))
	if err != nil {
		return oops.With("field", "delivery.stub.body_file").
			Hint("Create the file, or remove body_file to use the built-in text").
			Wrapf(err, "delivery.stub.body_file %q cannot be read", s.BodyFile)
	}
	c.stubBodyFileText = string(data)
	return nil
}

// DeliveryStubKey identifies a customised stub for the source hash, so editing
// [delivery.stub] (or the file behind body_file) rewrites the skill trees that
// carry the stub. It is empty while the built-in stub applies.
func (c *Config) DeliveryStubKey() string {
	description := strings.TrimSpace(c.stubSettings().Description)
	body := c.stubBody()
	if description == "" && strings.TrimSpace(body) == "" {
		return ""
	}
	return description + "\x00" + body
}

// validateDeliveryStub checks [delivery.stub]: a description that says
// something, body or body_file but not both, only the documented variable, and
// a body that still tells the agent to call find_skill and load_skill, since the
// stub is the only pointer to served skills for a harness that lists skills.
func (c *Config) validateDeliveryStub() error {
	if c == nil || c.Delivery == nil || c.Delivery.Stub == nil {
		return nil
	}
	s := c.Delivery.Stub
	fail := func(hint, format string, args ...any) error {
		return oops.With("field", "delivery.stub").Hint(hint).Errorf(format, args...)
	}
	if s.Description != "" && strings.TrimSpace(s.Description) == "" {
		return fail("Write a description, or remove the key to use the built-in one", "delivery.stub.description must not be blank")
	}
	if s.Body != "" && s.BodyFile != "" {
		return fail("Keep one of them", "delivery.stub.body and delivery.stub.body_file are mutually exclusive")
	}
	body := c.stubBody()
	if body == "" {
		if s.BodyFile != "" || s.Body != "" {
			return fail("Write the text the stub shows, or remove the key", "delivery.stub body must not be blank")
		}
		return nil
	}
	if strings.TrimSpace(body) == "" {
		return fail("Write the text the stub shows, or remove the key", "delivery.stub body must not be blank")
	}
	if unknown := unknownTemplateVars(body, varServer); len(unknown) > 0 {
		return fail("Available variable: {{server}}", "delivery.stub body uses unknown variable {{%s}}", unknown[0])
	}
	for _, tool := range []string{stubTool1, stubTool2} {
		if !strings.Contains(body, tool) {
			return fail("The stub is how an agent learns to load served skills; name "+stubTool1+" and "+stubTool2,
				"delivery.stub body must tell the agent to call %s", tool)
		}
	}
	return nil
}
