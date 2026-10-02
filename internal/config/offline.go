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
