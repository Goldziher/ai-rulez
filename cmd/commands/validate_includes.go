package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// checkLocalIncludes reports every include whose local path is missing or is not
// a directory. A silently skipped include would leave its rules out of every
// generated file. An include with a local_override (which the machine-local
// overlay sets for development) is not checked: it is expected to be absent on
// other machines.
func checkLocalIncludes(cfg *config.Config) error {
	var problems []string
	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
		if inc.Source == "" || inc.LocalOverride != "" || lockfile.IsGitSource(inc.Source) {
			continue
		}
		src := inc.Source
		if !filepath.IsAbs(src) {
			src = filepath.Join(cfg.BaseDir, src)
		}
		info, err := os.Stat(filepath.Clean(src))
		switch {
		case err != nil:
			problems = append(problems, "include "+inc.Name+": path "+inc.Source+" not found")
		case !info.IsDir():
			problems = append(problems, "include "+inc.Name+": path "+inc.Source+" is not a directory")
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return oops.With("config", cfg.ConfigDir).Errorf("local includes cannot be read:\n  %s", strings.Join(problems, "\n  "))
}
