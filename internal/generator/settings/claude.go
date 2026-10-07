package settings

import (
	"encoding/json"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// Keys of .claude/settings.json that [permissions] and
// [claude.settings.managed] own.
const (
	keyPermissions    = "permissions"
	keyEnv            = "env"
	keySkillOverrides = "skillOverrides"
)

// ClaudeKeys returns the owned keys of .claude/settings.json declared by
// [[hooks]], [permissions] and [claude.settings.managed]: hooks and permission
// rules element by element, env and skillOverrides entry by entry. docPath is the
// document on disk (empty for a fresh one).
func ClaudeKeys(cfg *config.Config, docPath string) ([]jsonmerge.OwnedKey, error) {
	if cfg == nil || inScopeRun(cfg) {
		return nil, nil
	}
	keys, err := HookKeys(cfg, config.HarnessClaude, docPath)
	if err != nil {
		return nil, err
	}
	keys = append(keys, permissionKeys(cfg, docPath)...)
	if managed := cfg.ManagedClaudeSettings(); !managed.IsEmpty() {
		for _, entries := range []struct {
			key    string
			values map[string]string
		}{{keyEnv, managed.Env}, {keySkillOverrides, managed.SkillOverrides}} {
			if owned := ownedEntries(cfg, docPath, entries.key, entries.values); len(owned) > 0 {
				keys = append(keys, jsonmerge.OwnedKey{Name: entries.key, Value: owned, Members: true})
			}
		}
	}
	return keys, nil
}

func permissionKeys(cfg *config.Config, docPath string) []jsonmerge.OwnedKey {
	if cfg.Permissions.IsEmpty() {
		return nil
	}
	lists := []struct {
		name  string
		rules []string
	}{{string(ActionAllow), cfg.Permissions.Allow}, {string(ActionAsk), cfg.Permissions.Ask}, {string(ActionDeny), cfg.Permissions.Deny}}
	var keys []jsonmerge.OwnedKey
	for _, list := range lists {
		if len(list.rules) == 0 {
			continue
		}
		var ours []json.RawMessage
		seen := map[string]bool{}
		for _, rule := range list.rules {
			if seen[rule] {
				continue
			}
			seen[rule] = true
			raw, err := json.Marshal(rule)
			if err != nil {
				continue
			}
			ours = append(ours, raw)
		}
		keys = append(keys, arrayKey(cfg, docPath, []string{keyPermissions, list.name}, ours))
	}
	return keys
}

// ownedEntries returns the entries of a managed object (env, skillOverrides) that
// ai-rulez owns. An entry the document already holds with the configured value,
// and that no earlier run wrote, is the consumer's: it is left out, so clean does
// not take back a setting the consumer had before declaring it here.
func ownedEntries(cfg *config.Config, docPath, key string, configured map[string]string) map[string]any {
	var existing map[string]json.RawMessage
	if raw := readPath(cfg, docPath, []string{key}); raw != nil {
		if json.Unmarshal(raw, &existing) != nil {
			existing = nil
		}
	}
	previous := cfg.Run.PreviousClaims(documentRel(cfg, docPath))

	names := make([]string, 0, len(configured))
	for name := range configured {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make(map[string]any, len(configured))
	for _, name := range names {
		if had, ok := existing[name]; ok && !claimedEarlier(previous, key, name) {
			want, err := json.Marshal(configured[name])
			if err == nil && equalJSON(had, want) {
				continue
			}
		}
		out[name] = configured[name]
	}
	return out
}

// claimedEarlier reports whether a previous run recorded owning key.name.
func claimedEarlier(claims []jsonmerge.Claim, key, name string) bool {
	for ix := range claims {
		claim := claims[ix]
		if equalPath(claim.Path, []string{key, name}) || equalPath(claim.Path, []string{key}) {
			return true
		}
	}
	return false
}
