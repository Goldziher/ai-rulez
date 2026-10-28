package exposure

import (
	"bytes"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	settingsShared = "settings.json"
	settingsLocal  = "settings.local.json"
)

// The skillOverrides values the load model acts on; any other value leaves the
// skill as its frontmatter has it.
const (
	OverrideOn                = "on"
	OverrideNameOnly          = "name-only"
	OverrideUserInvocableOnly = "user-invocable-only"
	OverrideOff               = "off"
)

func isSettingsFile(name string) bool { return name == settingsShared || name == settingsLocal }

// settingsFile is a decoded .claude/settings*.json.
type settingsFile struct {
	enabled   []keyBool
	overrides map[string]string
	excludes  []string
}

type keyBool struct {
	key string
	on  bool
}

// errNotObject is the error of a JSON value that must be an object.
var errNotObject = errors.New("not a JSON object")

// The settings and .mcp.json keys the load model reads.
const (
	jsonEnabledPlugins   = "enabledPlugins"
	jsonSkillOverrides   = "skillOverrides"
	jsonClaudeMdExcludes = "claudeMdExcludes"
	jsonMCPServers       = "mcpServers"
)

// topLevel decodes a JSON document into its top-level keys. Keys are matched
// exactly afterwards, as Claude Code reads them: encoding/json would match a
// struct field in any case, so `SkillOverrides` would count.
func topLevel(data string) (map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &top); err != nil {
		return nil, err //nolint:wrapcheck // reported with the path
	}
	return top, nil
}

// parseSettings decodes the settings keys the load model reads. A key of the
// wrong type makes the whole file unreadable, so none of it applies.
func parseSettings(data string) (*settingsFile, error) {
	top, err := topLevel(data)
	if err != nil {
		return nil, err
	}
	s := &settingsFile{}
	if raw, ok := top[jsonSkillOverrides]; ok {
		if err := json.Unmarshal(raw, &s.overrides); err != nil {
			return nil, errors.New(jsonSkillOverrides + ": " + err.Error())
		}
	}
	if raw, ok := top[jsonClaudeMdExcludes]; ok {
		if err := json.Unmarshal(raw, &s.excludes); err != nil {
			return nil, errors.New(jsonClaudeMdExcludes + ": " + err.Error())
		}
	}
	keys, values, err := orderedObject(top[jsonEnabledPlugins])
	if err != nil {
		return nil, errors.New(jsonEnabledPlugins + ": " + err.Error())
	}
	for i, k := range keys {
		var on bool
		if err := json.Unmarshal(values[i], &on); err != nil {
			return nil, errors.New(jsonEnabledPlugins + ": " + k + " is not a boolean")
		}
		s.enabled = set(s.enabled, keyBool{k, on})
	}
	return s, nil
}

// set replaces the entry of a key (a repeated JSON key: the last one wins) or
// appends it.
func set(list []keyBool, kb keyBool) []keyBool {
	for i := range list {
		if list[i].key == kb.key {
			list[i].on = kb.on
			return list
		}
	}
	return append(list, kb)
}

// orderedObject decodes a JSON object into its keys, in document order, and
// raw values. An absent or null value is an empty object.
func orderedObject(raw json.RawMessage) (keys []string, values []json.RawMessage, err error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, tokErr := dec.Token(); tokErr != nil || tok != json.Delim('{') {
		return nil, nil, errNotObject
	}
	for dec.More() {
		tok, tokErr := dec.Token()
		if tokErr != nil {
			return nil, nil, tokErr //nolint:wrapcheck // reported with the path
		}
		key, _ := tok.(string) //nolint:errcheck // an object key is always a string
		var v json.RawMessage
		if decErr := dec.Decode(&v); decErr != nil {
			return nil, nil, decErr //nolint:wrapcheck // reported with the path
		}
		if at := indexOf(keys, key); at >= 0 {
			values[at] = v
			continue
		}
		keys, values = append(keys, key), append(values, v)
	}
	return keys, values, nil
}

// readSettings decodes every listed .claude/settings*.json and merges the root
// ones, the local file over the shared one: overrides by key, excludes
// concatenated, enabled plugins by key.
func (b *builder) readSettings() {
	b.settings = map[string]*settingsFile{}
	for _, file := range b.files {
		if pruned(file) || !isSettingsFile(path.Base(file)) || path.Base(path.Dir(file)) != claudeDir {
			continue
		}
		r := b.read(file)
		if r.status != readOK {
			continue
		}
		s, err := parseSettings(r.text)
		if err != nil {
			b.problem(file, err.Error())
			continue
		}
		b.settings[file] = s
	}
	m := mergedSettings{overrides: map[string]string{}, enabled: map[string]bool{}}
	for _, name := range []string{settingsShared, settingsLocal} {
		s := b.settings[claudeDir+"/"+name]
		if s == nil {
			continue
		}
		for k, v := range s.overrides {
			m.overrides[k] = v
		}
		m.excludes = append(m.excludes, s.excludes...)
		for _, kb := range s.enabled {
			m.enabled[kb.key] = kb.on
		}
	}
	for _, pattern := range m.excludes {
		m.excludeRes = append(m.excludeRes, excludeRegexp(pattern))
	}
	b.rootSettings = m
}

// mergedSettings are the root project settings after the merge.
type mergedSettings struct {
	overrides  map[string]string
	excludes   []string
	excludeRes []*regexp.Regexp
	enabled    map[string]bool
}

func (m *mergedSettings) summary() Settings {
	s := Settings{SkillOverrides: m.overrides, ClaudeMdExcludes: m.excludes, EnabledPlugins: []string{}}
	if s.ClaudeMdExcludes == nil {
		s.ClaudeMdExcludes = []string{}
	}
	for k, on := range m.enabled {
		if on {
			s.EnabledPlugins = append(s.EnabledPlugins, k)
		}
	}
	sort.Strings(s.EnabledPlugins)
	return s
}

func (m *mergedSettings) excluded(abs string) bool {
	for _, re := range m.excludeRes {
		if re.MatchString(abs) {
			return true
		}
	}
	return false
}

// excludeRegexp compiles a claudeMdExcludes glob, matched against a whole
// absolute path: `**/` is any run of directories (none included), `**` any
// text, `*` and `?` stay inside one path segment, the rest is literal.
func excludeRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`\A(?:`)
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString(`(?:.*/)?`)
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(`.*`)
			i += 2
		case pattern[i] == '*':
			b.WriteString(`[^/]*`)
			i++
		case pattern[i] == '?':
			b.WriteString(`[^/]`)
			i++
		default:
			_, size := utf8.DecodeRuneInString(pattern[i:])
			b.WriteString(regexp.QuoteMeta(pattern[i : i+size]))
			i += size
		}
	}
	b.WriteString(`)\z`)
	return regexp.MustCompile(b.String())
}

// settingsArtifacts lists a settings file and the plugins it enables, as
// Claude Code merges the pair: a plugin the sibling settings.local.json turns
// off is not listed, and a plugin both files enable is the shared file's to
// list. settings.local.json is machine-local and usually untracked, so a
// finding on the plugin is then on the tracked file every checkout has; the
// local file lists only the plugins that it alone enables.
func (b *builder) settingsArtifacts(file, scope string) {
	b.artifacts = append(b.artifacts, Artifact{Kind: KindSettings, Source: SourceFile, Name: path.Base(file), Path: file, Scope: scope, Visibility: VisibilityAlways})
	s := b.settings[file]
	if s == nil {
		return
	}
	var shared, local *settingsFile
	switch path.Base(file) {
	case settingsShared:
		local = b.settings[path.Join(path.Dir(file), settingsLocal)]
	case settingsLocal:
		shared = b.settings[path.Join(path.Dir(file), settingsShared)]
	}
	for _, kb := range s.enabled {
		if !kb.on {
			continue
		}
		if on, set := local.value(kb.key); set && !on {
			continue
		}
		if on, _ := shared.value(kb.key); on {
			continue
		}
		b.artifacts = append(b.artifacts, Artifact{Kind: KindPlugin, Source: SourceFile, Name: kb.key, Path: file, Scope: scope, Visibility: VisibilityAlways, Text: kb.key})
	}
}

// value is what the file sets key to in enabledPlugins; set is false when it
// does not name the key, or the file is nil.
func (s *settingsFile) value(key string) (on, set bool) {
	if s == nil {
		return false, false
	}
	for _, kb := range s.enabled {
		if kb.key == key {
			return kb.on, true
		}
	}
	return false, false
}

// applySettings drops the CLAUDE.md files and rules claudeMdExcludes matches
// and applies skillOverrides to the repository skills.
func (b *builder) applySettings() []Artifact {
	out := make([]Artifact, 0, len(b.artifacts))
	for i := range b.artifacts {
		a := b.artifacts[i]
		if (a.Kind == KindClaudeMD || a.Kind == KindRule) && b.rootSettings.excluded(b.absRoot+"/"+a.Path) {
			continue
		}
		if a.Kind == KindSkill {
			var keep bool
			if a, keep = applyOverride(a, b.rootSettings.overrides[a.Name]); !keep {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// applyOverride applies one skillOverrides value; keep is false for "off".
func applyOverride(a Artifact, override string) (Artifact, bool) {
	switch override {
	case OverrideOff:
		return a, false
	case OverrideUserInvocableOnly:
		a.Visibility = VisibilityManualOnly
		a.Phase = ""
	case OverrideNameOnly:
		a.ListingText, a.Text, a.Truncated = a.Name, a.Name, false
	}
	return a, true
}

// mcpServers lists the servers of a .mcp.json. A file that does not decode,
// or names a server that is not an object, lists none.
func (b *builder) mcpServers(file, scope string) {
	r := b.read(file)
	if r.status != readOK {
		return
	}
	top, err := topLevel(r.text)
	if err != nil {
		b.problem(file, err.Error())
		return
	}
	names, values, err := orderedObject(top[jsonMCPServers])
	if err != nil {
		b.problem(file, jsonMCPServers+": "+err.Error())
		return
	}
	for i, name := range names {
		if v := bytes.TrimSpace(values[i]); len(v) == 0 || v[0] != '{' {
			b.problem(file, jsonMCPServers+": "+name+" is "+errNotObject.Error())
			return
		}
	}
	for _, name := range names {
		b.artifacts = append(b.artifacts, Artifact{Kind: KindMCPServer, Source: SourceFile, Name: name, Path: file, Scope: scope, Visibility: VisibilityAlways, Text: name})
	}
}
