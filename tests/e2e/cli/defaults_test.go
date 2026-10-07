package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// The v5 defaults a fresh project gets: AGENTS.md is canonical, headers carry
// only the per-file Content-Hash and no managed .gitignore block is written.
type DefaultsSuite struct {
	suite.Suite
	dir string
}

func TestDefaultsSuite(t *testing.T) { suite.Run(t, new(DefaultsSuite)) }

func (s *DefaultsSuite) SetupTest()     { s.dir = testutil.CreateTempDir(s.T()) }
func (s *DefaultsSuite) TearDownSuite() { testutil.CleanupTestBinary() }

func (s *DefaultsSuite) read(name string) string {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	s.Require().NoError(err, name)
	return string(data)
}

func (s *DefaultsSuite) TestFreshProjectGetsTheV5Defaults() {
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, ".ai-rulez", "rules"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir, ".ai-rulez", "config.toml"),
		[]byte("version = \"5.0\"\nname = \"defaults\"\npresets = [\"claude\"]\n"), 0o644))
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir, ".ai-rulez", "rules", "style.md"),
		[]byte("---\ndescription: style\n---\n# Style\n\nUse tabs.\n"), 0o644))

	testutil.RunCLIExpectSuccess(s.T(), s.dir, "generate")

	agents := s.read("AGENTS.md")
	s.Contains(agents, "Use tabs.", "AGENTS.md is the canonical instruction file")
	s.Contains(s.read("CLAUDE.md"), "@AGENTS.md", "CLAUDE.md imports AGENTS.md")
	s.Contains(agents, "Content-Hash:")
	s.False(strings.Contains(agents, "Source-Hash:"), "the project-wide Source-Hash is off by default")
	_, err := os.Stat(filepath.Join(s.dir, ".gitignore"))
	s.True(os.IsNotExist(err), "the managed .gitignore block is opt-in")
}

func (s *DefaultsSuite) TestMigratedProjectKeepsItsFourXOutput() {
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, ".ai-rulez", "rules"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir, ".ai-rulez", "config.toml"),
		[]byte("version = \"4.0\"\nname = \"defaults\"\npresets = [\"claude\"]\n"), 0o644))
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir, ".ai-rulez", "rules", "style.md"),
		[]byte("---\ndescription: style\n---\n# Style\n\nUse tabs.\n"), 0o644))

	testutil.RunCLIExpectSuccess(s.T(), s.dir, "migrate", "v5")
	testutil.RunCLIExpectSuccess(s.T(), s.dir, "generate")

	s.Contains(s.read(".claude/rules/style.md"), "Use tabs.", "agents_md = false is pinned, so Claude keeps its own rules folder")
	s.Contains(s.read("CLAUDE.md"), "Source-Hash:", "hashes = \"full\" is pinned")
	s.Contains(s.read(".gitignore"), "CLAUDE.md", "gitignore = true is pinned")
	_, err := os.Stat(filepath.Join(s.dir, "AGENTS.md"))
	s.True(os.IsNotExist(err))
}
