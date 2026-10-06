package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
)

// gitRepo creates a temp git repository with the user's global git config
// isolated from the real one. extraConfig is appended to the global config.
func gitRepo(t *testing.T, extraConfig string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	globalConfig := filepath.Join(home, "gitconfig")
	require.NoError(t, os.WriteFile(globalConfig, []byte(extraConfig), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	dir := t.TempDir()
	out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput() //nolint:gosec // test
	require.NoError(t, err, string(out))
	return dir
}

func gitignoreOutputs(dir string) []config.OutputFile {
	return []config.OutputFile{
		{Path: filepath.Join(dir, "CLAUDE.md")},
		{Path: filepath.Join(dir, "AGENTS.md")},
		{Path: filepath.Join(dir, ".claude", "skills", "demo", "SKILL.md")},
		{Path: filepath.Join(dir, "packages", "web", "AGENTS.md")},
	}
}

func readGitignore(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestUpdateGitignore_AddsOnlyWhatGitDoesNotIgnore(t *testing.T) {
	tests := []struct {
		name       string
		userRules  string
		subRules   string // packages/web/.gitignore
		global     string
		wantBlock  []string
		wantAbsent []string
	}{
		{
			name:       "nothing ignored yet",
			wantBlock:  []string{"CLAUDE.md", "AGENTS.md", ".claude/skills/", "packages/web/AGENTS.md"},
			wantAbsent: nil,
		},
		{
			name:       "user already ignores .claude/",
			userRules:  ".claude/\n",
			wantBlock:  []string{"CLAUDE.md", "AGENTS.md"},
			wantAbsent: []string{".claude/skills/"},
		},
		{
			name:       "user negates CLAUDE.md",
			userRules:  "!CLAUDE.md\n",
			wantBlock:  []string{"AGENTS.md", ".claude/skills/"},
			wantAbsent: []string{"CLAUDE.md"},
		},
		{
			name:       "user glob ignores markdown",
			userRules:  "*.md\n",
			wantBlock:  []string{".claude/skills/"},
			wantAbsent: []string{"CLAUDE.md", "AGENTS.md", "packages/web/AGENTS.md"},
		},
		{
			name:       "global excludes file covers a path",
			global:     "CLAUDE.md\n",
			wantBlock:  []string{"AGENTS.md", ".claude/skills/"},
			wantAbsent: []string{"CLAUDE.md"},
		},
		{
			name:       "user rule catching only one stand-in name does not cover the directory (tmp)",
			userRules:  ".claude/skills/*.tmp\n",
			wantBlock:  []string{".claude/skills/"},
			wantAbsent: nil,
		},
		{
			name:       "user rule catching only one stand-in name does not cover the directory (prefix)",
			userRules:  ".claude/skills/generated-*\n",
			wantBlock:  []string{".claude/skills/"},
			wantAbsent: nil,
		},
		{
			name:       "user rule covering everything under the directory does",
			userRules:  ".claude/skills/*\n",
			wantBlock:  []string{"CLAUDE.md"},
			wantAbsent: []string{".claude/skills/"},
		},
		{
			name:       "nested .gitignore covers its own subtree",
			subRules:   "AGENTS.md\n",
			wantBlock:  []string{"CLAUDE.md", "AGENTS.md", ".claude/skills/"},
			wantAbsent: []string{"packages/web/AGENTS.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			globalCfg := ""
			var globalIgnore string
			if tt.global != "" {
				globalIgnore = filepath.Join(t.TempDir(), "global-ignore")
				require.NoError(t, os.WriteFile(globalIgnore, []byte(tt.global), 0o644))
				globalCfg = "[core]\n\texcludesFile = " + filepath.ToSlash(globalIgnore) + "\n"
			}
			dir := gitRepo(t, globalCfg)
			if tt.userRules != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tt.userRules), 0o644))
			}
			if tt.subRules != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "packages", "web"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "packages", "web", ".gitignore"), []byte(tt.subRules), 0o644))
			}
			gen := NewGenerator(&config.Config{BaseDir: dir})

			// Act
			require.NoError(t, gen.updateGitignore(gitignoreOutputs(dir)))

			// Assert
			block := strings.Join(gitignore.PatternsInsideFence(readGitignore(t, dir)), "\n")
			lines := strings.Split(block, "\n")
			for _, want := range tt.wantBlock {
				assert.Contains(t, lines, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, lines, absent)
			}
		})
	}
}

func TestUpdateGitignore_SecondRunIsStable(t *testing.T) {
	dir := gitRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n.claude/\n!CLAUDE.md\n"), 0o644))
	gen := NewGenerator(&config.Config{BaseDir: dir})
	outputs := gitignoreOutputs(dir)

	require.NoError(t, gen.updateGitignore(outputs))
	first := readGitignore(t, dir)
	require.NoError(t, gen.updateGitignore(outputs))
	second := readGitignore(t, dir)

	assert.Equal(t, first, second)
	inside := gitignore.PatternsInsideFence(second)
	assert.Contains(t, inside, "AGENTS.md")
	assert.NotContains(t, inside, "CLAUDE.md")
	assert.NotContains(t, inside, ".claude/skills/")
}

func TestUpdateGitignore_RemovesBlockWhenNothingIsNeeded(t *testing.T) {
	dir := gitRepo(t, "")
	gen := NewGenerator(&config.Config{BaseDir: dir})
	outputs := gitignoreOutputs(dir)
	require.NoError(t, gen.updateGitignore(outputs))
	require.Contains(t, readGitignore(t, dir), gitignore.BeginMarker)

	// The user now covers everything themselves, below our block.
	content := readGitignore(t, dir) + "\n*.md\n.claude/\n.ai-rulez/.generated-manifest.json\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(content), 0o644))
	require.NoError(t, gen.updateGitignore(outputs))

	got := readGitignore(t, dir)
	assert.NotContains(t, got, gitignore.BeginMarker)
	assert.NotContains(t, got, gitignore.EndMarker)
	assert.Contains(t, got, "*.md")
}

func TestUpdateGitignore_NoBlockWhenEverythingIgnored(t *testing.T) {
	dir := gitRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.md\n.claude/\n.ai-rulez/.generated-manifest.json\n"), 0o644))
	gen := NewGenerator(&config.Config{BaseDir: dir})

	require.NoError(t, gen.updateGitignore(gitignoreOutputs(dir)))

	assert.Equal(t, "*.md\n.claude/\n.ai-rulez/.generated-manifest.json\n", readGitignore(t, dir))
}

func TestUpdateGitignore_OutsideRepositoryAddsEverything(t *testing.T) {
	dir := t.TempDir() // not a git repository
	gen := NewGenerator(&config.Config{BaseDir: dir})

	require.NoError(t, gen.updateGitignore(gitignoreOutputs(dir)))

	inside := gitignore.PatternsInsideFence(readGitignore(t, dir))
	assert.Contains(t, inside, "CLAUDE.md")
	assert.Contains(t, inside, ".claude/skills/")
}

func TestNeededGitignorePatterns_ProtectedOutputsUnignoredByUser(t *testing.T) {
	tests := []struct {
		name        string
		output      config.OutputFile
		rules       string
		wantWarning string
		wantNeeded  bool
	}{
		{
			name:        "local output negated",
			output:      config.OutputFile{Path: "CLAUDE.local.md", LocalOnly: true},
			rules:       "!CLAUDE.local.md\n",
			wantWarning: "CLAUDE.local.md",
		},
		{
			name:        "sensitive MCP config negated",
			output:      config.OutputFile{Path: ".mcp.json", Sensitive: true},
			rules:       "!.mcp.json\n",
			wantWarning: ".mcp.json",
		},
		{
			name:       "local output not mentioned",
			output:     config.OutputFile{Path: "CLAUDE.local.md", LocalOnly: true},
			wantNeeded: true,
		},
		{
			name:   "local output already ignored",
			output: config.OutputFile{Path: "CLAUDE.local.md", LocalOnly: true},
			rules:  "*.local.md\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t, "")
			if tt.rules != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tt.rules), 0o644))
			}
			gen := NewGenerator(&config.Config{BaseDir: dir})
			tt.output.Path = filepath.Join(dir, tt.output.Path)

			needed, overridden := gen.neededGitignorePatterns([]config.OutputFile{tt.output})

			assert.Equal(t, tt.wantNeeded, slices.Contains(needed, filepath.Base(tt.output.Path)), "needed: %v", needed)
			if tt.wantWarning == "" {
				assert.Empty(t, overridden)
				return
			}
			require.Len(t, overridden, 1)
			assert.Equal(t, tt.wantWarning, overridden[0].Pattern)
			assert.Equal(t, "!"+tt.wantWarning, overridden[0].Rule)
		})
	}
}

func TestEnsureSecretOutputsIgnored_RefusesWhenUserUnignoredSecretOutput(t *testing.T) {
	dir := gitRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("!.mcp.json\n"), 0o644))
	gen := NewGenerator(&config.Config{
		BaseDir: dir,
		MCPServers: map[string]*config.MCPServer{
			"s": {Command: "x", Env: map[string]string{"TOKEN": "supersecretvalue"}, SecretEnvKeys: []string{"TOKEN"}},
		},
	})

	err := gen.ensureSecretOutputsIgnored([]config.OutputFile{{
		Path: filepath.Join(dir, ".mcp.json"), Content: `{"env":{"TOKEN":"supersecretvalue"}}`,
	}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not gitignored")
}

func TestWithoutManagedBlock(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"no block", "a\nb\n", "a\nb"},
		{"fenced", "a\n\n" + gitignore.BeginMarker + "\nx\n" + gitignore.EndMarker + "\n\nb\n", "a\n\nb"},
		{"old header", "a\n" + gitignore.OldHeader + "\nx\n", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, strings.TrimRight(withoutManagedBlock(tt.in), "\n"))
		})
	}
}

func TestGitignoreProbes(t *testing.T) {
	tests := map[string][]string{
		"CLAUDE.md":                {"CLAUDE.md"},
		"/root-only.md":            {"root-only.md"},
		".claude/skills/":          {".claude/skills/generated-probe", ".claude/skills/x7q-probe.tmp"},
		".ai-rulez/config.local.*": {".ai-rulez/config.local.generated-probe", ".ai-rulez/config.local.x7q-probe.tmp"},
		".claude/rules/*.local.*": {
			".claude/rules/generated-probe.local.generated-probe",
			".claude/rules/x7q-probe.tmp.local.x7q-probe.tmp",
		},
	}
	for in, want := range tests {
		assert.Equal(t, want, gitignoreProbes(in), in)
	}
}

func TestNeededGitignorePatterns_NeverTouchesUserFiles(t *testing.T) {
	dir := gitRepo(t, "")
	gen := NewGenerator(&config.Config{BaseDir: dir})
	outputs := gitignoreOutputs(dir)
	user := "node_modules/\n.claude/\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(user), 0o644))
	require.NoError(t, gen.updateGitignore(outputs)) // now holds a managed block

	path := filepath.Join(dir, ".gitignore")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(before), gitignore.BeginMarker)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, past, past))

	needed, _ := gen.neededGitignorePatterns(outputs)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.True(t, info.ModTime().Equal(past), "mtime changed: %v", info.ModTime())
	assert.Contains(t, needed, "CLAUDE.md", "own block must not count as user rules")
	assert.NotContains(t, needed, ".claude/skills/")
}

func TestNeededGitignorePatterns_ManyNestedGitignores(t *testing.T) {
	dir := gitRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("x\n"), 0o644))
	var outputs []config.OutputFile
	for i := range 60 {
		pkg := filepath.Join(dir, "packages", "p"+strconv.Itoa(i))
		require.NoError(t, os.MkdirAll(pkg, 0o755))
		if i%2 == 0 {
			require.NoError(t, os.WriteFile(filepath.Join(pkg, ".gitignore"), []byte("AGENTS.md\n"), 0o644))
		}
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(pkg, "AGENTS.md")})
	}
	gen := NewGenerator(&config.Config{BaseDir: dir})
	require.NoError(t, gen.updateGitignore(outputs)) // creates a block, forcing the mirror path

	needed, _ := gen.neededGitignorePatterns(outputs)

	for i := range 60 {
		assert.Equal(t, i%2 == 1, slices.Contains(needed, "packages/p"+strconv.Itoa(i)+"/AGENTS.md"), "p%d", i)
	}
}

func TestNeededGitignorePatterns_SubdirectoryBaseDirWithBlock(t *testing.T) {
	dir := gitRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("CLAUDE.md\n"), 0o644))
	base := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(base, 0o755))
	outputs := []config.OutputFile{{Path: filepath.Join(base, "CLAUDE.md")}, {Path: filepath.Join(base, "AGENTS.md")}}
	gen := NewGenerator(&config.Config{BaseDir: base})
	require.NoError(t, os.WriteFile(filepath.Join(base, ".gitignore"), []byte("seed\n"), 0o644))
	require.NoError(t, gen.updateGitignore(outputs))

	needed, _ := gen.neededGitignorePatterns(outputs)

	assert.Equal(t, []string{"AGENTS.md"}, filterManifest(needed))
}

func filterManifest(in []string) []string {
	var out []string
	for _, p := range in {
		if !strings.HasSuffix(p, ".generated-manifest.json") {
			out = append(out, p)
		}
	}
	return out
}
