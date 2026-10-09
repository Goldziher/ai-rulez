package lint

import (
	"regexp"
	"sync"
)

var pluginWordRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)\bplugins?\b`) })

// pluginProvided reports whether a skill, command or agent that no local file
// defines may come from a plugin: the configuration installs plugins or
// marketplaces, or the file itself talks about a plugin. ai-rulez cannot read
// what an installed plugin ships, so the reference is unverifiable, not wrong.
func (r *runner) pluginProvided(it *item) bool {
	if r.cfg != nil && (len(r.cfg.Plugins) > 0 || len(r.cfg.Marketplaces) > 0) {
		return true
	}
	return pluginWordRe().MatchString(it.cf.Content)
}
