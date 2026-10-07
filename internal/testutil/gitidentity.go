package testutil

import "testing"

// The identity GitIdentity gives git.
const (
	GitIdentityName  = "ai-rulez test"
	GitIdentityEmail = "test@ai-rulez.invalid"
)

// GitIdentity gives every git process the test starts (its own and the ones
// the code under test runs) an author and committer identity through the
// environment. A test that makes the product commit needs it: passing
// `-c user.name` to the test's own git calls does not reach the product's, and
// a machine without a configured identity (a Linux CI runner, whose host name
// git cannot guess an e-mail from) refuses the commit. macOS hides the problem
// because git guesses `user@host.local` there.
func GitIdentity(tb testing.TB) {
	tb.Helper()
	tb.Setenv("GIT_AUTHOR_NAME", GitIdentityName)
	tb.Setenv("GIT_AUTHOR_EMAIL", GitIdentityEmail)
	tb.Setenv("GIT_COMMITTER_NAME", GitIdentityName)
	tb.Setenv("GIT_COMMITTER_EMAIL", GitIdentityEmail)
}
