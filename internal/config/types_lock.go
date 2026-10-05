package config

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Lock scopes select which authored items ai-rulez.lock pins.
const (
	// LockScopeAll pins every authored item and, with include_outputs, outputs.
	LockScopeAll = "all"
	// LockScopeSkills pins skills (and remote includes/installed skills) only.
	LockScopeSkills = "skills"
)

// LockConfig is the [lock] table: how strictly the committed ai-rulez.lock is
// enforced. Enforcement is on whenever ai-rulez.lock exists; `enforce = false`
// opts out.
type LockConfig struct {
	// Enforce makes `validate --strict` report content drift (AR981, AR982),
	// makes `lock --check` fail on a lock without content pins, makes `generate`
	// refuse a remote include or installed skill the lock does not pin (AR010 is
	// an error), and makes the skills server refuse a served skill whose digest
	// is not the one recorded in ai-rulez.lock. Unset means true when
	// ai-rulez.lock exists and false otherwise; false opts out.
	Enforce *bool `yaml:"enforce,omitempty" json:"enforce,omitempty" toml:"enforce,omitempty"`
	// IncludeOutputs also pins digests of the generated files. Default true.
	IncludeOutputs *bool `yaml:"include_outputs,omitempty" json:"include_outputs,omitempty" toml:"include_outputs,omitempty"` //nolint:tagliatelle
	// Scope is "all" (default) or "skills".
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty" toml:"scope,omitempty"`
}

// LockEnforced reports whether the lock is enforced: [lock] enforce when set,
// otherwise whether ai-rulez.lock exists in the configuration directory.
func (c *Config) LockEnforced() bool {
	if c == nil {
		return false
	}
	if c.Lock != nil && c.Lock.Enforce != nil {
		return *c.Lock.Enforce
	}
	if c.ConfigDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(c.ConfigDir, lockfile.FileName))
	return err == nil
}

// LockEnforceOptedOut reports an explicit `enforce = false`. `lock` records the
// pins enforcement needs (served skills) unless it is set, because the lock it
// writes turns enforcement on.
func (c *Config) LockEnforceOptedOut() bool {
	return c != nil && c.Lock != nil && c.Lock.Enforce != nil && !*c.Lock.Enforce
}

// LockIncludeOutputs reports whether generated outputs are pinned: true unless
// include_outputs is false or the scope is "skills".
func (c *Config) LockIncludeOutputs() bool {
	if c.LockScope() != LockScopeAll {
		return false
	}
	return c == nil || c.Lock == nil || c.Lock.IncludeOutputs == nil || *c.Lock.IncludeOutputs
}

// LockScope returns the configured scope, "all" by default.
func (c *Config) LockScope() string {
	if c == nil || c.Lock == nil || c.Lock.Scope == "" {
		return LockScopeAll
	}
	return c.Lock.Scope
}

func (c *Config) validateLock() error {
	switch c.LockScope() {
	case LockScopeAll, LockScopeSkills:
		return nil
	}
	return oops.With("field", "lock.scope").Hint("Use \"all\" or \"skills\"").
		Errorf("invalid lock scope %q", c.Lock.Scope)
}
