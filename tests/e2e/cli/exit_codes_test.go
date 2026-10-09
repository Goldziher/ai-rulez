package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// The exit-code contract of v5, documented in docs/cli.md: 0 ok, 1 the command
// could not run or the configuration is invalid, 2 findings or drift. `lock` is
// the one documented exception family (see docs/lockfile.md).
const (
	exitOK       = 0
	exitCannot   = 1
	exitFindings = 2
)

type ExitCodeSuite struct {
	suite.Suite
	dir string
}

func TestExitCodeSuite(t *testing.T) { suite.Run(t, new(ExitCodeSuite)) }

func (s *ExitCodeSuite) SetupTest()     { s.dir = testutil.CreateTempDir(s.T()) }
func (s *ExitCodeSuite) TearDownSuite() { testutil.CleanupTestBinary() }

const exitCodeConfig = "version = \"5.0\"\nname = \"exit-codes\"\npresets = [\"claude\"]\n"

func (s *ExitCodeSuite) write(rel, body string) {
	path := filepath.Join(s.dir, filepath.FromSlash(rel))
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	s.Require().NoError(os.WriteFile(path, []byte(body), 0o644))
}

func (s *ExitCodeSuite) project(rule string) {
	s.write(".ai-rulez/config.toml", exitCodeConfig)
	s.write(".ai-rulez/rules/style.md", rule)
}

const goodRule = "---\ndescription: style\n---\n# Style\n\nUse tabs.\n"
const brokenLinkRule = "---\ndescription: style\n---\n# Style\n\nSee [the guide](docs/missing.md) now.\n"

func (s *ExitCodeSuite) run(args ...string) int {
	return testutil.RunCLI(s.T(), s.dir, args...).ExitCode
}

func (s *ExitCodeSuite) TestNoConfigurationIsExitOne() {
	for _, args := range []string{
		"validate", "validate --config-only", "generate", "generate --check", "verify --attestation", "doctor", "tokens", "cost",
		"scan", "catalog", "roles list", "lock --check", "lock --diff", "okf validate",
		"verifiers run", "migrate v5", "clean",
	} {
		s.Equal(exitCannot, s.run(strings.Fields(args)...), args)
	}
}

func (s *ExitCodeSuite) TestYAMLConfigurationIsExitOneWithTheMigrationHint() {
	s.write(".ai-rulez/config.yaml", "version: \"4.0\"\nname: old\npresets: [claude]\n")
	for _, args := range []string{"validate", "generate", "generate --check", "verify --attestation", "doctor"} {
		result := testutil.RunCLI(s.T(), s.dir, strings.Fields(args)...)
		s.Equal(exitCannot, result.ExitCode, args)
		s.Contains(result.Stdout+result.Stderr, "ai-rulez migrate v5", args)
	}
}

func (s *ExitCodeSuite) TestFourPointZeroConfigurationIsExitOneWithTheMigrationHint() {
	s.write(".ai-rulez/config.toml", strings.Replace(exitCodeConfig, "5.0", "4.0", 1))
	result := testutil.RunCLI(s.T(), s.dir, "validate")
	s.Equal(exitCannot, result.ExitCode)
	s.Contains(result.Stdout+result.Stderr, "ai-rulez migrate v5")
}

func (s *ExitCodeSuite) TestValidProjectIsExitZero() {
	s.project(goodRule)
	s.Equal(exitOK, s.run("validate"))
	s.Equal(exitOK, s.run("validate", "--config-only"))
	s.Equal(exitOK, s.run("generate"))
	s.Equal(exitOK, s.run("generate", "--check"))
}

func (s *ExitCodeSuite) TestContentFindingsAreExitTwoAndConfigOnlyIsExitZero() {
	s.project(brokenLinkRule)
	s.Equal(exitFindings, s.run("validate"))
	s.Equal(exitOK, s.run("validate", "--config-only"))
	s.Equal(exitOK, s.run("validate", "--fail-on", "none"))
}

func (s *ExitCodeSuite) TestDriftIsExitTwo() {
	s.project(goodRule)
	s.Equal(exitOK, s.run("generate"))
	s.write(".ai-rulez/rules/style.md", goodRule+"\nAnd spaces.\n")
	s.Equal(exitFindings, s.run("generate", "--check"), "a changed source is drift")
	s.Equal(exitOK, s.run("generate"))
	generated := filepath.Join(s.dir, "AGENTS.md")
	body, err := os.ReadFile(generated)
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(generated, append(body, []byte("hand edit\n")...), 0o644))
	s.Equal(exitFindings, s.run("generate", "--check"), "a hand-edited generated file is drift")
}

func (s *ExitCodeSuite) TestVerifyWithoutAModePointsAtGenerateCheck() {
	s.project(goodRule)
	result := testutil.RunCLI(s.T(), s.dir, "verify")
	s.Equal(exitCannot, result.ExitCode)
	s.Contains(result.Stdout+result.Stderr, "generate --check")
}

func (s *ExitCodeSuite) TestMigrateCheckIsExitTwoUntilMigrated() {
	s.write(".ai-rulez/config.toml", strings.Replace(exitCodeConfig, "5.0", "4.0", 1))
	s.Equal(exitFindings, s.run("migrate", "v5", "--check"))
	s.Equal(exitOK, s.run("migrate", "v5"))
	s.Equal(exitOK, s.run("migrate", "v5", "--check"))
	s.Equal(exitOK, s.run("validate", "--config-only"))
}

func (s *ExitCodeSuite) TestUnknownFlagsAndTargetsAreExitOne() {
	s.project(goodRule)
	s.Equal(exitCannot, s.run("validate", "--no-such-flag"))
	s.Equal(exitCannot, s.run("migrate", "v4"))
	s.Equal(exitCannot, s.run("validate", "--strict", "--config-only"))
}
