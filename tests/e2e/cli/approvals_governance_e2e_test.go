package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// governedProject is a committed project with a rule and a skill whose
// [governance] block is gov; it is locked.
func governedProject(t *testing.T, env *isoEnv, gov string, extra map[string]string) string {
	t.Helper()
	root := minimalProject(t, "\n[governance]\nrequire_approval = [\"local\"]\nenforce = true\n"+gov)
	writeTree(t, root, extra)
	env.commitAll(root, "init")
	lock := env.run(root, "lock")
	require.Equal(t, 0, lock.ExitCode, lock.Stderr)
	return root
}

func TestApprovalsGovernanceE2E(t *testing.T) {
	t.Run("CODEOWNERS with a team authorizes only the owners of each path", func(t *testing.T) {
		// Arrange
		env := newIsoEnv(t)
		root := governedProject(t, env, "approvers_from = \"CODEOWNERS\"\n\n[governance.teams]\n\"@acme/security\" = [\"alice@example.org\"]\n",
			map[string]string{".github/CODEOWNERS": "/.ai-rulez/rules/ @acme/security\n/.ai-rulez/skills/ carol@example.org\n"})

		// Act
		alice := env.run(root, "approve", "rule:local", "--reviewer", "alice@example.org", "--yes")
		mallory := env.run(root, "approve", "rule:local", "--reviewer", "mallory@example.org", "--yes")
		aliceSkill := env.run(root, "approve", "skill:deploy", "--reviewer", "alice@example.org", "--yes")
		carol := env.run(root, "approve", "skill:deploy", "--reviewer", "carol@example.org", "--yes")
		check := env.run(root, "lock", "--check")

		// Assert
		assert.Equal(t, 0, alice.ExitCode, alice.Stderr)
		assert.Equal(t, 1, mallory.ExitCode)
		assert.Contains(t, mallory.Stderr, "is not a code owner")
		assert.Equal(t, 1, aliceSkill.ExitCode)
		assert.Equal(t, 0, carol.ExitCode, carol.Stderr)
		assert.Equal(t, 0, check.ExitCode, check.Stdout+check.Stderr)
	})

	t.Run("a repository cannot widen a team an organization policy pins", func(t *testing.T) {
		blockedOn(t, "RV-GOV-2")
		// Arrange
		env := newIsoEnv(t)
		root := governedProject(t, env, "approvers = [\"@acme/security\"]\n\n[governance.teams]\n\"@acme/security\" = [\"alice@example.org\", \"mallory@example.org\"]\n", nil)
		policy := filepath.Join(t.TempDir(), "policy.toml")
		require.NoError(t, os.WriteFile(policy, []byte("policy_version = 1\nname = \"org\"\n\n[governance]\nenforce = true\napprovers = [\"@acme/security\"]\n"), 0o600))

		// Act
		res := env.run(root, "approve", "rule:local", "--reviewer", "mallory@example.org", "--yes", "--policy", policy)
		strict := env.run(root, "validate", "--strict", "--format", "json", "--policy", policy)

		// Assert: mallory was added by the repository itself, in the change it gates.
		assert.NotEqual(t, 0, res.ExitCode, "mallory is not on the policy's team")
		assert.Regexp(t, regexp.MustCompile(`AR71[0-9]|AR740`), strict.Stdout)
	})

	t.Run("two keys of one signer count once toward min_approvers", func(t *testing.T) {
		blockedOn(t, "RV-GOV-3")
		// Arrange
		env := newIsoEnv(t)
		keys := t.TempDir()
		k1, p1 := writeKeyPair(t, keys, "one")
		k2, p2 := writeKeyPair(t, keys, "two")
		pub1, _ := os.ReadFile(p1) //nolint:errcheck // written above
		pub2, _ := os.ReadFile(p2) //nolint:errcheck // written above
		root := governedProject(t, env, "min_assurance = \"signed\"\nmin_approvers = 2\n\n[signing]\ntlog = \"off\"\n\n"+
			"[[signing.trust]]\nsubject = \"approval\"\nkey_file = \"one.pub\"\nreviewer = \"alice@example.org\"\n\n"+
			"[[signing.trust]]\nsubject = \"approval\"\nkey_file = \"two.pub\"\nreviewer = \"alice@example.org\"\n",
			map[string]string{"one.pub": string(pub1), "two.pub": string(pub2)})

		// Act
		require.Equal(t, 0, env.run(root, "approve", "rule:local", "--sign", "--key", k1, "--yes").ExitCode)
		require.Equal(t, 0, env.run(root, "approve", "rule:local", "--sign", "--key", k2, "--yes").ExitCode)
		list := env.run(root, "approve", "--list", "--format", "json")

		// Assert
		require.Equal(t, 0, list.ExitCode, list.Stderr)
		assert.Contains(t, list.Stdout, `"code": "AR714"`, "one person with two keys is one approver: %s", list.Stdout)
	})

	t.Run("a signed record whose reviewer is edited in the lock stops counting", func(t *testing.T) {
		// Arrange
		env := newIsoEnv(t)
		keys := t.TempDir()
		key, pub := writeKeyPair(t, keys, "approver")
		pubPEM, err := os.ReadFile(pub) //nolint:gosec // test key
		require.NoError(t, err)
		root := governedProject(t, env, "min_assurance = \"signed\"\n\n[signing]\ntlog = \"off\"\n\n[[signing.trust]]\nsubject = \"approval\"\nkey_file = \"approver.pub\"\n",
			map[string]string{"approver.pub": string(pubPEM)})
		require.Equal(t, 0, env.run(root, "approve", "rule:local", "--sign", "--key", key, "--yes").ExitCode)
		lockPath := filepath.Join(root, ".ai-rulez", "ai-rulez.lock")
		lock, err := os.ReadFile(lockPath)
		require.NoError(t, err)
		edited := regexp.MustCompile(`reviewer = '[^']*'`).ReplaceAll(lock, []byte(`reviewer = 'mallory@example.org'`))
		require.NotEqual(t, string(lock), string(edited), "the lock carries the signed record")
		require.NoError(t, os.WriteFile(lockPath, edited, 0o600))

		// Act
		res := env.run(root, "verify", "--approvals", "--format", "json")

		// Assert
		assert.Equal(t, 2, res.ExitCode, res.Stdout+res.Stderr)
		requireJSONDoc(t, res)
		assert.Contains(t, res.Stdout, "AR718")
	})

	t.Run("a denied digest can be neither approved nor locked", func(t *testing.T) {
		// Arrange
		env := newIsoEnv(t)
		root := governedProject(t, env, "", nil)

		require.Equal(t, 0, env.run(root, "approve", "skill:deploy", "--reviewer", "carol@example.org", "--yes").ExitCode)

		// Act
		deny := env.run(root, "approve", "skill:deploy", "--revoke", "--deny", "--reason", "exfiltrates ssh keys", "--yes")
		approve := env.run(root, "approve", "skill:deploy", "--reviewer", "carol@example.org", "--yes")
		strict := env.run(root, "validate", "--strict", "--format", "json")
		relock := env.run(root, "lock")

		// Assert
		require.Equal(t, 0, deny.ExitCode, deny.Stderr)
		assert.Equal(t, 1, approve.ExitCode)
		assert.Contains(t, approve.Stderr, "AR717")
		assert.Equal(t, 2, strict.ExitCode)
		assert.Contains(t, strict.Stdout, "AR717")
		assert.Equal(t, 1, relock.ExitCode)
		assert.Contains(t, relock.Stderr, "deny list")
	})

	t.Run("forbid_self_approval refuses the author of the change", func(t *testing.T) {
		// Arrange
		env := newIsoEnv(t)
		root := governedProject(t, env, "forbid_self_approval = true\n", nil)
		base := env.git(root, "rev-parse", "HEAD")
		writeTree(t, root, map[string]string{".ai-rulez/rules/local.md": "# Local\n\nChanged by alice.\n"})
		env.set("GIT_AUTHOR_EMAIL", "alice@example.org").set("GIT_COMMITTER_EMAIL", "alice@example.org")
		env.git(root, "add", "-A")
		env.git(root, "commit", "-q", "-m", "alice edits")
		env.set("GIT_AUTHOR_EMAIL", "e2e@example.test").set("GIT_COMMITTER_EMAIL", "e2e@example.test")
		require.Equal(t, 0, env.run(root, "lock").ExitCode)

		// Act
		self := env.run(root, "approve", "rule:local", "--reviewer", "alice@example.org", "--base", base, "--yes")
		other := env.run(root, "approve", "rule:local", "--reviewer", "bob@example.org", "--base", base, "--yes")

		// Assert
		assert.Equal(t, 1, self.ExitCode)
		assert.Contains(t, self.Stderr, "forbid_self_approval")
		assert.Equal(t, 0, other.ExitCode, other.Stderr)
	})

	t.Run("generate --locked names the missing approval, not drift", func(t *testing.T) {
		blockedOn(t, "MAN-3")
		// Arrange
		env := newIsoEnv(t)
		root := governedProject(t, env, "", nil)

		// Act
		res := env.run(root, "generate", "--locked")

		// Assert
		assert.Equal(t, 2, res.ExitCode)
		assert.Contains(t, res.Stderr, "AR710")
		assert.NotContains(t, res.Stderr, "authored content differs", "nothing drifted: an approval is missing")
	})
}
