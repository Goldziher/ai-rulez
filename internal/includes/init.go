package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Resolvers returns the include and installed-skill resolvers a load is given
// (config.WithResolvers). gitToken authenticates the fetches that need one and
// goes only to the hosts the fetch code allowlists.
func Resolvers(gitToken string) config.Resolvers {
	return config.Resolvers{
		Includes: func(ctx context.Context, cfg *config.Config) (*config.ContentTree, error) {
			return NewResolver(cfg.BaseDir, gitToken).ResolveIncludes(ctx, cfg)
		},
		Skills: func(ctx context.Context, cfg *config.Config) ([]config.ContentFile, error) {
			return ResolveInstalledSkills(ctx, cfg, gitToken)
		},
	}
}
