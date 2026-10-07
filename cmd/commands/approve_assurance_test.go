package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

const signedGovernance = "\nmin_assurance = \"signed\"\n\n[[signing.trust]]\nsubject = \"approval\"\nkey_file = \"keys/approver.pub\"\n"

// approverKey writes a fresh public key into the project (the trust entry reads
// it) and returns the path of the private key, kept outside the project.
func approverKey(t *testing.T, root string) string {
	t.Helper()
	priv, pub, err := signing.GenerateKeyPair(nil)
	require.NoError(t, err)
	writeFile(t, filepath.Join(root, "keys", "approver.pub"), string(pub))
	keyPath := filepath.Join(t.TempDir(), "approver.key")
	require.NoError(t, os.WriteFile(keyPath, priv, 0o600))
	return keyPath
}

func loadLockAt(t *testing.T, root string) *lockfile.File {
	t.Helper()
	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	require.NotNil(t, lock)
	return lock
}

func findingFor(findings []string, want string) bool {
	for _, f := range findings {
		if strings.Contains(f, want) {
			return true
		}
	}
	return false
}

func findingTexts(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range approvalFindingsFor(t.Context(), mustLoadConfig(t)) {
		out = append(out, f.Code+" "+f.Message)
	}
	return out
}

func TestApprove_SignedApprovalRoundTrip(t *testing.T) {
	// Arrange
	root := approveProject(t, signedGovernance)
	key := approverKey(t, root)
	approveYes, approveSign, signKey, approveAt = true, true, key, "2026-10-05T12:00:00Z"

	// Act
	code, stdout, stderr := runApproveCmd(t, "rule:style")

	// Assert: a signed record, its bundle next to the lock, and the signer as reviewer
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "assurance=signed")
	lock := loadLockAt(t, root)
	require.Len(t, lock.Approval, 1)
	rec := lock.Approval[0]
	assert.Equal(t, lockfile.AssuranceSigned, rec.Assurance)
	assert.True(t, strings.HasPrefix(rec.Reviewer, "key:"), rec.Reviewer)
	bundlePath, err := approval.AttestationFile(filepath.Join(root, ".ai-rulez"), rec.Attestation)
	require.NoError(t, err)
	assert.FileExists(t, bundlePath)

	// The rule is approved at the required assurance; the skill still needs approval
	texts := findingTexts(t)
	assert.False(t, findingFor(texts, "rule:style"), "%v", texts)
	assert.True(t, findingFor(texts, "skill:deploy"))

	// list reports the assurance
	resetApproveFlags()
	approveList, approveFormat = true, formatJSON
	code, stdout, stderr = runApproveCmd(t)
	require.Equal(t, 0, code, stderr)
	validateAgainst(t, "../../schema/approve-list.schema.json", []byte(stdout))
	var doc approveListDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, "signed", doc.Policy.MinAssurance)
	for _, it := range doc.Items {
		if it.Ref == "rule:style" {
			assert.Equal(t, "ok", it.Status)
			assert.Equal(t, "signed", it.Assurance)
		}
	}

	// verify --approvals re-checks the signature offline
	resetApproveFlags()
	verifyApprovals = true
	var vcode int
	vout, verr := capture(t, func() { vcode = runVerifyApprovals(nil, os.Stdout) })
	assert.Equal(t, 0, vcode, verr)
	assert.Contains(t, vout, "OK    signed")

	// A tampered bundle no longer counts: AR718 from validate and verify
	require.NoError(t, os.WriteFile(bundlePath, []byte(`{"mediaType":"x"}`), 0o644))
	texts = findingTexts(t)
	assert.True(t, findingFor(texts, "AR718"), "%v", texts)
	_, verr = capture(t, func() { vcode = runVerifyApprovals(nil, os.Stdout) })
	assert.Equal(t, exitDrift, vcode)
	assert.Contains(t, verr, "AR718")
}

func TestApprove_ApprovalsAreInsideTheLockSubject(t *testing.T) {
	// Arrange
	root := approveProject(t, "")
	var before, after struct {
		ApprovalsDigest string `json:"approvals_digest"`
		Subject         string `json:"subject"`
	}
	lockFormat = formatJSON
	t.Cleanup(func() { lockFormat = "" })
	out, _ := capture(t, func() { require.Equal(t, 0, lockSubjectAt("")) })
	require.NoError(t, json.Unmarshal([]byte(out), &before))

	// Act
	approveYes, approveReviewer = true, "alice@example.org"
	require.Equal(t, 0, mustApprove(t, "rule:style"))
	out, _ = capture(t, func() { require.Equal(t, 0, lockSubjectAt("")) })
	require.NoError(t, json.Unmarshal([]byte(out), &after))

	// Assert
	assert.Empty(t, before.ApprovalsDigest)
	assert.Equal(t, loadLockAt(t, root).ApprovalsDigest(), after.ApprovalsDigest)
	assert.NotEmpty(t, after.ApprovalsDigest)
	assert.NotEqual(t, before.Subject, after.Subject, "the signed subject moves with the approval set")
}

func TestApprove_MinAssuranceRejectsAnAssertedApproval(t *testing.T) {
	// Arrange
	approveProject(t, signedGovernance)
	approveYes, approveReviewer = true, "alice@example.org"
	require.Equal(t, 0, mustApprove(t, "rule:style"))

	// Act
	texts := findingTexts(t)

	// Assert
	assert.True(t, findingFor(texts, "AR714 rule:style has no approval of the required assurance"), "%v", texts)
}

func TestApprove_AssuranceFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"sign needs a way to sign", func() { approveSign = true }, "choose how to sign"},
		{"sign with key and keyless", func() { approveSign, signKey, signKeyless = true, "k", true }, "mutually exclusive"},
		{"sign and review are separate", func() { approveSign, signKey, approveFromReview = true, "k", 3 }, "pick one"},
		{"sign takes no reviewer", func() { approveSign, signKey, approveReviewer = true, "k", "alice" }, "--reviewer does not apply"},
		{"a signing flag without sign", func() { signKey = "k" }, "apply to --sign"},
		{"tlog is for keys", func() { approveSign, signKeyless, signTLog = true, true, true }, "--tlog applies to --key"},
		{"deny belongs to revoke", func() { approveDeny = true }, "--deny applies to --revoke"},
		{"reason belongs to deny", func() { approveReason = "x" }, "--reason applies to --deny"},
		{"a reason with a control character", func() { approveRevoke, approveDeny, approveReason = true, true, "a\x01b" }, "invalid --reason"},
		{"review is not a revoke option", func() { approveFromReview, approveRevoke = 3, true }, "do not combine"},
		{"resolve-teams does not revoke", func() { approveResolveTeams, approveRevoke = true, true }, "--resolve-teams applies"},
		{"base is not an option", func() { approveBase = "--output=x" }, "--base applies"},
		{"insecure rekor", func() { approveSign, signKeyless, signRekorURL = true, true, "http://rekor.example.org" }, "must be https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetApproveFlags()
			t.Cleanup(resetApproveFlags)
			tt.set()

			err := validateApproveFlags([]string{"rule:x"})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestApprove_DenyListBlocksApprovingAndPinning(t *testing.T) {
	// Arrange: approve the rule, then revoke and deny its digest
	root := approveProject(t, "")
	approveYes, approveReviewer = true, "alice@example.org"
	require.Equal(t, 0, mustApprove(t, "rule:style"))
	digest := loadLockAt(t, root).Item[0].Digest
	for _, it := range loadLockAt(t, root).Item {
		if it.Kind == "rule" {
			digest = it.Digest
		}
	}
	resetApproveFlags()
	approveRevoke, approveDeny, approveReason = true, true, "exfiltrates ~/.ssh"

	// Act
	code, stdout, stderr := runApproveCmd(t, "rule:style")

	// Assert: revoked, denied, written to the lock
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "denied rule:style")
	lock := loadLockAt(t, root)
	assert.Empty(t, lock.Approval)
	assert.Equal(t, []lockfile.Deny{{Digest: digest, Reason: "exfiltrates ~/.ssh"}}, lock.Deny)

	// validate reports AR717 whether or not the policy selects the item
	texts := findingTexts(t)
	assert.True(t, findingFor(texts, "AR717 rule:style is on the deny list"), "%v", texts)

	// approving it is refused
	resetApproveFlags()
	approveYes, approveReviewer = true, "alice@example.org"
	code, _, stderr = runApproveCmd(t, "rule:style")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "AR717")

	// lock refuses to pin it again, and keeps the previous lock
	before := lockText(t, root)
	var lockCode int
	_, stderr = capture(t, func() { lockCode = writeLockAt("", "", nil) })
	assert.Equal(t, 1, lockCode)
	assert.Contains(t, stderr, "AR717")
	assert.Equal(t, before, lockText(t, root))

	// changing the content makes it pinnable again, and the deny entry survives the re-lock
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	assert.Equal(t, lock.Deny, loadLockAt(t, root).Deny)
}

func TestApprove_CodeownersAndTeamsDecideWhoMayApprove(t *testing.T) {
	// Arrange
	root := approveProject(t, "\napprovers_from = \"CODEOWNERS\"\n\n[governance.teams]\n\"@acme/sec\" = [\"carol@example.org\"]\n")
	writeFile(t, filepath.Join(root, ".github", "CODEOWNERS"), "/.ai-rulez/rules/ @alice @acme/sec\n/.ai-rulez/skills/ @bob\n")
	tests := []struct {
		name     string
		item     string
		reviewer string
		wantErr  string
	}{
		{"a direct owner", "rule:style", "github:alice", ""},
		{"a team member", "rule:style", "carol@example.org", ""},
		{"someone else", "rule:style", "mallory", "is not a code owner"},
		{"the owner of another path", "skill:deploy", "github:alice", "is not a code owner"},
		{"the owner of this path", "skill:deploy", "bob", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetApproveFlags()
			approveYes, approveReviewer = true, tt.reviewer

			// Act
			code, _, stderr := runApproveCmd(t, tt.item)

			// Assert
			if tt.wantErr == "" {
				assert.Equal(t, 0, code, stderr)
				return
			}
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, tt.wantErr)
		})
	}

	// a CODEOWNERS file that disappears authorizes nobody, and validate says why (AR719)
	require.NoError(t, os.Remove(filepath.Join(root, ".github", "CODEOWNERS")))
	resetApproveFlags()
	approveYes, approveReviewer = true, "github:alice"
	code, _, stderr := runApproveCmd(t, "rule:style")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "AR719")
	assert.True(t, findingFor(findingTexts(t), "AR719 approvers_from"), "%v", findingTexts(t))
}

func TestApprove_AnOwnerTeamWithoutMembersIsReportedAndFailsClosed(t *testing.T) {
	// Arrange: the owner is a team nobody listed
	root := approveProject(t, "\napprovers_from = \"CODEOWNERS\"\n")
	writeFile(t, filepath.Join(root, ".github", "CODEOWNERS"), "* @acme/sec\n")

	// Act
	approveYes, approveReviewer = true, "carol@example.org"
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--resolve-teams")
	assert.True(t, findingFor(findingTexts(t), "AR719 no members are known for @acme/sec"), "%v", findingTexts(t))
}

func TestApprove_ResolveTeamsReadsMembersFromTheForge(t *testing.T) {
	// Arrange
	root := approveProject(t, "\napprovers_from = \"CODEOWNERS\"\n")
	writeFile(t, filepath.Join(root, ".github", "CODEOWNERS"), "* @acme/sec\n")
	t.Setenv("GITHUB_REPOSITORY", "acme/config")
	fake := &forge.Fake{Teams: map[string][]string{"github.com/acme/sec": {"carol"}}}
	approveForge = func() forge.Client { return fake }
	approveYes, approveResolveTeams = true, true

	// Act: a member
	approveReviewer = "github:carol"
	code, _, stderr := runApproveCmd(t, "rule:style")
	// Act: a non-member
	approveReviewer = "github:mallory"
	badCode, _, badErr := runApproveCmd(t, "skill:deploy")

	// Assert
	assert.Equal(t, 0, code, stderr)
	assert.Equal(t, 1, badCode)
	assert.Contains(t, badErr, "is not a code owner")
	assert.Equal(t, []string{"TeamMembers github.com/acme/sec", "TeamMembers github.com/acme/sec"}, fake.Calls())

	// A forge that refuses the team read fails the command: authorization never guesses
	fake.Err = forge.ErrForbidden
	approveReviewer = "github:carol"
	code, _, stderr = runApproveCmd(t, "skill:deploy")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "read:org")
}

// reviewedProject is a committed project whose HEAD pull request 7 contains.
func reviewedProject(t *testing.T, extra string, reviews ...forge.Review) (root, head string, fake *forge.Fake) {
	t.Helper()
	root = approveProject(t, extra)
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "content and lock")
	head = crossGit(t, root, "rev-parse", "HEAD")
	t.Setenv("GITHUB_REPOSITORY", "acme/config")
	repo := "github.com/acme/config"
	fake = &forge.Fake{
		PRByNumber: map[string]forge.PullRequest{repo + "#7": {Number: 7, Author: "dave", HeadSHA: head}},
		ReviewsBy:  map[string][]forge.Review{repo + "#7": reviews},
	}
	approveForge = func() forge.Client { return fake }
	return root, head, fake
}

func ghReview(id int64, login, state, commit string) forge.Review {
	return forge.Review{ID: id, Login: login, State: state, CommitID: commit, Submitted: time.Date(2026, 10, 1, 12, int(id), 0, 0, time.UTC), AuthorAssociation: forge.AssociationMember}
}

func TestApprove_FromGithubReviewLinksTheApprovingReviews(t *testing.T) {
	// Arrange
	head := ""
	root, head, fake := reviewedProject(t, "\nmin_assurance = \"review-linked\"\n")
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{
		ghReview(1, "alice", forge.ReviewApproved, head), ghReview(2, "bob", forge.ReviewChangesRequested, head),
		ghReview(3, "erin", forge.ReviewApproved, head),
	}
	approveYes, approveFromReview, approveAt = true, 7, "2026-10-05T12:00:00Z"

	// Act
	code, stdout, stderr := runApproveCmd(t, "rule:style")

	// Assert: one review-linked record per approving reviewer, with the review URL
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "assurance=review-linked")
	lock := loadLockAt(t, root)
	require.Len(t, lock.Approval, 2)
	assert.Equal(t, "github:alice", lock.Approval[0].Reviewer)
	assert.Equal(t, "https://github.com/acme/config/pull/7#pullrequestreview-1", lock.Approval[0].Ref)
	assert.Equal(t, lockfile.AssuranceReviewLinked, lock.Approval[0].Assurance)
	assert.Equal(t, "github:erin", lock.Approval[1].Reviewer)
	assert.False(t, findingFor(findingTexts(t), "rule:style"), "review-linked meets min_assurance")

	// --reviewer picks one of them
	resetApproveFlags()
	approveForge = func() forge.Client { return fake }
	approveYes, approveFromReview, approveReviewer = true, 7, "github:erin"
	code, _, stderr = runApproveCmd(t, "skill:deploy")
	require.Equal(t, 0, code, stderr)
	var skillReviewers []string
	for _, a := range loadLockAt(t, root).Approval {
		if a.Kind == "skill" {
			skillReviewers = append(skillReviewers, a.Reviewer)
		}
	}
	assert.Equal(t, []string{"github:erin"}, skillReviewers)
}

func TestApprove_FromGithubReviewRefusesContentTheReviewDidNotSee(t *testing.T) {
	// Arrange: the review approved the committed content; the rule changed afterwards
	root, head, fake := reviewedProject(t, "")
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{ghReview(1, "alice", forge.ReviewApproved, head)}
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	approveYes, approveFromReview = true, 7

	// Act
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no approving review of rule:style")
	assert.Empty(t, loadLockAt(t, root).Approval)
}

func TestApprove_FromGithubReviewHonoursCodeownersAndSelfApproval(t *testing.T) {
	// Arrange: erin is not an owner; dave authored the pull request
	root, head, fake := reviewedProject(t, "\napprovers_from = \"CODEOWNERS\"\nforbid_self_approval = true\n")
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{
		ghReview(1, "erin", forge.ReviewApproved, head), ghReview(2, "dave", forge.ReviewApproved, head), ghReview(3, "alice", forge.ReviewApproved, head),
	}
	writeFile(t, filepath.Join(root, ".github", "CODEOWNERS"), "* @alice @dave\n")
	approveYes, approveFromReview, approveBase = true, 7, "main"

	// Act
	code, stdout, stderr := runApproveCmd(t, "rule:style")

	// Assert: erin is dropped (not an owner), dave is dropped (the author), alice is recorded
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "skipped github:erin")
	assert.Contains(t, stdout, "skipped github:dave")
	lock := loadLockAt(t, root)
	require.Len(t, lock.Approval, 1)
	assert.Equal(t, "github:alice", lock.Approval[0].Reviewer)
}

func TestVerifyApprovals_OnlineReChecksReviewLinkedApprovals(t *testing.T) {
	// Arrange: approve from a review, commit the lock, then the review is dismissed
	root, head, fake := reviewedProject(t, "")
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{ghReview(1, "alice", forge.ReviewApproved, head)}
	approveYes, approveFromReview = true, 7
	require.Equal(t, 0, mustApprove(t, "rule:style"))
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "record the approval")

	run := func(online bool, format string) (int, string, string) {
		resetApproveFlags()
		approveForge = func() forge.Client { return fake }
		verifyApprovals, verifyOnline, verifyFormat = true, online, format
		var code int
		stdout, stderr := capture(t, func() { code = runVerifyApprovals(nil, os.Stdout) })
		return code, stdout, stderr
	}

	// Act / Assert: offline, the link is reported unchecked
	code, stdout, stderr := run(false, "")
	assert.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "SKIP  review-linked")

	// online, the review still approves
	code, stdout, stderr = run(true, formatJSON)
	require.Equal(t, 0, code, stderr)
	validateAgainst(t, "../../schema/verify-approvals.schema.json", []byte(stdout))
	assert.Contains(t, stdout, `"status": "valid"`)

	// dismissed: the approval no longer holds
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{
		ghReview(1, "alice", forge.ReviewApproved, head), ghReview(2, "alice", forge.ReviewDismissed, head),
	}
	code, _, stderr = run(true, "")
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR718")
	assert.Contains(t, stderr, "no longer has an approving review")

	// a forge that cannot be reached means the check could not run: exit 1, never a pass
	fake.Err = forge.ErrOffline
	code, _, stderr = run(true, "")
	assert.Equal(t, 1, code, stderr)
}

func TestVerifyApprovals_FlagValidation(t *testing.T) {
	resetApproveFlags()
	t.Cleanup(resetApproveFlags)
	verifyOnline = true
	err := rejectApprovalFlags(VerifyCmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--online applies to --approvals")

	resetApproveFlags()
	verifyApprovals, verifyAttestation = true, true
	t.Cleanup(func() { verifyAttestation = false })
	err = rejectApprovalFlags(VerifyCmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")
}

func TestApprove_ForbidSelfApprovalComparesCommitAuthors(t *testing.T) {
	// Arrange: alice authors an edit to the rule on a branch
	root := approveProject(t, "\nforbid_self_approval = true\n")
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	crossGit(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "edit the rule", "--author=Alice <alice@example.org>")

	// Act: alice approves her own change
	approveYes, approveReviewer, approveBase = true, "alice@example.org", "main"
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "forbid_self_approval")

	// Act: bob approves it; alice may approve the skill she did not touch
	resetApproveFlags()
	approveYes, approveReviewer, approveBase = true, "bob@example.org", "main"
	assert.Equal(t, 0, mustApprove(t, "rule:style"))
	resetApproveFlags()
	approveYes, approveReviewer, approveBase = true, "alice@example.org", "main"
	assert.Equal(t, 0, mustApprove(t, "skill:deploy"))

	// Without a base the branch has no upstream to count from: fail closed
	resetApproveFlags()
	approveYes, approveReviewer = true, "bob@example.org"
	code, _, stderr = runApproveCmd(t, "rule:style")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--base")
}

func TestApprove_VerifyBaseReportsAnApprovalByAnAuthor(t *testing.T) {
	// Arrange: alice's approval of the rule she edited is already in the lock
	root := approveProject(t, "\nforbid_self_approval = true\n")
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	crossGit(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "edit the rule", "--author=Alice <alice@example.org>")
	lock := loadLockAt(t, root)
	for _, s := range approval.SubjectsOf(lock, lock.Item) {
		if s.Ref() == "rule:style" {
			lock.SetApproval(lockfile.Approval{Kind: s.Kind, ID: s.ID, Digest: s.Digest, Reviewer: "alice@example.org",
				Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-05T12:00:00Z"})
		}
	}
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), lock))
	approveVerifyBase = "main"

	// Act
	code, stdout, stderr := runApproveCmd(t)

	// Assert
	assert.Equal(t, 2, code, stderr)
	assert.Contains(t, stdout, "forbid_self_approval")
	assert.Contains(t, stdout, "alice@example.org")
}

func TestApprove_RoleOutputsAreApprovalSubjects(t *testing.T) {
	// Arrange: role dev pins its outputs; the policy selects them by kind
	root := lockProject(t, "\n[governance]\nrequire_approval = [\"kind:role-output\"]\nenforce = true\n"+strings.Replace(roleLockConfig, "%s", "off", 1))
	t.Cleanup(func() {
		lockRoles, lockServeRole, generateRole = false, "", ""
		resetApproveFlags()
	})
	resetApproveFlags()
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Act: the role output is listed and missing
	approveList, approveFormat = true, formatJSON
	code, stdout, stderr := runApproveCmd(t)
	require.Equal(t, 0, code, stderr)
	var doc approveListDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Items, 1)
	assert.Equal(t, "role-output:dev", doc.Items[0].Ref)
	assert.Equal(t, "missing", doc.Items[0].Status)

	// Act: approve it
	resetApproveFlags()
	approveYes, approveReviewer = true, "alice@example.org"
	code, stdout, stderr = runApproveCmd(t, "role-output:dev")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "role-output:dev")
	assert.Contains(t, stdout, "CLAUDE.md", "the reviewer is shown the rendered files")
	assert.Empty(t, findingTexts(t))

	// Act: the skill_mode changes and the lock is re-pinned: the old approval is stale
	setRoleMode(t, root, "off", "name-only")
	require.Equal(t, 0, writeLockAt("", "", nil))
	texts := findingTexts(t)
	assert.True(t, findingFor(texts, "AR711 role-output:dev"), "%v", texts)

	// Act: the config changes again without a re-lock: approving refuses a stale pin
	setRoleMode(t, root, "name-only", "off")
	resetApproveFlags()
	approveYes, approveReviewer = true, "alice@example.org"
	code, _, stderr = runApproveCmd(t, "role-output:dev")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "changed since they were pinned")
}
