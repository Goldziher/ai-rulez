package lint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// lintWithTargets lints the project at root with the targets spec resolves to.
func lintWithTargets(t *testing.T, root string, spec TargetSpec) ([]Finding, *Targets) {
	t.Helper()
	cfg, err := loadWithResolvers(context.Background(), root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	spec.Cwd = root
	targets, err := ResolveTargets(context.Background(), tree, spec)
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	rep, err := Run(cfg, tree, WithTargets(targets), WithCwd(root))
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings, targets
}

const oddMarketplace = `{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":"plugins/a"}]}`

func TestTargetsFindOddLocationMarketplaceByContent(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":     baseConfig,
		"catalog/odd/registry.json": oddMarketplace,
	})
	gitAdd(t, root)

	tests := []struct {
		name string
		spec TargetSpec
		want bool
	}{
		{name: "no target, no finding", spec: TargetSpec{}, want: false},
		{name: "glob", spec: TargetSpec{Globs: []string{"catalog/**/*.json"}}, want: true},
		{name: "directory path", spec: TargetSpec{Paths: []string{"catalog"}}, want: true},
		{name: "file path", spec: TargetSpec{Paths: []string{"catalog/odd/registry.json"}}, want: true},
		{name: "glob matching another file", spec: TargetSpec{Globs: []string{"nothing/**"}}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, _ := lintWithTargets(t, root, tc.spec)
			if got := has(fs, CodePluginManifest, "registry.json", 0); got != tc.want {
				t.Errorf("AR963 on registry.json = %v, want %v\n%s", got, tc.want, dump(fs))
			}
		})
	}
}

func TestTargetsSniffPluginAndHooksOutsideTheirDirectories(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		"x/plug.json":           `{"name":"Bad Name","version":"1.0.0"}`,
		"x/hooklist.json":       `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"sh ${CLAUDE_PLUGIN_ROOT}/a.sh"}]}]}}`,
		"x/package.json":        `{"name":"Bad Name","version":"1.0.0","scripts":{}}`,
	})
	gitAdd(t, root)
	fs, targets := lintWithTargets(t, root, TargetSpec{Paths: []string{"x"}})
	if !has(fs, CodePluginManifest, "plug.json", 0) {
		t.Errorf("a plugin manifest outside .claude-plugin/ was not checked\n%s", dump(fs))
	}
	if !has(fs, CodePluginManifest, "hooklist.json", 0) {
		t.Errorf("a hook file outside hooks/ was not checked\n%s", dump(fs))
	}
	for _, f := range targets.Files {
		if strings.HasSuffix(f.Rel, "package.json") {
			t.Errorf("package.json is not a plugin manifest: classified as %q", f.Kind)
		}
	}
}

func TestTargetsLintUntrackedSkillAnywhere(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	// Written after the index was taken: git does not track it.
	writeFiles(t, root, map[string]string{"elsewhere/skills/loose/SKILL.md": "---\nname: loose\n---\nbody\n"})

	fs, _ := lintWithTargets(t, root, TargetSpec{Paths: []string{"elsewhere"}})
	if !has(fs, CodeDescriptionMissing, "elsewhere/skills/loose/SKILL.md", 0) {
		t.Errorf("an untracked SKILL.md named by --path was not linted\n%s", dump(fs))
	}
	fs, _ = lintWithTargets(t, root, TargetSpec{})
	if countCode(fs, CodeDescriptionMissing) != 0 {
		t.Errorf("a file nobody named was linted\n%s", dump(fs))
	}
}

func TestResolveTargetsRefusesPathsOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, "in/a.md": "# a\n"})
	writeFiles(t, outside, map[string]string{"secret.md": "# secret\n"})
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
	}{
		{name: "parent directory", path: ".."},
		{name: "absolute outside", path: filepath.Join(outside, "secret.md")},
		{name: "missing file", path: "nope.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ResolveTargets(context.Background(), tree, TargetSpec{Paths: []string{tc.path}, Cwd: root}); err == nil {
				t.Errorf("--path %q was accepted", tc.path)
			}
		})
	}
}

func TestResolveTargetsRefusesASymlinkedPathLeavingTheProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, "in/a.md": "# a\n"})
	writeFiles(t, outside, map[string]string{"secret.md": "# secret\n"})
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "in", "link"))
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveTargets(context.Background(), tree, TargetSpec{Paths: []string{"in/link/secret.md"}, Cwd: root}); err == nil {
		t.Error("a path through a symlink that leaves the project was accepted")
	}
}

func TestResolveTargetsSkipsSymlinkedFilesLeavingTheProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, "in/ok.md": "# ok\n"})
	writeFiles(t, outside, map[string]string{"secret.md": "# secret\n"})
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.md"), filepath.Join(root, "in", "leak.md"))
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Paths: []string{"in"}, Cwd: root})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(targets.Rels(), ",")
	if got != "in/ok.md" {
		t.Errorf("targets = %q, want only in/ok.md", got)
	}
}

func TestResolveTargetsReportsGlobsThatMatchNothing(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Globs: []string{"zzz/**"}, Cwd: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets.Unmatched) != 1 || targets.Unmatched[0] != "zzz/**" {
		t.Errorf("Unmatched = %v, want [zzz/**]", targets.Unmatched)
	}
}

func TestTreeWithFilesLeavesTheReceiverAlone(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"a.md": "# a\n", "d/b.md": "# b\n"})
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	extended := tree.WithFiles([]string{"d/new.md"})
	if tree.Exists("d/new.md") {
		t.Error("the receiver gained a file")
	}
	found := false
	for _, p := range extended.Paths() {
		found = found || p == "d/new.md"
	}
	if !found {
		t.Error("the copy does not list the added file")
	}
}

func TestClassifyPath(t *testing.T) {
	tests := []struct {
		rel          string
		kind         string
		conventional bool
	}{
		{"AGENTS.md", kindContext, true},
		{"pkg/CLAUDE.md", kindContext, true},
		{"anything/SKILL.md", kindSkill, true},
		{".claude/agents/reviewer.md", kindAgent, true},
		{".claude/commands/ship.md", kindCommand, true},
		{".claude/rules/style.md", kindRule, true},
		{".cursor/rules/style.mdc", kindRule, true},
		{".agents/checks/api.md", kindCheck, true},
		{"docs/rules/style.md", kindRule, false},
		{"docs/guide.md", kindContext, false},
		{".claude/skills/x/references/a.md", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.rel, func(t *testing.T) {
			kind, conventional := classifyPath(tc.rel)
			if kind != tc.kind || conventional != tc.conventional {
				t.Errorf("classifyPath(%q) = %q, %v; want %q, %v", tc.rel, kind, conventional, tc.kind, tc.conventional)
			}
		})
	}
}

func TestSniffJSON(t *testing.T) {
	tests := []struct {
		name, file, body, kind, key string
	}{
		{"marketplace with owner", "m.json", `{"name":"a","owner":{"name":"x"},"plugins":[]}`, targetMarketplace, ""},
		{"marketplace by plugin sources", "m.json", `{"plugins":[{"name":"a","source":"./a"}]}`, targetMarketplace, ""},
		{"plugin manifest", "p.json", `{"name":"p","version":"1.0.0"}`, targetPlugin, ""},
		{"plugin with inline hooks", "p.json", `{"name":"p","version":"1.0.0","hooks":{"Stop":[]}}`, targetPlugin, ""},
		{"hooks", "h.json", `{"hooks":{"Stop":[{"hooks":[]}]}}`, targetHooks, ""},
		{"mcp servers", ".mcp.json", `{"mcpServers":{"a":{"command":"x"}}}`, targetMCP, "mcpServers"},
		{"vscode servers", "mcp.json", `{"servers":{"a":{"command":"x"}}}`, targetMCP, "servers"},
		{"npm package", "package.json", `{"name":"p","version":"1.0.0"}`, "", ""},
		{"npm package under another name", "pkg.json", `{"name":"p","version":"1.0.0","scripts":{"a":"b"}}`, "", ""},
		{"unrelated json", "data.json", `{"a":1}`, "", ""},
		{"not json", "x.json", `nope`, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, key := sniffJSON(tc.file, []byte(tc.body))
			if kind != tc.kind || key != tc.key {
				t.Errorf("sniffJSON = %q, %q; want %q, %q", kind, key, tc.kind, tc.key)
			}
		})
	}
}
