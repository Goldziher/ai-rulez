package commands

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// catalogDefaultInput is the value --with-eval and --with-usage take when given
// without a path: read the project's own file.
const catalogDefaultInput = "default"

// catalogUsageLogFile is the machine-local usage log the hooks write, relative to
// the configuration directory.
var catalogUsageLogFile = filepath.Join("local", "usage.jsonl")

// addCatalogSignals reads the eval results and usage log the flags name into
// opts. A file that does not exist is not an error: the catalog says so in its
// notes and leaves those fields out. A file that cannot be read or parsed is.
func addCatalogSignals(cfg *config.Config, opts *govview.CatalogOptions) error {
	if catalogWithEval == "" && catalogWithUsage == "" {
		return nil
	}
	cfgDir := cfg.ConfigDir
	if cfgDir == "" {
		cfgDir = configDirName()
	}
	cfgDir, err := filepath.Abs(cfgDir)
	if err != nil {
		return oops.Wrapf(err, "resolve config directory")
	}
	if catalogWithEval != "" {
		opts.WithEval = true
		path := catalogWithEval
		if path == catalogDefaultInput {
			path = evals.DefaultStorePath(cfgDir)
		}
		switch _, statErr := os.Stat(path); {
		case errors.Is(statErr, os.ErrNotExist):
			opts.EvalNote = filepath.Base(path) + " not found"
		case statErr != nil:
			return oops.With("path", path).Wrapf(statErr, "read eval results")
		default:
			store, loadErr := evals.LoadStoreKeyed(path, evals.ExistingUserKey())
			if loadErr != nil {
				return loadErr //nolint:wrapcheck // already contextual
			}
			opts.Eval = store
		}
	}
	if catalogWithUsage != "" {
		opts.WithUsage = true
		path := catalogWithUsage
		if path == catalogDefaultInput {
			path = filepath.Join(cfgDir, catalogUsageLogFile)
		}
		switch _, statErr := os.Stat(path); {
		case errors.Is(statErr, os.ErrNotExist):
			opts.UsageNote = filepath.Base(path) + " not found"
		case statErr != nil:
			return oops.With("path", path).Wrapf(statErr, "read usage log")
		default:
			entries, _, readErr := usage.ReadLog(path)
			if readErr != nil {
				return readErr //nolint:wrapcheck // already contextual
			}
			opts.Usage, opts.UsageLoaded = entries, true
		}
	}
	return nil
}
