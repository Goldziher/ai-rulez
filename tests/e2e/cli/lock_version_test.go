package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type LockVersionCLITestSuite struct {
	suite.Suite
	dir  string
	home string
	repo *tagtest.Repo
}

func TestLockVersionCLISuite(t *testing.T) {
	suite.Run(t, new(LockVersionCLITestSuite))
}

func (s *LockVersionCLITestSuite) TearDownSuite() { testutil.CleanupTestBinary() }

func (s *LockVersionCLITestSuite) release(body, tag string) {
	s.repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\n"+body+"\n")
	s.repo.Commit(body)
	s.repo.AnnotatedTag(tag)
}

func (s *LockVersionCLITestSuite) SetupTest() {
	s.dir = testutil.CreateTempDir(s.T())
	s.home = testutil.CreateTempDir(s.T())
	s.repo = tagtest.New(s.T())
	s.release("one", "v1.0.0")
	s.release("two", "v1.1.0")
	s.release("three", "v2.0.0")
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, ".ai-rulez", "rules"), 0o755))
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/config.toml", `version = "5.0"
name = "p"
presets = ["claude"]
gitignore = false

[[includes]]
name = "shared"
source = "`+s.repo.URL+`"
version = "^1"
`)
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/rules/local.md", "# Local\n\nlocal rule\n")
}

func (s *LockVersionCLITestSuite) run(args ...string) *testutil.CLIResult {
	return testutil.RunCLIWithEnv(s.T(), s.dir, map[string]string{"HOME": s.home}, args...)
}

func (s *LockVersionCLITestSuite) lockText() string {
	data, err := os.ReadFile(filepath.Join(s.dir, ".ai-rulez", "ai-rulez.lock"))
	s.Require().NoError(err)
	return string(data)
}

func (s *LockVersionCLITestSuite) TestLockOutdatedUpdateAndSubject() {
	s.Equal(0, s.run("lock").ExitCode)
	s.Contains(s.lockText(), `tag = 'v1.1.0'`)
	s.Contains(s.lockText(), `ref = '^1'`)

	s.release("one point two", "v1.2.0")

	out := s.run("lock", "--outdated", "--format", "json")
	s.Equal(0, out.ExitCode)
	var rep struct {
		Sources []struct {
			Status  string `json:"status"`
			Allowed struct {
				Tag string `json:"tag"`
			} `json:"allowed"`
		} `json:"sources"`
	}
	s.Require().NoError(json.Unmarshal([]byte(out.Stdout), &rep), out.Stdout)
	s.Require().Len(rep.Sources, 1)
	s.Equal("updatable", rep.Sources[0].Status)
	s.Equal("v1.2.0", rep.Sources[0].Allowed.Tag)
	s.Equal(2, s.run("lock", "--outdated", "--fail-on-outdated").ExitCode)

	dry := s.run("update", "--dry-run")
	s.Equal(0, dry.ExitCode, dry.Stderr)
	s.True(strings.Contains(dry.Stdout, "v1.1.0 -> v1.2.0"), dry.Stdout)
	s.Contains(s.lockText(), `tag = 'v1.1.0'`, "dry run writes nothing")

	s.Equal(0, s.run("update").ExitCode)
	s.Contains(s.lockText(), `tag = 'v1.2.0'`)
	s.Equal(0, s.run("lock", "--outdated", "--fail-on-outdated").ExitCode)

	first := s.run("lock", "--subject")
	s.Equal(0, first.ExitCode, first.Stderr)
	s.Contains(first.Stdout, "sha256:")
	s.Equal(first.Stdout, s.run("lock", "--subject").Stdout)
}

func (s *LockVersionCLITestSuite) TestAMovedTagIsRefused() {
	s.Equal(0, s.run("lock").ExitCode)
	s.release("evil", "v1.1.0")

	out := s.run("update")

	s.Equal(2, out.ExitCode)
	out.AssertStderrContains(s.T(), "AR732")
	s.Contains(s.lockText(), `tag = 'v1.1.0'`)
}
