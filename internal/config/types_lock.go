package config

import (
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
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
	// MinReleaseAge is the default for sources that use a version constraint and
	// set none of their own ("7d"). A tag younger than this is held back by
	// `lock` and `update` (AR733).
	MinReleaseAge string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"` //nolint:tagliatelle
	// MinReleaseAgeSource is where a tag's release time comes from: "auto"
	// (default: the forge, else the first time this machine saw the tag, else the
	// commit date), "forge", "first-seen" or "commit".
	MinReleaseAgeSource string `yaml:"min_release_age_source,omitempty" json:"min_release_age_source,omitempty" toml:"min_release_age_source,omitempty"` //nolint:tagliatelle
	// VerifyTags makes `generate` and `lock --check` ask the remotes whether a
	// tag pinned in ai-rulez.lock moved (AR732) or was deleted (AR735). It needs
	// the network, so it is off by default and the offline checks stay offline.
	VerifyTags bool `yaml:"verify_tags,omitempty" json:"verify_tags,omitempty" toml:"verify_tags,omitempty"` //nolint:tagliatelle
}

// Sources of a tag's release time (see LockConfig.MinReleaseAgeSource).
const (
	AgeSourceAuto      = "auto"
	AgeSourceForge     = "forge"
	AgeSourceFirstSeen = "first-seen"
	AgeSourceCommit    = "commit"
)

// LockVerifyTags reports whether [lock] verify_tags is on.
func (c *Config) LockVerifyTags() bool { return c != nil && c.Lock != nil && c.Lock.VerifyTags }

// LockMinReleaseAge is the global default minimum release age ("" when unset).
func (c *Config) LockMinReleaseAge() string {
	if c == nil || c.Lock == nil {
		return ""
	}
	return strings.TrimSpace(c.Lock.MinReleaseAge)
}

// LockMinReleaseAgeSource is the configured release-time source, "auto" by default.
func (c *Config) LockMinReleaseAgeSource() string {
	if c == nil || c.Lock == nil || c.Lock.MinReleaseAgeSource == "" {
		return AgeSourceAuto
	}
	return c.Lock.MinReleaseAgeSource
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
	return c.View().Exists(filepath.Join(c.ConfigDir, lockfile.FileName))
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
	default:
		return oops.With("field", "lock.scope").Hint("Use \"all\" or \"skills\"").
			Errorf("invalid lock scope %q", c.Lock.Scope)
	}
	if _, err := semver.ParseAge(c.LockMinReleaseAge()); err != nil {
		return oops.With("field", "lock.min_release_age").Wrapf(err, "invalid lock min_release_age")
	}
	switch c.LockMinReleaseAgeSource() {
	case AgeSourceAuto, AgeSourceForge, AgeSourceFirstSeen, AgeSourceCommit:
		return nil
	}
	return oops.With("field", "lock.min_release_age_source").Hint("Use \"auto\", \"forge\", \"first-seen\" or \"commit\"").
		Errorf("invalid lock min_release_age_source %q", c.Lock.MinReleaseAgeSource)
}
