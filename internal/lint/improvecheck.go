package lint

import (
	"path/filepath"
	"strings"
)

// checkImproveConfig reports what a repository [improve] table asks for that `improve run` will not honour
// without --trust-repo-optimizer (AR9J6): an optimizer command, forwarded environment variables, or a gate
// looser than the defaults. The values themselves are checked by the config schema.
func (r *runner) checkImproveConfig() {
	t := r.cfg.Improve
	if t == nil || r.cfg.ConfigDir == "" {
		return
	}
	file := filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)
	var keys []string
	if strings.TrimSpace(t.Optimizer) != "" {
		keys = append(keys, "optimizer")
	}
	if len(t.EnvPass) > 0 {
		keys = append(keys, "env_pass")
	}
	keys = append(keys, t.LooserGateKeys()...)
	if len(keys) > 0 {
		r.add(CodeImproveRepoOptimizerIgnored, file, 1,
			"[improve] %s set in the repository config: ignored by `improve run` without --trust-repo-optimizer (an optimizer and its environment choose what runs on your machine, and a repository may tighten the gate, not weaken it)", strings.Join(keys, ", "))
	}
}
