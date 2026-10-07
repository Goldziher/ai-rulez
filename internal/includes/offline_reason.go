package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// offlineReason names why a source is read from the cache only: the load's
// policy (--offline, --frozen), or a command that never fetches includes by
// design (sbom, list).
func offlineReason(ctx context.Context) string {
	if config.NoFetchRequested(ctx) {
		return "--offline specified"
	}
	return "this command does not fetch includes"
}
