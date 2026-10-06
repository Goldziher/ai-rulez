// Package publish turns a verified plugin bundle into deterministic release
// artifacts: a reproducible tar.gz, a manifest, SHA256SUMS and a publish plan,
// and drives the `gh` CLI to upload them. It never reads the environment, the
// clock or the working directory, and never touches credentials: every
// process starts through an injected runner.Runner.
package publish

import "fmt"

// Codes of the publish report (docs/publish.md). They are registered in
// internal/lint so `validate --explain AR9N0` works; `validate` never emits them.
const (
	// CodePreflight: a preflight gate (strict validation, lock check, plugin verification) failed.
	CodePreflight = "AR9N0"
	// CodeBundleUnsafe: the bundle holds a symlink, a path outside the project or an unusable name.
	CodeBundleUnsafe = "AR9N1"
	// CodeSecret: the secret scan found a credential in the bundle.
	CodeSecret = "AR9N2"
	// CodeSource: the source tree is dirty or has no commit, or the plugin has no version.
	CodeSource = "AR9N3"
	// CodeTarget: the upload step failed (gh missing, release exists, gh exited non-zero).
	CodeTarget = "AR9N4"
	// CodeVerify: `publish verify` found a digest, manifest or archive mismatch.
	CodeVerify = "AR9N5"
)

// Exit codes: 1 the run could not complete, 2 a gate or verification failed.
const (
	ExitFailed = 1
	ExitGate   = 2
)

// Error is a publish failure that carries its rule code and exit status.
type Error struct {
	Code string
	// Exit is the process exit status for this failure.
	Exit int
	Msg  string
	Hint string
}

// Error implements error.
func (e *Error) Error() string { return e.Code + " " + e.Msg }

func newError(code string, exit int, hint, format string, args ...any) *Error {
	return &Error{Code: code, Exit: exit, Msg: fmt.Sprintf(format, args...), Hint: hint}
}

// Errorf builds an Error for callers outside the package (the preflight gates).
func Errorf(code string, exit int, hint, format string, args ...any) *Error {
	return newError(code, exit, hint, format, args...)
}
