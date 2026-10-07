package lockrun

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// PinScans writes the [[scan]] records of the new lock: for every staged
// egress = false scanner, the result the scanner result cache holds for the
// current content. It starts no program (run `scan --external` first), so a
// scanner that was not run on this content has no record. A refresh limited to
// some sources keeps the records of the lock being replaced. tree indexes the
// repository; cwd is the directory paths are shown relative to.
func PinScans(cfg *config.Config, current, next *lockfile.File, full bool, tree func(base string) (*lint.Tree, error), cwd string) {
	if !full {
		if current != nil {
			next.Scan = current.Scan
		}
		return
	}
	if cfg.Lint == nil || (len(cfg.Lint.External) == 0 && cfg.Lint.ScannerPolicy == nil) {
		return
	}
	log := cfg.Log()
	t, err := tree(cfg.BaseDir)
	if err != nil {
		log.Warn("Scan results are not recorded: the repository files could not be indexed", "error", err)
		return
	}
	records, err := lint.ScanRecords(cfg, t, lint.Options{Cwd: cwd, Scanner: lint.ScannerOptions{Cache: lint.UserScanCache(log, cfg.ConfigDir)}})
	if err != nil {
		log.Warn("Scan results are not recorded", "error", err)
		return
	}
	for _, rec := range records {
		if !rec.Cached {
			log.Info("No cached scan result for the current content; run `ai-rulez scan --external` before `lock` to record it", "scanner", rec.Scanner)
			continue
		}
		result := lockfile.ScanPass
		if !rec.Pass {
			result = lockfile.ScanFail
		}
		next.SetScan(lockfile.Scan{Scanner: rec.Scanner, Version: rec.Version, Tree: rec.Tree, Findings: rec.Findings, MaxSeverity: rec.MaxSeverity, Result: result})
	}
}
