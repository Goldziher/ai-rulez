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
	org      map[string]orgResult
}

// NewEnforcer returns an enforcer that asks opts for the anchors on every use.
func NewEnforcer(opts func() DiscoverOptions) *Enforcer { return &Enforcer{opts: opts} }

// Load returns the policy in force from the anchors that do not depend on the
// repository: --policy, AI_RULEZ_POLICY and the managed path. nil when none applies.
func (e *Enforcer) Load() (*Resolved, error) {
	o := e.opts()
	key := o.cacheKey()
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

// LoadFor is Load plus, when organization discovery is on (--discover-org, or
// [policy] discover = "org" in the user config), the policy of the GitHub owner of
// the repository at dir, as the weakest layer. The owner's policy is fetched once
// per owner.
func (e *Enforcer) LoadFor(dir string) (*Resolved, error) {
	base, err := e.Load()
	if err != nil {
		return nil, err
	}
	o := e.opts()
	o.ProjectDir = dir
	wanted, err := OrgWanted(o)
	if err != nil || !wanted || dir == "" {
		return base, err
	}
	ref, ok, err := OrgRef(o)
	if err != nil || !ok {
		return base, err
	}
	e.mu.Lock()
	memo, seen := e.org[ref.Location]
	e.mu.Unlock()
	if !seen {
		layers, lerr := LoadOrg(o, ref)
		memo = orgResult{layers: layers, err: lerr}
		e.mu.Lock()
		if e.org == nil {
			e.org = map[string]orgResult{}
		}
		e.org[ref.Location] = memo
		e.mu.Unlock()
	}
	if memo.err != nil {
		return nil, memo.err
	}
	if len(memo.layers) == 0 {
		return base, nil
	}
	var layers []Layer
	warn := false
	if base != nil {
		layers, warn = append(layers, base.Layers...), base.Warn
	}
	for _, l := range memo.layers {
		if !hasLayer(layers, l.key) {
			layers = append(layers, l)
		}
	}
	res := Resolve(layers)
	res.Warn = warn || o.Mode == ModeWarn
	return res, nil
}

func hasLayer(layers []Layer, key string) bool {
	for _, l := range layers {
		if l.key != "" && l.key == key {
			return true
		}
	}
	return false
}

// orgResult is a memoized organization policy lookup.
type orgResult struct {
	layers []Layer
	err    error
}

// Enforce implements config.PolicyEnforcer.
func (e *Enforcer) Enforce(_ context.Context, cfg *config.Config) (*config.PolicyOutcome, error) {
	r, err := e.LoadFor(cfg.BaseDir)
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
