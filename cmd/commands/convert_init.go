package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// `init --from` is `convert --write` with the sources `init` always took: a list
// of importer names and project paths (.claude, CLAUDE.md). It runs the same code,
// so the scan, the validation and the lossiness report are the same too.

func initConvertOptions(workingDir, configDir string, write bool) importer.ConvertOptions {
	from, paths := importer.SplitSources(fromFlag)
	return importer.ConvertOptions{Source: workingDir, Into: configDir, From: from, NativePaths: paths, Write: write}
}

// previewInitImport converts into a scratch directory and writes nothing, so a
// source that cannot be imported (nothing found, a blocked scan) is reported
// before the existing configuration directory is removed.
func previewInitImport(ctx context.Context, workingDir string) error {
	scratch, err := os.MkdirTemp("", "ai-rulez-init-*")
	if err != nil {
		return oops.Wrapf(err, "create scratch directory")
	}
	defer removeScratch(scratch)

	opts := initConvertOptions(workingDir, filepath.Join(scratch, importer.DefaultConfigDir), false)
	report, err := importer.Convert(ctx, opts)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if report.Security.Blocked || report.Validation.Errors > 0 {
		if werr := report.WriteText(os.Stderr); werr != nil {
			return oops.Wrapf(werr, "print the convert report")
		}
		return oops.Hint("Remove the flagged text from the source and rerun, or use `ai-rulez convert --allow-findings CODE`").
			Errorf("the imported content failed the security scan or validation; nothing was written")
	}
	return nil
}

// runInitImport writes the converted tree and prints the report.
func runInitImport(ctx context.Context, workingDir, configDir string) error {
	report, err := importer.Convert(ctx, initConvertOptions(workingDir, configDir, true))
	if report != nil {
		if werr := report.WriteText(os.Stdout); werr != nil && err == nil {
			err = oops.Wrapf(werr, "print the convert report")
		}
	}
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if !report.Written {
		return oops.Errorf("the imported content failed the security scan or validation; nothing was written")
	}
	displayImportSuccessMessage(fromFlag, configDir)
	return nil
}

func displayImportSuccessMessage(sources, configDir string) {
	logger.Info("✅ Successfully imported content to " + configDir + "/")
	logger.Info(fmt.Sprintf("   Sources: %s", sources))
	logger.Info("\nNext steps:")
	logger.Info("  1. Review the report above and the imported content in " + configDir + "/")
	logger.Info("  2. Edit " + configDir + "/config.toml to customize presets")
	logger.Info("  3. Run 'ai-rulez generate' to create tool-specific outputs")
	logger.Info("  `ai-rulez convert` offers more: --dry-run, --merge, --fetch, --enable-hooks")
}

// removeScratch deletes a scratch directory; a failure only leaves temporary
// files behind, so it is logged rather than returned.
func removeScratch(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		logger.Debug("could not remove the scratch directory", "path", dir, "error", err)
	}
}
