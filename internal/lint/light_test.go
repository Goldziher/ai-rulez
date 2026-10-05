package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

func TestReferenceUnknown_KindWordDoesNotHideAnotherNamespace(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":            baseConfig,
		".ai-rulez/agents/test-writer.md":  "---\nname: test-writer\ndescription: Writes tests. Use when tests are missing.\n---\nBody\n",
		".ai-rulez/context/style-guide.md": "# Style\n",
		".ai-rulez/rules/r.md": "# R\n" +
			"Follow the `test-writer` rules for new code.\n" +
			"Delegate to the `test-writer` agent.\n" +
			"See the `style-guide` rule and the `test-writer` skill.\n" +
			"Also the `ghost-thing` rules and the `ghost-thing` agent.\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)
	if n := countCode(fs, CodeReferenceUnknown); n != 2 {
		t.Fatalf("want AR301 only for ghost-thing (2 phrases), got %d: %+v", n, fs)
	}
	for _, f := range fs {
		if f.Code == CodeReferenceUnknown && !strings.Contains(f.Message, "ghost-thing") {
			t.Errorf("false positive: %+v", f)
		}
	}
}

func TestLoadTreeAt_ExplicitRepoRootResolvesPathsOfADetachedConfig(t *testing.T) {
	repo := t.TempDir()
	writeFiles(t, repo, map[string]string{
		"scripts/deploy.sh":     "#!/bin/sh\n",
		"src/app.py":            "x = 1\n",
		"shared/schema.json":    "{}\n",
		".ai-rulez/config.toml": baseConfig,
	})
	gitAdd(t, repo)

	scratch := t.TempDir()
	writeFiles(t, scratch, map[string]string{
		".ai-rulez/config.toml":            baseConfig,
		".ai-rulez/rules/scoped.md":        "---\npaths:\n  - \"src/**/*.py\"\n---\n# Scoped\nSee `shared/schema.json`.\n",
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploys. Use when shipping.\n---\nRun `scripts/deploy.sh` and read `references/guide.md`.\n",
	})
	cfg, err := config.LoadConfig(context.Background(), scratch)
	if err != nil {
		t.Fatal(err)
	}

	run := func(tree *Tree) []Finding {
		rep, rerr := Run(cfg, tree)
		if rerr != nil {
			t.Fatal(rerr)
		}
		return rep.Findings
	}
	detached, err := LoadTree(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if got := run(detached); !has(got, CodeSkillResourceMissing, "deploy/SKILL.md", 0) {
		t.Fatalf("without a repo root the detached config must report AR402: %+v", got)
	}
	tree, err := LoadTreeAt(scratch, repo)
	if err != nil {
		t.Fatal(err)
	}
	got := run(tree)
	if has(got, CodeGlobNoMatch, "scoped.md", 0) {
		t.Errorf("AR101 must resolve against the explicit repo root: %+v", got)
	}
	var missing []Finding
	for _, f := range got {
		if f.Code == CodeSkillResourceMissing {
			missing = append(missing, f)
		}
	}
	if len(missing) != 1 || !strings.Contains(missing[0].Message, "references/guide.md") ||
		!strings.Contains(missing[0].Message, "skill's directory") || !strings.Contains(missing[0].Message, "repo root") {
		t.Fatalf("want only the real missing references/guide.md, naming both bases: %+v", missing)
	}
}
