package templates

import (
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// InitSubdirs are the content directories `ai-rulez init` creates inside the
// configuration directory.
var InitSubdirs = []string{"rules", "context", "skills", "agents", "domains"}

// InitConfigTOML is the .ai-rulez/config.toml `ai-rulez init` writes for a new
// project; the MCP init_project tool writes the same document.
func InitConfigTOML(projectName string, presets []string) string {
	if len(presets) == 0 {
		presets = []string{"claude"}
	}
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = strconv.Quote(p)
	}
	var b strings.Builder
	b.WriteString(`# AI-Rulez Configuration
# Directory-based configuration with domain scoping
# Documentation: https://github.com/Goldziher/ai-rulez

# Version (required)
version = "4.0"

# Project name (required)
name = "`)
	b.WriteString(projectName)
	b.WriteString(`"

# Optional description
# description = "AI-powered development governance for `)
	b.WriteString(projectName)
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
