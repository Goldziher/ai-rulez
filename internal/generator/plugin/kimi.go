package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeKimi, renderKimi)
}

// sessionStartDoc is Kimi's session-start hook (run a skill on session init).
type sessionStartDoc struct {
	Skill string `json:"skill,omitempty"`
}

// kimiManifest is the shape of kimi.plugin.json. Kimi shares the top-level
// skills/ directory (skills: "./skills/") and adds session-start + free-text
// skill-routing instructions plus the shared interface{} block.
type kimiManifest struct {
	Name              string              `json:"name"`
	Version           string              `json:"version"`
	Description       string              `json:"description,omitempty"`
	Author            *config.Author      `json:"author,omitempty"`
	Homepage          string              `json:"homepage,omitempty"`
	License           string              `json:"license,omitempty"`
	Keywords          []string            `json:"keywords,omitempty"`
	Skills            string              `json:"skills,omitempty"`
	MCPServers        map[string]mcpEntry `json:"mcpServers,omitempty"`
	SessionStart      *sessionStartDoc    `json:"sessionStart,omitempty"`
	SkillInstructions string              `json:"skillInstructions,omitempty"`
	Interface         *interfaceDoc       `json:"interface,omitempty"`
}

func renderKimi(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	doc := kimiManifest{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Homepage:    m.Homepage,
		License:     m.License,
		Keywords:    m.Keywords,
		Skills:      skillsDirRef(m),
		MCPServers:  mcpServersFor(m, config.PluginRuntimeKimi),
		Interface:   buildInterface(m.Interface),
	}
	if m.Kimi != nil {
		doc.SkillInstructions = m.Kimi.SkillInstructions
		if m.Kimi.SessionStartSkill != "" {
			doc.SessionStart = &sessionStartDoc{Skill: m.Kimi.SessionStartSkill}
		}
	}

	manifest, err := jsonOutput(filepath.Join(baseDir, "kimi.plugin.json"), doc)
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{manifest}

	// Kimi shares the top-level skills/commands directories (skills: "./skills/").
	// Bundle them here too so a Kimi-only plugin is self-contained; when Claude is
	// also emitted, the identical root files are de-duplicated in renderRuntimes.
	content, err := bundleContent(m, baseDir, contentLayout{Skills: true, Commands: true, Agents: true})
	if err != nil {
		return nil, err
	}
	return append(outputs, content...), nil
}
