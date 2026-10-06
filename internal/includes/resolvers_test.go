package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// loadWithResolvers loads a configuration the way the commands do, with the
// include and installed-skill resolvers wired in.
func loadWithResolvers(ctx context.Context, baseDir string, opts ...config.LoadOption) (*config.Config, error) {
	return config.LoadConfig(ctx, baseDir, append([]config.LoadOption{config.WithResolvers(Resolvers(""))}, opts...)...)
}
