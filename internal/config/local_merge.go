package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Errors and warnings in this file name list keys, entry positions and key
// names only. They never include entry values: local files may hold secrets.

const (
	docKeySchema    = "schema"
	docKeySchemaDol = "$schema"
	docKeyVersion   = "version"
	docKeyBuiltins  = "builtins"
	docKeyPresets   = "presets"
	docKeyName      = "name"
	docKeyPath      = "path"
	docKeyRemove    = "remove"
	presetDropMark  = "!"
	docKeyProfiles  = "profiles"
	docKeyDefault   = "default"
	docKeyDefaults  = "defaults"
	docKeyHeader    = "header"
)

// namedListKeys are the top-level list keys merged entry-by-entry. The value
// reports whether an entry without a name is identified by its path (scopes).
var namedListKeys = map[string]bool{
	"mcp_servers":      false,
	"plugins":          false,
	"includes":         false,
	"installed_skills": false,
	"marketplaces":     false,
	"scopes":           true,
	"roles":            false,
	"verifiers":        false,
}

// mapValuedKeys are top-level tables merged per key.
var mapValuedKeys = map[string]bool{
	docKeyProfiles: true, docKeyHeader: true, docKeyDefaults: true, string(PresetMCP): true,
	"plugin": true, "marketplace": true, "placement": true, "claude": true, rulesDir: true, "lint": true,
	"permissions": true, "guard": true, "role_manifest": true, "lock": true, "llm": true,
}

// knownConfigDocKeys returns the TOML keys of Config (plus "schema").
func knownConfigDocKeys() map[string]bool {
	keys := map[string]bool{docKeySchema: true}
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("toml"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		keys[tag] = true
	}
	return keys
}

// normalizeConfigDocKeys returns a shallow copy of doc with the YAML/JSON
// "$schema" key renamed to the TOML key "schema".
func normalizeConfigDocKeys(doc map[string]any) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		if k == docKeySchemaDol {
			if _, ok := doc[docKeySchema]; ok {
				continue
			}
			k = docKeySchema
		}
		out[k] = v
	}
	return out
}

// DocForJSON returns a shallow copy of a merged document with the TOML key
// "schema" renamed to "$schema", ready to be marshaled to JSON and decoded
// into Config.
func DocForJSON(doc map[string]any) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		if k == docKeySchema {
			k = docKeySchemaDol
		}
		out[k] = v
	}
	return out
}

// MergeConfigDocs overlays a machine-local config document onto the shared one.
// Both inputs are generic decoded documents (map[string]any from TOML/YAML/JSON).
// Inputs are never mutated; the result shares no maps/slices with them. The
// result uses the TOML key "schema" for the schema reference; use DocForJSON
// before a JSON round trip.
func MergeConfigDocs(shared, local map[string]any) (merged map[string]any, warnings []string, err error) {
	merged = deepCopyMap(normalizeConfigDocKeys(shared))
	loc := deepCopyMap(normalizeConfigDocKeys(local))

	if err := checkUnknownLocalKeys(loc); err != nil {
		return nil, nil, err
	}

	for _, key := range sortedKeys(loc) {
		w, err := mergeTopLevelKey(merged, loc[key], key)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, w...)
	}
	return merged, warnings, nil
}

func checkUnknownLocalKeys(loc map[string]any) error {
	known := knownConfigDocKeys()
	var unknown []string
	for _, k := range sortedKeys(loc) {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return oops.
		Hint("Check for typos; valid keys: "+strings.Join(sortedKeys(known), ", ")).
		Errorf("unknown key(s) in local config: %s", strings.Join(unknown, ", "))
}

func mergeTopLevelKey(merged map[string]any, lv any, key string) ([]string, error) {
	sv, hasShared := merged[key]
	if pathFallback, named := namedListKeys[key]; named {
		out, w, err := mergeNamedList(key, sv, hasShared, lv, pathFallback)
		if err != nil {
			return nil, err
		}
		if hasShared || len(out) > 0 {
			merged[key] = out
		}
		return w, nil
	}
	switch {
	case key == docKeyVersion:
		return nil, mergeVersion(merged, sv, hasShared, lv)
	case key == docKeyPresets:
		out, w, err := mergePresets(sv, hasShared, lv)
		if err != nil {
			return nil, err
		}
		merged[key] = out
		return w, nil
	case key == docKeyBuiltins:
		merged[key] = lv
	case mapValuedKeys[key]:
		out, err := mergeMapKey(key, sv, lv)
		if err != nil {
			return nil, err
		}
		merged[key] = out
	default:
		merged[key] = mergeValue(sv, lv)
	}
	return nil, nil
}

func mergeVersion(merged map[string]any, sv any, hasShared bool, lv any) error {
	ls, ok := asString(lv)
	if !ok {
		return oops.Hint("version must be a string").Errorf("local config key version has type %T, expected string", lv)
	}
	if hasShared {
		ss, ok := asString(sv)
		if !ok {
			return oops.Hint("version must be a string").Errorf("shared config key version has type %T, expected string", sv)
		}
		if ss != ls {
			return oops.
				Hint("Remove version from the local config or make it match the shared config").
				Errorf("local config version %q does not match shared version %q", ls, ss)
		}
	}
	merged[docKeyVersion] = ls
	return nil
}

// mergeMapKey merges a table-valued top-level key; type mismatches (including
// a null local value, which would silently wipe shared settings) are errors.
func mergeMapKey(key string, shared, local any) (any, error) {
	lm, ok := local.(map[string]any)
	if !ok {
		return nil, oops.
			Hint("Key "+key+" must be a table").
			Errorf("local config key %s has type %T, expected table", key, local)
	}
	if shared == nil {
		return lm, nil
	}
	sm, ok := shared.(map[string]any)
	if !ok {
		return nil, oops.
			Hint("Key "+key+" must be a table").
			Errorf("shared config key %s has type %T, expected table", key, shared)
	}
	return mergeMaps(sm, lm), nil
}

// mergeValue merges maps per key (recursively); anything else, including
// lists, is replaced by the local value.
func mergeValue(shared, local any) any {
	sm, sok := shared.(map[string]any)
	lm, lok := local.(map[string]any)
	if sok && lok {
		return mergeMaps(sm, lm)
	}
	return local
}

// mergeMaps merges local into shared in place (callers pass deep copies).
func mergeMaps(shared, local map[string]any) map[string]any {
	for _, k := range sortedKeys(local) {
		shared[k] = mergeValue(shared[k], local[k])
	}
	return shared
}

// listOf requires a present value to be a list.
func listOf(side, key string, v any, present bool) ([]any, error) {
	if !present {
		return nil, nil
	}
	switch v.(type) {
	case []any, []string, []map[string]any:
		return asList(v), nil
	}
	return nil, oops.
		Hint("Key "+key+" must be a list").
		Errorf("%s config key %s has type %T, expected list", side, key, v)
}

// entryDesc describes a list entry by position and key names only.
func entryDesc(listKey string, idx int, e any) string {
	if m, ok := e.(map[string]any); ok {
		return fmt.Sprintf("%s entry #%d (keys: %s)", listKey, idx+1, strings.Join(sortedKeys(m), ", "))
	}
	return fmt.Sprintf("%s entry #%d (type %T)", listKey, idx+1, e)
}

// entryName returns the string name of an entry map; a non-string name is an error.
func entryName(m map[string]any, desc string) (string, error) {
	v, ok := m[docKeyName]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := asString(v)
	if !ok {
		return "", oops.Hint("name must be a string").Errorf("%s has a name of type %T, expected string", desc, v)
	}
	return s, nil
}

// asString accepts a string or a YAML scalar kept as source text.
func asString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case rawScalar:
		return string(t), true
	}
	return "", false
}

func entryPath(m map[string]any) string {
	s, ok := asString(m[docKeyPath])
	if !ok {
		return ""
	}
	return s
}

// removeFlag reads the optional boolean "remove" marker.
func removeFlag(m map[string]any, desc string) (bool, error) {
	v, ok := m[docKeyRemove]
	if !ok {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, oops.Hint("remove must be true or false").Errorf("%s has remove of type %T, expected boolean", desc, v)
	}
	return b, nil
}

func mergePresets(shared any, hasShared bool, local any) (merged any, warnings []string, err error) {
	out, err := listOf("shared", docKeyPresets, shared, hasShared)
	if err != nil {
		return nil, nil, err
	}
	localList, err := listOf("local", docKeyPresets, local, true)
	if err != nil {
		return nil, nil, err
	}
	if err := validatePresets(out, false); err != nil {
		return nil, nil, err
	}
	if err := validatePresets(localList, true); err != nil {
		return nil, nil, err
	}
	for _, entry := range localList {
		s, isStr := entry.(string)
		if !isStr || !strings.HasPrefix(s, presetDropMark) {
			out = append(out, entry)
			continue
		}
		name := strings.TrimPrefix(s, presetDropMark)
		idx := indexOfPreset(out, name)
		if idx < 0 {
			warnings = append(warnings, fmt.Sprintf("local config drops preset %q which is not in the shared config", name))
			continue
		}
		out = append(out[:idx], out[idx+1:]...)
	}
	return dedupePresets(out), warnings, nil
}

// validatePresets checks entry shapes and strips "remove" from shared objects.
func validatePresets(list []any, local bool) error {
	for i, e := range list {
		switch t := e.(type) {
		case string:
			if t == "" || (local && t == presetDropMark) {
				return oops.Hint("Preset names must not be empty").Errorf("presets entry #%d has an empty name", i+1)
			}
		case map[string]any:
			if err := validatePresetTable(t, entryDesc(docKeyPresets, i, t), local); err != nil {
				return err
			}
		default:
			return oops.
				Hint("Preset entries must be a name string or a table with a name").
				Errorf("presets entry #%d has type %T", i+1, e)
		}
	}
	return nil
}

func validatePresetTable(t map[string]any, desc string, local bool) error {
	name, err := entryName(t, desc)
	if err != nil {
		return err
	}
	if name == "" {
		return oops.Hint("Preset tables need a name").Errorf("%s has no name", desc)
	}
	if _, has := t[docKeyRemove]; has {
		if local {
			return oops.
				Hint(`Use "!name" in presets to drop a preset; remove is not supported on preset tables`).
				Errorf("%s uses remove", desc)
		}
		delete(t, docKeyRemove)
	}
	return nil
}

// presetName returns the name of a validated preset entry.
func presetName(e any) string {
	switch t := e.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t[docKeyName].(string); ok {
			return s
		}
	}
	return ""
}

func indexOfPreset(list []any, name string) int {
	for i, e := range list {
		if presetName(e) == name {
			return i
		}
	}
	return -1
}

// dedupePresets keeps one entry per name at its first position; a table wins
// over a bare name, and two tables merge with the later one winning.
func dedupePresets(list []any) []any {
	pos := map[string]int{}
	out := make([]any, 0, len(list))
	for _, e := range list {
		name := presetName(e)
		idx, seen := pos[name]
		if !seen {
			pos[name] = len(out)
			out = append(out, e)
			continue
		}
		em, isMap := e.(map[string]any)
		if !isMap {
			continue
		}
		if om, ok := out[idx].(map[string]any); ok {
			out[idx] = mergeMaps(om, em)
		} else {
			out[idx] = em
		}
	}
	return out
}

type namedEntry struct {
	m    map[string]any
	name string
	desc string
}

func parseEntries(side, listKey string, list []any) ([]namedEntry, error) {
	out := make([]namedEntry, len(list))
	for i, e := range list {
		desc := entryDesc(listKey, i, e)
		m, ok := e.(map[string]any)
		if !ok {
			return nil, oops.
				Hint("Every "+listKey+" entry must be a table").
				Errorf("%s config %s is not a table", side, desc)
		}
		name, err := entryName(m, desc)
		if err != nil {
			return nil, err
		}
		out[i] = namedEntry{m: m, name: name, desc: desc}
	}
	return out, nil
}

func entriesMatch(se, le namedEntry, pathFallback bool) bool {
	if le.name != "" {
		return se.name == le.name
	}
	if pathFallback {
		p := entryPath(le.m)
		return p != "" && entryPath(se.m) == p
	}
	return false
}

func entryLabel(e namedEntry, pathFallback bool) string {
	if e.name == "" && pathFallback {
		return entryPath(e.m)
	}
	return e.name
}

func mergeNamedList(
	listKey string, shared any, hasShared bool, local any, pathFallback bool,
) (merged []any, warnings []string, err error) {
	sharedList, err := listOf("shared", listKey, shared, hasShared)
	if err != nil {
		return nil, nil, err
	}
	localList, err := listOf("local", listKey, local, true)
	if err != nil {
		return nil, nil, err
	}
	se, err := parseEntries("shared", listKey, sharedList)
	if err != nil {
		return nil, nil, err
	}
	le, err := parseEntries("local", listKey, localList)
	if err != nil {
		return nil, nil, err
	}
	claimed, err := matchLocalEntries(listKey, se, le, pathFallback)
	if err != nil {
		return nil, nil, err
	}

	out, err := applyClaimed(se, claimed)
	if err != nil {
		return nil, nil, err
	}
	claimedDescs := map[string]bool{}
	for _, l := range claimed {
		claimedDescs[l.desc] = true
	}
	for _, l := range le {
		if claimedDescs[l.desc] {
			continue
		}
		drop, err := removeFlag(l.m, l.desc)
		if err != nil {
			return nil, nil, err
		}
		if drop {
			warnings = append(warnings, fmt.Sprintf(
				"local config removes %s entry %q which is not in the shared config", listKey, entryLabel(l, pathFallback)))
			continue
		}
		out = append(out, stripRemove(l.m))
	}
	if pathFallback {
		if err := checkDuplicatePaths(listKey, out); err != nil {
			return nil, nil, err
		}
	}
	return out, warnings, nil
}

// applyClaimed walks the shared entries in order, merging or dropping the
// ones a local entry targets.
func applyClaimed(shared []namedEntry, claimed map[int]namedEntry) ([]any, error) {
	out := make([]any, 0, len(shared))
	for i, s := range shared {
		l, ok := claimed[i]
		if !ok {
			out = append(out, stripRemove(s.m))
			continue
		}
		drop, err := removeFlag(l.m, l.desc)
		if err != nil {
			return nil, err
		}
		if !drop {
			out = append(out, stripRemove(mergeMaps(s.m, l.m)))
		}
	}
	return out, nil
}

// matchLocalEntries pairs each local entry with at most one shared entry,
// returning shared index -> local entry.
func matchLocalEntries(listKey string, shared, local []namedEntry, pathFallback bool) (map[int]namedEntry, error) {
	claimed := map[int]namedEntry{}
	seen := map[string]bool{}
	for _, l := range local {
		label := entryLabel(l, pathFallback)
		if label == "" {
			hint := "Every " + listKey + " entry needs a name"
			if pathFallback {
				hint = "Every " + listKey + " entry needs a name or path"
			}
			return nil, oops.Hint(hint).Errorf("local config %s has none", l.desc)
		}
		if seen[label] {
			return nil, oops.
				Hint("Merge the duplicate entries in the local config").
				Errorf("duplicate %s entry %q in local config", listKey, label)
		}
		seen[label] = true

		var hits []int
		for i, s := range shared {
			if entriesMatch(s, l, pathFallback) {
				hits = append(hits, i)
			}
		}
		if len(hits) > 1 {
			return nil, oops.
				Hint("Make the entry names in the shared config unique").
				Errorf("ambiguous %s entry %q: it matches %d shared entries", listKey, label, len(hits))
		}
		if len(hits) == 0 {
			continue
		}
		if _, dup := claimed[hits[0]]; dup {
			return nil, oops.
				Hint("Merge the duplicate entries in the local config").
				Errorf("duplicate %s entry %q in local config targets the same shared entry", listKey, label)
		}
		claimed[hits[0]] = l
	}
	return claimed, nil
}

func checkDuplicatePaths(listKey string, list []any) error {
	seen := map[string]bool{}
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		p := entryPath(m)
		if p == "" {
			continue
		}
		if seen[p] {
			return oops.
				Hint("Give each "+listKey+" entry a unique path").
				Errorf("duplicate %s path %q after merging local config", listKey, p)
		}
		seen[p] = true
	}
	return nil
}

// stripRemove drops the "remove" marker from an entry (maps only).
func stripRemove(v any) any {
	if m, ok := v.(map[string]any); ok {
		delete(m, docKeyRemove)
	}
	return v
}

func asList(v any) []any {
	switch l := v.(type) {
	case []any:
		return l
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	case []map[string]any:
		out := make([]any, len(l))
		for i, m := range l {
			out[i] = m
		}
		return out
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case map[any]any: // yaml.v2-style maps; yaml.v3 decodes to map[string]any
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[fmt.Sprint(k)] = deepCopyValue(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopyValue(e)
		}
		return out
	case []map[string]any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopyMap(e)
		}
		return out
	case []string:
		return append([]string(nil), t...)
	}
	return v
}
