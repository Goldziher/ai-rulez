package crud

import (
	"context"
	"slices"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// AddMarketplace adds an [[installed_marketplaces]] entry to config.toml. The
// entry is validated like the loader validates it, so the project is never left
// unloadable.
func (op *OperatorImpl) AddMarketplace(_ context.Context, m *config.InstalledMarketplaceConfig) error {
	if m == nil || m.Name == "" || m.Source == "" {
		return oops.Hint("Provide a name and a source").Errorf("marketplace name and source are required")
	}
	if err := rejectCredentialedSource("marketplace", m.Source); err != nil {
		return err
	}
	return op.editSharedConfig(func(cfg *config.Config) error {
		next := append(slices.Clone(cfg.InstalledMarketplaces), *m)
		if err := config.ValidateInstalledMarketplaces(next); err != nil {
			return err //nolint:wrapcheck // already contextual
		}
		cfg.InstalledMarketplaces = next
		return nil
	})
}

// RemoveMarketplace removes a marketplace; one an installed plugin still names is refused.
func (op *OperatorImpl) RemoveMarketplace(_ context.Context, name string) error {
	return op.editSharedConfig(func(cfg *config.Config) error {
		for i := range cfg.InstalledPlugins {
			if cfg.InstalledPlugins[i].Marketplace == name {
				return oops.With("name", name).Hint("Remove the plugin first: ai-rulez plugin remove "+cfg.InstalledPlugins[i].Name).
					Errorf("marketplace %q is used by installed plugin %q", name, cfg.InstalledPlugins[i].Name)
			}
		}
		for i := range cfg.InstalledMarketplaces {
			if cfg.InstalledMarketplaces[i].Name == name {
				cfg.InstalledMarketplaces = slices.Delete(cfg.InstalledMarketplaces, i, i+1)
				return nil
			}
		}
		return oops.With("name", name).Hint("Use 'marketplace list' to see installed marketplaces").Errorf("installed marketplace %q not found", name)
	})
}

// ListMarketplaces returns the configured marketplaces of the shared config.
func (op *OperatorImpl) ListMarketplaces(ctx context.Context) ([]config.InstalledMarketplaceConfig, error) {
	cfg, err := op.load(config.WithUnresolvedIncludesTolerated(ctx), config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return nil, oops.With("base_dir", op.baseDir).Wrapf(err, "load config")
	}
	return cfg.InstalledMarketplaces, nil
}

// InstallPlugin adds an [[installed_plugins]] entry to config.toml.
func (op *OperatorImpl) InstallPlugin(_ context.Context, p *config.InstalledPluginConfig) error {
	if p == nil || p.Name == "" {
		return oops.Hint("Provide a plugin name").Errorf("plugin name is required")
	}
	if p.Source != "" {
		if err := rejectCredentialedSource("plugin", p.Source); err != nil {
			return err
		}
	}
	return op.editSharedConfig(func(cfg *config.Config) error {
		next := append(slices.Clone(cfg.InstalledPlugins), *p)
		if err := config.ValidateInstalledPlugins(next, cfg.InstalledMarketplaces); err != nil {
			return err //nolint:wrapcheck // already contextual
		}
		cfg.InstalledPlugins = next
		return nil
	})
}

// RemovePlugin removes an installed plugin.
func (op *OperatorImpl) RemovePlugin(_ context.Context, name string) error {
	return op.editSharedConfig(func(cfg *config.Config) error {
		for i := range cfg.InstalledPlugins {
			if cfg.InstalledPlugins[i].Name == name {
				cfg.InstalledPlugins = slices.Delete(cfg.InstalledPlugins, i, i+1)
				return nil
			}
		}
		return oops.With("name", name).Hint("Use 'plugin list' to see installed plugins").Errorf("installed plugin %q not found", name)
	})
}

// ListPlugins returns the configured plugins of the shared config.
func (op *OperatorImpl) ListPlugins(ctx context.Context) ([]config.InstalledPluginConfig, error) {
	cfg, err := op.load(config.WithUnresolvedIncludesTolerated(ctx), config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return nil, oops.With("base_dir", op.baseDir).Wrapf(err, "load config")
	}
	return cfg.InstalledPlugins, nil
}
