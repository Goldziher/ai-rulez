package config

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// LockMode says how a load resolves remote sources against ai-rulez.lock.
type LockMode int

const (
	// LockAuto uses the lock when one covers a source and fetches an uncovered
	// source as before. Without a lock file nothing changes.
	LockAuto LockMode = iota
	// LockRequire (generate --locked) fails when the lock is missing or does not
	// cover a configured remote source.
	LockRequire
	// LockFrozen (generate --frozen) is LockRequire that never touches the
	// network; set Offline with it.
	LockFrozen
	// LockRefresh (ai-rulez lock) re-resolves the sources Refresh selects from
	// the remote, ignoring their pins, so new pins can be recorded.
	LockRefresh
)

// LockPolicy is how one load treats ai-rulez.lock and the network
// (WithLockPolicy). It belongs to the load, not to the process: two loads in one
// process (an MCP server, an embedding service) each get their own. The zero
// value fetches what the lock does not cover and only reports an enforced lock;
// a load that writes outputs sets RequireWhenEnforced.
type LockPolicy struct {
	Mode LockMode
	// Refresh selects what LockRefresh re-resolves; nil selects everything.
	Refresh func(kind, name string) bool
	// Offline resolves remote sources from the local cache only (--no-fetch,
	// --frozen): the load never touches the network.
	Offline bool
	// RequireWhenEnforced makes an enforced lock ([lock] enforce, on by default
	// when ai-rulez.lock exists) behave like LockRequire: a remote source the
	// lock does not cover is a violation instead of an unpinned fetch. Every
	// writer of outputs sets it (generate, the MCP generate_outputs tool, the
	// pkg/airulez facade); commands that only read or report leave it off.
	RequireWhenEnforced bool
	// VersionPolicy is how a refresh moves the sources that ask for a version
	// range (`ai-rulez update`, and the release age gate of `lock`).
	VersionPolicy
}

// VersionPolicy is how a refresh resolves the sources that ask for a version
// range. `ai-rulez update` sets Advance, AllowDowngrade and AcceptMovedTag;
// `lock` and `update` set ReleaseGate. The zero value keeps a pin that still
// satisfies its constraint and applies no minimum release age.
type VersionPolicy struct {
	// Advance selects the sources a refresh moves to the newest allowed tag.
	// nil moves none: a pin that still satisfies its constraint is kept.
	Advance func(kind, name string) bool
	// AllowDowngrade lets Advance select a tag with lower precedence than the pin.
	AllowDowngrade bool
	// AcceptMovedTag re-pins a tag that now points to another commit instead of
	// failing with AR732.
	AcceptMovedTag bool
	// ReleaseGate returns the minimum release age gate for a source, or nil for
	// none. It is only consulted when a range is resolved to a new tag (a pin
	// that still satisfies its constraint is kept without a lookup, its age
	// having been decided when it was pinned).
	ReleaseGate func(w lockfile.Want) *tagresolve.AgeGate
}

// Advancing reports whether the refresh moves the source to its newest allowed tag.
func (v VersionPolicy) Advancing(kind, name string) bool {
	return v.Advance != nil && v.Advance(kind, name)
}

// WithLockPolicy loads the configuration under p (see LockPolicy). The loaded
// Config keeps it (Config.LockPolicy), so reloads of the same project and the
// resolvers see the same policy.
func WithLockPolicy(p LockPolicy) LoadOption {
	return func(o *loadOptions) { o.lockPolicy = p }
}

// Strict reports whether a source that cannot be resolved must fail the load
// instead of being skipped with a warning: --locked, --frozen, or an enforced
// lock under a policy that requires it.
func (p LockPolicy) Strict(enforced bool) bool {
	return p.Mode == LockRequire || p.Mode == LockFrozen || (p.RequireWhenEnforced && enforced)
}

// Refreshing reports whether the source is being re-resolved by `ai-rulez lock`.
func (p LockPolicy) Refreshing(kind, name string) bool {
	return p.Mode == LockRefresh && (p.Refresh == nil || p.Refresh(kind, name))
}

type noFetchKey struct{}

// WithNoFetch marks ctx offline because the load's policy asked for it
// (LockPolicy.Offline), which error messages name differently from a command
// that never fetches by design (WithOfflineIncludes).
func WithNoFetch(ctx context.Context) context.Context {
	return context.WithValue(WithOfflineIncludes(ctx), noFetchKey{}, true)
}

// NoFetchRequested reports whether ctx is offline because the load's
// LockPolicy asked for it (--no-fetch, --frozen), rather than by design of the
// command.
func NoFetchRequested(ctx context.Context) bool {
	v, _ := ctx.Value(noFetchKey{}).(bool) //nolint:errcheck // absent key reads as false
	return v
}
