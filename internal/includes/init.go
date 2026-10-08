package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// Resolvers returns the include and installed-skill resolvers a load is given
// (config.WithResolvers). gitToken authenticates the fetches that need one and
// goes only to the hosts the fetch code allowlists. scan is the security scan
// an OKF include runs over its text before conversion; the caller supplies it
// (lint cannot be imported here — lint imports this package).
func Resolvers(gitToken string, scan okfbridge.Scanner) config.Resolvers {
	return config.Resolvers{
		Includes: func(ctx context.Context, cfg *config.Config) (*config.ContentTree, error) {
			return NewResolver(cfg.BaseDir, gitToken).WithOKFScan(scan).ResolveIncludes(ctx, cfg)
		},
		Skills: func(ctx context.Context, cfg *config.Config) ([]config.ContentFile, error) {
			return ResolveInstalledSkills(ctx, cfg, gitToken)
		},
	}
}
