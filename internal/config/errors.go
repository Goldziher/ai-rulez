package config

import "errors"

// Configuration errors
var (
	ErrMissingName           = errors.New("missing required field: name")
	ErrInvalidDefaultProfile = errors.New("default profile does not exist in profiles")
	ErrInvalidPreset         = errors.New("invalid preset configuration")
	ErrNoContent             = errors.New("no content loaded")
	// ErrLockViolation marks a remote include or installed skill that does not
	// match ai-rulez.lock. Unlike other resolution failures it is fatal: loading
	// continues with local content only for a flaky remote, never for a lock mismatch.
	ErrLockViolation = errors.New("ai-rulez.lock violation")
	// ErrIncludeUnresolved marks an include that could not be fetched (no network
	// and no cached copy), read or merged. It is fatal: rendering without it
	// would produce outputs the configuration never described. Only --no-fetch
	// downgrades it to a warning.
	ErrIncludeUnresolved = errors.New("include could not be resolved")
	// ErrIncludeOutsideProject marks a local include declared in the committed
	// project config that resolves outside the project. It is fatal like a lock
	// violation: dropping the include with a warning would hide a path a
	// repository should never be able to name.
	ErrIncludeOutsideProject = errors.New("local include is outside the project")
)
