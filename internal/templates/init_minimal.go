package templates

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// InitMinimalConfigTOML is the ai-rulez.toml written by `init --minimal`: a
// configuration without a .ai-rulez/ folder, with commented examples of the
// settings that file is for. The #:schema line is the directive taplo and Even
// Better TOML read to find the JSON schema.
func InitMinimalConfigTOML(projectName string, presets []string) string {
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
	b.WriteString("#:schema https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json\n")
	b.WriteString(`# AI-Rulez configuration without a .ai-rulez/ folder.
# CLAUDE.md, AGENTS.md and the other instruction files stay yours: with this file
# alone ai-rulez never writes or deletes them. It installs skills, wires MCP
# servers, writes hooks and permissions, lints, and pins everything in ai-rulez.lock.
# Documentation: https://github.com/Goldziher/ai-rulez/blob/main/docs/config-without-folder.md

version = "5.0"
name = `)
	b.WriteString(tomlString(projectName))
	b.WriteString(`

# Tools to write skills, MCP servers, hooks and permissions for.
# Built-in presets: `)
	b.WriteString(strings.Join(config.IndividualPresetNames(), ", "))
	b.WriteString(`
presets = [`)
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(`]

# Skills fetched from another repository and written into each tool's skill folder.
# [[installed_skills]]
# name = "my-skill"
# source = "https://github.com/org/repo"
# ref = "v1.0.0"

# Lint settings for 'ai-rulez validate' (the configuration, the lock and imported content).
# [lint]
# fail_on = "warning"
# ignore = ["AR401"]
`)
	return b.String()
}
