package plugin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

const (
	catalogPathScheme    = "generated://"
	catalogMaxSkillNames = 8
	catalogDescription   = "Lists the plugins this repository offers, what each one covers, and how to install or " +
		"enable them. Use when a task needs skills, agents or commands that are not loaded."
)

// CatalogSkill builds the generated catalog skill for the planned plugins. It is
// a synthetic content file: it has no source file and is never bundled into a
// plugin.
func CatalogSkill(cfg *config.Config, plan []PlannedPlugin) config.ContentFile {
	cat := cfg.Marketplace.CatalogSkill
	name := cat.SkillName()
	description := cat.Description
	if description == "" {
		description = catalogDescription
	}
	return config.ContentFile{
		Name:     name,
		Path:     catalogPathScheme + name + "/SKILL.md",
		Content:  catalogBody(cfg, plan),
		Metadata: &config.Metadata{Extra: map[string]string{"description": description}},
	}
}

func catalogBody(cfg *config.Config, plan []PlannedPlugin) string {
	mkt := cfg.Marketplace
	var b strings.Builder
	fmt.Fprintf(&b, "# Plugin catalog\n\nThe `%s` marketplace offers the plugins below. They are not loaded until enabled.\n\n", mkt.Name)
	b.WriteString("| Plugin | Default | Covers | Skills |\n| --- | --- | --- | --- |\n")
	for i := range plan {
		p := &plan[i]
		state := "on"
		if p.DefaultEnabled != nil && !*p.DefaultEnabled {
			state = "off"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", p.Name, state, cell(p.Description), skillList(p.Skills))
	}
	dir := marketplaceDir(mkt)
	fmt.Fprintf(&b, "\n## Enable a plugin\n\n")
	fmt.Fprintf(&b, "1. Register the marketplace once (skip when `.claude/settings.json` already lists `%s` under `extraKnownMarketplaces`): `claude plugin marketplace add %s`\n", mkt.Name, dir)
	fmt.Fprintf(&b, "2. Install: `claude plugin install <plugin>@%s`, or run `/plugin` and pick it.\n", mkt.Name)
	fmt.Fprintf(&b, "3. Or enable it for this project in `.claude/settings.local.json`:\n\n```json\n{\n  \"enabledPlugins\": { \"<plugin>@%s\": true }\n}\n```\n\n", mkt.Name)
	b.WriteString("Run `/reload-plugins` or start a new session afterwards.\n")
	return b.String()
}

func marketplaceDir(mkt *config.MarketplaceAuthoring) string {
	if mkt.OutputDir == "" || mkt.OutputDir == "." {
		return "."
	}
	return "./" + strings.TrimPrefix(strings.ReplaceAll(mkt.OutputDir, `\`, "/"), "./")
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "/"), "\n", " ")
}

func skillList(skills []config.ContentFile) string {
	names := make([]string, 0, len(skills))
	for i := range skills {
		names = append(names, skills[i].Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "-"
	}
	more := 0
	if len(names) > catalogMaxSkillNames {
		more = len(names) - catalogMaxSkillNames
		names = names[:catalogMaxSkillNames]
	}
	out := "`" + strings.Join(names, "`, `") + "`"
	if more > 0 {
		out += fmt.Sprintf(" (+%d more)", more)
	}
	return out
}
