package commands

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
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
		cfg, err := config.LoadConfigFromFile(context.Background(), path)
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

func runRecursivePluginVerify() {
	if !verifyPlugin {
		fmtError(oops.Errorf("verify --recursive requires --plugin"))
		os.Exit(1)
	}
	paths, err := selectRecursivePluginConfigs(findConfigFilesRecursively())
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if len(paths) == 0 {
		if verifyIfConfigured {
			logger.Info("Skipping plugin verification: no plugin authoring configuration")
			return
		}
		fmtError(oops.Errorf("no plugin authoring configuration found"))
		os.Exit(1)
	}
	for _, path := range paths {
		cfg, err := config.LoadConfigFromFile(context.Background(), path)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := cfg.Validate(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := generator.NewGenerator(cfg).VerifyPlugin(profile); err != nil {
			fmtError(oops.With("config", path).Wrapf(err, "verify plugin outputs"))
			os.Exit(1)
		}
	}
	logger.Success("Generated plugin artifacts are valid", "configs", len(paths))
}
