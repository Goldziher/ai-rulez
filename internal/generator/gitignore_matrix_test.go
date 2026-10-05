package generator

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
)

// matrixFixtures is the content every preset is generated from: one item of each
// kind plus an MCP server, so every output a preset can emit is exercised.
var matrixFixtures = map[string]string{
	"rules/style.md":       "---\npriority: high\n---\n\nUse tabs.\n",
	"rules/scoped.md":      "---\npriority: medium\nglobs: [\"src/**/*.go\"]\n---\n\nScoped rule.\n",
	"context/overview.md":  "---\npriority: medium\nsummary: Overview.\n---\n\nProject overview.\n",
	"skills/demo/SKILL.md": "---\nname: demo\ndescription: Demo skill.\n---\n\n# Demo\n\nDo the demo.\n",
	"agents/helper.md":     "---\nname: helper\ndescription: Helper agent.\nmodel: sonnet\n---\n\nHelp.\n",
	"commands/build.md":    "---\npriority: medium\nusage: \"/build\"\ndescription: \"Build\"\n---\n\n# Build\n",
	"checks/review.md":     "---\ndescription: Review\nseverity: high\n---\n\nCheck the diff.\n",
}

// generateWithGitignore writes a project enabling preset (and agentsMD), generates
// it inside a fresh git repository with gitignore management on, and returns the
// project directory with every output the run produced.
func generateWithGitignore(t *testing.T, preset string, agentsMD bool) (string, []config.OutputFile) {
	t.Helper()
	return generateWithUserGitignore(t, preset, agentsMD, "")
}

// generateWithUserGitignore is generateWithGitignore with a hand-written root
// .gitignore in place before the run.
func generateWithUserGitignore(t *testing.T, preset string, agentsMD bool, userGitignore string) (string, []config.OutputFile) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	cmd := exec.Command("git", "-C", base, "init", "-q") //nolint:gosec // test
	require.NoError(t, cmd.Run())
	if userGitignore != "" {
		require.NoError(t, os.WriteFile(filepath.Join(base, ".gitignore"), []byte(userGitignore), 0o600))
	}

	dir := filepath.Join(base, ".ai-rulez")
	for rel, body := range matrixFixtures {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	var sb strings.Builder
	sb.WriteString("version = \"4.0\"\nname = \"matrix\"\ngitignore = true\n")
	if agentsMD {
		sb.WriteString("agents_md = true\n")
	}
	sb.WriteString("presets = [\"" + preset + "\"]\n\n[[mcp_servers]]\nname = \"demo\"\ncommand = \"npx\"\nargs = [\"-y\", \"demo\"]\n")
	// A [[hooks]] group every hook renderer accepts (a plain command on an event
	// without a matcher), so the hooks documents join the matrix.
	sb.WriteString("\n[[hooks]]\nevent = \"SessionStart\"\n[[hooks.hooks]]\ncommand = \"echo matrix\"\n\n" +
		"[[hooks]]\nevent = \"PreToolUse\"\n[[hooks.hooks]]\ncommand = \"echo matrix\"\n")
	// [permissions] rules every translator can express (a command prefix, a path, a
	// domain), so the permission documents join the matrix.
	sb.WriteString("\n[permissions]\nallow = [\"Bash(npm run test:*)\", \"Read(./src/**)\", \"Edit(src/**)\"]\n" +
		"ask = [\"Bash(git push:*)\"]\ndeny = [\"Bash(rm -rf:*)\", \"Read(./.env)\", \"WebFetch(domain:evil.com)\"]\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(sb.String()), 0o600))

	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	outputs, _, err := gen.collectOutputs("")
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))
	return base, outputs
}

// assertGitignoreMatrix checks every written file of one generation against git:
// owned outputs are ignored; partially owned (hand-authored) and committed (check) ones are not.
func assertGitignoreMatrix(t *testing.T, base string, outputs []config.OutputFile) {
	t.Helper()
	checked := 0
	for _, out := range outputs {
		if out.IsDir {
			continue
		}
		rel := filepath.ToSlash(out.Path)
		if filepath.IsAbs(out.Path) {
			r, err := filepath.Rel(base, out.Path)
			require.NoError(t, err)
			rel = filepath.ToSlash(r)
		}
		if strings.HasPrefix(rel, ".ai-rulez/") {
			continue
		}
		cmd := exec.Command("git", "-C", base, "check-ignore", "--no-index", "-q", rel) //nolint:gosec // test
		ignored := cmd.Run() == nil
		switch {
		case out.Committed:
			assert.False(t, ignored, "check output %s must stay committed, not git-ignored", rel)
		case out.PartiallyOwned:
			assert.False(t, ignored, "partially owned %s must not be git-ignored", rel)
		default:
			assert.True(t, ignored, "generated %s must be git-ignored", rel)
		}
		checked++
	}
	assert.Positive(t, checked, "the preset wrote no files, so nothing was verified")
}

// siblingDirs lists the directories where a hand-authored file may sit beside a
// generated one: the top-level directory of every output, and the direct parent of
// an output when that directory is shared (.config/<tool>/, .vscode/), a rules
// folder, or holds a sidecar file. Directories wholly owned by a generated tree
// (.claude/skills/demo/, .github/agents/) are not listed.
func siblingDirs(base string, outputs []config.OutputFile) []string {
	sidecars := providers.SidecarPaths()
	set := map[string]bool{}
	for _, out := range outputs {
		rel := filepath.ToSlash(out.Path)
		if filepath.IsAbs(out.Path) {
			if r, err := filepath.Rel(base, out.Path); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		if out.IsDir || strings.HasPrefix(rel, ".ai-rulez/") || !strings.Contains(rel, "/") {
			continue
		}
		segments := strings.Split(rel, "/")
		set[segments[0]] = true
		parent := path.Dir(rel)
		isSidecar := slices.Contains(sidecars, rel)
		if (providers.IsSharedDir(segments[0]) && segments[0] != ".github") || isSidecar || config.InRulesDir(rel) {
			set[parent] = true
		}
	}
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// assertSiblingsNotIgnored writes a hand-authored <dir>/user-authored.txt beside
// the generated output of every shared directory and asserts git does not ignore it.
func assertSiblingsNotIgnored(t *testing.T, base string, outputs []config.OutputFile) {
	t.Helper()
	for _, dir := range siblingDirs(base, outputs) {
		rel := dir + "/user-authored.txt"
		abs := filepath.Join(base, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte("mine\n"), 0o600))
		cmd := exec.Command("git", "-C", base, "check-ignore", "--no-index", "-q", rel) //nolint:gosec // test
		assert.Error(t, cmd.Run(), "hand-authored %s must not be git-ignored", rel)
	}
}

func TestGitignoreMatrix_EveryPresetIgnoresItsOutputs(t *testing.T) {
	for _, preset := range config.IndividualPresetNames() {
		for _, agentsMD := range []bool{false, true} {
			name := preset
			if agentsMD {
				name += "/agents_md"
			}
			t.Run(name, func(t *testing.T) {
				// Arrange + Act
				base, outputs := generateWithGitignore(t, preset, agentsMD)

				// Assert
				assertGitignoreMatrix(t, base, outputs)
				assertSiblingsNotIgnored(t, base, outputs)
			})
		}
	}
}

// TestGitignoreMatrix_UserRulesMatchingTheProbeNameDoNotHideOutputs: a user rule
// such as the Go-binary pattern "/ai-rulez-*" matched the stand-in name used to
// ask git about directory patterns, so directories were judged already ignored
// and never written to the managed block.
func TestGitignoreMatrix_UserRulesMatchingTheProbeNameDoNotHideOutputs(t *testing.T) {
	user := "bin/\n/ai-rulez\n/ai-rulez-*\n**/ai-rulez-*\n"
	for _, preset := range []string{"codex", "opencode", "claude"} {
		t.Run(preset, func(t *testing.T) {
			// Arrange + Act
			base, outputs := generateWithUserGitignore(t, preset, false, user)

			// Assert
			assertGitignoreMatrix(t, base, outputs)
		})
	}
}

func TestGitignoreMatrix_MCPPresetIgnoresItsConfig(t *testing.T) {
	// Arrange + Act
	base, outputs := generateWithGitignore(t, string(config.PresetMCP), false)

	// Assert
	assertGitignoreMatrix(t, base, outputs)
	assertSiblingsNotIgnored(t, base, outputs)
}

// TestGitignore_PiKeepsHandAuthoredFilesTracked is the regression test for #211:
// the .pi/ folder also holds files a user writes and commits (settings.json,
// extensions/*.ts), so only what ai-rulez generates there is ignored, never the
// directory itself.
func TestGitignore_PiKeepsHandAuthoredFilesTracked(t *testing.T) {
	// Arrange + Act
	base, outputs := generateWithGitignore(t, "pi", false)
	data, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")

	// Assert: no pattern covers the whole folder.
	for _, broad := range []string{".pi", ".pi/", "/.pi", "/.pi/", ".pi/*", ".pi/**"} {
		assert.NotContains(t, lines, broad)
	}
	isIgnored := func(rel string) bool {
		return exec.Command("git", "-C", base, "check-ignore", "--no-index", "-q", rel).Run() == nil //nolint:gosec // test
	}
	for _, handAuthored := range []string{
		".pi/settings.json", ".pi/extensions/my-extension.ts", ".pi/themes/dark.json", ".pi/SYSTEM.md",
	} {
		assert.False(t, isIgnored(handAuthored), "hand-authored %s must stay tracked", handAuthored)
	}
	assertGitignoreMatrix(t, base, outputs)
	written := map[string]bool{}
	for _, out := range outputs {
		if rel, err := filepath.Rel(base, out.Path); err == nil {
			written[filepath.ToSlash(rel)] = true
		}
	}
	for _, generated := range []string{".pi/extensions/ai-rulez-hooks.ts", ".pi/mcp.json", ".pi/agents/helper.md"} {
		assert.True(t, written[generated], "%s is generated", generated)
		assert.True(t, isIgnored(generated), "generated %s must be ignored", generated)
	}
}
