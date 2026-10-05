package llm

import (
	"errors"
	"fmt"
	"time"
)

// Kind classifies a failure so callers can react without parsing messages.
type Kind string

// Error kinds.
const (
	KindRateLimit       Kind = "rate_limit"
	KindAuth            Kind = "auth"
	KindContextLength   Kind = "context_length"
	KindProvider        Kind = "provider"
	KindBudget          Kind = "budget"
	KindNetworkDisabled Kind = "network_disabled"
	KindConfig          Kind = "config"
	KindDryRun          Kind = "dry_run"
	KindTimeout         Kind = "timeout"
)

// Sentinels for errors.Is.
var (
	ErrRateLimit       = &Error{Kind: KindRateLimit}
	ErrAuth            = &Error{Kind: KindAuth}
	ErrContextLength   = &Error{Kind: KindContextLength}
	ErrProvider        = &Error{Kind: KindProvider}
	ErrBudget          = &Error{Kind: KindBudget}
	ErrNetworkDisabled = &Error{Kind: KindNetworkDisabled}
	ErrConfig          = &Error{Kind: KindConfig}
	ErrDryRun          = &Error{Kind: KindDryRun}
	ErrTimeout         = &Error{Kind: KindTimeout}
)

// Error is the typed error every Client returns. Messages never contain
// prompts or API keys.
type Error struct {
	Kind    Kind
	Status  int // HTTP status when known
	Message string
	// RetryAfter is the provider's requested delay for rate limits, if any.
	RetryAfter time.Duration
	Cause      error
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = string(e.Kind)
	}
	if e.Status != 0 {
		return fmt.Sprintf("llm %s (HTTP %d): %s", e.Kind, e.Status, msg)
	}
	return fmt.Sprintf("llm %s: %s", e.Kind, msg)
}

// Unwrap exposes the underlying cause.
func (e *Error) Unwrap() error { return e.Cause }

// Is matches sentinels by kind.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Kind == e.Kind && t.Message == "" && t.Status == 0
}

// Transient reports whether retrying the same request may succeed.
func (e *Error) Transient() bool {
	switch e.Kind {
	case KindRateLimit, KindTimeout:
		return true
	case KindProvider:
		return e.Status == 0 || e.Status >= 500 || e.Status == 408 || e.Status == 409
	default:
		return false
	}
}

func newError(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// IsTransient reports whether err is worth retrying.
func IsTransient(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Transient()
}
