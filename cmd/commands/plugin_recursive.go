package commands

import (
	"errors"
	"path/filepath"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// selectRecursivePluginConfigs keeps plugin producers and marketplace roots.
// Member configs owned by a marketplace root are excluded because the root
// generator already renders and verifies them as one atomic unit.
func selectRecursivePluginConfigs(paths []string) ([]string, error) {
	type candidate struct {
		path string
		base string
		cfg  *config.Config
	}
	candidates := make([]candidate, 0, len(paths))
	memberRoots := make([]string, 0)
	for _, path := range paths {
		cfg, err := loadProjectFile(cmdContext(), path, config.WithoutLocal())
		if err != nil {
			return nil, oops.With("config", path).Wrapf(err, "load recursive plugin configuration")
		}
		if !cfg.HasPluginAuthoring() {
			continue
		}
		base, err := filepath.Abs(cfg.BaseDir)
		if err != nil {
			return nil, oops.With("config", path).Wrapf(err, "resolve plugin project directory")
		}
		candidates = append(candidates, candidate{path: path, base: base, cfg: cfg})
		if cfg.Marketplace != nil {
			for _, member := range cfg.Marketplace.Members {
				memberRoot, absErr := filepath.Abs(filepath.Join(cfg.BaseDir, member))
				if absErr == nil {
					memberRoots = append(memberRoots, memberRoot)
				}
			}
		}
	}

	selected := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		isMember := false
		for _, memberRoot := range memberRoots {
			if candidate.base == memberRoot {
				isMember = true
				break
			}
		}
		if !isMember {
			selected = append(selected, candidate.path)
		}
	}
	sort.Strings(selected)
	return selected, nil
}

func runRecursivePluginVerify() error {
	if !verifyPlugin {
		return fail(oops.Errorf("verify --recursive requires --plugin"))
	}
	paths, err := selectRecursivePluginConfigs(findConfigFilesRecursively())
	if err != nil {
		return fail(err)
	}
	if len(paths) == 0 {
		if verifyIfConfigured {
			logger.Info("Skipping plugin verification: no plugin authoring configuration")
			return nil
		}
		return fail(oops.Errorf("no plugin authoring configuration found"))
	}
	for _, path := range paths {
		cfg, err := loadProjectFile(cmdContext(), path, config.WithoutLocal())
		if err != nil {
			return fail(err)
		}
		if err := cfg.Validate(); err != nil {
			return fail(err)
		}
		if err := generator.NewGenerator(cfg).VerifyPlugin(profile); err != nil {
			if verifyIfGenerated && errors.Is(err, generator.ErrPluginNotGenerated) {
				logger.Info("Skipping plugin verification: the plugin bundle has not been generated", "config", path)
				continue
			}
			return failWithCode(pluginVerifyExitCode(err), oops.With("config", path).Wrapf(err, "verify plugin outputs"))
		}
	}
	logger.Success("Generated plugin artifacts are valid", "configs", len(paths))
	return nil
}
