package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

// ApproveCLITestSuite drives `approve` end to end against a file:// git include:
// approving, bumping the source so the approval goes stale (AR711), and the places
// that enforce it.
type ApproveCLITestSuite struct {
	suite.Suite
	dir  string
	home string
	repo *tagtest.Repo
}

func TestApproveCLISuite(t *testing.T) {
	suite.Run(t, new(ApproveCLITestSuite))
}

func (s *ApproveCLITestSuite) TearDownSuite() { testutil.CleanupTestBinary() }

func (s *ApproveCLITestSuite) release(body, tag string) {
	s.repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\n"+body+"\n")
	s.repo.Commit(body)
	s.repo.AnnotatedTag(tag)
}

func (s *ApproveCLITestSuite) SetupTest() {
	s.dir = testutil.CreateTempDir(s.T())
	s.home = testutil.CreateTempDir(s.T())
	s.repo = tagtest.New(s.T())
	s.release("one", "v1.0.0")
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, ".ai-rulez", "rules"), 0o755))
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/config.toml", `version = "4.0"
name = "p"
presets = ["claude"]
gitignore = false

[[includes]]
name = "shared"
source = "`+s.repo.URL+`"
version = "^1"

[governance]
require_approval = ["remote"]
enforce = true
`)
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/rules/local.md", "# Local\n\nlocal rule\n")
}

func (s *ApproveCLITestSuite) run(args ...string) *testutil.CLIResult {
	return testutil.RunCLIWithEnv(s.T(), s.dir, map[string]string{"HOME": s.home}, args...)
}

func (s *ApproveCLITestSuite) strict() *testutil.CLIResult {
	return s.run("validate", "--strict", "--format", "json")
}

type approveListJSON struct {
	Items []struct {
		Ref       string   `json:"ref"`
		Status    string   `json:"status"`
		Reviewers []string `json:"reviewers"`
	} `json:"items"`
}

func (s *ApproveCLITestSuite) status(ref string) string {
	out := s.run("approve", "--list", "--format", "json")
	s.Require().Equal(0, out.ExitCode, out.Stderr)
	var doc approveListJSON
	s.Require().NoError(json.Unmarshal([]byte(out.Stdout), &doc), out.Stdout)
	for _, it := range doc.Items {
		if it.Ref == ref {
			return it.Status
		}
	}
	return ""
}

func (s *ApproveCLITestSuite) TestApproveStaleAndEnforcement() {
	s.Require().Equal(0, s.run("lock").ExitCode)

	// Nothing is approved yet: strict validation, the lock check and generate --locked all refuse.
	s.Equal("missing", s.status("include:shared"))
	strict := s.strict()
	s.Equal(2, strict.ExitCode, strict.Stderr)
	s.Contains(strict.Stdout, "AR710")
	check := s.run("lock", "--check")
	s.Equal(2, check.ExitCode)
	s.Contains(check.Stderr, "include:shared")
	s.Equal(2, s.run("generate", "--locked").ExitCode)

	// Without --yes a script cannot approve.
	refused := s.run("approve", "include:shared", "--reviewer", "alice@example.org")
	s.Equal(1, refused.ExitCode)
	s.Contains(refused.Stderr, "--yes")

	// Approving records the digest and everything passes.
	ok := s.run("approve", "include:shared", "--reviewer", "alice@example.org", "--note", "read the rule", "--yes")
	s.Require().Equal(0, ok.ExitCode, ok.Stderr)
	s.Equal("ok", s.status("include:shared"))
	s.NotContains(s.strict().Stdout, "AR71")
	s.Equal(0, s.run("lock", "--check").ExitCode)
	s.Equal(0, s.run("generate", "--locked").ExitCode)
	lock := s.lockText()
	s.Contains(lock, "[[approval]]")
	s.Contains(lock, "reviewer = 'alice@example.org'")

	// The source moves to a new version: the pin follows, the approval goes stale.
	s.release("two", "v1.1.0")
	s.Require().Equal(0, s.run("update").ExitCode)
	s.Contains(s.lockText(), "[[approval]]", "update keeps the approvals")
	s.Require().Equal(0, s.run("lock", "--content-only").ExitCode)
	s.Contains(s.lockText(), "[[approval]]", "so does a content re-pin")
	s.Equal("stale", s.status("include:shared"))
	stale := s.strict()
	s.Equal(2, stale.ExitCode)
	s.Contains(stale.Stdout, "AR711")
	s.Contains(s.run("lock", "--check").Stderr, "AR711")
	s.Equal(2, s.run("generate", "--locked").ExitCode)

	// lock --diff --format json carries the approval scope.
	diff := s.run("lock", "--diff", "--format", "json")
	s.Equal(0, diff.ExitCode)
	s.Contains(diff.Stdout, `"scope": "approval"`)

	// A second look at the new version makes it pass again; revoking undoes it.
	s.Require().Equal(0, s.run("approve", "include:shared", "--reviewer", "alice@example.org", "--yes").ExitCode)
	s.Equal("ok", s.status("include:shared"))
	s.Equal(0, s.run("lock", "--check").ExitCode)
	s.Require().Equal(0, s.run("approve", "--revoke", "include:shared").ExitCode)
	s.Equal("missing", s.status("include:shared"))
}

func (s *ApproveCLITestSuite) TestRelockKeepsApprovalBytes() {
	s.Require().Equal(0, s.run("lock").ExitCode)
	s.Require().Equal(0, s.run("approve", "include:shared", "--reviewer", "alice@example.org", "--at", "2026-10-05T12:00:00Z", "--yes").ExitCode)
	before := s.lockText()
	s.Require().Equal(0, s.run("lock").ExitCode)
	s.Equal(before, s.lockText(), "re-locking unchanged content rewrites the same bytes, timestamps included")
	s.True(strings.Contains(before, "approved_at = '2026-10-05T12:00:00Z'"), before)
}

func (s *ApproveCLITestSuite) lockText() string {
	data, err := os.ReadFile(filepath.Join(s.dir, ".ai-rulez", "ai-rulez.lock"))
	s.Require().NoError(err)
	return string(data)
}

// A deleted or stripped lock must not switch the approvals off. The project has
// no remote include, whose own lock checks would refuse first.
func (s *ApproveCLITestSuite) TestEnforcedGovernanceFailsClosedWithoutALock() {
	testutil.WriteFile(s.T(), s.dir, ".ai-rulez/config.toml", `version = "4.0"
name = "p"
presets = ["claude"]
gitignore = false

[governance]
require_approval = ["local"]
enforce = true
`)
	s.Require().Equal(0, s.run("lock").ExitCode)
	lockPath := filepath.Join(s.dir, ".ai-rulez", "ai-rulez.lock")
	s.Require().NoError(os.WriteFile(lockPath, []byte("version = 1\n"), 0o600)) // a lock stripped of its pins

	locked := s.run("generate", "--locked")
	s.NotEqual(0, locked.ExitCode, locked.Stdout)
	s.Contains(locked.Stderr+locked.Stdout, "AR710")
	check := s.run("lock", "--check")
	s.Equal(2, check.ExitCode, check.Stdout)
	s.Contains(check.Stderr, "AR710")
	strict := s.strict()
	s.Equal(2, strict.ExitCode)
	s.Contains(strict.Stdout, "AR710")

	// Locking again is still possible: the refusal is about approvals, not about writing the lock.
	s.Equal(0, s.run("lock").ExitCode)
}

// The reviewer of record is compared case-insensitively.
func (s *ApproveCLITestSuite) TestReviewerIsNormalised() {
	s.Require().Equal(0, s.run("lock").ExitCode)
	s.Require().Equal(0, s.run("approve", "include:shared", "--reviewer", " Alice@Example.ORG ", "--yes").ExitCode)
	s.Contains(s.lockText(), "reviewer = 'alice@example.org'")
	s.Equal(0, s.run("approve", "--revoke", "include:shared", "--reviewer", "ALICE@example.org").ExitCode)
}

// The CI control for forgeable approvals: approvals added with their content are reported.
func (s *ApproveCLITestSuite) TestVerifyBaseFlagsAnApprovalAddedWithItsContent() {
	s.Require().Equal(0, s.run("lock").ExitCode)
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = s.dir
		out, err := cmd.CombinedOutput()
		s.Require().NoError(err, string(out))
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-qm", "base")

	// A change that moves the include to a new version and approves it in one go.
	s.release("two", "v1.1.0")
	s.Require().Equal(0, s.run("update").ExitCode)
	s.Require().Equal(0, s.run("approve", "include:shared", "--reviewer", "mallory@example.org", "--yes").ExitCode)

	verify := s.run("approve", "--verify-base", "main")
	s.Equal(2, verify.ExitCode, verify.Stderr)
	s.Contains(verify.Stdout, "AR716")
	s.Contains(verify.Stdout, "include:shared")

	strict := s.run("validate", "--strict", "--approvals-base", "main", "--format", "json")
	s.Equal(2, strict.ExitCode)
	s.Contains(strict.Stdout, "AR716")

	missing := s.run("approve", "--verify-base", "no-such-branch")
	s.Equal(1, missing.ExitCode, "an unreadable base never passes")
}
