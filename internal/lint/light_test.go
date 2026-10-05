package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func lightFindings(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	cfg, err := config.LoadConfig(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	return FrontmatterWarnings(cfg, tree)
}

func TestFrontmatterWarnings_AgentUnknownKeyAndMissingSkill(t *testing.T) {
	files := map[string]string{
		".ai-rulez/config.toml":           baseConfig,
		".ai-rulez/skills/alpha/SKILL.md": "---\nname: alpha\ndescription: Deploy things. Use when deploying.\n---\nBody\n",
		".ai-rulez/agents/a.md": "---\nname: a\ndescription: Does things. Use when needed.\nskills: [alpha, ghost]\n" +
			"disallowedTools: [Write]\ndisallowed_tool: [Edit]\npermission_mode: plan\nmaxTurns: 3\nmemmory: project\n---\nBody\n",
	}
	got := lightFindings(t, files)
	var unknown, skill []Finding
	for _, f := range got {
		if f.Severity != SeverityWarning {
			t.Errorf("plain validate findings must be warnings: %+v", f)
		}
		switch f.Code {
		case CodeFrontmatterKey:
			unknown = append(unknown, f)
		case CodeFrontmatterSkill:
			skill = append(skill, f)
		}
	}
	if len(skill) != 1 || !strings.Contains(skill[0].Message, `"ghost"`) {
		t.Fatalf("want one missing-skill warning for ghost, got %+v", skill)
	}
	if len(unknown) != 2 {
		t.Fatalf("want 2 unknown-key warnings (disallowed_tool, memmory), got %+v", unknown)
	}
	for _, f := range unknown {
		if !strings.Contains(f.Message, "did you mean") {
			t.Errorf("unknown key warning lacks a suggestion: %s", f.Message)
		}
	}
	if !strings.Contains(unknown[0].Message+unknown[1].Message, `"disallowedTools"`) || !strings.Contains(unknown[0].Message+unknown[1].Message, `"memory"`) {
		t.Errorf("suggestions wrong: %+v", unknown)
	}
}

func TestFrontmatterWarnings_SkillFromDomainIsKnown(t *testing.T) {
	files := map[string]string{
		".ai-rulez/config.toml":                         baseConfig,
		".ai-rulez/domains/backend/skills/dom/SKILL.md": "---\nname: dom\ndescription: Domain skill. Use when needed.\n---\nBody\n",
		".ai-rulez/agents/a.md":                         "---\nname: a\ndescription: Does things. Use when needed.\nskills: [dom]\n---\nBody\n",
	}
	for _, f := range lightFindings(t, files) {
		if f.Code == CodeFrontmatterSkill {
			t.Fatalf("domain skill flagged: %+v", f)
		}
	}
}

func TestStrictAgentSkillStillError(t *testing.T) {
	files := map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".ai-rulez/agents/a.md": "---\nname: a\ndescription: Does things. Use when needed.\nskills: [ghost]\n---\nBody\n",
	}
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	fs := lintDir(t, root)
	for _, f := range fs {
		if f.Code == CodeFrontmatterSkill && f.Severity != SeverityError {
			t.Fatalf("strict AR302 must stay an error: %+v", f)
		}
	}
	if countCode(fs, CodeFrontmatterSkill) != 1 {
		t.Fatalf("want one AR302, got %+v", fs)
	}
}
