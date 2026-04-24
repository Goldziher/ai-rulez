package includes

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/viper"
)

func init() {
	// Register the ResolveIncludes callback with the config package
	// This avoids circular imports between config and includes packages
	config.SetResolveIncludesCallback(func(ctx context.Context, cfg *config.Config) (*config.ContentTree, error) {
		gitToken := viper.GetString("git_token")
		resolver := NewResolver(cfg.BaseDir, gitToken)
		return resolver.ResolveIncludes(ctx, cfg)
	})

	// Register the ResolveInstalledSkills callback
	config.SetResolveInstalledSkillsCallback(func(ctx context.Context, cfg *config.Config) ([]config.ContentFile, error) {
		gitToken := viper.GetString("git_token")
		return ResolveInstalledSkills(ctx, cfg, gitToken)
	})
}
