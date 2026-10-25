package lint

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// lintWithTargets lints the project at root with the targets spec resolves to.
func lintWithTargets(t *testing.T, root string, spec TargetSpec, opts ...Option) ([]Finding, *Targets) {
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
	rep, err := Run(cfg, tree, append([]Option{WithTargets(targets), WithCwd(root)}, opts...)...)
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

// onCaseInsensitiveFS skips the test unless the file system of dir treats two
// spellings of a name as one file (the default on macOS and Windows).
func onCaseInsensitiveFS(t *testing.T, dir, name string) {
	t.Helper()
	a, errA := os.Stat(filepath.Join(dir, name))
	b, errB := os.Stat(filepath.Join(dir, strings.ToLower(name)))
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Skip("the file system is case-sensitive")
	}
}

func TestResolveTargetsTakesTheRealCaseOfAPathOnACaseInsensitiveFS(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, "AGENTS.md": "# a\n"})
	gitAdd(t, root)
	onCaseInsensitiveFS(t, root, "AGENTS.md")
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "lower case spelling", paths: []string{"agents.md"}},
		{name: "both spellings", paths: []string{"agents.md", "AGENTS.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Paths: tc.paths, Cwd: root})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(targets.Rels(), ","); got != "AGENTS.md" {
				t.Fatalf("targets = %q, want AGENTS.md once", got)
			}
			if targets.Files[0].Kind != kindContext {
				t.Errorf("kind = %q, want %q", targets.Files[0].Kind, kindContext)
			}
		})
	}
}

const plainToken = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"

func TestNamedTextFilesOfAnyExtensionGetTheSecretScan(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".env":                  "API_TOKEN=" + plainToken + "\n",
		"deploy/values.yaml":    "token: " + plainToken + "\n",
		"notes.txt":             "the key is " + plainToken + "\n",
		"data/odd.json":         `{"token":"` + plainToken + `"}`,
	})
	gitAdd(t, root)
	tests := []struct {
		name string
		spec TargetSpec
		want string
	}{
		{name: ".env by path", spec: TargetSpec{Paths: []string{".env"}}, want: ".env"},
		{name: "yaml by path", spec: TargetSpec{Paths: []string{"deploy/values.yaml"}}, want: "deploy/values.yaml"},
		{name: "text by glob", spec: TargetSpec{Globs: []string{"*.txt"}}, want: "notes.txt"},
		{name: ".env by glob", spec: TargetSpec{Globs: []string{".env*"}}, want: ".env"},
		{name: "json that is no known kind", spec: TargetSpec{Paths: []string{"data/odd.json"}}, want: "data/odd.json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, _ := lintWithTargets(t, root, tc.spec)
			if !has(fs, CodeSecretDetected, tc.want, 0) {
				t.Errorf("no AR001 on %s\n%s", tc.want, dump(fs))
			}
		})
	}
}

func TestFilesPickedThroughADirectoryAreNotScannedAsPlainText(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, "d/.env": "API_TOKEN=" + plainToken + "\n"})
	gitAdd(t, root)

	fs, _ := lintWithTargets(t, root, TargetSpec{Paths: []string{"d"}})

	if countCode(fs, CodeSecretDetected) != 0 {
		t.Errorf("a directory pick lints recognized kinds only\n%s", dump(fs))
	}
}

func TestEveryPickThatIsNotLintedIsReported(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		"big.log":               strings.Repeat("A", maxReadBytes+1),
		"blob.bin":              "ab\x00cd",
		"gen/AGENTS.md":         "<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\n\n# a\n",
		"d/.env":                "API_TOKEN=" + plainToken + "\n",
		"d/blob.bin":            "ab\x00cd",
	})
	writeFiles(t, outside, map[string]string{"secret.txt": "x\n"})
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt"))
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		spec TargetSpec
		want string
	}{
		{name: "oversize by glob", spec: TargetSpec{Globs: []string{"*.log"}}, want: "big.log: not linted (larger than 2 MiB)"},
		{name: "binary by glob", spec: TargetSpec{Globs: []string{"*.bin"}}, want: "blob.bin: not linted (binary file)"},
		{name: "generated by path", spec: TargetSpec{Paths: []string{"gen/AGENTS.md"}}, want: "gen/AGENTS.md: not linted (generated file: lint it with --outputs)"},
		{name: "symlink leaving the project by glob", spec: TargetSpec{Globs: []string{"leak.txt"}}, want: "leak.txt: not linted (resolves outside the project)"},
		{name: "files a directory leaves out", spec: TargetSpec{Paths: []string{"d"}}, want: "2 file(s) below a --path directory were not linted (not content, or generated): d/.env, d/blob.bin"},
		{name: "glob matching nothing", spec: TargetSpec{Globs: []string{"zzz/**"}}, want: "no file matches --glob \"zzz/**\""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.Cwd = root
			targets, err := ResolveTargets(context.Background(), tree, tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			notices := strings.Join(targets.Notices(), "\n")
			if !strings.Contains(notices, tc.want) {
				t.Errorf("notices = %q, want one containing %q", notices, tc.want)
			}
		})
	}
}

func TestNoticesReachTheCombinedReport(t *testing.T) {
	combined := Combine([]*Report{{Root: ".", Notices: []string{"a.log: not linted (binary file)"}}})
	var sb strings.Builder

	if err := Write(&sb, FormatJSON, combined, WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(sb.String(), `"notices": [`) || !strings.Contains(sb.String(), "a.log: not linted (binary file)") {
		t.Errorf("the JSON report lacks the notice:\n%s", sb.String())
	}
}

func TestAnExplicitFileThatCannotBeReadIsAnError(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".env": strings.Repeat("A", maxReadBytes+1)})
	gitAdd(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ResolveTargets(context.Background(), tree, TargetSpec{Paths: []string{".env"}, Cwd: root})

	if err == nil || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("an oversize file named with --path was accepted silently: %v", err)
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
		{"sub/.claude/rules/r.md", kindRule, true},
		{"a/b/.claude/commands/c.md", kindCommand, true},
		{"sub/.claude/agents/a.md", kindAgent, true},
		{"sub/.claude/rules/deep/r.md", kindRule, true},
		{"sub/.claude/docs/guide.md", kindContext, false},
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

func TestNamedFiles(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		limit int
		want  string
	}{
		{"none", nil, 3, ""},
		{"sorted", []string{"b", "a"}, 3, ": a, b"},
		{"at the limit", []string{"a", "b", "c"}, 3, ": a, b, c"},
		{"over the limit", []string{"d", "c", "b", "a", "e"}, 3, ": a, b, c and 2 more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := namedFiles(tt.names, tt.limit); got != tt.want {
				t.Errorf("namedFiles = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTargetsExclude(t *testing.T) {
	skill := "---\nname: s\ndescription: Use when testing the exclusion of targets.\n---\nbody\n"
	tests := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{"nothing excluded", nil, []string{".claude/skills/s/SKILL.md", "x/src/public/skills/s/SKILL.md"}},
		{"directory glob", []string{"x/**"}, []string{".claude/skills/s/SKILL.md"}},
		{"exact file", []string{"x/src/public/skills/s/SKILL.md"}, []string{".claude/skills/s/SKILL.md"}},
		{"glob that matches nothing", []string{"nope/**"}, []string{".claude/skills/s/SKILL.md", "x/src/public/skills/s/SKILL.md"}},
		{"everything", []string{"**"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".claude/skills/s/SKILL.md":      skill,
				"x/src/public/skills/s/SKILL.md": skill,
			})
			gitAdd(t, root)
			tree, err := LoadTree(root)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ResolveTargets(context.Background(), tree, TargetSpec{Detect: true, Cwd: root, Exclude: tt.exclude})
			if err != nil {
				t.Fatal(err)
			}
			if rels := got.Rels(); !slices.Equal(rels, tt.want) {
				t.Errorf("targets = %v, want %v", rels, tt.want)
			}
		})
	}
}
