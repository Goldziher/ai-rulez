package config

import "github.com/samber/oops"

// Lock scopes select which authored items ai-rulez.lock pins.
const (
	// LockScopeAll pins every authored item and, with include_outputs, outputs.
	LockScopeAll = "all"
	// LockScopeSkills pins skills (and remote includes/installed skills) only.
	LockScopeSkills = "skills"
)

// LockConfig is the [lock] table: how strictly the committed ai-rulez.lock is
// enforced. With no table the lock still pins and `lock --check` still runs; the
// table only controls whether strict validation reports drift.
type LockConfig struct {
	// Enforce makes `validate --strict` report content drift (AR981, AR982) and
	// makes `lock --check` fail on a lock without content pins.
	Enforce bool `yaml:"enforce,omitempty" json:"enforce,omitempty" toml:"enforce,omitempty"`
	// IncludeOutputs also pins digests of the generated files. Default true.
	IncludeOutputs *bool `yaml:"include_outputs,omitempty" json:"include_outputs,omitempty" toml:"include_outputs,omitempty"` //nolint:tagliatelle
	// Scope is "all" (default) or "skills".
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty" toml:"scope,omitempty"`
}

// LockEnforced reports whether [lock] enforce is set.
func (c *Config) LockEnforced() bool { return c != nil && c.Lock != nil && c.Lock.Enforce }

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
