// Package versionpatch rewrites the version constraint of one source in
// config.toml text, changing only that value so comments, key order and
// formatting survive. `ai-rulez update --major --write-config` is its caller:
// re-serializing the whole TOML would lose comments, so the line is patched in
// place and the result is checked to differ from the input in that value only.
package versionpatch

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// Tables are the array tables that hold sources with a version constraint.
var Tables = map[string]bool{"includes": true, "installed_skills": true, "skill_sources": true}

var (
	headerRe = regexp.MustCompile(`^\s*\[\[?\s*[A-Za-z0-9_.\-"' ]+\s*\]\]?\s*(#.*)?$`)
	arrayRe  = regexp.MustCompile(`^\s*\[\[\s*([A-Za-z0-9_]+)\s*\]\]\s*(#.*)?$`)
	nameRe   = regexp.MustCompile(`^\s*name\s*=\s*("(?:[^"\\]|\\.)*"|'[^']*')\s*(#.*)?$`)
	keyRe    = regexp.MustCompile(`^(\s*)(version|ref)(\s*=\s*)("(?:[^"\\]|\\.)*"|'[^']*')(\s*(#.*)?)$`)
)

// SetConstraint sets the constraint of the entry called name in [[table]] to
// constraint. The entry's version key is rewritten; when it has none but a ref
// that is the shorthand for a constraint, that ref is rewritten (the shorthand
// stays the shorthand). It fails when the entry is missing or ambiguous, has no
// constraint line to rewrite, or the patched text would differ in anything else.
func SetConstraint(src []byte, table, name, constraint string) ([]byte, error) {
	if !Tables[table] {
		return nil, oops.Errorf("%q is not a table of sources with version constraints", table)
	}
	if strings.ContainsAny(constraint, "\"\\\n\r") || strings.TrimSpace(constraint) == "" {
		return nil, oops.Errorf("refusing to write the constraint %q", constraint)
	}
	lines := strings.SplitAfter(string(src), "\n")
	block, err := findBlock(lines, table, name)
	if err != nil {
		return nil, err
	}
	idx, key, err := findKey(lines, block)
	if err != nil {
		return nil, oops.With("table", table).With("name", name).Wrap(err)
	}
	m := keyRe.FindStringSubmatch(strings.TrimRight(lines[idx], "\r\n"))
	quote := m[4][:1]
	eol := lines[idx][len(strings.TrimRight(lines[idx], "\r\n")):]
	lines[idx] = m[1] + key + m[3] + quote + constraint + quote + m[5] + eol
	out := []byte(strings.Join(lines, ""))
	if err := verify(src, out, table, name, key, constraint); err != nil {
		return nil, err
	}
	return out, nil
}

type span struct{ start, end int } // lines [start, end)

// findBlock locates the [[table]] block whose name is name, and fails when there are none or several.
func findBlock(lines []string, table, name string) (span, error) {
	var found []span
	cur := ""
	start := -1
	closeBlock := func(end int) {
		if cur == table && start >= 0 && blockName(lines[start:end]) == name {
			found = append(found, span{start, end})
		}
	}
	depth := 0 // unclosed `[` of a multi-line array value: its lines are never headers
	for i, l := range lines {
		trimmed := strings.TrimRight(l, "\r\n")
		if depth > 0 || !headerRe.MatchString(trimmed) {
			depth = max(0, depth+bracketDelta(trimmed))
			continue
		}
		closeBlock(i)
		cur, start = "", -1
		if m := arrayRe.FindStringSubmatch(trimmed); m != nil {
			cur, start = m[1], i
		}
	}
	closeBlock(len(lines))
	switch len(found) {
	case 0:
		return span{}, oops.Hint("Edit config.toml by hand: the source is not a [["+table+"]] table with a name key").
			Errorf("no [[%s]] entry named %q in config.toml", table, name)
	case 1:
		return found[0], nil
	}
	return span{}, oops.Errorf("%d [[%s]] entries are named %q in config.toml; edit it by hand", len(found), table, name)
}

// bracketDelta is the net count of `[` opened on a line outside strings and
// comments: a `key = [` line opens an array value, its `]` line closes it.
func bracketDelta(line string) int {
	delta := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return delta
		case c == '[':
			delta++
		case c == ']':
			delta--
		}
	}
	return delta
}

func blockName(block []string) string {
	for _, l := range block {
		if m := nameRe.FindStringSubmatch(strings.TrimRight(l, "\r\n")); m != nil {
			return unquote(m[1])
		}
	}
	return ""
}

func unquote(s string) string {
	if strings.HasPrefix(s, "'") {
		return strings.Trim(s, "'")
	}
	var v string
	if err := toml.Unmarshal([]byte("v = "+s), &struct{ V *string }{&v}); err != nil {
		return strings.Trim(s, `"`)
	}
	return v
}

// findKey returns the line of the version key, else of a ref that is the shorthand for a constraint.
func findKey(lines []string, b span) (idx int, key string, err error) {
	refLine := -1
	for i := b.start; i < b.end; i++ {
		m := keyRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
		if m == nil {
			continue
		}
		switch m[2] {
		case "version":
			return i, "version", nil
		case "ref":
			if strings.ContainsAny(unquote(m[4]), "^~* ") {
				refLine = i
			}
		}
	}
	if refLine >= 0 {
		return refLine, "ref", nil
	}
	return 0, "", oops.Hint("Edit config.toml by hand").Errorf("the entry has no `version = \"...\"` line (or a ref shorthand) to rewrite")
}

// verify checks that out is src with exactly one value changed.
func verify(src, out []byte, table, name, key, constraint string) error {
	var before, after map[string]any
	if err := toml.NewDecoder(bytes.NewReader(src)).Decode(&before); err != nil {
		return oops.Wrapf(err, "parse config.toml")
	}
	if err := toml.NewDecoder(bytes.NewReader(out)).Decode(&after); err != nil {
		return oops.Wrapf(err, "the patched config.toml does not parse")
	}
	entries, _ := before[table].([]any)
	set := 0
	for _, e := range entries {
		if m, ok := e.(map[string]any); ok && m["name"] == name {
			m[key] = constraint
			set++
		}
	}
	if set != 1 || !reflect.DeepEqual(before, after) {
		return oops.Errorf("the patched config.toml would differ from the original in more than the %s of %q; edit it by hand", key, name)
	}
	return nil
}

// Describe is "version = \"^2.0\"" for messages.
func Describe(key, constraint string) string { return fmt.Sprintf("%s = %q", key, constraint) }
