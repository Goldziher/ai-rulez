package settings

import (
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
)

// OwnedHooksKeys is the content of the hooks file of a harness whose hooks live in
// a file of their own (OwnedHooksDocument) as keys: one array per native event.
// Presets that write the same file combine their keys (see RenderOwnedHooks)
// instead of being reported as writing different content. ok is false when no
// hook applies.
func OwnedHooksKeys(cfg *config.Config, harness string) (keys []jsonmerge.OwnedKey, ok bool, err error) {
	spec, known := specFor(harness)
	if !known || !spec.ownedFile || cfg == nil || len(cfg.Hooks) == 0 || inScopeRun(cfg) {
		return nil, false, nil
	}
	rendered, err := renderHooks(cfg, spec)
	if err != nil || len(rendered.events) == 0 {
		return nil, false, err
	}
	keys = append(keys, jsonmerge.OwnedKey{Path: []string{keyVersion}, Value: copilotHooksVersion})
	for _, event := range rendered.events {
		keys = append(keys, jsonmerge.OwnedKey{Path: []string{keyHooks, event}, Value: rendered.entries[event]})
	}
	return keys, true, nil
}

// RenderOwnedHooks renders the keys of OwnedHooksKeys as the file body.
func RenderOwnedHooks(keys []jsonmerge.OwnedKey) (string, error) {
	version := any(copilotHooksVersion)
	hooks := map[string][]json.RawMessage{}
	for _, key := range keys {
		switch {
		case len(key.Path) == 1 && key.Path[0] == keyVersion:
			version = key.Value
		case len(key.Path) == 2 && key.Path[0] == keyHooks:
			entries, ok := key.Value.([]json.RawMessage)
			if !ok {
				return "", fmt.Errorf("hook event %s has invalid entries", key.Path[1])
			}
			hooks[key.Path[1]] = entries
		}
	}
	data, err := json.MarshalIndent(map[string]any{keyVersion: version, keyHooks: hooks}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal the owned hooks document: %w", err)
	}
	return string(data) + "\n", nil
}
