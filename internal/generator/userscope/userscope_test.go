package userscope_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/generator/userscope"
)

func TestMap(t *testing.T) {
	tests := []struct {
		name, preset, rel string
		want              string
		ok                bool
	}{
		{"claude root file", "claude", "CLAUDE.md", ".claude/CLAUDE.md", true},
		{"claude skill file", "claude", ".claude/skills/x/SKILL.md", ".claude/skills/x/SKILL.md", true},
		{"claude skill resource", "claude", ".claude/skills/x/scripts/run.sh", ".claude/skills/x/scripts/run.sh", true},
		{"skills directory marker", "claude", ".claude/skills", ".claude/skills", true},
		{"claude plugins sidecar has no user location", "claude", ".claude/plugins.json", "", false},
		{"the parent directory marker is not mapped", "claude", ".claude", "", false},
		{"codex AGENTS.md", "codex", "AGENTS.md", ".codex/AGENTS.md", true},
		{"opencode AGENTS.md", "opencode", "AGENTS.md", ".config/opencode/AGENTS.md", true},
		{"opencode skills", "opencode", ".opencode/skills/x/SKILL.md", ".config/opencode/skills/x/SKILL.md", true},
		{"copilot skills", "copilot", ".github/skills/x/SKILL.md", ".copilot/skills/x/SKILL.md", true},
		{"copilot instructions are not documented", "copilot", ".github/copilot-instructions.md", "", false},
		{"pi skills", "pi", ".agents/skills/x/SKILL.md", ".pi/agent/skills/x/SKILL.md", true},
		{"codex commands are not documented", "codex", ".codex/commands/ship.md", "", false},
		{"a prefix is not a path prefix", "claude", ".claude/skills-extra/x.md", "", false},
		{"unsupported preset", "windsurf", ".windsurf/rules/x.md", "", false},
		{"backslashes are normalised", "claude", `.claude\skills\x\SKILL.md`, ".claude/skills/x/SKILL.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, entry, ok := userscope.Map(tt.preset, tt.rel)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
			if ok {
				assert.Equal(t, tt.preset, entry.Preset)
			}
		})
	}
}

func TestTableInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range userscope.Entries() {
		key := e.Preset + "|" + e.From
		assert.False(t, seen[key], "duplicate row %s", key)
		seen[key] = true
		for _, p := range []string{e.From, e.To} {
			assert.NotContains(t, p, "..", "%s", key)
			assert.False(t, filepath.IsAbs(p), "%s is relative", p)
			assert.Equal(t, strings.TrimSuffix(p, "/"), p, "%s has no trailing slash", p)
		}
		assert.True(t, strings.HasPrefix(e.Source, "https://"), "%s cites a vendor page", key)
		assert.NotEqual(t, "", e.To)
	}
	assert.NotEmpty(t, userscope.Roots())
	assert.NotContains(t, userscope.Roots(), ".claude", "a harness home is never a root clean may remove")
	assert.True(t, userscope.Supports("claude"))
	assert.False(t, userscope.Supports("windsurf"))
}

// TestDocsTable pins the "where things go" table of docs/user-scope.md to the
// code, including the verification date.
func TestDocsTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "user-scope.md"))
	require.NoError(t, err)

	type row struct{ harness, kind, from, to, verified string }
	var docs []row
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| Harness | Output |") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 5 || strings.HasPrefix(cells[0], "-") {
			continue
		}
		clean := func(s string) string { return strings.TrimSuffix(strings.Trim(s, "` "), "/") }
		docs = append(docs, row{clean(cells[0]), clean(cells[1]), clean(cells[2]), strings.TrimPrefix(clean(cells[3]), "~/"), clean(cells[4])})
	}

	var code []row
	for _, e := range userscope.Entries() {
		code = append(code, row{e.Preset, string(e.Kind), e.From, e.To, userscope.VerifiedOn})
	}
	assert.Equal(t, code, docs)
}
