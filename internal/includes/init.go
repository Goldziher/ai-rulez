package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	// Register the ResolveIncludes callback with the config package
	// This avoids circular imports between config and includes packages
	config.SetResolveIncludesCallback(func(ctx context.Context, cfg *config.ConfigV3) (*config.ContentTreeV3, error) {
		resolver := NewResolver(cfg.BaseDir)
		return resolver.ResolveIncludes(ctx, cfg)
	})
}
