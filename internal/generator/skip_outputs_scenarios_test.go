package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

const skipClaudeCodexConfig = "agents_md = true\n"

func skipScenarioProject(t *testing.T, flag string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "codex"}, skipClaudeCodexConfig+flag, skipMCPTable))
	return root
}

func TestSkipOutputs_ScopeRootsFollowTheSameValues(t *testing.T) {
	tests := []struct {
		name      string
		skip      string
		wantAgent bool
		wantClaud bool
	}{
		{name: "nothing skipped", skip: "", wantAgent: true, wantClaud: true},
		{name: "claude_md keeps the scope AGENTS.md", skip: "skip_outputs = [\"claude_md\"]\n", wantAgent: true},
		{name: "agents_md keeps the scope CLAUDE.md", skip: "skip_outputs = [\"agents_md\"]\n", wantClaud: true},
		{name: "instructions skips both", skip: "skip_outputs = [\"instructions\"]\n"},
		{name: "rules keeps both roots", skip: "skip_outputs = [\"rules\"]\n", wantAgent: true, wantClaud: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			quietWarnings(t)
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, skipClaudeCodexConfig+tt.skip, claudeMDScopeBlock))
			writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			paths := agentsMDPaths(t, root)
			assert.Equal(t, tt.wantAgent, contains(paths, "packages/api/AGENTS.md"), "scope AGENTS.md in %v", paths)
			assert.Equal(t, tt.wantClaud, contains(paths, "packages/api/CLAUDE.md"), "scope CLAUDE.md in %v", paths)
		})
	}
}

func contains(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

func TestSkipOutputs_LocalOverlayCanSetIt(t *testing.T) {
	// Arrange: the shared config skips nothing; a machine-local overlay skips the instruction files
	quietWarnings(t)
	root := skipScenarioProject(t, "")
	writeAgentsMDFile(t, root, ".ai-rulez/config.local.toml", "skip_outputs = [\"instructions\"]\n")
	writeAgentsMDFile(t, root, "CLAUDE.md", "HAND CLAUDE\n")
	writeAgentsMDFile(t, root, "AGENTS.md", "HAND AGENTS\n")

	// Act
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	gen.SetAllowLocalDrift(true)
	require.NoError(t, gen.Generate(""))

	// Assert
	assert.Equal(t, []string{"instructions"}, cfg.SkipOutputs)
	assert.Equal(t, "HAND CLAUDE\n", readAgentsMDFile(t, root, "CLAUDE.md"))
	assert.Equal(t, "HAND AGENTS\n", readAgentsMDFile(t, root, "AGENTS.md"))
	assert.FileExists(t, filepath.Join(root, ".mcp.json"))
}

func TestSkipOutputs_GitignoreBlockOmitsSkippedPaths(t *testing.T) {
	tests := []struct {
		name    string
		skip    string
		listed  []string
		missing []string
	}{
		{
			name: "instructions", skip: "skip_outputs = [\"instructions\"]\n",
			listed:  []string{".claude/skills/", ".mcp.json"},
			missing: []string{"CLAUDE.md", "AGENTS.md"},
		},
		{
			name: "skills", skip: "skip_outputs = [\"skills\"]\n",
			listed:  []string{"CLAUDE.md", "AGENTS.md", ".mcp.json"},
			missing: []string{".claude/skills", ".agents/skills"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			quietWarnings(t)
			root := t.TempDir()
			initGitRepo(t, root)
			writeAgentsMDProject(t, root, "version = \"5.0\"\nname = \"ignore\"\ngitignore = true\npresets = [\"claude\", \"codex\"]\nagents_md = true\n"+tt.skip+skipMCPTable)

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			block := readAgentsMDFile(t, root, ".gitignore")
			for _, want := range tt.listed {
				assert.Contains(t, block, want)
			}
			for _, absent := range tt.missing {
				assert.NotContains(t, block, absent)
			}
		})
	}
}

func TestSkipOutputs_LockOutputDigestsOmitSkippedPaths(t *testing.T) {
	tests := []struct {
		name    string
		skip    string
		present []string
		absent  []string
	}{
		{name: "no skip", skip: "", present: []string{"CLAUDE.md", "AGENTS.md", ".claude/skills/alpha/SKILL.md"}},
		{
			name: "instructions", skip: "skip_outputs = [\"instructions\"]\n",
			present: []string{".claude/skills/alpha/SKILL.md"}, absent: []string{"CLAUDE.md", "AGENTS.md"},
		},
		{
			name: "skills", skip: "skip_outputs = [\"skills\"]\n",
			present: []string{"CLAUDE.md", "AGENTS.md"}, absent: []string{".claude/skills/alpha/SKILL.md", ".agents/skills/alpha/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			quietWarnings(t)
			root := skipScenarioProject(t, tt.skip)
			cfg, err := config.LoadConfig(context.Background(), root)
			require.NoError(t, err)

			// Act
			outputs, err := NewGenerator(cfg).LockOutputs("default")
			require.NoError(t, err)
			snap, err := contentlock.Compute(cfg, contentlock.Options{IncludeOutputs: true, Outputs: outputs})
			require.NoError(t, err)

			// Assert
			var digested []string
			for _, o := range snap.Outputs {
				digested = append(digested, o.Path)
			}
			for _, p := range tt.present {
				assert.Contains(t, digested, p)
			}
			for _, p := range tt.absent {
				assert.NotContains(t, digested, p)
			}
		})
	}
}

func TestSkipOutputs_UserScopeLeavesHandWrittenHomeFilesAlone(t *testing.T) {
	// Arrange
	quietWarnings(t)
	home, _ := newUserHome(t, "version = \"5.0\"\nname = \"me\"\npresets = [\"claude\", \"codex\"]\nskip_outputs = [\"instructions\"]\n", userFixture())
	writeAgentsMDFile(t, home, ".claude/CLAUDE.md", "HAND USER CLAUDE\n")
	writeAgentsMDFile(t, home, ".codex/AGENTS.md", "HAND USER CODEX\n")
	gen := loadUserGenerator(t, home)

	// Act
	plan, err := gen.GenerateUser("")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "HAND USER CLAUDE\n", readAgentsMDFile(t, home, ".claude/CLAUDE.md"))
	assert.Equal(t, "HAND USER CODEX\n", readAgentsMDFile(t, home, ".codex/AGENTS.md"))
	assert.FileExists(t, filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md"))
	assert.FileExists(t, filepath.Join(home, ".claude", "rules", "personal.md"))
	for _, w := range plan.Writes {
		assert.NotEqual(t, "CLAUDE.md", filepath.Base(w))
		assert.NotEqual(t, "AGENTS.md", filepath.Base(w))
	}
}

func TestSkipOutputs_PreviouslyGeneratedFileWarnsOnceEverAndLeavesTheManifest(t *testing.T) {
	// Arrange: a run that generated CLAUDE.md and AGENTS.md
	warned := quietWarnings(t)
	root := skipScenarioProject(t, "")
	runAgentsMDGenerate(t, root)
	require.Contains(t, sharedManifestFiles(t, root), "CLAUDE.md")
	generated := readAgentsMDFile(t, root, "CLAUDE.md")
	*warned = nil
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "codex"}, skipClaudeCodexConfig+"skip_outputs = [\"claude_md\"]\n", skipMCPTable))

	// Act: the run that skips it, then another one (the notice is recorded, so it is not repeated)
	runAgentsMDGenerate(t, root)
	firstWarnings := countContaining(*warned, "CLAUDE.md")
	runAgentsMDGenerate(t, root)

	// Assert
	assert.Equal(t, 1, firstWarnings, "warned once: %v", *warned)
	assert.Equal(t, 1, countContaining(*warned, "CLAUDE.md"), "the notice is recorded, so a later run stays quiet: %v", *warned)
	assert.Equal(t, generated, readAgentsMDFile(t, root, "CLAUDE.md"), "the earlier file stays")
	files := sharedManifestFiles(t, root)
	assert.NotContains(t, files, "CLAUDE.md")
	assert.Contains(t, files, "AGENTS.md")
}

func TestSkipOutputs_SkippedFileTheUserEditsIsNeverTouched(t *testing.T) {
	// Arrange
	quietWarnings(t)
	root := skipScenarioProject(t, "")
	runAgentsMDGenerate(t, root)
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "codex"}, skipClaudeCodexConfig+"skip_outputs = [\"claude_md\"]\n", skipMCPTable))
	runAgentsMDGenerate(t, root)
	writeAgentsMDFile(t, root, "CLAUDE.md", "MY OWN CLAUDE\n")

	// Act
	runAgentsMDGenerate(t, root)
	drift, err := skipGenerator(t, root).CheckDrift("")
	require.NoError(t, err)
	_, err = skipGenerator(t, root).Clean("", CleanOptions{})
	require.NoError(t, err)

	// Assert
	assert.Empty(t, drift)
	assert.Equal(t, "MY OWN CLAUDE\n", readAgentsMDFile(t, root, "CLAUDE.md"))
	_, statErr := os.Stat(filepath.Join(root, "AGENTS.md"))
	assert.True(t, os.IsNotExist(statErr), "clean removed the output it still manages")
}

func TestSkipOutputs_DoesNotMisclassifyNestedFilesNamedLikeARoot(t *testing.T) {
	tests := []struct {
		name string
		skip []string
		rel  string
		want bool
	}{
		{"skill resource CLAUDE.md under instructions", []string{"instructions"}, ".claude/skills/x/references/CLAUDE.md", false},
		{"skill resource AGENTS.md under agents_md", []string{"agents_md"}, ".claude/skills/x/references/AGENTS.md", false},
		{"rule named AGENTS.md under instructions", []string{"instructions"}, ".cursor/rules/AGENTS.md", false},
		{"scope GEMINI.md under instructions", []string{"instructions"}, "packages/api/GEMINI.md", true},
		{"skill resource CLAUDE.md under skills", []string{"skills"}, ".claude/skills/x/references/CLAUDE.md", true},
		{"rule named AGENTS.md under rules", []string{"rules"}, ".cursor/rules/AGENTS.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := newTestRegistryConfig(t, tt.skip)

			// Act
			got := skipOutputsMatcher(cfg).skips(tt.rel, false)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSkipOutputs_CommandRenderedAsSkillCountsAsCommand(t *testing.T) {
	tests := []struct {
		name        string
		agentsMD    string
		skip        string
		wantCommand bool
		wantSkill   bool
	}{
		{name: "nothing skipped", agentsMD: "true", skip: "", wantCommand: true, wantSkill: true},
		{name: "commands skipped", agentsMD: "true", skip: "commands", wantSkill: true},
		{name: "skills skipped", agentsMD: "true", skip: "skills", wantCommand: true},
		{name: "commands skipped without agents_md", agentsMD: "false", skip: "commands", wantSkill: true},
		{name: "skills skipped without agents_md", agentsMD: "false", skip: "skills", wantCommand: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			quietWarnings(t)
			root := t.TempDir()
			skip := ""
			if tt.skip != "" {
				skip = "skip_outputs = [\"" + tt.skip + "\"]\n"
			}
			writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "codex", "devin"}, "agents_md = "+tt.agentsMD+"\n"+skip, skipMCPTable))
			writeAgentsMDFile(t, root, ".ai-rulez/commands/deploy.md", "---\ndescription: Deploy\n---\nDEPLOY_BODY\n")
			writeAgentsMDFile(t, root, ".ai-rulez/skills/s1/SKILL.md", "---\nname: s1\ndescription: Skill one\n---\nS1_BODY\n")

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			paths := agentsMDPaths(t, root)
			for _, rel := range []string{".claude/skills/deploy/SKILL.md"} {
				assert.Equal(t, tt.wantCommand, contains(paths, rel), "%s in %v", rel, paths)
			}
			assert.Equal(t, tt.wantSkill, contains(paths, ".claude/skills/s1/SKILL.md"), "skill in %v", paths)
			if tt.agentsMD == "true" {
				assert.Equal(t, tt.wantCommand, contains(paths, ".agents/skills/deploy/SKILL.md"), "shared command skill in %v", paths)
				assert.Equal(t, tt.wantCommand, contains(paths, ".agents/skills/deploy/agents/openai.yaml"), "codex policy in %v", paths)
				assert.Equal(t, tt.wantSkill, contains(paths, ".agents/skills/s1/SKILL.md"), "shared skill in %v", paths)
			}
		})
	}
}
