package commands

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
)

// pinScans writes the [[scan]] records of the new lock (see lockrun.PinScans)
// from the repository index of strict validation.
func pinScans(cfg *config.Config, current, next *lockfile.File, full bool) {
	lockrun.PinScans(cfg, current, next, full, strictTreeCache.Load, workingDir())
}

// printScans lists the scan records of a lock after `lock` wrote it.
func printScans(next *lockfile.File) {
	for _, s := range next.Scan {
		fmt.Printf("recorded scan %s %s findings=%d %s\n", s.Scanner, s.Tree, s.Findings, s.Result)
	}
}
