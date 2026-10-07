package migrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// commandRewrites renames CLI invocations that moved in v5, inside the hook,
// verifier and script commands a config.toml carries.
var commandRewrites = []struct{ old, new string }{
	{"ai-rulez usage hook", "ai-rulez telemetry hook"},
	{"ai-rulez usage record", "ai-rulez telemetry record"},
	{"ai-rulez usage feedback", "ai-rulez telemetry feedback"},
	{"ai-rulez report usage", "ai-rulez telemetry report"},
	{"ai-rulez report evals", "ai-rulez telemetry report evals"},
}

// checkSourceVersion accepts the versions migrate can read: 4.x, and 5.0 for a
// YAML or JSON file that only needs converting.
func checkSourceVersion(v string) error {
	if v == "" {
		return oops.
			Hint("Add version = \"4.0\" to the configuration and run migrate again").
			Errorf("the configuration has no version key")
	}
	if config.CheckVersion(v) == nil {
		return nil
	}
	if config.IsLegacyVersion(v) && strings.HasPrefix(strings.TrimSpace(v), "4") {
		return nil
	}
	return config.CheckVersion(v)
}

// planMCPMerge folds a legacy mcp.toml/mcp.yaml/mcp.json (the 4.x loader read
// the first of these) into config.toml as [[mcp_servers]] tables.
func planMCPMerge(p *plan, dir string, doc *tomlDoc) error {
	var path string
	for _, n := range []string{"mcp.toml", "mcp.yaml", "mcp.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			path = filepath.Join(dir, n)
			break
		}
	}
	if path == "" {
		return nil
	}
	name := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return oops.With("path", path).Wrapf(err, "read %s", name)
	}
	servers, err := legacyServers(name, data)
	if err != nil {
		return oops.With("path", path).Wrapf(err, "parse %s", name)
	}
	if doc.rootKey("mcp_servers") >= 0 {
		p.warnings = append(p.warnings, name+" was not merged: config.toml defines mcp_servers inline; move the servers over by hand and remove "+name)
		return nil
	}
	existing := map[string]bool{}
	if cfg, err := config.DecodeTOML([]byte(doc.text()), "config.toml"); err == nil {
		for _, s := range cfg.MCPServersRaw {
			existing[s.Name] = true
		}
	}
	var b strings.Builder
	merged := 0
	for _, srv := range servers {
		nameNode := scalarKey(srv, "name")
		if nameNode != "" && existing[nameNode] {
			continue // the 4.x loader kept the config.toml definition
		}
		b.WriteString("[[mcp_servers]]\n")
		if err := emitTable(&b, nameFirst(srv), []string{"mcp_servers"}, false); err != nil {
			return oops.With("path", path).Wrap(err)
		}
		b.WriteString("\n")
		merged++
	}
	file := relFile(dir, "config.toml")
	if merged > 0 {
		doc.appendBlock(strings.TrimRight(b.String(), "\n"))
		p.change(file, RuleMCPMerge, strconv.Itoa(merged)+" server(s) from "+name+" added as [[mcp_servers]]; "+name+" is removed")
	} else {
		p.change(relFile(dir, name), RuleMCPMerge, name+" held no server missing from config.toml; it is removed")
	}
	p.removes = append(p.removes, path)
	return nil
}

// legacyServers parses the mcp_servers list of a legacy file into YAML nodes.
func legacyServers(name string, data []byte) ([]*yaml.Node, error) {
	if strings.HasSuffix(name, ".toml") {
		var m map[string]any
		if err := toml.Unmarshal(data, &m); err != nil {
			return nil, oops.Wrap(err)
		}
		out, err := yaml.Marshal(m)
		if err != nil {
			return nil, oops.Wrap(err)
		}
		data = out
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, oops.Wrap(err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, oops.Errorf("the top level must be a mapping with mcp_servers")
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "mcp_servers" {
			continue
		}
		seq := resolveAlias(top.Content[i+1])
		if seq.Kind != yaml.SequenceNode {
			return nil, oops.Errorf("mcp_servers must be a list")
		}
		var out []*yaml.Node
		for _, c := range seq.Content {
			if c = resolveAlias(c); c.Kind == yaml.MappingNode {
				out = append(out, c)
			}
		}
		return out, nil
	}
	return nil, nil
}

func scalarKey(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return resolveAlias(m.Content[i+1]).Value
		}
	}
	return ""
}

// nameFirst returns m with its name key first (TOML maps decode unordered).
func nameFirst(m *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: m.Kind, Tag: m.Tag, Style: m.Style}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "name" {
			out.Content = append(out.Content, m.Content[i], m.Content[i+1])
		}
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != "name" {
			out.Content = append(out.Content, m.Content[i], m.Content[i+1])
		}
	}
	return out
}

// planLocalOverlay converts config.local.{yaml,yml,json} to config.local.toml
// and keeps the overlay's version (when it states one) equal to the main
// config's, which the loader requires.
func planLocalOverlay(p *plan, dir string) error {
	toml := filepath.Join(dir, "config.local.toml")
	var legacy []string
	for _, n := range []string{"config.local.yaml", "config.local.yml", "config.local.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			legacy = append(legacy, filepath.Join(dir, n))
		}
	}
	_, tomlErr := os.Stat(toml)
	hasTOML := tomlErr == nil
	switch {
	case len(legacy) > 1 || (len(legacy) == 1 && hasTOML):
		p.warnings = append(p.warnings, "more than one config.local.* file exists; keep one (config.local.toml) and migrate again")
		return nil
	case len(legacy) == 1:
		src := legacy[0]
		raw, err := os.ReadFile(src)
		if err != nil {
			return oops.With("path", src).Wrapf(err, "read %s", filepath.Base(src))
		}
		out, err := YAMLToTOML(raw)
		if err != nil {
			return oops.With("path", src).Wrapf(err, "convert %s to TOML", filepath.Base(src))
		}
		d := parseTOMLDoc(string(out))
		bumpLocalVersion(d)
		p.writes = append(p.writes, fileWrite{path: toml, data: []byte(d.text()), perm: 0o600})
		p.removes = append(p.removes, src)
		p.change(relFile(dir, filepath.Base(src)), RuleLocalOverlay, "converted to config.local.toml; "+filepath.Base(src)+" is removed")
	case hasTOML:
		raw, err := os.ReadFile(toml)
		if err != nil {
			return oops.With("path", toml).Wrapf(err, "read config.local.toml")
		}
		d := parseTOMLDoc(string(raw))
		if bumpLocalVersion(d) {
			info, _ := os.Stat(toml)
			p.writes = append(p.writes, fileWrite{path: toml, data: []byte(d.text()), perm: info.Mode().Perm()})
			p.change(relFile(dir, "config.local.toml"), RuleLocalOverlay, "version -> "+config.ConfigVersionV5)
		}
	}
	return nil
}

func bumpLocalVersion(d *tomlDoc) bool {
	if d.rootKey("version") < 0 {
		return false
	}
	_, changed := d.setVersion(config.ConfigVersionV5)
	return changed
}

var frontmatterAliases = []struct {
	re       *regexp.Regexp
	old, new string
}{
	{regexp.MustCompile(`(?m)^permission_mode(\s*:)`), "permission_mode", "permissionMode"},
	{regexp.MustCompile(`(?m)^user_invocable(\s*:)`), "user_invocable", "user-invocable"},
}

// planFrontmatter finds the pre-4.24 frontmatter spellings Claude Code ignores.
// Without --write they are reported; with it the markdown files are rewritten.
func planFrontmatter(p *plan, dir string, write bool) error {
	counts := map[string]int{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped, never fatal
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // see above
		}
		text := string(raw)
		fm, rest, ok := splitFrontmatter(text)
		if !ok {
			return nil
		}
		changed := false
		for _, a := range frontmatterAliases {
			if !a.re.MatchString(fm) {
				continue
			}
			if regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(a.new) + `\s*:`).MatchString(fm) {
				p.warnings = append(p.warnings, relFile(dir, relTo(dir, path))+": has both "+a.old+" and "+a.new+"; remove "+a.old)
				continue
			}
			fm = a.re.ReplaceAllString(fm, a.new+"${1}")
			counts[a.old+" -> "+a.new]++
			changed = true
		}
		if changed && write {
			perm := fs.FileMode(0o644)
			if info, serr := d.Info(); serr == nil {
				perm = info.Mode().Perm()
			}
			p.writes = append(p.writes, fileWrite{path: path, data: []byte(fm + rest), perm: perm})
		}
		return nil
	})
	if err != nil {
		return oops.With("path", dir).Wrapf(err, "scan markdown frontmatter")
	}
	for _, a := range frontmatterAliases {
		key := a.old + " -> " + a.new
		n := counts[key]
		if n == 0 {
			continue
		}
		if write {
			p.change(relFile(dir, "**/*.md"), RuleFrontmatter, key+" in "+plural(n, "file"))
		} else {
			p.warnings = append(p.warnings, plural(n, "markdown file")+" use the frontmatter key "+a.old+", which Claude Code ignores; run `ai-rulez migrate v5 --write` to rename it to "+a.new)
		}
	}
	return nil
}

// splitFrontmatter splits a markdown file into its frontmatter block (through
// the closing delimiter line) and the rest.
func splitFrontmatter(text string) (fm, rest string, ok bool) {
	if !strings.HasPrefix(text, "---\n") {
		return "", "", false
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return "", "", false
	}
	cut := 4 + end + 4
	return text[:cut], text[cut:], true
}
