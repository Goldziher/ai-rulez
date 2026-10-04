package config

import "errors"

// Configuration errors
var (
	ErrInvalidVersion        = errors.New("invalid version: must be '3.0' or '4.0'")
	ErrMissingName           = errors.New("missing required field: name")
	ErrInvalidDefaultProfile = errors.New("default profile does not exist in profiles")
	ErrInvalidPreset         = errors.New("invalid preset configuration")
	ErrNoContent             = errors.New("no content loaded")
	// ErrLockViolation marks a remote include or installed skill that does not
	// match ai-rulez.lock. Unlike other resolution failures it is fatal: loading
	// continues with local content only for a flaky remote, never for a lock mismatch.
	ErrLockViolation = errors.New("ai-rulez.lock violation")
)
