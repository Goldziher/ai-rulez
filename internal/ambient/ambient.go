// Package ambient names what library code may not reach for on its own: the
// process environment, the home directory, the wall clock, subprocesses and the
// log destination. Library packages take these as values (Env, Clock, a
// runner.Runner, a logger.Logger) bundled in a Host, so an embedding service can
// pin or deny each of them; the zero Host is the real process, which is what the
// CLI runs with. This package and internal/runner are the only library code that
// touches the real facilities (the archlint test enforces that).
package ambient

import (
	"context"
	"os"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Env is the process environment as library code may read it.
type Env interface {
	// LookupEnv is os.LookupEnv.
	LookupEnv(name string) (string, bool)
	// UserHomeDir is os.UserHomeDir.
	UserHomeDir() (string, error)
}

// Getenv returns the value of name in e, or "".
func Getenv(e Env, name string) string {
	v, _ := OrOS(e).LookupEnv(name)
	return v
}

// Expand replaces ${var} and $var in s from e, like os.ExpandEnv.
func Expand(e Env, s string) string {
	return os.Expand(s, func(name string) string { return Getenv(e, name) })
}

// OrOS returns e, or the real environment when e is nil.
func OrOS(e Env) Env {
	if e == nil {
		return osEnv{}
	}
	return e
}

type osEnv struct{}

func (osEnv) LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }
func (osEnv) UserHomeDir() (string, error)         { return os.UserHomeDir() }

// OS is the real process environment.
func OS() Env { return osEnv{} }

// MapEnv is a fixed environment for tests and services. Home is what
// UserHomeDir returns; empty means no home directory.
type MapEnv struct {
	Vars map[string]string
	Home string
}

// LookupEnv implements Env.
func (m MapEnv) LookupEnv(name string) (string, bool) {
	v, ok := m.Vars[name]
	return v, ok
}

// UserHomeDir implements Env.
func (m MapEnv) UserHomeDir() (string, error) {
	if m.Home == "" {
		return "", os.ErrNotExist
	}
	return m.Home, nil
}

// Clock reports the current time. The zero value (nil) is the wall clock.
type Clock func() time.Time

// Now returns the clock's time.
func (c Clock) Now() time.Time {
	if c == nil {
		return time.Now()
	}
	return c()
}

// Fixed is a Clock that always reports t.
func Fixed(t time.Time) Clock { return func() time.Time { return t } }

// Host bundles the injected facilities. Every field is optional: nil means the
// real one.
type Host struct {
	Env    Env
	Clock  Clock
	Runner runner.Runner
	Log    logger.Logger
}

// GetEnv looks name up in the host's environment.
func (h Host) GetEnv(name string) string { return Getenv(h.Env, name) }

// LookupEnv looks name up in the host's environment.
func (h Host) LookupEnv(name string) (string, bool) { return OrOS(h.Env).LookupEnv(name) }

// Home is the host's home directory.
func (h Host) Home() (string, error) { return OrOS(h.Env).UserHomeDir() }

// Now is the host's time.
func (h Host) Now() time.Time { return h.Clock.Now() }

// Logger is the host's logger (the CLI's when unset).
func (h Host) Logger() logger.Logger { return logger.Or(h.Log) }

// Run returns the host's runner (real processes when unset).
func (h Host) Run() runner.Runner { return runner.Or(h.Runner) }

type ctxKey struct{}

// WithContext returns ctx carrying h, and its Runner and Logger under the keys
// those packages read, for call chains that pass a context but no options.
func WithContext(ctx context.Context, h Host) context.Context {
	ctx = runner.WithContext(ctx, h.Runner)
	ctx = logger.WithContext(ctx, h.Log)
	return context.WithValue(ctx, ctxKey{}, h)
}

// FromContext returns the Host ctx carries, or the zero Host.
func FromContext(ctx context.Context) Host {
	if ctx != nil {
		if h, ok := ctx.Value(ctxKey{}).(Host); ok {
			return h
		}
	}
	return Host{}
}
