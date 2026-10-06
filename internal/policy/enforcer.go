package policy

import (
	"context"
	"fmt"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Enforcer implements config.PolicyEnforcer. It discovers the policy lazily on
// first use and again whenever the anchors it depends on change, so a process
// that never loads a configuration never touches the filesystem.
type Enforcer struct {
	opts func() DiscoverOptions

	mu       sync.Mutex
	have     bool
	key      string
	resolved *Resolved
	err      error
}

// NewEnforcer returns an enforcer that asks opts for the anchors on every use.
func NewEnforcer(opts func() DiscoverOptions) *Enforcer { return &Enforcer{opts: opts} }

// Load returns the policy in force, nil when none applies.
func (e *Enforcer) Load() (*Resolved, error) {
	o := e.opts()
	key := o.Flag + "\x00" + o.envPolicy() + "\x00" + o.GOOS + "\x00" + o.Mode
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.have && e.key == key {
		return e.resolved, e.err
	}
	if !ValidMode(o.Mode) {
		e.have, e.key, e.resolved = true, key, nil
		e.err = &ParseError{Path: "--policy-mode", Msg: fmt.Sprintf("%q is not a policy mode (use enforce or warn)", o.Mode)}
		return nil, e.err
	}
	layers, err := Discover(o)
	e.have, e.key, e.err, e.resolved = true, key, err, nil
	if err == nil {
		e.resolved = Resolve(layers)
		if e.resolved != nil && o.Mode == ModeWarn {
			e.resolved.Warn = true
			logger.Warn("policy mode warn: violations of the organization policy are reported but do not fail this run; the policy values are still enforced")
		}
	}
	return e.resolved, e.err
}

// Enforce implements config.PolicyEnforcer.
func (e *Enforcer) Enforce(_ context.Context, cfg *config.Config) (*config.PolicyOutcome, error) {
	r, err := e.Load()
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return r.Apply(cfg).Outcome, nil
}

// Locks implements config.PolicyEnforcer; an unusable policy locks everything.
func (e *Enforcer) Locks(feature string) bool {
	r, err := e.Load()
	if err != nil {
		return true
	}
	if r == nil {
		return false
	}
	switch feature {
	case "telemetry":
		return r.Policy.Telemetry.Disabled
	case "llm":
		return r.Policy.LLM.Disabled
	}
	return false
}
