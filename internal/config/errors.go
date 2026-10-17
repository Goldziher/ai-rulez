package config

import "errors"

// Configuration errors
var (
	ErrInvalidPreset = errors.New("invalid preset configuration")
	ErrNoContent     = errors.New("no content loaded")
	// ErrRoleReference marks a role that names something that does not exist (AR971),
	// such as a domain; commands exit 2 on it, the code `validate` uses for findings.
	ErrRoleReference = errors.New("role reference does not resolve")
	// ErrPolicyLoosens marks a repository configuration that tries to loosen the
	// organization policy; commands exit 2 on it, the code `validate` uses for findings.
	ErrPolicyLoosens = errors.New("configuration loosens the organization policy")
	// ErrLockViolation marks a remote include or installed skill that does not
	// match ai-rulez.lock. Unlike other resolution failures it is fatal: loading
	// continues with local content only for a flaky remote, never for a lock mismatch.
	ErrLockViolation = errors.New("ai-rulez.lock violation")
	// ErrIncludeUnresolved marks an include that could not be fetched (no network
	// and no cached copy), read or merged. It is fatal: rendering without it
	// would produce outputs the configuration never described. Only --offline
	// downgrades it to a warning.
	ErrIncludeUnresolved = errors.New("include could not be resolved")
	// ErrSkillUnresolved is ErrIncludeUnresolved for an installed skill: one that
	// cannot be fetched, found or scanned fails the load, because generating
	// without it removes its outputs as stale and lets `generate --check` pass.
	ErrSkillUnresolved = errors.New("installed skill could not be resolved")
	// ErrPluginUnresolved is ErrSkillUnresolved for an installed marketplace or plugin.
	ErrPluginUnresolved = errors.New("installed plugin could not be resolved")
	// ErrIncludeOutsideProject marks a local include declared in the committed
	// project config that resolves outside the project. It is fatal like a lock
	// violation: dropping the include with a warning would hide a path a
	// repository should never be able to name.
	ErrIncludeOutsideProject = errors.New("local include is outside the project")
	// ErrOutputRefused marks a generate run that refused to overwrite a file
	// ai-rulez cannot prove it wrote, so a caller can tell "a user file is in the
	// way" from an I/O failure without matching the message.
	ErrOutputRefused = errors.New("refusing to overwrite a file ai-rulez did not write")
)

// policyLoosening carries ErrPolicyLoosens without changing the message of the
// error it wraps.
type policyLoosening struct{ err error }

func (e policyLoosening) Error() string        { return e.err.Error() }
func (e policyLoosening) Unwrap() error        { return e.err }
func (e policyLoosening) Is(target error) bool { return target == ErrPolicyLoosens }

// ErrNoConfig marks a project that has no configuration: no config file in the
// directory searched. The read-only commands (validate, scan, doctor, okf
// validate) lint such a project through LoadImplicit; generate, clean and lock
// need a configuration and tell the user to run `ai-rulez init`.
var ErrNoConfig = errors.New("no ai-rulez configuration found")

// noConfig carries ErrNoConfig without changing the message of the error it wraps.
type noConfig struct{ err error }

func (e noConfig) Error() string        { return e.err.Error() }
func (e noConfig) Unwrap() error        { return e.err }
func (e noConfig) Is(target error) bool { return target == ErrNoConfig }
