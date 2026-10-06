package templates

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/pelletier/go-toml/v2"
)

// InitSubdirs are the content directories `ai-rulez init` creates inside the
// configuration directory.
var InitSubdirs = []string{"rules", "context", "skills", "agents", "domains"}

// InitConfigTOML is the .ai-rulez/config.toml `ai-rulez init` writes for a new
// project; the MCP init_project tool writes the same document.
//
// The name is written through the TOML encoder, so a quote or newline in it cannot
// add keys; an empty name is written as "project" (name is required and non-empty).
func InitConfigTOML(projectName string, presets []string) string {
	if len(presets) == 0 {
		presets = []string{"claude"}
	}
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = tomlString(p)
	}
	if strings.TrimSpace(projectName) == "" {
		projectName = "project"
	}
	var b strings.Builder
	b.WriteString(`# AI-Rulez Configuration
# Directory-based configuration with domain scoping
# Documentation: https://github.com/Goldziher/ai-rulez

# Version (required)
version = "4.0"

# Project name (required)
name = `)
	b.WriteString(tomlString(projectName))
	b.WriteString(`

# Optional description
# description = "AI-powered development governance for `)
	b.WriteString(commentText(projectName))
	b.WriteString(`"

# Presets: built-in tools or custom outputs
# Built-in presets: `)
	b.WriteString(strings.Join(config.IndividualPresetNames(), ", "))
	b.WriteString(`
presets = [`)
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(`]

# Default profile to use when generating
# default = "full"

# Named profiles (domain combinations)
# [profiles]
# full = ["backend", "frontend", "qa"]
# backend = ["backend"]
# frontend = ["frontend"]

# Gitignore management
# gitignore = true

# MCP Servers (optional)
# [[mcp_servers]]
# name = "ai-rulez"
# command = "npx"
# args = ["-y", "ai-rulez@latest", "mcp"]
`)
	return b.String()
}

// tomlString renders s as a TOML string value using the encoder, so every quote,
// backslash and control character is escaped.
func tomlString(s string) string {
	out, err := toml.Marshal(map[string]string{"v": s})
	if err != nil {
		return `""`
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "v = "))
}

// commentText makes s safe inside a one-line TOML comment: line breaks and other
// control characters become spaces.
func commentText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			return ' '
		}
		return r
	}, s)
}
