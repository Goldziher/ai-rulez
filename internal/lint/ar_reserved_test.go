package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func reservedSkill(name string) string {
	return "---\nname: " + name + "\ndescription: Use when testing reserved names of the lint rules.\n---\nbody\n"
}

// lintDetected lints the files the way `validate` does: the content tree plus
// the hand-written files a convention names (.claude/skills, ...).
func lintDetected(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	got, _ := lintWithTargets(t, root, TargetSpec{Detect: true})
	return got
}

func TestReservedNameAR808(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		config string
		want   []string // paths expected to carry AR808
		msg    string
	}{
		{
			name:  "authored skill shadows a built-in",
			files: map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			want:  []string{"skills/init/SKILL.md"},
			msg:   `skill "init" shadows the Claude Code built-in /init (preset claude)`,
		},
		{
			name:  "hand-written claude skills",
			files: map[string]string{".claude/skills/init/SKILL.md": reservedSkill("init"), ".claude/skills/security-review/SKILL.md": reservedSkill("security-review")},
			want:  []string{"skills/init/SKILL.md", "skills/security-review/SKILL.md"},
		},
		{
			name:  "command shadows a built-in",
			files: map[string]string{".ai-rulez/commands/clear.md": "---\ndescription: Clear things up for the test.\n---\nbody\n"},
			want:  []string{"commands/clear.md"},
			msg:   `command "clear" shadows the Claude Code built-in /clear (preset claude)`,
		},
		{
			name:  "name is normalized",
			files: map[string]string{".ai-rulez/skills/x/SKILL.md": reservedSkill(" Add_Dir ")},
			want:  []string{"skills/x/SKILL.md"},
		},
		{
			name:  "ordinary name is fine",
			files: map[string]string{".ai-rulez/skills/deploy-helper/SKILL.md": reservedSkill("deploy-helper")},
		},
		{
			name:   "allowed by name",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"init\"]\n",
		},
		{
			name:   "allowed by preset and name",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"claude:Init\"]\n",
		},
		{
			name:   "another preset entry does not allow it",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"codex:init\"]\n",
			want:   []string{"skills/init/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".ai-rulez/config.toml": baseConfig + tt.config}
			for k, v := range tt.files {
				files[k] = v
			}
			got := lintDetected(t, files)
			n := 0
			for _, f := range got {
				if f.Code != "AR808" {
					continue
				}
				n++
				if f.Severity != SeverityError {
					t.Errorf("severity = %s, want error", f.Severity)
				}
				if tt.msg != "" && !strings.Contains(f.Message, tt.msg) {
					t.Errorf("message %q lacks %q", f.Message, tt.msg)
				}
			}
			if n != len(tt.want) {
				t.Fatalf("AR808 count = %d, want %d:\n%s", n, len(tt.want), dump(got))
			}
			for _, w := range tt.want {
				if !has(got, "AR808", w, 0) {
					t.Errorf("missing AR808 on %s:\n%s", w, dump(got))
				}
			}
		})
	}
}

func TestReservedNameConfigless(t *testing.T) {
	files := map[string]string{
		"ai-rulez.toml":                           "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n",
		".claude/skills/init/SKILL.md":            reservedSkill("init"),
		".claude/skills/review/SKILL.md":          reservedSkill("review"),
		".claude/skills/security-review/SKILL.md": reservedSkill("security-review"),
	}
	got := lintDetected(t, files)
	if n := countCode(got, "AR808"); n != 3 {
		t.Fatalf("AR808 count = %d, want 3:\n%s", n, dump(got))
	}
}

func TestReservedNameOtherPresetOnly(t *testing.T) {
	files := map[string]string{
		".ai-rulez/config.toml":          strings.Replace(baseConfig, `presets = ["claude"]`, `presets = ["cursor"]`, 1),
		".ai-rulez/skills/init/SKILL.md": reservedSkill("init"),
	}
	if got := lintDetected(t, files); countCode(got, "AR808") != 0 {
		t.Fatalf("AR808 reported without the claude preset:\n%s", dump(got))
	}
}

// lintConfigFree lints the files with the implicit configuration `validate` uses without one.
func lintConfigFree(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	cfg, err := config.LoadImplicit(context.Background(), root)
	if err != nil {
		t.Fatalf("load implicit config: %v", err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Detect: true, Cwd: root})
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	rep, err := Run(cfg, tree, WithTargets(targets), WithCwd(root))
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func TestReservedNameSkillRoots(t *testing.T) {
	pluginJSON := `{"name":"p","version":"1.0.0","description":"A plugin for the test."}`
	tests := []struct {
		name   string
		files  map[string]string
		config string // appended to baseConfig; "-" runs without a configuration
		want   []string
	}{
		{
			name:   "application skill folder is not a harness root",
			files:  map[string]string{"x/src/public/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "-",
		},
		{
			name:  "application skill folder next to a configuration",
			files: map[string]string{"x/src/public/skills/code-review/SKILL.md": reservedSkill("code-review")},
		},
		{
			name:   "nested claude skills dir is a root",
			files:  map[string]string{"x/.claude/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "-",
			want:   []string{"x/.claude/skills/code-review/SKILL.md"},
		},
		{
			name: "plugin skills dir is a root",
			files: map[string]string{
				"plug/.claude-plugin/plugin.json":    pluginJSON,
				"plug/skills/code-review/SKILL.md":   reservedSkill("code-review"),
				"other/skills/code-review/SKILL.md":  reservedSkill("code-review"),
				".claude/commands/compact.md":        "---\ndescription: Compact things for the test.\n---\nbody\n",
				"tools/src/skills/review/SKILL.md":   reservedSkill("review"),
				"tools/src/skills/review/notes/x.md": "x\n",
			},
			config: "-",
			want:   []string{"plug/skills/code-review/SKILL.md", ".claude/commands/compact.md"},
		},
		{
			name:   "local skill source dir is a root",
			files:  map[string]string{"vendor/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "\n[[skill_sources]]\nname = \"vendored\"\nurl = \"./vendor\"\npath = \"skills\"\n",
			want:   []string{"vendor/skills/code-review/SKILL.md"},
		},
		{
			name:   "local skill source with backslashes is a root",
			files:  map[string]string{"vendor/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "\n[[skill_sources]]\nname = \"vendored\"\nurl = '.\\vendor'\npath = 'skills'\n",
			want:   []string{"vendor/skills/code-review/SKILL.md"},
		},
		{
			name:   "windows drive path skill source is not a project root",
			files:  map[string]string{"vendor/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "\n[[skill_sources]]\nname = \"abs\"\nurl = 'C:\\vendor'\npath = 'skills'\n",
		},
		{
			name:   "skill_roots widens the roots",
			files:  map[string]string{"x/src/public/skills/code-review/SKILL.md": reservedSkill("code-review")},
			config: "\n[lint]\nskill_roots = [\"x/**/skills\"]\n",
			want:   []string{"x/src/public/skills/code-review/SKILL.md"},
		},
		{
			name:   "skill_roots narrows the roots",
			files:  map[string]string{".claude/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nskill_roots = [\"plugins/*/skills\"]\n",
		},
		{
			name:   "authored content is checked whatever skill_roots says",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nskill_roots = [\"plugins/*/skills\"]\n",
			want:   []string{"skills/init/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.config != "-" {
				files[".ai-rulez/config.toml"] = baseConfig + tt.config
			}
			for k, v := range tt.files {
				files[k] = v
			}
			lintFiles := lintDetected
			if tt.config == "-" {
				lintFiles = lintConfigFree
			}
			got := lintFiles(t, files)
			if n := countCode(got, "AR808"); n != len(tt.want) {
				t.Fatalf("AR808 count = %d, want %d:\n%s", n, len(tt.want), dump(got))
			}
			for _, w := range tt.want {
				if !has(got, "AR808", w, 0) {
					t.Errorf("missing AR808 on %s:\n%s", w, dump(got))
				}
			}
		})
	}
}

// A SKILL.md outside every harness root is still linted for format (#457).
func TestReservedNameOutsideRootStillFormatLinted(t *testing.T) {
	got := lintConfigFree(t, map[string]string{
		"x/src/public/skills/code-review/SKILL.md": "---\nname: code-review\n---\nbody\n",
	})
	if countCode(got, "AR808") != 0 {
		t.Fatalf("AR808 outside a harness root:\n%s", dump(got))
	}
	if !has(got, CodeDescriptionMissing, "x/src/public/skills/code-review/SKILL.md", 0) {
		t.Errorf("the skill was not format-linted:\n%s", dump(got))
	}
}
