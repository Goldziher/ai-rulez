package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/schema"
)

// LocalDoc is an editable machine-local config.local.* document. Edits happen on
// the generic document so unrelated keys survive; Save validates the merged
// result before keeping the change.
type LocalDoc struct {
	// Path is the overlay file path (existing, or where Save will create it).
	Path string
	// Format is "toml", "yaml" or "json".
	Format string
	// Doc is the overlay document being edited.
	Doc map[string]any

	configDir     string
	mainFile      string
	baseDir       string
	configDirName string
	release       func()
}

// ParseLocalPath splits a dotted overlay key path such as
// "mcp_servers.github.env.TOKEN" into segments and checks the leading key. For
// named lists (mcp_servers, plugins, includes, installed_skills, marketplaces,
// scopes) the segment after the list is the entry name.
func ParseLocalPath(s string) ([]string, error) {
	segs := strings.Split(strings.TrimSpace(s), ".")
	for _, seg := range segs {
		if seg == "" {
			return nil, oops.
				Hint("Use dotted keys like profiles.dev or mcp_servers.<name>.command").
				Errorf("invalid key path %q", s)
		}
	}
	if segs[0] == docKeySchemaDol {
		segs[0] = docKeySchema
	}
	if !knownConfigDocKeys()[segs[0]] {
		return nil, oops.
			Hint("Valid keys: "+strings.Join(sortedKeys(knownConfigDocKeys()), ", ")).
			Errorf("unknown config key %q", segs[0])
	}
	if _, named := namedListKeys[segs[0]]; named && len(segs) < 2 {
		return nil, oops.
			Hint("Address a list entry by name, e.g. "+segs[0]+".<name>.<field>").
			Errorf("%s is a list of named entries", segs[0])
	}
	return segs, nil
}

func mainConfigFileName(configDir string) string {
	for _, name := range []string{configTOMLFilename, configYAMLFilename, configYMLFilename, configJSONFilename} {
		if fileExists(filepath.Join(configDir, name)) {
			return name
		}
	}
	return ""
}

// localDocLocation resolves the config directory and main config file of the
// project at baseDir the way loading does.
func localDocLocation(baseDir string) (configDir, mainFile string) {
	dirName := ResolveConfigDirName(baseDir)
	if dirName == "" {
		dirName = aiRulezDirName
	}
	configDir = filepath.Join(baseDir, filepath.FromSlash(dirName))
	return configDir, mainConfigFileName(configDir)
}

// OpenLocalDocInDir opens the overlay for editing, holding the overlay's
// advisory lock until Close. Callers must defer Close.
func OpenLocalDocInDir(baseDir string) (*LocalDoc, error) {
	configDir, mainFile := localDocLocation(baseDir)
	return OpenLocalDoc(configDir, mainFile)
}

// ViewLocalDoc opens the overlay read-only, without taking the lock.
func ViewLocalDoc(baseDir string) (*LocalDoc, error) {
	configDir, mainFile := localDocLocation(baseDir)
	return openLocalDoc(configDir, mainFile, false)
}

// OpenLocalDoc opens the config.local.* file in configDir for editing, or starts
// an in-memory one in the same format as the main config (TOML by default). It
// holds an advisory lock (<configDir>/.config.local.lock) until Close so two
// concurrent edits cannot interleave; callers must defer Close.
func OpenLocalDoc(configDir, mainConfigFile string) (*LocalDoc, error) {
	return openLocalDoc(configDir, mainConfigFile, true)
}

func openLocalDoc(configDir, mainConfigFile string, lock bool) (*LocalDoc, error) {
	baseDir := projectBaseDir(configDir)
	d := &LocalDoc{
		configDir: configDir, mainFile: mainConfigFile,
		baseDir: baseDir, configDirName: filepath.ToSlash(relConfigDirName(baseDir, configDir)),
	}
	if lock {
		release, err := lockLocalConfig(filepath.Join(configDir, localLockName))
		if err != nil {
			return nil, err
		}
		d.release = release
	}
	if err := d.load(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *LocalDoc) load() error {
	existing, err := findLocalConfigFile(d.configDir, d.mainFile)
	if err != nil {
		return err
	}
	if existing != "" {
		doc, err := readConfigDoc(existing)
		if err != nil {
			return err
		}
		d.Path = existing
		d.Format = localConfigFormatByExt[filepath.Ext(existing)]
		d.Doc = normalizeConfigDocKeys(doc)
		return nil
	}
	ext := extTOML
	if f := localConfigFormatByExt[filepath.Ext(d.mainFile)]; f != "" {
		ext = filepath.Ext(d.mainFile)
	}
	d.Format = localConfigFormatByExt[ext]
	d.Path = filepath.Join(d.configDir, localVariantName("config"+ext))
	d.Doc = map[string]any{}
	return nil
}

// Close releases the advisory lock taken by OpenLocalDoc. It is safe to call
// more than once and on a read-only doc.
func (d *LocalDoc) Close() {
	if d != nil && d.release != nil {
		d.release()
		d.release = nil
	}
}

// Exists reports whether the overlay file is on disk.
func (d *LocalDoc) Exists() bool { return fileExists(d.Path) }

// Set stores v at the key path (see ParseLocalPath).
func (d *LocalDoc) Set(path []string, v any) error {
	if err := checkPath(path); err != nil {
		return err
	}
	v = toDocValue(v)
	if _, named := namedListKeys[path[0]]; named {
		if len(path) < 2 {
			return oops.Errorf("%s needs an entry name", path[0])
		}
		entry := d.namedEntry(path[0], path[1], true)
		if len(path) == 2 {
			fields, ok := v.(map[string]any)
			if !ok {
				return oops.
					Hint("Set a field, e.g. "+path[0]+"."+path[1]+".<field>").
					Errorf("%s.%s needs a field to set", path[0], path[1])
			}
			for k, fv := range fields {
				entry[k] = fv
			}
			return nil
		}
		return setIn(entry, path[2:], v)
	}
	return setIn(d.Doc, path, v)
}

// Unset removes the value at the key path from the overlay. For a named list,
// "<list>.<name>" removes the whole local entry.
func (d *LocalDoc) Unset(path []string) error {
	if err := checkPath(path); err != nil {
		return err
	}
	if _, named := namedListKeys[path[0]]; named {
		if len(path) == 2 {
			d.dropNamed(path[0], path[1])
			return nil
		}
		entry := d.namedEntry(path[0], path[1], false)
		if entry == nil {
			return nil
		}
		unsetIn(entry, path[2:])
		if len(entry) == 1 { // only the name is left: no override remains
			d.dropNamed(path[0], path[1])
		}
		return nil
	}
	unsetIn(d.Doc, path)
	return nil
}

// UpsertNamed creates or updates the local entry list/name with the given
// fields. If the entry is a local remove marker it is replaced, not extended.
func (d *LocalDoc) UpsertNamed(list, name string, fields map[string]any) error {
	if _, named := namedListKeys[list]; !named {
		return oops.Errorf("%s is not a list of named entries", list)
	}
	if name == "" {
		return oops.Errorf("%s entry name is required", list)
	}
	entry := d.namedEntry(list, name, true)
	if removing, _ := entry[docKeyRemove].(bool); removing { //nolint:errcheck // absent marker reads as false
		// A local remove marker is replaced wholesale: the new entry does not
		// inherit anything from the deleted one.
		for k := range entry {
			if k != docKeyName {
				delete(entry, k)
			}
		}
	}
	for k, v := range fields {
		entry[k] = toDocValue(v)
	}
	return nil
}

// RemoveNamed drops the local entry list/name. When the shared config also has
// it, a {name, remove = true} entry is written so the shared one is deleted.
func (d *LocalDoc) RemoveNamed(list, name string, sharedHas bool) error {
	if _, named := namedListKeys[list]; !named {
		return oops.Errorf("%s is not a list of named entries", list)
	}
	d.dropNamed(list, name)
	if sharedHas {
		d.namedEntry(list, name, true)[docKeyRemove] = true
	}
	return nil
}

// SetProfile defines (or replaces) a local profile.
func (d *LocalDoc) SetProfile(name string, domains []string) error {
	return d.Set([]string{docKeyProfiles, name}, domains)
}

// RemoveProfile deletes a profile from the overlay.
func (d *LocalDoc) RemoveProfile(name string) error {
	return d.Unset([]string{docKeyProfiles, name})
}

// SetDefault sets the local default profile.
func (d *LocalDoc) SetDefault(name string) error {
	return d.Set([]string{"default"}, name)
}

// HasLocalProfile reports whether the overlay itself defines the profile.
func (d *LocalDoc) HasLocalProfile(name string) bool {
	profiles, ok := d.Doc[docKeyProfiles].(map[string]any)
	if !ok {
		return false
	}
	_, ok = profiles[name]
	return ok
}

// HasLocalNamed reports whether the overlay has a (non-removing) entry list/name.
func (d *LocalDoc) HasLocalNamed(list, name string) bool {
	e := d.namedEntry(list, name, false)
	removing, _ := e[docKeyRemove].(bool) //nolint:errcheck // absent marker reads as false
	return e != nil && !removing
}

func checkPath(path []string) error {
	if len(path) == 0 {
		return oops.Errorf("empty key path")
	}
	if _, err := ParseLocalPath(strings.Join(path, ".")); err != nil {
		return err
	}
	return nil
}

func (d *LocalDoc) namedEntry(list, name string, create bool) map[string]any {
	entries, _ := d.Doc[list].([]any) //nolint:errcheck // absent list reads as empty
	for _, e := range entries {
		if m, ok := e.(map[string]any); ok && entryIdentity(m) == name {
			return m
		}
	}
	if !create {
		return nil
	}
	m := map[string]any{docKeyName: name}
	d.Doc[list] = append(entries, m)
	return m
}

func entryIdentity(m map[string]any) string {
	if s, ok := asString(m[docKeyName]); ok && s != "" {
		return s
	}
	s, _ := asString(m[docKeyPath]) //nolint:errcheck // absent path reads as empty
	return s
}

func (d *LocalDoc) dropNamed(list, name string) {
	entries, _ := d.Doc[list].([]any) //nolint:errcheck // absent list reads as empty
	kept := make([]any, 0, len(entries))
	for _, e := range entries {
		if m, ok := e.(map[string]any); ok && entryIdentity(m) == name {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == 0 {
		delete(d.Doc, list)
		return
	}
	d.Doc[list] = kept
}

func setIn(m map[string]any, path []string, v any) error {
	if len(path) == 0 {
		return nil
	}
	if len(path) == 1 {
		m[path[0]] = v
		return nil
	}
	child, ok := m[path[0]].(map[string]any)
	if !ok {
		if existing, present := m[path[0]]; present && existing != nil {
			return oops.
				Hint("Unset "+path[0]+" first").
				Errorf("%s is not a table, cannot set a key under it", path[0])
		}
		child = map[string]any{}
		m[path[0]] = child
	}
	return setIn(child, path[1:], v)
}

// unsetIn deletes the key and prunes tables left empty.
func unsetIn(m map[string]any, path []string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		delete(m, path[0])
		return
	}
	child, ok := m[path[0]].(map[string]any)
	if !ok {
		return
	}
	unsetIn(child, path[1:])
	if len(child) == 0 {
		delete(m, path[0])
	}
}

// toDocValue normalizes caller values to the generic document types.
func toDocValue(v any) any {
	switch t := v.(type) {
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, s := range t {
			out[k] = s
		}
		return out
	}
	return deepCopyValue(v)
}

// localLockName is the advisory lock file guarding overlay edits.
const localLockName = ".config.local.lock"

const localFileHeaderTOML = "# Machine-local ai-rulez overrides, merged onto config.toml. Gitignored; may hold secrets.\n\n"

// marshal encodes the overlay in its format.
func (d *LocalDoc) marshal() ([]byte, error) {
	switch d.Format {
	case formatJSON:
		return json.MarshalIndent(DocForJSON(d.Doc), "", "  ")
	case formatYAML:
		return yaml.Marshal(DocForJSON(d.Doc))
	default:
		data, err := toml.Marshal(tomlSafeValue(d.Doc))
		if err != nil {
			return nil, err //nolint:wrapcheck // wrapped by Save
		}
		return append([]byte(localFileHeaderTOML), data...), nil
	}
}

// gitignorePatterns are the entries that keep the overlay (and its lock and
// temp files) out of version control.
func (d *LocalDoc) gitignorePatterns() []string {
	return []string{d.configDirName + "/config.local.*", d.configDirName + "/.config.local.*"}
}

// Save writes the overlay (owner-only, atomically), then validates it against the
// local schema and loads the merged configuration from disk to validate that.
// If anything fails the previous file content is restored and the error
// returned. The ignore entries are ensured before anything is written, so a
// secret-bearing overlay is never on disk unignored. Includes are not fetched
// for the validation load.
func (d *LocalDoc) Save(ctx context.Context) error {
	if d.mainFile == "" {
		return oops.Hint("Run 'ai-rulez init' first").Errorf("no main config file found in %s", d.configDir)
	}
	data, err := d.marshal()
	if err != nil {
		return oops.With("path", d.Path).Wrapf(err, "encode %s", filepath.Base(d.Path))
	}
	if err := refuseSymlink(d.Path); err != nil {
		return err
	}
	if err := gitignore.EnsureEntries(d.baseDir, d.gitignorePatterns()); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	previous, readErr := os.ReadFile(d.Path) //nolint:gosec // overlay path from the config directory
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return oops.With("path", d.Path).Wrapf(readErr, "read %s", filepath.Base(d.Path))
	}
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o755); err != nil {
		return oops.With("path", d.Path).Wrapf(err, "create config directory")
	}
	if err := writeFileAtomic(d.Path, data, 0o600); err != nil {
		return err
	}

	if err := d.validate(ctx); err != nil {
		invalid := oops.
			With("path", d.Path).
			Hint("The change was not kept; fix the value and try again").
			Wrapf(err, "local overlay is invalid")
		if restoreErr := d.restore(previous, existed); restoreErr != nil {
			return errors.Join(invalid, oops.
				With("path", d.Path).
				Hint("Fix or delete the overlay file by hand").
				Wrapf(restoreErr, "could not restore the previous overlay; the invalid overlay may remain at %s", d.Path))
		}
		return invalid
	}
	return nil
}

// validate checks the freshly written overlay: the local schema, the merged
// configuration, and that new MCP servers are complete.
func (d *LocalDoc) validate(ctx context.Context) error {
	if err := schema.ValidateLocalFile(d.Path); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	cfg, err := d.validateMerged(ctx)
	if err != nil {
		return err
	}
	return d.checkNewServersComplete(cfg)
}

func (d *LocalDoc) validateMerged(ctx context.Context) (*Config, error) {
	cfg, err := LoadConfigFromFile(WithOfflineIncludes(ctx), filepath.Join(d.configDir, d.mainFile))
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// checkNewServersComplete rejects an MCP server that exists only in the overlay
// and has neither a command (stdio) nor a url (http/sse).
func (d *LocalDoc) checkNewServersComplete(cfg *Config) error {
	shared, err := readConfigDoc(filepath.Join(d.configDir, d.mainFile))
	if err != nil {
		return err
	}
	inShared := map[string]bool{}
	for _, e := range asList(shared["mcp_servers"]) {
		if m, ok := e.(map[string]any); ok {
			inShared[entryIdentity(m)] = true
		}
	}
	for _, e := range asList(d.Doc["mcp_servers"]) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		name := entryIdentity(m)
		server := cfg.MCPServers[name]
		if inShared[name] || server == nil || server.Command != "" || server.URL != "" {
			continue
		}
		return oops.
			Hint("Set command (stdio server) or url (http/sse server) first, e.g. mcp_servers."+name+".command").
			Errorf("local entry mcp_servers.%s is new and incomplete", name)
	}
	return nil
}

func (d *LocalDoc) restore(previous []byte, existed bool) error {
	if !existed {
		if err := os.Remove(d.Path); err != nil && !os.IsNotExist(err) {
			return err //nolint:wrapcheck // wrapped by Save
		}
		return nil
	}
	return writeFileAtomic(d.Path, previous, 0o600)
}

// refuseSymlink fails when path is a symbolic link: the overlay may hold
// secrets and is replaced atomically, which would silently swap the link.
func refuseSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return nil //nolint:nilerr // a missing file is fine
	}
	return oops.
		With("path", path).
		Hint("Replace the symlink with a regular file, or edit the file it points to directly").
		Errorf("%s is a symbolic link; refusing to replace it", filepath.Base(path))
}

// InitLocalOverlay creates a commented config.local.* skeleton in the main
// config's format. created is false when an overlay already exists.
func InitLocalOverlay(baseDir string) (path string, created bool, err error) {
	d, err := OpenLocalDocInDir(baseDir)
	if err != nil {
		return "", false, err
	}
	defer d.Close()
	if d.Exists() {
		return d.Path, false, nil
	}
	if d.mainFile == "" {
		return "", false, oops.Hint("Run 'ai-rulez init' first").Errorf("no main config file found in %s", d.configDir)
	}
	body := localSkeletonTOML
	switch d.Format {
	case formatYAML:
		body = localSkeletonYAML
	case formatJSON:
		body = "{}\n"
	}
	if err := refuseSymlink(d.Path); err != nil {
		return "", false, err
	}
	if err := gitignore.EnsureEntries(d.baseDir, d.gitignorePatterns()); err != nil {
		return "", false, err //nolint:wrapcheck // already contextual
	}
	if err := writeFileAtomic(d.Path, []byte(body), 0o600); err != nil {
		return "", false, err
	}
	return d.Path, true, nil
}

const localSkeletonTOML = `# Machine-local ai-rulez overrides, merged onto config.toml. Gitignored; may hold secrets.
# Scalars and tables override the shared value; lists of tables (mcp_servers,
# includes, plugins, ...) merge by name; set remove = true to delete a shared entry.

# default = "dev"

# presets = ["codex", "!cursor"]   # "!name" drops a shared preset

# [profiles]
# dev = ["backend"]

# [[mcp_servers]]
# name = "my-server"
# command = "npx"
# args = ["-y", "my-server"]
# [mcp_servers.env]
# MY_TOKEN = "..."
`

const localSkeletonYAML = `# Machine-local ai-rulez overrides, merged onto config.yaml. Gitignored; may hold secrets.
# Scalars and maps override the shared value; lists of maps (mcp_servers,
# includes, plugins, ...) merge by name; set remove: true to delete a shared entry.

# default: dev

# presets: [codex, "!cursor"]   # "!name" drops a shared preset

# profiles:
#   dev: [backend]

# mcp_servers:
#   - name: my-server
#     command: npx
#     args: ["-y", "my-server"]
#     env: {MY_TOKEN: "..."}
`

// OverlayChange is one key set by the overlay with the shared value it overrides.
type OverlayChange struct {
	Path      string
	Shared    any
	Local     any
	HasShared bool
	Redacted  bool
}

// RedactedValue is shown in place of secret values.
const RedactedValue = "<redacted>"

// DescribeLocalOverlay lists the keys the overlay sets, each with the shared
// value it replaces. Values of env/headers entries and of secret-looking keys
// are flagged Redacted; callers must not print them.
func DescribeLocalOverlay(baseDir string) (*LocalOverlay, []OverlayChange, error) {
	d, err := ViewLocalDoc(baseDir)
	if err != nil {
		return nil, nil, err
	}
	if !d.Exists() {
		return nil, nil, nil
	}
	shared := map[string]any{}
	if d.mainFile != "" {
		doc, err := readConfigDoc(filepath.Join(d.configDir, d.mainFile))
		if err != nil {
			return nil, nil, err
		}
		shared = normalizeConfigDocKeys(doc)
	}
	overlay := &LocalOverlay{Path: d.Path, Format: d.Format, Doc: d.Doc}
	var changes []OverlayChange
	walkLeaves(nil, d.Doc, func(segs []string, v any) {
		sv, has := lookupSegs(shared, segs)
		changes = append(changes, OverlayChange{
			Path: strings.Join(segs, "."), Shared: sv, Local: v, HasShared: has, Redacted: !showAllowed(segs),
		})
	})
	return overlay, changes, nil
}

func isNamedListKey(key string) bool {
	_, ok := namedListKeys[key]
	return ok
}

var showScalarKeys = map[string]bool{
	"name": true, "description": true, "default": true, "presets": true,
	"gitignore": true, "compact": true, "builtins": true,
}

// showAllowed is the default-deny display policy: only keys known to hold no
// credentials, URLs or paths have their values shown. Everything else is listed
// by key path with the value withheld (the overlay may hold secrets).
//
// Each allowed shape is spelled out by position, never by a trailing segment
// name alone, so a user-chosen key such as env.remove or headers.transport
// cannot masquerade as a marker. Of the table keys only enum and bool leaves
// qualify; free-form strings (defaults.model_by_preset, header.text) are denied.
func showAllowed(segs []string) bool {
	if len(segs) == 0 {
		return false
	}
	if rule, ok := showRules[segs[0]]; ok {
		return rule(segs)
	}
	if isNamedListKey(segs[0]) {
		// <list>.<name>.<marker>: entry-level fields only.
		last := segs[len(segs)-1]
		return len(segs) == 3 && (last == "transport" || last == "enabled" || last == docKeyRemove)
	}
	return len(segs) == 1 && showScalarKeys[segs[0]]
}

// showRules holds the allowed shapes of the table-valued keys, by position.
var showRules = map[string]func(segs []string) bool{
	docKeyProfiles: func(s []string) bool { return len(s) == 2 }, // profile name -> list of domain names
	"defaults": func(s []string) bool {
		last := s[len(s)-1]
		return (len(s) == 2 && (last == "effort" || last == "omit_agent_fields")) ||
			(len(s) == 3 && s[1] == "effort_by_preset")
	},
	rulesDir: func(s []string) bool {
		return (len(s) == 2 && s[1] == "mode") || (len(s) == 3 && s[1] == "mode_by_preset")
	},
	"header": func(s []string) bool {
		return len(s) == 2 && (s[1] == "style" || s[1] == "hashes" || s[1] == "timestamp")
	},
	string(PresetMCP): func(s []string) bool {
		return len(s) == 2 && (s[1] == "self_server" || s[1] == "self_server_version")
	},
}

// lookupSegs finds the shared value at an overlay key path.
func lookupSegs(doc map[string]any, segs []string) (any, bool) {
	var cur any = doc
	for i, seg := range segs {
		if i == 1 {
			if _, named := namedListKeys[segs[0]]; named {
				entry := findNamed(cur, seg)
				if entry == nil {
					return nil, false
				}
				cur = entry
				continue
			}
		}
		if i == 0 {
			cur = doc[seg]
			if cur == nil {
				return nil, false
			}
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, present := m[seg]
		if !present {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func findNamed(list any, name string) map[string]any {
	for _, e := range asList(list) {
		if m, ok := e.(map[string]any); ok && entryIdentity(m) == name {
			return m
		}
	}
	return nil
}

// walkLeaves visits every leaf value of an overlay document with its key path
// segments. Named-list entries are addressed by name.
func walkLeaves(prefix []string, v any, fn func(segs []string, v any)) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 && len(prefix) > 0 {
			fn(prefix, v)
			return
		}
		for _, k := range sortedKeys(t) {
			walkLeaves(appendSeg(prefix, k), t[k], fn)
		}
	case []any:
		if names, ok := namedEntryNames(t); ok {
			for i, e := range t {
				walkLeaves(appendSeg(prefix, names[i]), e, fn)
			}
			return
		}
		if hasTable(t) {
			for i, e := range t {
				walkLeaves(appendSeg(prefix, "["+strconv.Itoa(i)+"]"), e, fn)
			}
			return
		}
		fn(prefix, v)
	default:
		fn(prefix, v)
	}
}

func hasTable(list []any) bool {
	for _, e := range list {
		if _, ok := e.(map[string]any); ok {
			return true
		}
	}
	return false
}

func appendSeg(prefix []string, seg string) []string {
	out := make([]string, len(prefix)+1)
	copy(out, prefix)
	out[len(prefix)] = seg
	return out
}

// namedEntryNames returns the names of a list of named tables, or false for any
// other list.
func namedEntryNames(list []any) ([]string, bool) {
	if len(list) == 0 {
		return nil, false
	}
	names := make([]string, len(list))
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, false
		}
		if names[i] = entryIdentity(m); names[i] == "" {
			return nil, false
		}
	}
	return names, true
}
