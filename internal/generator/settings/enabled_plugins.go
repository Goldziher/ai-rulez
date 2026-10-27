package settings

import (
	"encoding/json"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// keyOpencodePlugin is the array of npm packages OpenCode loads as plugins.
const keyOpencodePlugin = "plugin"

// OpencodePluginKeys returns the owned key of opencode.json that
// [[enabled_plugins]] writes: the npm packages of the entries that set
// `opencode`, element by element, so the consumer's own plugins and any other
// key survive. docPath is the document on disk.
func OpencodePluginKeys(cfg *config.Config, docPath string) []jsonmerge.OwnedKey {
	if cfg == nil || !cfg.RendersEnabledPlugins() {
		return nil
	}
	var ours []json.RawMessage
	seen := map[string]bool{}
	for i := range cfg.EnabledPlugins {
		pkg := cfg.EnabledPlugins[i].OpenCode
		if pkg == "" || seen[pkg] {
			continue
		}
		seen[pkg] = true
		raw, err := json.Marshal(pkg)
		if err != nil {
			continue
		}
		ours = append(ours, raw)
	}
	if len(ours) == 0 {
		return nil
	}
	return []jsonmerge.OwnedKey{arrayKey(cfg, docPath, []string{keyOpencodePlugin}, ours)}
}
