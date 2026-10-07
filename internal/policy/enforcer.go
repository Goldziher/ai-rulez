package policy

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Enforcer implements config.PolicyEnforcer. It discovers the policy lazily on
// first use and again whenever the anchors it depends on change, so a process
// that never loads a configuration never touches the filesystem. The anchors
// include the content of the local policy files (size, mtime and SHA-256 of each,
// and whether it exists), so a long-running process such as the skills server
// sees an edited policy file on its next load.
type Enforcer struct {
	opts func() DiscoverOptions

	mu       sync.Mutex
	have     bool
	key      string
	files    []string
	stamp    string
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
	if e.have && e.key == key && fileStamp(e.files) == e.stamp {
		return e.resolved, e.err
	}
	// The stamp is taken before the read, so an edit made while it runs is seen
	// on the next load rather than lost.
	e.files = localFiles(o, nil)
	e.stamp = fileStamp(e.files)
	if !ValidMode(o.Mode) {
		e.have, e.key, e.resolved = true, key, nil
		e.err = &ParseError{Path: "--policy-mode", Msg: fmt.Sprintf("%q is not a policy mode (use enforce or warn)", o.Mode)}
		return nil, e.err
	}
	layers, err := Discover(o)
	e.have, e.key, e.err, e.resolved = true, key, err, nil
	if files := localFiles(o, layers); !slices.Equal(files, e.files) {
		// extends pulled in more local files: stamp them as read
		e.files, e.stamp = files, fileStamp(files)
	}
	if err == nil {
		e.resolved = Resolve(layers)
		if e.resolved != nil && o.Mode == ModeWarn {
			e.resolved.Warn = true
			o.logger().Warn("policy mode warn: violations of the organization policy are reported but do not fail this run; the policy values are still enforced")
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
	for i := range memo.layers {
		l := &memo.layers[i]
		if !hasLayer(layers, l.key) {
			layers = append(layers, *l)
		}
	}
	res := Resolve(layers)
	res.Warn = warn || o.Mode == ModeWarn
	return res, nil
}

func hasLayer(layers []Layer, key string) bool {
	for i := range layers {
		l := &layers[i]
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
	r, err := e.LoadFor(cfg.PolicyProjectDir())
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return r.Apply(cfg).Outcome, nil
}

// EnforceContent implements config.ContentEnforcer.
func (e *Enforcer) EnforceContent(_ context.Context, cfg *config.Config) []config.PolicyViolation {
	r, err := e.LoadFor(cfg.PolicyProjectDir())
	if err != nil || r == nil {
		return nil // Enforce already failed the load on a policy that cannot be used
	}
	return r.ApplyContent(cfg)
}

// Locks implements config.PolicyEnforcer; an unusable policy locks everything.
func (e *Enforcer) Locks(feature string) bool {
	r, err := e.Load()
	return locked(r, err, feature)
}

// LocksIn implements config.DirLocker: Locks for the project at dir, whose
// organization policy (--discover-org) is part of the answer. Without a
// directory it is Locks. An organization policy that cannot be read locks
// everything, like any unusable policy.
func (e *Enforcer) LocksIn(feature, dir string) bool {
	if dir == "" {
		return e.Locks(feature)
	}
	r, err := e.LoadFor(dir)
	return locked(r, err, feature)
}

func locked(r *Resolved, err error, feature string) bool {
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
