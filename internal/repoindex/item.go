package repoindex

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
)

// Hash is the content hash an item carries: "sha256:" and the hex digest.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// newItem builds the item a candidate and its content describe.
func newItem(c *candidate, content []byte) Item {
	block := frontmatter.Split(content)
	var front map[string]any
	if block.Closed {
		_ = yaml.Unmarshal([]byte(block.Raw), &front) //nolint:errcheck // malformed frontmatter only costs the description
	}
	it := Item{
		Kind: c.kind, Scope: c.scope, Source: c.source, Path: c.unit,
		Hash: Hash(content),
	}
	switch c.kind {
	case KindSkill:
		it.Name = stringOf(front["name"])
		if it.Name == "" {
			it.Name = path.Base(c.unit)
		}
	case KindInstruction:
		it.Name = path.Base(c.file)
	default:
		it.Name = safeName(c.rel)
	}
	desc := stringOf(front["description"])
	if desc == "" {
		desc = firstHeading(block.Body)
	}
	it.Description = boundText(desc, maxDescriptionBytes)
	if c.kind == KindRule {
		it.Only = ruleGlobs(front)
	}
	return it
}

// ruleGlobs reads the path restriction of a rule from its frontmatter: `paths`
// (Claude Code) and `globs` (Cursor, ignored for a rule that is always applied).
func ruleGlobs(front map[string]any) *PathGlobs {
	g := &PathGlobs{Paths: globList(front["paths"])}
	if always, _ := front["alwaysApply"].(bool); !always { //nolint:errcheck // absent or not a bool means not always applied
		g.Globs = globList(front["globs"])
	}
	if len(g.Paths) == 0 && len(g.Globs) == 0 {
		return nil
	}
	return g
}

// globList reads a frontmatter value that is one string or a list of strings,
// each holding one glob or several separated by commas outside braces.
func globList(v any) []string {
	var raw []string
	switch t := v.(type) {
	case string:
		raw = []string{t}
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	return config.NormalizeGlobs(raw)
}

func stringOf(v any) string {
	s, _ := v.(string) //nolint:errcheck // a non-string value is no description or name
	return strings.TrimSpace(s)
}

// firstHeading is the text of the first markdown heading of a body.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

// boundText collapses whitespace and cuts s to at most n bytes without splitting a rune.
func boundText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// safeName keeps an item name usable in a URI: letters, digits, dot, dash and
// underscore; every other run becomes a dash.
func safeName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
		switch {
		case ok:
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func pathExt(p string) string { return path.Ext(p) }
