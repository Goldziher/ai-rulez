package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
)

// reservedPreset is the built-in slash commands and bundled skills of one
// harness. A project skill or command with one of these names replaces the
// built-in (Claude Code) or hides it, so the built-in stops working (AR808).
type reservedPreset struct {
	// Title names the harness in the message.
	Title string
	// Source is the vendor page the names were taken from, verified on Verified.
	Source, Verified string
	// Names are the built-in names and aliases as the table records them; a name is
	// compared with harnessnames.Normalize.
	Names []string
	// Aliases maps an alias to the command it belongs to. The harness keeps running the
	// built-in for an alias, whatever a project skill does.
	Aliases map[string]string
	// SkillRoots and CommandRoots are the directories (slash paths below any project directory)
	// the harness loads project skills and commands from. A hand-written skill or command
	// elsewhere cannot shadow a built-in. Plugin says the harness also loads the skills/ and
	// commands/ directories of a plugin.
	SkillRoots, CommandRoots []string
	Plugin                   bool
}

// reservedPresetTable is the table, keyed by preset name. The names are not listed here: they
// come from the per-harness tables of internal/harnessnames (the commands and bundled skills a
// user item replaces), so AR808 and AR9U0 read one source. Add a harness by adding an entry,
// and list a name only when the vendor documents it.
func reservedPresetTable() map[string]reservedPreset {
	claude := reservedPreset{
		Title:        "Claude Code",
		Source:       "https://code.claude.com/docs/en/commands",
		SkillRoots:   []string{".claude/skills"},
		CommandRoots: []string{".claude/commands"},
		Plugin:       true,
		Aliases:      map[string]string{},
	}
	if t, ok := harnessnames.For(presetClaude); ok {
		claude.Verified = t.VerifiedOn
		for _, e := range t.Entries([]string{harnessnames.KindCommand, harnessnames.KindSkill}, harnessnames.UserWins) {
			claude.Names = append(claude.Names, e.Name)
			if e.Alias {
				claude.Aliases[harnessnames.Normalize(e.Name)] = e.AliasOf
			}
		}
	}
	return map[string]reservedPreset{presetClaude: claude}
}
