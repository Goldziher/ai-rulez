package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const (
	rootHandAgentsMD = "# Hand written\n\nKeep it small.\n"
	rootHandClaudeMD = "@AGENTS.md\n\nMy own Claude notes.\n"
	rootDemoSkill    = "---\nname: demo\ndescription: A demo skill that the project installs from a local source\n---\n\nDo the demo.\n"
)

// rootConfigProject is a git repository with a hand-written CLAUDE.md and AGENTS.md,
// a local skill source and an ai-rulez.toml that installs the skill. It has no
// .ai-rulez directory.
func rootConfigProject(t *testing.T, tomlBody string) string {
	t.Helper()
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	writeAgentsFile(t, dir, "AGENTS.md", rootHandAgentsMD)
	writeAgentsFile(t, dir, "CLAUDE.md", rootHandClaudeMD)
	writeAgentsFile(t, dir, "shared/skills/demo/SKILL.md", rootDemoSkill)
	writeAgentsFile(t, dir, "ai-rulez.toml", tomlBody)
	return dir
}

func rootFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, relErr := filepath.Rel(dir, p)
			require.NoError(t, relErr)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	}))
	return out
}

func TestRootConfigLifecycle(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	dir := rootConfigProject(t, `version = "5.0"
name = "solo"
gitignore = true
presets = ["claude", "codex", "cursor"]

[[installed_skills]]
name = "demo"
source = "./shared"
`)

	// Act: generate twice, then check
	first := testutil.RunCLI(t, dir, "generate", "--yes")
	require.Equal(t, 0, first.ExitCode, first.Stderr)
	second := testutil.RunCLI(t, dir, "generate", "--yes")
	require.Equal(t, 0, second.ExitCode, second.Stderr)
	check := testutil.RunCLI(t, dir, "generate", "--check")

	// Assert: hand-written files untouched, the installed skill emitted
	assert.Equal(t, rootHandAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Equal(t, rootHandClaudeMD, readAgentsFile(t, dir, "CLAUDE.md"))
	assert.Contains(t, readAgentsFile(t, dir, ".claude/skills/demo/SKILL.md"), "Do the demo.")
	assert.Contains(t, readAgentsFile(t, dir, ".agents/skills/demo/SKILL.md"), "Do the demo.")
	assert.Equal(t, 0, check.ExitCode, "generate --check is clean: %s%s", check.Stdout, check.Stderr)

	// Assert: the manifests are loose hidden files in the root, not under .ai-rulez
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez-manifest.json"))
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez-manifest.local.json"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	assert.Contains(t, readAgentsFile(t, dir, ".ai-rulez-manifest.json"), ".claude/skills/demo/SKILL.md")

	// Assert: the managed block lists the manifests and the generated trees
	block := readAgentsFile(t, dir, ".gitignore")
	for _, want := range []string{".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json", ".claude/skills/", ".agents/skills/"} {
		assert.Contains(t, block, want+"\n")
	}
	assert.NotContains(t, block, "ai-rulez.lock")
	assert.NotContains(t, block, "AGENTS.md")
	assert.NotContains(t, block, "CLAUDE.md")

	// the lock lives in the project root and pins the skill
	testutil.RunCLIExpectSuccess(t, dir, "lock")
	assert.FileExists(t, filepath.Join(dir, "ai-rulez.lock"))
	assert.Contains(t, readAgentsFile(t, dir, "ai-rulez.lock"), "demo")
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "lock", "--check").ExitCode)
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "generate", "--check", "--locked").ExitCode)

	// validate and doctor accept the project with no content tree
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "validate").ExitCode)
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "doctor").ExitCode)

	// clean removes the generated files and leaves the rest alone
	testutil.RunCLIExpectSuccess(t, dir, "clean", "--yes")
	assert.ElementsMatch(t,
		[]string{"AGENTS.md", "CLAUDE.md", "ai-rulez.lock", "ai-rulez.toml", "shared/skills/demo/SKILL.md"},
		rootFiles(t, dir))
	assert.Equal(t, rootHandAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Equal(t, rootHandClaudeMD, readAgentsFile(t, dir, "CLAUDE.md"))
}

func TestRootConfigWritesNoRootFileForAnyPreset(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	names := make([]string, 0, len(config.AllPresetNames()))
	for _, name := range config.AllPresetNames() {
		names = append(names, `"`+name+`"`)
	}
	dir := rootConfigProject(t, "version = \"5.0\"\nname = \"solo\"\npresets = ["+strings.Join(names, ", ")+"]\n\n"+
		"[[installed_skills]]\nname = \"demo\"\nsource = \"./shared\"\n")
	before := rootFiles(t, dir)

	// Act
	run := testutil.RunCLI(t, dir, "generate", "--yes")

	// Assert: no instruction or index file appears at the project root, and the hand-written ones are unchanged
	require.Equal(t, 0, run.ExitCode, run.Stderr)
	for _, file := range rootFiles(t, dir) {
		isDoc := strings.HasSuffix(file, ".md") || strings.HasSuffix(file, ".txt") || strings.HasSuffix(file, ".mdc")
		if !strings.Contains(file, "/") && isDoc {
			assert.Contains(t, before, file, "generate wrote the root file %s", file)
		}
	}
	assert.NoDirExists(t, filepath.Join(dir, "docs", "okf"), "the OKF bundle documents authored content, which this project has none of")
	assert.Equal(t, rootHandAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Equal(t, rootHandClaudeMD, readAgentsFile(t, dir, "CLAUDE.md"))
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "generate", "--check").ExitCode)

	// and clean takes back exactly what generate added
	testutil.RunCLIExpectSuccess(t, dir, "clean", "--yes")
	assert.ElementsMatch(t, before, rootFiles(t, dir))
}

func TestRootConfigCommandsWriteAiRulezToml(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	dir := rootConfigProject(t, "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n")

	// Act
	testutil.RunCLIExpectSuccess(t, dir, "skill", "install", "demo", "--source", "./shared")
	testutil.RunCLIExpectSuccess(t, dir, "local", "init")
	testutil.RunCLIExpectSuccess(t, dir, "local", "set", "presets", `["claude", "opencode"]`)

	// Assert: the shared file holds the skill, the overlay is its own gitignored file
	assert.Contains(t, readAgentsFile(t, dir, "ai-rulez.toml"), "[[installed_skills]]")
	assert.Contains(t, readAgentsFile(t, dir, "ai-rulez.toml"), "name = 'demo'")
	assert.FileExists(t, filepath.Join(dir, "ai-rulez.local.toml"))
	assert.NoFileExists(t, filepath.Join(dir, "config.toml"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	ignored := readAgentsFile(t, dir, ".gitignore")
	assert.Contains(t, ignored, "ai-rulez.local.*\n")
	assert.NotContains(t, ignored, "./")

	// the overlay applies on generate
	testutil.RunCLIExpectSuccess(t, dir, "generate", "--yes")
	assert.FileExists(t, filepath.Join(dir, "opencode.json"), "the overlay's opencode preset is generated")
}

func TestRootConfigRefusesContentWrites(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	dir := rootConfigProject(t, "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n")

	// Act
	result := testutil.RunCLI(t, dir, "add", "rule", "style", "--content", "# Style\n")

	// Assert
	assert.NotEqual(t, 0, result.ExitCode)
	assert.Contains(t, result.Stderr+result.Stdout, "no .ai-rulez/ directory")
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	assert.NoDirExists(t, filepath.Join(dir, "rules"))
}

func TestInitMinimal(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"creates ai-rulez.toml", []string{"init", "--minimal", "demo"}, ""},
		{"rejects --source agents", []string{"init", "--minimal", "--source", "agents"}, "cannot be combined with --source agents"},
		{"rejects --from", []string{"init", "--minimal", "--from", "auto"}, "cannot be combined with --from"},
		{"rejects --domains", []string{"init", "--minimal", "--domains", "a"}, "cannot be combined with --domains"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := testutil.CreateTempDir(t)
			writeAgentsFile(t, dir, "AGENTS.md", rootHandAgentsMD)

			// Act
			result := testutil.RunCLI(t, dir, tt.args...)

			// Assert
			if tt.wantErr != "" {
				assert.NotEqual(t, 0, result.ExitCode)
				assert.Contains(t, result.Stderr+result.Stdout, tt.wantErr)
				assert.NoFileExists(t, filepath.Join(dir, "ai-rulez.toml"))
				return
			}
			require.Equal(t, 0, result.ExitCode, result.Stderr)
			body := readAgentsFile(t, dir, "ai-rulez.toml")
			assert.True(t, strings.HasPrefix(body, "#:schema https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json\n"))
			assert.Contains(t, body, "# [[installed_skills]]")
			assert.Contains(t, body, "# [lint]")
			assert.Contains(t, body, `name = "demo"`)
			assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
			assert.Equal(t, rootHandAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
			assert.Equal(t, []string{"AGENTS.md", "ai-rulez.toml"}, rootFiles(t, dir))

			// the file it writes loads, validates, and is not overwritten without --force
			assert.Equal(t, 0, testutil.RunCLI(t, dir, "validate").ExitCode)
			again := testutil.RunCLI(t, dir, "init", "--minimal")
			assert.NotEqual(t, 0, again.ExitCode)
			assert.Contains(t, again.Stderr+again.Stdout, "already exists")
		})
	}
}

func TestRootConfigDirectoryWinsOverFile(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange: both layouts exist
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	writeAgentsFile(t, dir, "ai-rulez.toml", "version = \"5.0\"\nname = \"file\"\npresets = [\"cursor\"]\n")
	writeAgentsFile(t, dir, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"dir\"\npresets = [\"claude\"]\n")

	// Act
	run := testutil.RunCLI(t, dir, "generate", "--yes")
	doctor := testutil.RunCLI(t, dir, "doctor")

	// Assert
	require.Equal(t, 0, run.ExitCode, run.Stderr)
	assert.Contains(t, run.Stderr+run.Stdout, "ai-rulez.toml is ignored")
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", ".generated-manifest.json"))
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez-manifest.json"))
	assert.Contains(t, doctor.Stdout+doctor.Stderr, "ai-rulez.toml is ignored")
}

func TestRootConfigLegacyFlatFileStillFails(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeAgentsFile(t, dir, "ai-rulez.yaml", "name: legacy\n")

	result := testutil.RunCLI(t, dir, "generate", "--yes")

	assert.NotEqual(t, 0, result.ExitCode)
	assert.Contains(t, result.Stderr+result.Stdout, "migrate")
}
