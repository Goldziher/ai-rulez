package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// The Claude skill listing total (AR964) measures Claude Code's listing whatever [exposure] harness is set.
func TestClaudeListingTotalIgnoresTheExposureHarness(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\", \"codex\"]\n\n[exposure]\nharness = \"codex\"\n",
	}
	for i := range 41 {
		files[fmt.Sprintf(".claude/skills/s%02d/SKILL.md", i)] = fmt.Sprintf("---\nname: s%02d\ndescription: %s\n---\nbody\n", i, strings.Repeat("d", 290))
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	run := func(table string) []string {
		cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
		body := strings.SplitN(files[".ai-rulez/config.toml"], "\n[exposure]", 2)[0] + "\n[exposure]\n" + table
		require.NoError(t, os.WriteFile(cfgPath, []byte(body), 0o600))
		cfg, err := exposureConfigFor(t.Context(), root)
		require.NoError(t, err)
		tree, err := lint.LoadTree(root)
		require.NoError(t, err)
		rep, err := lint.Run(cfg, tree, exposureLintOptions(t.Context(), cfg, nil)...)
		require.NoError(t, err)
		var msgs []string
		for _, f := range rep.Findings {
			if f.Code == lint.CodeLoadBudget && strings.Contains(f.Message, "skill listing budget") {
				msgs = append(msgs, f.Message)
			}
		}
		return msgs
	}

	claude := run("harness = \"claude\"\n")
	codex := run("harness = \"codex\"\n")

	require.NotEmpty(t, claude, "the claude listing warning fires under harness = claude")
	hasClaude := func(ms []string) bool {
		for _, m := range ms {
			if strings.Contains(m, "Claude") || strings.Contains(m, "claude") {
				return true
			}
		}
		return false
	}
	require.True(t, hasClaude(claude), "%v", claude)
	require.True(t, hasClaude(codex), "the claude listing warning still fires under harness = codex: %v", codex)
}
