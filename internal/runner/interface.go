package runner

import (
	"context"
	"sync"
)

// Runner runs an external command. Library code that has to start a process
// takes a Runner instead of calling os/exec, so an embedding service can deny,
// record or fake every command. Run never returns an error: a failure is the
// Result's Status, as for the package-level Run.
type Runner interface {
	Run(ctx context.Context, spec Spec) Result
}

// Exec is the Runner that starts a real process through Run. It is the default
// everywhere a Runner is optional, and keeps the behaviour callers had before
// the runner was injectable.
type Exec struct{}

// Run implements Runner.
func (Exec) Run(ctx context.Context, spec Spec) Result { return Run(ctx, spec) }

// Func adapts a function to a Runner.
type Func func(ctx context.Context, spec Spec) Result

// Run implements Runner.
func (f Func) Run(ctx context.Context, spec Spec) Result { return f(ctx, spec) }

// Deny is the Runner of a context that must not start a process: every command
// is StatusUnavailable, with ErrDenied carrying the argv.
type Deny struct{}

// Run implements Runner.
func (Deny) Run(_ context.Context, spec Spec) Result {
	return Result{Status: StatusUnavailable, ExitCode: -1, Err: &DeniedError{Argv: append([]string(nil), spec.Argv...)}}
}

// DeniedError reports a command a Deny runner refused.
type DeniedError struct{ Argv []string }

func (e *DeniedError) Error() string {
	if len(e.Argv) == 0 {
		return "running commands is not allowed here"
	}
	return "running " + e.Argv[0] + " is not allowed here"
}

// Fake is a Runner for tests: it records every Spec and answers with Handle (a
// zero Result with StatusOK when Handle is nil). It is safe for concurrent use.
type Fake struct {
	// Handle computes the answer for a spec; nil answers StatusOK with no output.
	Handle func(spec Spec) Result

	mu    sync.Mutex
	calls []Spec
}

// Run implements Runner.
func (f *Fake) Run(_ context.Context, spec Spec) Result {
	f.mu.Lock()
	f.calls = append(f.calls, spec)
	f.mu.Unlock()
	if f.Handle == nil {
		return Result{Status: StatusOK}
	}
	return f.Handle(spec)
}

// Calls returns a copy of the recorded specs, in call order.
func (f *Fake) Calls() []Spec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Spec(nil), f.calls...)
}

type ctxKey struct{}

// WithContext returns ctx carrying r, for call chains that pass a context but no
// options (an include fetch). A nil r leaves ctx unchanged.
func WithContext(ctx context.Context, r Runner) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the Runner ctx carries, or Exec when it carries none.
func FromContext(ctx context.Context) Runner {
	if ctx != nil {
		if r, ok := ctx.Value(ctxKey{}).(Runner); ok {
			return r
		}
	}
	return Exec{}
}

// Or returns r, or Exec when r is nil.
func Or(r Runner) Runner {
	if r == nil {
		return Exec{}
	}
	return r
}
