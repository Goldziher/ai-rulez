package agentssource_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// allKinds is one file of every content kind the standard and the documented
// extension layout under .agents can hold.
var allKinds = map[string]string{
	".agents/rules/style.md":                          "---\ndescription: Style\n---\n\nRULE_BODY\n",
	".agents/context/architecture.md":                 "# Architecture\n\nCONTEXT_BODY\n",
	".agents/skills/review/SKILL.md":                  "---\nname: review\ndescription: Review a change\n---\n\nSKILL_BODY\n",
	".agents/skills/review/references/guide.md":       "# Guide\n\nREF_BODY\n",
	".agents/skills/review/scripts/check.sh":          "#!/bin/sh\necho SCRIPT_BODY\n",
	".agents/skills/review/assets/template.txt":       "ASSET_BODY\n",
	".agents/agents/planner.md":                       "---\ndescription: Plans work\n---\n\nAGENT_BODY\n",
	".agents/commands/ship.md":                        "---\ndescription: Ship it\n---\n\nCOMMAND_BODY\n",
	".agents/checks/naming.md":                        "---\ndescription: Naming\nseverity: warning\n---\n\nCHECK_BODY\n",
	".agents/domains/backend/rules/db.md":             "---\ndescription: DB\n---\n\nDOMAIN_RULE_BODY\n",
	".agents/domains/backend/context/layout.md":       "# Layout\n\nDOMAIN_CONTEXT_BODY\n",
	".agents/domains/backend/skills/migrate/SKILL.md": "---\nname: migrate\ndescription: Migrate\n---\n\nDOMAIN_SKILL_BODY\n",
	".agents/domains/backend/agents/dba.md":           "---\ndescription: DBA\n---\n\nDOMAIN_AGENT_BODY\n",
	".agents/domains/backend/commands/seed.md":        "---\ndescription: Seed\n---\n\nDOMAIN_COMMAND_BODY\n",
	".agents/domains/backend/checks/sql.md":           "---\ndescription: SQL\nseverity: warning\n---\n\nDOMAIN_CHECK_BODY\n",
}

func kindsRoot(t *testing.T, cfgPath, cfgBody string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, cfgPath, cfgBody)
	for rel, content := range allKinds {
		write(t, root, rel, content)
	}
	return root
}

func resourcePaths(f *config.ContentFile) []string {
	out := make([]string, 0, len(f.Resources))
	for _, r := range f.Resources {
		out = append(out, r.RelPath)
	}
	sort.Strings(out)
	return out
}

func TestEveryContentKindUnderAgentsIsLoaded(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := kindsRoot(t, layout.cfgPath, layout.cfgBody)

			// Act
			cfg := load(t, root)

			// Assert
			tree := cfg.Content
			assert.Equal(t, []string{"style"}, names(tree.Rules))
			assert.Equal(t, []string{"architecture"}, names(tree.Context))
			assert.Equal(t, []string{"review"}, names(tree.Skills))
			assert.Equal(t, []string{"planner"}, names(tree.Agents))
			assert.Equal(t, []string{"ship"}, names(tree.Commands))
			assert.Equal(t, []string{"naming"}, names(tree.Checks))
			require.Len(t, tree.Skills, 1)
			assert.Equal(t, []string{"assets/template.txt", "references/guide.md", "scripts/check.sh"}, resourcePaths(&tree.Skills[0]))
			domain := tree.Domains["backend"]
			require.NotNil(t, domain)
			assert.Equal(t, []string{"db"}, names(domain.Rules))
			assert.Equal(t, []string{"layout"}, names(domain.Context))
			assert.Equal(t, []string{"migrate"}, names(domain.Skills))
			assert.Equal(t, []string{"dba"}, names(domain.Agents))
			assert.Equal(t, []string{"seed"}, names(domain.Commands))
			assert.Equal(t, []string{"sql"}, names(domain.Checks))
		})
	}
}

func TestEveryContentKindUnderAgentsIsRendered(t *testing.T) {
	// Arrange
	root := kindsRoot(t, ".agents/ai-rulez.toml", presetConfig(""))

	// Act
	generate(t, root)
	out := snapshot(t, root)

	// Assert
	all := strings.Builder{}
	for rel, content := range out {
		if !strings.HasPrefix(rel, ".agents/") {
			all.WriteString(rel + "\n" + content + "\n")
		}
	}
	for _, marker := range []string{
		"RULE_BODY", "CONTEXT_BODY", "SKILL_BODY", "AGENT_BODY", "COMMAND_BODY",
		"DOMAIN_RULE_BODY", "DOMAIN_CONTEXT_BODY", "DOMAIN_SKILL_BODY", "DOMAIN_AGENT_BODY", "DOMAIN_COMMAND_BODY",
	} {
		assert.Contains(t, all.String(), marker)
	}
	assert.Contains(t, out, ".claude/skills/review/references/guide.md")
	assert.Equal(t, "#!/bin/sh\necho SCRIPT_BODY\n", out[".claude/skills/review/scripts/check.sh"])
	assert.Equal(t, "ASSET_BODY\n", out[".claude/skills/review/assets/template.txt"])
	assert.Contains(t, out, ".claude/agents/planner.md")
	assert.Contains(t, out, ".claude/skills/ship/SKILL.md", "Claude renders a command as a user-invocable skill")
	assert.Contains(t, out, ".cursor/commands/ship.md")
	assert.Contains(t, out[".cursor/BUGBOT.md"], "CHECK_BODY")
	assert.Contains(t, out[".cursor/BUGBOT.md"], "DOMAIN_CHECK_BODY")
	for rel, content := range allKinds {
		assert.Equal(t, content, out[rel], "%s must not change", rel)
	}
}
