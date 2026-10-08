package config

import (
	"context"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

// LocalDoc is an editable machine-local config.local.* document. Edits happen on
// the generic document so unrelated keys survive; Save validates the merged
// result before keeping the change.
type LocalDoc struct {
	// Path is the overlay file path (existing, or where Save will create it).
	Path string
	// Doc is the overlay document being edited.
	Doc map[string]any

	configDir     string
	mainFile      string
	baseDir       string
	configDirName string
	release       func()
	resolvers     Resolvers
	log           logger.Logger
}

// WithLog makes Save report to log instead of the CLI's logger.
func (d *LocalDoc) WithLog(log logger.Logger) *LocalDoc {
	d.log = log
	return d
}

// WithResolvers makes the validation load of Save resolve includes and installed
// skills the way the caller's own loads do.
func (d *LocalDoc) WithResolvers(r Resolvers) *LocalDoc {
	d.resolvers = r
	return d
}

// ParseLocalPath splits an overlay key path such as
// "mcp_servers.github.env.TOKEN" into segments and checks the leading key. For
// named lists (mcp_servers, plugins, includes, installed_skills, marketplaces,
// scopes) the segment after the list is the entry name. A segment that contains
// a dot is written in brackets with double quotes: mcp_servers["foo.bar"].command.
// Inside the quotes, \" and \\ are escapes.
func ParseLocalPath(s string) ([]string, error) {
	segs, err := splitLocalPath(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	if segs[0] == docKeySchemaDol {
		segs[0] = docKeySchema
	}
	return segs, checkSegments(segs)
}

func errInvalidPath(s string) error {
	return oops.
		Hint(`Use dotted keys like profiles.dev or mcp_servers.<name>.command; quote dotted names: mcp_servers["a.b"].command`).
		Errorf("invalid key path %q", s)
}

// splitLocalPath tokenizes a dotted path with optional ["quoted"] segments.
func splitLocalPath(s string) ([]string, error) {
	var segs []string
	i := 0
	for {
		if i < len(s) && s[i] == '[' {
			seg, next, ok := readQuotedSegment(s, i)
			if !ok {
				return nil, errInvalidPath(s)
			}
			segs = append(segs, seg)
			i = next
		} else {
			j := i
			for j < len(s) && s[j] != '.' && s[j] != '[' {
				j++
			}
			if j == i {
				return nil, errInvalidPath(s)
			}
			segs = append(segs, s[i:j])
			i = j
		}
		switch {
		case i == len(s):
			return segs, nil
		case s[i] == '.':
			i++
		case s[i] != '[':
			return nil, errInvalidPath(s)
		}
	}
}

// readQuotedSegment reads ["..."] starting at s[i] and returns the unescaped
// text and the index after the closing bracket.
func readQuotedSegment(s string, i int) (seg string, next int, ok bool) {
	if i+1 >= len(s) || s[i+1] != '"' {
		return "", 0, false
	}
	var b strings.Builder
	for j := i + 2; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
			if j >= len(s) {
				return "", 0, false
			}
			b.WriteByte(s[j])
		case '"':
			if j+1 < len(s) && s[j+1] == ']' && b.Len() > 0 {
				return b.String(), j + 2, true
			}
			return "", 0, false
		default:
			b.WriteByte(s[j])
		}
	}
	return "", 0, false
}

// checkSegments checks the leading key and the entry name of named lists.
func checkSegments(segs []string) error {
	if !knownConfigDocKeys()[segs[0]] {
		return oops.
			Hint("Valid keys: "+strings.Join(slices.Sorted(maps.Keys(knownConfigDocKeys())), ", ")).
			Errorf("unknown config key %q", segs[0])
	}
	if _, named := namedListKeys[segs[0]]; named && len(segs) < 2 {
		return oops.
			Hint("Address a list entry by name, e.g. "+segs[0]+".<name>.<field>").
			Errorf("%s is a list of named entries", segs[0])
	}
	return nil
}

func mainConfigFileName(configDir string) string {
	for _, name := range []string{configTOMLFilename} {
		if safefs.IsFile(filepath.Join(configDir, name)) {
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

// ResolveLocalConfigDir returns the config directory the local commands act on,
// resolving the project the way loading does: configFlag (the global --config,
// a config directory or a file inside one) wins, then dirName (--config-dir)
// under baseDir, then discovery under baseDir.
func ResolveLocalConfigDir(baseDir, configFlag, dirName string) string {
	switch {
	case configFlag != "":
		abs, err := filepath.Abs(configFlag)
		if err != nil {
			abs = configFlag
		}
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
			return filepath.Dir(abs)
		}
		return abs
	case dirName != "":
		return filepath.Join(baseDir, filepath.FromSlash(dirName))
	}
	configDir, _ := localDocLocation(baseDir)
	return configDir
}

// OpenLocalDocAt is OpenLocalDocInDir for an explicit config directory.
func OpenLocalDocAt(configDir string) (*LocalDoc, error) {
	return OpenLocalDoc(configDir, mainConfigFileName(configDir))
}

// ViewLocalDocAt is ViewLocalDoc for an explicit config directory.
func ViewLocalDocAt(configDir string) (*LocalDoc, error) {
	return openLocalDoc(configDir, mainConfigFileName(configDir), false)
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
	if lock && mainConfigFile == "" {
		return nil, oops.Hint("Run 'ai-rulez init' first").Errorf("no main config file found in %s", configDir)
	}
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
	existing, err := findLocalConfigFile(osView(d.configDir), d.configDir)
	if err != nil {
		return err
	}
	if existing != "" {
		doc, err := readConfigDoc(osView(d.configDir), existing)
		if err != nil {
			return err
		}
		d.Path = existing
		d.Doc = normalizeConfigDocKeys(doc)
		return nil
	}
	d.Path = filepath.Join(d.configDir, localVariantName(configTOMLFilename))
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
func (d *LocalDoc) Exists() bool { return safefs.IsFile(d.Path) }

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
	return d.Set([]string{docKeyDefault}, name)
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
	return checkSegments(path)
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
	if namedListKeys[list] && d.sharedHasPathOnlyEntry(list, name) {
		m = map[string]any{docKeyPath: name}
	}
	d.Doc[list] = append(entries, m)
	return m
}

// sharedHasPathOnlyEntry reports whether the shared config has a nameless entry
// of the list identified by path == key, so the overlay entry must be keyed by
// path too to merge with it.
func (d *LocalDoc) sharedHasPathOnlyEntry(list, key string) bool {
	if d.mainFile == "" {
		return false
	}
	doc, err := readConfigDoc(osView(d.configDir), filepath.Join(d.configDir, d.mainFile))
	if err != nil {
		return false
	}
	entries, _ := normalizeConfigDocKeys(doc)[list].([]any) //nolint:errcheck // absent list reads as empty
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if n, _ := asString(m[docKeyName]); n != "" { //nolint:errcheck // absent name reads as empty
			continue
		}
		if p, _ := asString(m[docKeyPath]); p == key { //nolint:errcheck // absent path reads as empty
			return true
		}
	}
	return false
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
	data, err := toml.Marshal(tomlSafeValue(d.Doc))
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by Save
	}
	return append([]byte(localFileHeaderTOML), data...), nil
}

// gitignorePatterns are the entries that keep the overlay (and its lock and
// temp files) out of version control.
func (d *LocalDoc) gitignorePatterns() []string {
	return overlayGitignorePatterns(d.configDirName)
}

func overlayGitignorePatterns(configDirName string) []string {
	return []string{configDirName + "/config.local.*", configDirName + "/.config.local.*"}
}

// LocalGitignorePatterns are the stable ignore entries for every machine-local
// input of the project whose config directory is configDir: the overlay, its
// lock and temp files, and the local/ content tree. Writers of any of them
// ensure these before writing.
func LocalGitignorePatterns(configDir string) []string {
	name := filepath.ToSlash(relConfigDirName(projectBaseDir(configDir), configDir))
	return append(overlayGitignorePatterns(name), name+"/"+localDir+"/")
}

// WriteFileAtomic writes data to path with perm via an exclusively created temp
// file in the same directory, then renames it over the target.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(path, data, perm)
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
	if err := gitignore.EnsureEntries(d.log, d.baseDir, d.gitignorePatterns()); err != nil {
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
	cfg, err := LoadConfigFromFile(WithOfflineIncludes(ctx), filepath.Join(d.configDir, d.mainFile), WithResolvers(d.resolvers))
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
	shared, err := readConfigDoc(osView(d.configDir), filepath.Join(d.configDir, d.mainFile))
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
	configDir, mainFile := localDocLocation(baseDir)
	return initLocalOverlay(configDir, mainFile)
}

// InitLocalOverlayAt is InitLocalOverlay for an explicit config directory.
func InitLocalOverlayAt(configDir string) (path string, created bool, err error) {
	return initLocalOverlay(configDir, mainConfigFileName(configDir))
}

func initLocalOverlay(configDir, mainFile string) (path string, created bool, err error) {
	if mainFile == "" {
		return "", false, oops.Hint("Run 'ai-rulez init' first").Errorf("no main config file found in %s", configDir)
	}
	d, err := OpenLocalDoc(configDir, mainFile)
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
	if err := refuseSymlink(d.Path); err != nil {
		return "", false, err
	}
	if err := gitignore.EnsureEntries(d.log, d.baseDir, d.gitignorePatterns()); err != nil {
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

// OverlayChange is one key set by the overlay with the shared value it overrides.
type OverlayChange struct {
	Path      string
	Shared    any
	Local     any
	HasShared bool
	Redacted  bool
	// Merged is the value the key has after the overlay is merged, set for keys
	// the overlay combines with the shared value instead of replacing (presets:
	// an ordered union with "!name" drops).
	Merged    any
	HasMerged bool
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
	return describeLocalOverlay(d)
}

// DescribeLocalOverlayAt is DescribeLocalOverlay for an explicit config directory.
func DescribeLocalOverlayAt(configDir string) (*LocalOverlay, []OverlayChange, error) {
	d, err := ViewLocalDocAt(configDir)
	if err != nil {
		return nil, nil, err
	}
	return describeLocalOverlay(d)
}

func describeLocalOverlay(d *LocalDoc) (*LocalOverlay, []OverlayChange, error) {
	if !d.Exists() {
		return nil, nil, nil
	}
	shared := map[string]any{}
	if d.mainFile != "" {
		doc, err := readConfigDoc(osView(d.configDir), filepath.Join(d.configDir, d.mainFile))
		if err != nil {
			return nil, nil, err
		}
		shared = normalizeConfigDocKeys(doc)
	}
	overlay := &LocalOverlay{Path: d.Path, Doc: d.Doc}
	var changes []OverlayChange
	walkLeaves(nil, d.Doc, func(segs []string, v any) {
		sv, has := lookupSegs(shared, segs)
		change := OverlayChange{
			Path: strings.Join(segs, "."), Shared: sv, Local: v, HasShared: has, Redacted: !showAllowed(segs) || !showTypeMatches(segs, v),
		}
		if len(segs) == 1 && segs[0] == docKeyPresets {
			if merged, _, mergeErr := mergePresets(sv, has, v); mergeErr == nil {
				change.Merged, change.HasMerged = merged, true
			}
		}
		changes = append(changes, change)
	})
	return overlay, changes, nil
}

func isNamedListKey(key string) bool {
	_, ok := namedListKeys[key]
	return ok
}

var showScalarKeys = map[string]bool{
	"name": true, "description": true, docKeyDefault: true, "presets": true,
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

// valueKind is the type a displayable overlay value must have.
type valueKind int

const (
	kindString valueKind = iota
	kindBool
	kindStringList
	kindBoolOrStringList
)

// showTypeMatches reports whether v has the type its allowlisted key expects. A
// mistyped value (a secret pasted into a bool key, say) is withheld like any
// other: allowlisted keys are only safe to print when they hold what they should.
func showTypeMatches(segs []string, v any) bool {
	kind, ok := showKind(segs)
	if !ok {
		return false
	}
	switch kind {
	case kindBool:
		_, ok = v.(bool)
		return ok
	case kindString:
		_, ok = v.(string)
		return ok
	case kindStringList:
		return isStringList(v)
	default:
		_, ok = v.(bool)
		return ok || isStringList(v)
	}
}

func isStringList(v any) bool {
	list, ok := v.([]any)
	if !ok {
		if _, isStrs := v.([]string); isStrs {
			return true
		}
		return false
	}
	for _, e := range list {
		if _, ok := e.(string); !ok {
			return false
		}
	}
	return true
}

var showScalarKinds = map[string]valueKind{
	"name": kindString, "description": kindString, docKeyDefault: kindString, "presets": kindStringList,
	"gitignore": kindBool, "compact": kindBool, "builtins": kindBoolOrStringList,
}

// showKind returns the expected type of an allowlisted key path.
func showKind(segs []string) (valueKind, bool) {
	if len(segs) == 0 {
		return 0, false
	}
	last := segs[len(segs)-1]
	switch segs[0] {
	case docKeyProfiles:
		return kindStringList, true
	case docKeyDefaults:
		if last == "omit_agent_fields" {
			return kindStringList, true
		}
		return kindString, true
	case rulesDir:
		return kindString, true
	case docKeyHeader:
		if last == "timestamp" {
			return kindBool, true
		}
		return kindString, true
	case string(PresetMCP):
		if last == "self_server" {
			return kindBool, true
		}
		return kindString, true
	}
	if isNamedListKey(segs[0]) {
		if last == "transport" {
			return kindString, true
		}
		return kindBool, true
	}
	kind, ok := showScalarKinds[segs[0]]
	return kind, ok && len(segs) == 1
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
		for _, k := range slices.Sorted(maps.Keys(t)) {
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

// tomlSafeValue prepares a decoded document for TOML encoding: nulls (which
// TOML cannot hold) are dropped and whole-valued floats become integers.
func tomlSafeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			if e != nil {
				out[k] = tomlSafeValue(e)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			if e != nil {
				out = append(out, tomlSafeValue(e))
			}
		}
		return out
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t)
		}
		return t
	}
	return v
}
