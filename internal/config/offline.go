package config

import "context"

type offlineIncludesKey struct{}

// WithOfflineIncludes marks ctx so include and skill sources resolve from their
// cache only and never touch the network. Validation loads use it.
func WithOfflineIncludes(ctx context.Context) context.Context {
	return context.WithValue(ctx, offlineIncludesKey{}, true)
}

// OfflineIncludes reports whether ctx asks for cache-only include resolution.
func OfflineIncludes(ctx context.Context) bool {
	v, _ := ctx.Value(offlineIncludesKey{}).(bool) //nolint:errcheck // absent key reads as false
	return v
}

type tolerateUnresolvedKey struct{}

// WithUnresolvedIncludesTolerated marks ctx so an include that cannot be resolved
// is a warning and the load goes on without it. Commands that inventory or
// compare sources (lock, sbom) use it: they report the missing include
// themselves and render nothing.
func WithUnresolvedIncludesTolerated(ctx context.Context) context.Context {
	return context.WithValue(ctx, tolerateUnresolvedKey{}, true)
}

// UnresolvedIncludesTolerated reports whether ctx tolerates unresolved includes.
func UnresolvedIncludesTolerated(ctx context.Context) bool {
	v, _ := ctx.Value(tolerateUnresolvedKey{}).(bool) //nolint:errcheck // absent key reads as false
	return v
}
