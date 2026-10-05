package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CodePluginManifest reports a plugin or marketplace manifest the Claude Code
// loader rejects or tolerates with a warning.
const CodePluginManifest = "AR963"

func init() {
	registerRules(RuleInfo{CodePluginManifest, "plugin-manifest-invalid", SeverityError, "a .claude-plugin/plugin.json or marketplace.json breaks the documented schema (required or reserved names, non-./ paths, wrong types, unknown fields) or a shell-form plugin hook leaves ${CLAUDE_PLUGIN_ROOT} unquoted"})
	registerRunCheck(checkPluginManifests)
}

var (
	kebabRe  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

	pluginKeys = []string{
		"$schema", keyName, "version", keyDescription, "author", "homepage", "repository", keyLicense, keyKeywords,
		keyCommands, keyAgents, keySkills, keyHooks, keyMCPServers, "outputStyles", "lspServers", "userConfig", "channels", "dependencies", "monitors", "themes",
	}
	pluginPathKeys = []string{keyCommands, keyAgents, keySkills, keyHooks, keyMCPServers, "outputStyles", "lspServers"}
	// Marketplace names Anthropic reserves (plugins reference, marketplace schema).
	reservedMarketplaceNames = []string{
		"claude-code-marketplace", "claude-code-plugins", "claude-plugins-official", "anthropic-marketplace",
		"anthropic-plugins", "agent-skills", "knowledge-work-plugins", "life-sciences",
	}
	marketplaceKeys = []string{"$schema", keyName, "owner", "plugins", keyMetadata, keyDescription, "version", "allowCrossMarketplaceDependenciesOn"}
	entryKeys       = append([]string{"source", "strict", keyCategory, "tags", "defaultEnabled", "relevance"}, pluginKeys...)
)

func checkPluginManifests(r *runner) {
	var pluginDirs []string
	for _, rel := range r.tree.Paths() {
		dir, base := path.Dir(rel), path.Base(rel)
		abs := filepath.Join(r.tree.Top, filepath.FromSlash(rel))
		switch {
		case path.Base(dir) == ".claude-plugin" && base == "plugin.json":
			r.checkPluginJSON(abs)
			pluginDirs = append(pluginDirs, filepath.Dir(filepath.Dir(abs)))
		case path.Base(dir) == ".claude-plugin" && base == "marketplace.json":
			r.checkMarketplaceJSON(abs)
		case base == "hooks.json" && path.Base(dir) == keyHooks:
			r.checkPluginHookQuoting(abs)
		}
	}
	if r.opts.External {
		sort.Strings(pluginDirs)
		for _, dir := range pluginDirs {
			r.delegatePluginValidate(dir)
		}
	}
}

func (r *runner) readManifest(abs string) (m map[string]json.RawMessage, lines []string, ok bool) {
	data, err := readSmallFile(abs)
	if err != nil {
		return nil, nil, false
	}
	lines = r.fileLines(abs)
	if json.Unmarshal(data, &m) != nil {
		r.add(CodePluginManifest, abs, 1, "the manifest is not valid JSON")
		return nil, nil, false
	}
	return m, lines, true
}

func rawString(raw json.RawMessage) (string, bool) {
	var s string
	err := json.Unmarshal(raw, &s)
	return s, err == nil
}

func (r *runner) checkPluginJSON(abs string) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	m, lines, ok := r.readManifest(abs)
	if !ok {
		return
	}
	bad := func(key, format string, args ...any) {
		r.add(CodePluginManifest, abs, lineContaining(lines, quoteNeedle(key)), "%s", key+": "+fmt.Sprintf(format, args...))
	}
	if raw, has := m[keyName]; !has {
		r.add(CodePluginManifest, abs, 1, "name is required")
	} else if s, isStr := rawString(raw); !isStr || !kebabRe.MatchString(s) {
		bad(keyName, "must be a kebab-case string (lowercase letters, digits and hyphens, no spaces)")
	}
	if raw, has := m["version"]; has {
		if s, isStr := rawString(raw); !isStr || !semverRe.MatchString(s) {
			bad("version", "must be a semantic version string such as 1.2.3")
		}
	}
	for _, k := range []string{keyDescription, "homepage", keyLicense} {
		if raw, has := m[k]; has {
			if _, isStr := rawString(raw); !isStr {
				bad(k, "must be a string")
			}
		}
	}
	if raw, has := m["repository"]; has {
		var obj map[string]any
		if _, isStr := rawString(raw); !isStr && json.Unmarshal(raw, &obj) != nil {
			bad("repository", "must be a URL string")
		}
	}
	if raw, has := m["author"]; has {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			bad("author", "must be an object such as {\"name\": \"...\"}")
		} else if anyString(obj[keyName]) == "" {
			bad("author", "needs a \"name\"")
		}
	}
	if raw, has := m[keyKeywords]; has {
		var kw []string
		if json.Unmarshal(raw, &kw) != nil {
			bad(keyKeywords, "must be a list of strings")
		}
	}
	for _, k := range pluginPathKeys {
		if raw, has := m[k]; has {
			r.checkManifestPaths(abs, lines, k, raw)
		}
	}
	for _, k := range sortedRawKeys(m) {
		if !inSet(pluginKeys, k) {
			hint := ""
			if near := closest(k, pluginKeys, 2); near != "" {
				hint = fmt.Sprintf("; did you mean %q?", near)
			}
			r.addSev(SeverityWarning, CodePluginManifest, abs, lineContaining(lines, quoteNeedle(k)), "unknown field %q is ignored at load time%s", k, hint)
		}
	}
	if raw, has := m[keyHooks]; has {
		r.checkInlineHookQuoting(abs, lines, raw)
	}
}

func sortedRawKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkManifestPaths requires component paths to be ./-relative and inside the plugin.
func (r *runner) checkManifestPaths(abs string, lines []string, key string, raw json.RawMessage) {
	var paths []string
	var one string
	var many []string
	switch {
	case json.Unmarshal(raw, &one) == nil:
		paths = []string{one}
	case json.Unmarshal(raw, &many) == nil:
		paths = many
	default:
		return // an inline object (hooks, mcpServers)
	}
	for _, p := range paths {
		switch {
		case !strings.HasPrefix(p, "./"):
			r.add(CodePluginManifest, abs, lineContaining(lines, quoteNeedle(key)), "%s: path %q must start with ./ (relative to the plugin root)", key, p)
		case strings.Contains(p, ".."):
			r.add(CodePluginManifest, abs, lineContaining(lines, quoteNeedle(key)), "%s: path %q must stay inside the plugin", key, p)
		}
	}
}

func (r *runner) checkMarketplaceJSON(abs string) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	m, lines, ok := r.readManifest(abs)
	if !ok {
		return
	}
	at := func(needle string) int { return lineContaining(lines, needle) }
	if raw, has := m[keyName]; !has {
		r.add(CodePluginManifest, abs, 1, "name is required")
	} else if s, isStr := rawString(raw); !isStr || !kebabRe.MatchString(s) {
		r.add(CodePluginManifest, abs, at(`"name"`), "name must be a kebab-case string")
	} else if inSet(reservedMarketplaceNames, s) {
		r.add(CodePluginManifest, abs, at(`"name"`), "the marketplace name %q is reserved for Anthropic", s)
	}
	var owner map[string]any
	if raw, has := m["owner"]; !has || json.Unmarshal(raw, &owner) != nil {
		r.add(CodePluginManifest, abs, at(`"owner"`), "owner must be an object such as {\"name\": \"...\"}")
	} else if anyString(owner[keyName]) == "" {
		r.add(CodePluginManifest, abs, at(`"owner"`), "owner needs a \"name\"")
	}
	var plugins []map[string]json.RawMessage
	if raw, has := m["plugins"]; !has || json.Unmarshal(raw, &plugins) != nil {
		r.add(CodePluginManifest, abs, at(`"plugins"`), "plugins must be a list of plugin entries")
	}
	seen := map[string]bool{}
	for _, p := range plugins {
		name, _ := rawString(p[keyName])
		line := at(quoteNeedle(name))
		switch {
		case name == "" || !kebabRe.MatchString(name):
			r.add(CodePluginManifest, abs, at(`"plugins"`), "a plugin entry needs a kebab-case name")
		case seen[name]:
			r.add(CodePluginManifest, abs, line, "plugin %q is listed twice", name)
		}
		seen[name] = true
		src, has := p["source"]
		var obj map[string]any
		switch s, isStr := rawString(src); {
		case !has:
			r.add(CodePluginManifest, abs, line, "plugin %q has no source", name)
		case isStr && !strings.HasPrefix(s, "./"):
			r.add(CodePluginManifest, abs, line, "plugin %q: source %q must start with ./ or be a source object", name, s)
		case !isStr && json.Unmarshal(src, &obj) != nil:
			r.add(CodePluginManifest, abs, line, "plugin %q: source must be a ./ path or an object", name)
		case !isStr:
			if kind := anyString(obj["source"]); !inSet([]string{"github", "url", "git-subdir", cmdNPM, "git"}, kind) {
				r.add(CodePluginManifest, abs, line, "plugin %q: source.source %q is not one of github, url, git-subdir, npm", name, kind)
			}
		}
		for _, k := range sortedRawKeys(p) {
			if !inSet(entryKeys, k) {
				r.addSev(SeverityWarning, CodePluginManifest, abs, line, "plugin %q: unknown field %q is ignored at load time", name, k)
			}
		}
	}
	for _, k := range sortedRawKeys(m) {
		if !inSet(marketplaceKeys, k) {
			r.addSev(SeverityWarning, CodePluginManifest, abs, at(quoteNeedle(k)), "unknown field %q is ignored at load time", k)
		}
	}
}

var pluginRootRe = regexp.MustCompile(`\$\{?CLAUDE_PLUGIN_ROOT\}?`)

// unquotedPluginRoot reports whether a shell-form command uses the plugin root
// outside double quotes; a plugin installed under a path with a space then splits.
func unquotedPluginRoot(command string) bool {
	for _, loc := range pluginRootRe.FindAllStringIndex(command, -1) {
		inD, inS := false, false
		for _, c := range command[:loc[0]] {
			switch {
			case c == '"' && !inS:
				inD = !inD
			case c == '\'' && !inD:
				inS = !inS
			}
		}
		if !inD && !inS {
			return true
		}
	}
	return false
}

func (r *runner) checkPluginHookQuoting(abs string) {
	data, err := readSmallFile(abs)
	if err != nil {
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return
	}
	if raw, ok := root[keyHooks]; ok {
		r.checkInlineHookQuoting(abs, r.fileLines(abs), raw)
	}
}

func (r *runner) checkInlineHookQuoting(abs string, lines []string, raw json.RawMessage) {
	var byEvent map[string][]struct {
		Hooks []struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &byEvent) != nil {
		return
	}
	for _, event := range sortedEventKeys(byEvent) {
		for _, g := range byEvent[event] {
			for _, h := range g.Hooks {
				if (h.Type == "" || h.Type == hookTypeCommand) && len(h.Args) == 0 && unquotedPluginRoot(h.Command) {
					r.addSev(SeverityWarning, CodePluginManifest, abs, lineContaining(lines, "CLAUDE_PLUGIN_ROOT"),
						"%s hook: ${CLAUDE_PLUGIN_ROOT} is not quoted in a shell-form command; a plugin path with a space splits the command (write \"${CLAUDE_PLUGIN_ROOT}/...\")", event)
				}
			}
		}
	}
}

func sortedEventKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type claudeValidation struct {
	Manifest struct {
		Errors   []claudeIssue `json:"errors"`
		Warnings []claudeIssue `json:"warnings"`
	} `json:"manifest"`
	Contents []struct {
		File     string        `json:"file"`
		Errors   []claudeIssue `json:"errors"`
		Warnings []claudeIssue `json:"warnings"`
	} `json:"contents"`
}

type claudeIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// delegatePluginValidate runs `claude plugin validate --json` on a plugin
// directory and merges its report. It is skipped when the binary is missing,
// times out or prints something unparseable: the built-in checks already ran.
func (r *runner) delegatePluginValidate(dir string) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "plugin", "validate", dir, "--json") //nolint:gosec // bin comes from LookPath and the arguments are fixed; dir is a tracked plugin directory
	cmd.Stdout = &out
	_ = cmd.Run() //nolint:errcheck // a non-zero exit is how it reports errors; the JSON decides
	var rep claudeValidation
	if json.Unmarshal(out.Bytes(), &rep) != nil {
		return
	}
	manifest := filepath.Join(dir, ".claude-plugin", "plugin.json")
	lines := r.fileLines(manifest)
	emit := func(file string, sev Severity, is []claudeIssue) {
		for _, i := range is {
			target, line := manifest, lineContaining(lines, quoteNeedle(strings.SplitN(i.Path, ".", 2)[0]))
			if file != "" && file != manifest {
				target, line = file, 1
			}
			r.addSev(sev, CodePluginManifest, target, line, "claude plugin validate: %s%s", pathPrefix(i.Path), i.Message)
		}
	}
	emit("", SeverityError, rep.Manifest.Errors)
	emit("", SeverityWarning, rep.Manifest.Warnings)
	for _, c := range rep.Contents {
		emit(c.File, SeverityError, c.Errors)
		emit(c.File, SeverityWarning, c.Warnings)
	}
}

func pathPrefix(p string) string {
	if p == "" {
		return ""
	}
	return p + ": "
}

// anyString returns v when it is a string, else "".
func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
