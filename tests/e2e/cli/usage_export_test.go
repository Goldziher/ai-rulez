package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type UsageExportCLITestSuite struct {
	suite.Suite
	workingDir string
	env        map[string]string
}

func TestUsageExportCLISuite(t *testing.T) {
	suite.Run(t, new(UsageExportCLITestSuite))
}

func (s *UsageExportCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	cfg := filepath.Join(s.workingDir, ".ai-rulez")
	s.Require().NoError(os.MkdirAll(filepath.Join(cfg, "local"), 0o750))
	s.Require().NoError(os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("version = \"5.0\"\nname = \"e2e\"\npresets = [\"claude\"]\n"), 0o600))
	lines := `{"ts":"2026-10-01T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","invocation":"tool","harness":"claude"}
{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","digest":"sha256:` + strings.Repeat("a", 64) + `","digest_scheme":"ai-rulez/skill/v1","event_id":"0123456789abcdef","invocation":"slash","harness":"claude","prompt":"TOPSECRET"}
`
	s.Require().NoError(os.WriteFile(filepath.Join(cfg, "local", "usage.jsonl"), []byte(lines), 0o600))
	s.env = map[string]string{"XDG_CONFIG_HOME": s.T().TempDir(), "CLAUDE_PROJECT_DIR": s.workingDir}
}

func (s *UsageExportCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *UsageExportCLITestSuite) TestPreviewPrintsTheBodyAndSendsNothing() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, s.env, "telemetry", "preview")

	s.Equal(0, result.ExitCode, result.Stderr)
	result.AssertStdoutContains(s.T(), "2 events, previewing 2")
	result.AssertStdoutContains(s.T(), "POST <no endpoint configured>/v1/logs")
	result.AssertStdoutContains(s.T(), "fields withheld: ai_rulez.item.path, ai_rulez.session")
	result.AssertStdoutContains(s.T(), "Nothing was sent.")
	s.NotContains(result.Stdout, "TOPSECRET")
}

func (s *UsageExportCLITestSuite) TestExportWritesTheSameBytesTwiceAndNoSecrets() {
	dest := filepath.Join(s.workingDir, "out", "usage.ndjson")

	first := testutil.RunCLIWithEnv(s.T(), s.workingDir, s.env, "telemetry", "export", "--to", "file", dest)
	s.Equal(0, first.ExitCode, first.Stderr)
	one, err := os.ReadFile(dest)
	s.Require().NoError(err)
	testutil.RunCLIWithEnv(s.T(), s.workingDir, s.env, "telemetry", "export", "--to", "file", dest)
	two, err := os.ReadFile(dest)
	s.Require().NoError(err)

	s.Equal(string(one), string(two))
	s.Contains(string(one), `"stringValue":"deploy"`)
	s.Contains(string(one), `"stringValue":"0123456789abcdef"`)
	s.NotContains(string(one), "TOPSECRET")
}

func (s *UsageExportCLITestSuite) TestExportRequiresADestinationKind() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, s.env, "telemetry", "export", "out.ndjson")

	s.NotEqual(0, result.ExitCode)
	result.AssertStderrContains(s.T(), `required flag(s) "to" not set`)
}
