package generator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets" // Register remaining legacy preset generators
)

func (g *Generator) ensureSecretOutputsIgnored(outputs []config.OutputFile) error {
	unsafe, shared := g.unignoredSecretOutputs(outputs)
	if len(unsafe) == 0 {
		return nil
	}
	return oops.
		With("paths", unsafe).
		With("env_keys", g.secretMCPEnvKeys()).
		Hint(g.secretIgnoreHint(unsafe, shared)).
		Errorf("generated MCP config contains secrets but is not gitignored: %s", strings.Join(unsafe, ", "))
}

// reasonSecretUnignored is why generate refuses an MCP config ensureSecretOutputsIgnored rejects.
const reasonSecretUnignored = "an MCP config holding a secret that is not gitignored"

// secretRefusals is what ensureSecretOutputsIgnored would refuse, in the form
// --check and --dry-run report as blocked.
func (g *Generator) secretRefusals(outputs []config.OutputFile) []outputRefusal {
	unsafe, _ := g.unignoredSecretOutputs(outputs)
	refused := make([]outputRefusal, 0, len(unsafe))
	for _, rel := range unsafe {
		refused = append(refused, outputRefusal{rel, reasonSecretUnignored})
	}
	return refused
}

// unignoredSecretOutputs lists the MCP configs that would hold a resolved secret
// and are not gitignored, sorted, and whether one of them is a partially owned
// (hand-authored) document.
func (g *Generator) unignoredSecretOutputs(outputs []config.OutputFile) (unsafe []string, shared bool) {
	if len(g.secretMCPEnvKeys()) == 0 {
		return nil, false
	}

	var pending []string
	if g.config.ShouldUpdateGitignore() {
		pending = g.pendingIgnorePatterns(outputs)
	}

	var candidates []string
	// A file needs ignoring because of what it holds, not because of its name: a
	// merged document that is not an MCP config (a check file) or one that only
	// references the secret by name passes.
	secrets := g.newSecretMatcher()
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir {
			continue
		}
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if !isMCPConfigOutput(relPath) || !secrets.holdsSecret(output, relPath) {
			continue
		}
		candidates = append(candidates, relPath)
	}
	ignored := g.ignoredSet(candidates, pending)
	for _, relPath := range candidates {
		if !ignored[relPath] {
			unsafe = append(unsafe, relPath)
		}
	}
	// A partially owned document is the consumer's file: ai-rulez merges one key
	// into it and cannot gitignore it on their behalf, so --gitignore is not the
	// remedy and the hint must not suggest it.
	for _, output := range outputs {
		if output.PartiallyOwned && !output.IsDir {
			relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
			shared = shared || (isMCPConfigOutput(relPath) && !ignored[relPath])
		}
	}
	sort.Strings(unsafe)
	return unsafe, shared
}

// secretIgnoreHint tells how to fix MCP configs that hold secrets and are not
// ignored. handAuthored marks a partially owned document, which ai-rulez cannot
// gitignore on the consumer's behalf, so --gitignore is not the remedy there.
func (g *Generator) secretIgnoreHint(unsafe []string, handAuthored bool) string {
	switch {
	case handAuthored:
		return "These files hold hand-authored settings alongside the generated mcpServers key, so ai-rulez will " +
			"not gitignore them for you. Either ignore them yourself, or move the secret-bearing server into a " +
			"config whose MCP output is not shared."
	case g.sharedWithTeam(unsafe):
		return "These MCP config files are shared with your team (committed) but would carry secrets. Add them to " +
			".gitignore, or move the secret-bearing server out of config.local.* and out of the shared config."
	}
	return "Enable gitignore generation with --gitignore or add these generated MCP config paths to .gitignore"
}

// sharedWithTeam reports whether any of the project-relative paths is a file the
// shared baseline also produces, that is, one teammates generate and commit.
func (g *Generator) sharedWithTeam(rels []string) bool {
	if g.plan == nil {
		return false
	}
	for _, rel := range rels {
		if slices.Contains(g.plan.baselineFiles, rel) || slices.Contains(g.plan.drift, rel) {
			return true
		}
	}
	return false
}

// sensitiveFileMode is the mode of generated files that carry MCP secrets.
const sensitiveFileMode os.FileMode = 0o600

// minSecretMatchLen is the shortest secret value matched against output content.
// A shorter value ("1", "dev") would match unrelated files and tighten their
// permissions for no benefit; secret key names are handled separately and always
// count.
const minSecretMatchLen = 8

// secretMCPValues lists the resolved values of MCP env entries and headers that
// are secret (placeholder-sourced or sensitively named) and long enough to match
// output content reliably.
func (g *Generator) secretMCPValues() []string {
	var values []string
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		for _, key := range server.SecretEnvKeys {
			if v := server.Env[key]; len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
		for _, key := range server.SecretHeaderKeys {
			if v := server.Headers[key]; len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
		for _, v := range literalSecrets(server) {
			if len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
	}
	return values
}

// markSensitiveOutputs flags every output file whose content contains a resolved
// MCP secret value, whichever preset produced it, so the writer keeps it
// owner-only.
func (g *Generator) markSensitiveOutputs(outputs []config.OutputFile) {
	secrets := g.newSecretMatcher()
	if secrets.empty() {
		return
	}
	for i := range outputs {
		o := &outputs[i]
		if o.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(o.Path)))
		if secrets.holdsSecret(o, rel) {
			o.Sensitive = true
		}
	}
}

// secretMatcher decides from rendered content whether an output holds a resolved
// MCP secret.
type secretMatcher struct {
	values, names, refValues []string
	scopeDirs                []string
}

func (g *Generator) newSecretMatcher() secretMatcher {
	return secretMatcher{
		values: g.secretMCPValues(), names: g.secretMCPNames(), refValues: g.referencedMCPValues(),
		scopeDirs: g.scopeDirs(),
	}
}

func (m secretMatcher) empty() bool {
	return len(m.values) == 0 && len(m.names) == 0 && len(m.refValues) == 0
}

// holdsSecret reports whether o, the output at project-relative path rel, writes a
// resolved secret value. An MCP config file naming a secret key is sensitive
// whatever the value's length, so a short secret cannot loosen it.
func (m secretMatcher) holdsSecret(o *config.OutputFile, rel string) bool {
	if m.empty() {
		return false
	}
	if outputContainsAny(o, m.values) {
		return true
	}
	return isMCPConfigOutputIn(rel, m.scopeDirs) && (outputContainsAny(o, m.names) || outputContainsAny(o, m.refValues))
}

// outputContainsAny reports whether the output's content holds any of the strings.
// Each needle is also tried in the escaped forms JSON, TOML and YAML encoders
// write (a secret with a quote or backslash never appears raw in such a file).
func outputContainsAny(o *config.OutputFile, needles []string) bool {
	for _, n := range needles {
		for _, form := range secretForms(n) {
			if strings.Contains(o.Content, form) || (o.RawContent != nil && bytes.Contains(o.RawContent, []byte(form))) {
				return true
			}
		}
	}
	return false
}

// secretForms lists the spellings of v a structured document may hold: raw, JSON
// string content (both with and without HTML escaping), Go/TOML basic-string
// escapes, and a YAML single-quoted scalar.
func secretForms(v string) []string {
	if v == "" {
		return nil
	}
	forms := []string{v}
	add := func(form string) {
		if !slices.Contains(forms, form) {
			forms = append(forms, form)
		}
	}
	if b, err := json.Marshal(v); err == nil {
		add(string(b[1 : len(b)-1]))
	}
	var plain bytes.Buffer
	enc := json.NewEncoder(&plain)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) == nil {
		if s := strings.TrimSuffix(plain.String(), "\n"); len(s) >= 2 {
			add(s[1 : len(s)-1])
		}
	}
	if q := strconv.Quote(v); len(q) >= 2 {
		add(q[1 : len(q)-1])
	}
	add(strings.ReplaceAll(v, "'", "''"))
	return forms
}

// secretMCPNames lists the names of secret MCP env entries and headers, plus the
// URL and flag credentials of servers, which an MCP config holds whatever their length.
func (g *Generator) secretMCPNames() []string {
	var names []string
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		// A key written as an environment reference holds no secret in the file,
		// so its name alone does not make the file sensitive (see referencedMCPValues).
		for _, key := range server.SecretEnvKeys {
			if _, ref := server.EnvRefs[key]; !ref {
				names = append(names, key)
			}
		}
		for _, key := range server.SecretHeaderKeys {
			if _, ref := server.HeaderRefs[key]; !ref {
				names = append(names, key)
			}
		}
		names = append(names, literalSecrets(server)...)
	}
	return names
}

// referencedMCPValues lists the resolved values, of any length, of secret keys that
// a tool may be given as an environment reference. A config file is sensitive when
// it holds one verbatim as a whole quoted string (the tool did not support
// references), not when it only names the key. Quoting keeps a short value from
// matching unrelated text.
func (g *Generator) referencedMCPValues() []string {
	var values []string
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		for _, key := range server.SecretEnvKeys {
			if _, ref := server.EnvRefs[key]; ref && server.Env[key] != "" {
				values = append(values, quotedForms(server.Env[key])...)
			}
		}
		for _, key := range server.SecretHeaderKeys {
			if _, ref := server.HeaderRefs[key]; ref && server.Headers[key] != "" {
				values = append(values, quotedForms(server.Headers[key])...)
			}
		}
	}
	return values
}

func quotedForms(v string) []string {
	return []string{`"` + v + `"`, `'` + v + `'`}
}

func (g *Generator) secretMCPEnvKeys() []string {
	keys := make(map[string]bool)
	for name, server := range g.config.MCPServers {
		if hasLiteralSecrets(server) {
			keys["url or args of "+name] = true
		}
		for _, key := range server.SecretEnvKeys {
			keys[key] = true
		}
		for _, key := range server.SecretHeaderKeys {
			keys["header:"+key] = true
		}
	}
	return sortedMapKeys(keys)
}

// legacyMCPConfigPaths are the MCP config files of the Go-implemented presets.
var legacyMCPConfigPaths = [...]string{
	".mcp.json", "opencode.json", ".claude/settings.json", ".gemini/settings.json",
	".agents/settings.json", ".xum/mcp.jsonc", ".pi/mcp.json",
}

// mergedMCPDocumentPaths is every merged document a preset writes, less the hooks
// documents, which never hold MCP servers.
func mergedMCPDocumentPaths() []string {
	var paths []string
	for _, p := range presets.MergedDocumentPaths() {
		if p != presets.MergedDocCodexHooks && p != presets.MergedDocCursorHooks &&
			p != presets.MergedDocAntigravityHooks && p != presets.MergedDocDevinHooks {
			paths = append(paths, p)
		}
	}
	return paths
}

// isMCPConfigOutput reports whether relPath, relative to the project root, is an
// MCP config file a preset writes.
func isMCPConfigOutput(relPath string) bool { return isMCPConfigOutputIn(relPath, nil) }

// isMCPConfigOutputIn is isMCPConfigOutput for a project with scoped output roots:
// besides the project root, each scope directory (relative to it) holds the same
// layout. Matching is exact; a path nested at any other depth is not a config file
// of this project.
func isMCPConfigOutputIn(relPath string, scopeDirs []string) bool {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "./")
	bases := []string{""}
	for _, dir := range scopeDirs {
		if dir = strings.Trim(filepath.ToSlash(dir), "/"); dir != "" && dir != "." {
			bases = append(bases, dir+"/")
		}
	}
	for _, base := range bases {
		rest, ok := strings.CutPrefix(relPath, base)
		if !ok {
			continue
		}
		if slices.Contains(legacyMCPConfigPaths[:], rest) || slices.Contains(mergedMCPDocumentPaths(), rest) ||
			slices.Contains(specMCPConfigPaths(), rest) {
			return true
		}
	}
	return false
}

// scopeDirs lists the scoped output roots of the project, relative to its base.
func (g *Generator) scopeDirs() []string {
	dirs := make([]string, 0, len(g.config.Scopes))
	for _, scope := range g.config.Scopes {
		dirs = append(dirs, scope.Path)
	}
	return dirs
}
