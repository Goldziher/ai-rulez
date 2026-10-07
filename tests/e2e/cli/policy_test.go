package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

// PolicyCLITestSuite drives the organization policy end to end: discovery from
// a flag and the environment (fail closed), clamp and report for each kind of
// loosening, `generate` refusing, and --show-policy.
type PolicyCLITestSuite struct {
	suite.Suite
	dir    string
	home   string
	policy string
}

func TestPolicyCLISuite(t *testing.T) {
	suite.Run(t, new(PolicyCLITestSuite))
}

func (s *PolicyCLITestSuite) TearDownSuite() { testutil.CleanupTestBinary() }

const policyBaseConfig = `version = "5.0"
name = "p"
presets = ["claude"]
gitignore = false
`

func (s *PolicyCLITestSuite) SetupTest() {
	s.dir = testutil.CreateTempDir(s.T())
	s.home = testutil.CreateTempDir(s.T())
	policyDir := testutil.CreateTempDir(s.T()) // outside the repository, like a real policy
	s.policy = filepath.Join(policyDir, "policy.toml")
	s.Require().NoError(os.WriteFile(s.policy, []byte(`policy_version = 1
name = "e2e baseline"

[sources]
allowed_hosts = ["github.com/example-org"]

[lint]
required_codes = ["AR001"]

[lint.severity_floor]
AR008 = "warning"

[lint.security]
allowed_hosts = ["github.com", "*.example.org"]
scan_imports = "error"

[lock]
enforce = true
`), 0o644))
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, ".ai-rulez", "rules"), 0o755))
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/rules/local.md", "# Local\n\nlocal rule\n")
	s.config("")
}

func (s *PolicyCLITestSuite) config(extra string) {
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/config.toml", policyBaseConfig+extra)
}

func (s *PolicyCLITestSuite) run(env map[string]string, args ...string) *testutil.CLIResult {
	merged := map[string]string{"HOME": s.home, "USERPROFILE": s.home, "XDG_CONFIG_HOME": filepath.Join(s.home, ".config"), "AI_RULEZ_POLICY": ""}
	for k, v := range env {
		merged[k] = v
	}
	return testutil.RunCLIWithEnv(s.T(), s.dir, merged, args...)
}

func (s *PolicyCLITestSuite) strict(args ...string) (*testutil.CLIResult, []struct{ Code, Message string }) {
	res := s.run(nil, append([]string{"validate", "--format", "json", "--policy", s.policy}, args...)...)
	var rep struct {
		Findings []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	s.Require().NoError(json.Unmarshal([]byte(res.Stdout), &rep), res.Stdout+res.Stderr)
	var out []struct{ Code, Message string }
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.Code, "AR74") {
			out = append(out, struct{ Code, Message string }{f.Code, f.Message})
		}
	}
	return res, out
}

func (s *PolicyCLITestSuite) TestWithoutAPolicyNothingChanges() {
	s.config("[lint]\nignore = [\"AR001\"]\n[lint.severity]\nAR008 = \"off\"\n[lock]\nenforce = false\n")
	res := s.run(nil, "validate", "--format", "json")
	s.NotContains(res.Stdout, "AR74", res.Stdout)
	s.Equal(0, res.ExitCode, res.Stdout+res.Stderr)
	show := s.run(nil, "validate", "--show-policy")
	s.Equal(0, show.ExitCode, show.Stderr)
	s.Contains(show.Stdout, "policy: none")
}

func (s *PolicyCLITestSuite) TestATighteningRepositoryPasses() {
	s.config("[lint]\n[lint.severity]\nAR008 = \"error\"\n[lint.security]\nallowed_hosts = [\"git.example.org\"]\nscan_imports = \"error\"\n")
	lock := s.run(nil, "lock") // the policy enforces the lock, and validate reports a missing one
	s.Equal(0, lock.ExitCode, lock.Stdout+lock.Stderr)
	res, found := s.strict()
	s.Empty(found, res.Stdout)
	s.Equal(0, res.ExitCode, res.Stdout+res.Stderr)
	show := s.run(nil, "validate", "--show-policy", "--policy", s.policy)
	s.Equal(0, show.ExitCode, show.Stdout+show.Stderr)
	s.Contains(show.Stdout, "repo overrides accepted: lint.security.allowed_hosts")
}

func (s *PolicyCLITestSuite) TestEachLooseningIsClampedAndReported() {
	s.config(`[lint]
ignore = ["AR001"]

[lint.severity]
AR008 = "off"

[lint.security]
allowed_hosts = ["github.com", "evil.test"]
scan_imports = "warn"

[lock]
enforce = false

[[includes]]
name = "typo"
source = "https://github.com/exmaple-org/rules"
`)
	res, found := s.strict()
	s.Equal(2, res.ExitCode, res.Stdout)
	var all []string
	for _, f := range found {
		all = append(all, f.Code+" "+f.Message)
	}
	joined := strings.Join(all, "\n")
	s.Contains(joined, "AR744", "ignoring a required code")
	s.Contains(joined, "AR745", "a source from a host the policy does not allow")
	s.Contains(joined, "below the policy floor")
	s.Contains(joined, `"evil.test" is not provably within`)
	s.Contains(joined, "scan_imports")
	s.Contains(joined, "switches the lock off")
	for _, f := range found {
		s.NotEmpty(f.Message)
	}
}

func (s *PolicyCLITestSuite) TestGenerateRefusesALoosenedConfiguration() {
	s.config("[lock]\nenforce = false\n")
	res := s.run(nil, "generate", "--policy", s.policy)
	s.Equal(2, res.ExitCode, res.Stdout)
	s.Contains(res.Stderr, "AR740")
	s.Contains(res.Stderr, "loosens the organization policy")
	_, err := os.Stat(filepath.Join(s.dir, ".claude"))
	s.True(os.IsNotExist(err), "nothing is generated")

	s.config("")
	ok := s.run(nil, "generate", "--policy", s.policy)
	s.Equal(0, ok.ExitCode, ok.Stdout+ok.Stderr)
}

func (s *PolicyCLITestSuite) TestAPolicyThatCannotBeReadFailsClosed() {
	missing := filepath.Join(s.home, "missing.toml")
	byFlag := s.run(nil, "validate", "--policy", missing)
	s.Equal(1, byFlag.ExitCode, byFlag.Stdout)
	s.Contains(byFlag.Stderr, "AR742")
	byEnv := s.run(map[string]string{"AI_RULEZ_POLICY": missing}, "validate")
	s.Equal(1, byEnv.ExitCode)
	s.Contains(byEnv.Stderr, "AR742")
	s.Contains(byEnv.Stderr, "fails closed")
}

func (s *PolicyCLITestSuite) TestAnInvalidPolicyIsRefused() {
	bad := filepath.Join(s.home, "bad.toml")
	s.Require().NoError(os.WriteFile(bad, []byte("policy_version = 1\n[lint]\nrequired_code = [\"AR001\"]\n"), 0o644))
	res := s.run(map[string]string{"AI_RULEZ_POLICY": bad}, "validate")
	s.Equal(1, res.ExitCode)
	s.Contains(res.Stderr, "AR743")
	s.Contains(res.Stderr, "required_code")
}

func (s *PolicyCLITestSuite) TestShowPolicyJSONNamesTheOriginOfEveryValue() {
	s.config("[lint.severity]\nAR008 = \"off\"\n")
	res := s.run(map[string]string{"AI_RULEZ_POLICY": s.policy}, "validate", "--show-policy", "--format", "json")
	s.Equal(2, res.ExitCode, "a loosening repository exits 2: %s", res.Stderr)
	var rep struct {
		Layers     []struct{ Origin, Source, Digest string }
		Effective  map[string]any
		Provenance map[string]string
		Overrides  struct{ Rejected int }
		Violations []struct{ Code, Key string }
	}
	s.Require().NoError(json.Unmarshal([]byte(res.Stdout), &rep), res.Stdout)
	s.Require().Len(rep.Layers, 1)
	s.Equal("env", rep.Layers[0].Origin)
	s.Equal("env", rep.Provenance["lint.severity_floor.AR008"])
	s.Equal("env", rep.Provenance["sources.allowed_hosts"])
	s.Equal(1, rep.Overrides.Rejected)
	s.Equal("AR740", rep.Violations[0].Code)
	s.Equal("lint.severity.AR008", rep.Violations[0].Key)
	s.Contains(rep.Layers[0].Digest, "sha256:")
}

func (s *PolicyCLITestSuite) TestFlagAndEnvironmentLayersMerge() {
	team := filepath.Join(s.home, "team.toml")
	s.Require().NoError(os.WriteFile(team, []byte("policy_version = 1\n[lint.severity_floor]\nAR008 = \"error\"\n"), 0o644))
	res := s.run(map[string]string{"AI_RULEZ_POLICY": s.policy}, "validate", "--show-policy", "--policy", team)
	s.Equal(0, res.ExitCode, res.Stderr)
	s.Contains(res.Stdout, "policy: 2 layers")
	s.Contains(res.Stdout, "lint.severity_floor.AR008")
	s.Contains(res.Stdout, "[flag]", "the flag raised AR008 from warning to error")
	s.Contains(res.Stdout, "[env]")
}

// A repository cannot approve its own way past an organization's [governance] floor.
func (s *PolicyCLITestSuite) TestGovernanceFloorIsClampedAndReported() {
	extra := `policy_version = 1

[governance]
enforce = true
require_approval = ["local"]
min_approvers = 2
approvers = ["alice@example.org", "bob@example.org"]
`
	s.Require().NoError(os.WriteFile(s.policy, []byte(extra), 0o644))
	s.config(`[governance]
min_approvers = 1
approvers = ["mallory@example.org"]
exempt = ["rule:*"]
`)

	res, found := s.strict()

	s.Equal(2, res.ExitCode, res.Stdout)
	var joined []string
	for _, f := range found {
		joined = append(joined, f.Message)
	}
	all := strings.Join(joined, "\n")
	s.Contains(all, "does not enable enforce")
	s.Contains(all, "min_approvers = 1 is below the policy minimum 2")
	s.Contains(all, `"mallory@example.org" is not in the policy list`)

	// The policy value is what runs: the repository's exempt glob does not remove the floor,
	// so the local rule still needs approval.
	s.config("[governance]\nenforce = true\nexempt = [\"rule:*\"]\n")
	locked := s.run(nil, "lock", "--policy", s.policy)
	s.Require().Equal(0, locked.ExitCode, locked.Stderr)
	list := s.run(nil, "approve", "--list", "--format", "json", "--policy", s.policy)
	s.Require().Equal(0, list.ExitCode, list.Stderr)
	s.Contains(list.Stdout, `"enforce": true`)
	s.Contains(list.Stdout, `"min_approvers": 2`)
	s.Contains(list.Stdout, "alice@example.org")
	s.Contains(list.Stdout, `"status": "missing"`, "the exempt glob did not remove the policy floor")
}
