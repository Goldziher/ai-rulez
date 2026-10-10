package migrate

import (
	"regexp"
	"strings"
)

// stmt is one TOML statement: a table header or a key/value pair, with the
// lines it spans. Comment-only and blank lines belong to no statement.
type stmt struct {
	start, end int // line range [start, end)
	header     bool
	array      bool     // [[...]] header
	path       []string // header path
	key        []string // key path of a key/value pair
	table      []string // the table the statement lives in
}

// tomlDoc is a TOML file split into lines plus its statements. The rewrite
// rules edit lines in place and never reformat what they do not touch, so
// comments, ordering and spacing survive.
type tomlDoc struct {
	lines []string
	stmts []stmt
	crlf  bool // the source used CRLF line endings; text() writes them back
}

func parseTOMLDoc(text string) *tomlDoc {
	d := &tomlDoc{crlf: strings.Contains(text, "\r\n")}
	d.lines = strings.Split(strings.TrimRight(text, "\r\n"), "\n")
	if d.crlf {
		for i, l := range d.lines {
			d.lines[i] = strings.TrimSuffix(l, "\r")
		}
	}
	var table []string
	for i := 0; i < len(d.lines); {
		trimmed := strings.TrimSpace(d.lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		end := statementEnd(d.lines, i)
		s := stmt{start: i, end: end}
		if strings.HasPrefix(trimmed, "[") {
			s.header = true
			s.array = strings.HasPrefix(trimmed, "[[")
			s.path = headerPath(trimmed)
			table = s.path
		} else {
			s.key = keyPath(trimmed)
			s.table = table
		}
		d.stmts = append(d.stmts, s)
		i = end
	}
	return d
}

func (d *tomlDoc) text() string {
	if d.crlf {
		return strings.Join(d.lines, "\r\n") + "\r\n"
	}
	return strings.Join(d.lines, "\n") + "\n"
}

// statementEnd returns the index after the last line of the statement that
// starts at line i, following multi-line strings and multi-line arrays.
func statementEnd(lines []string, i int) int {
	var st lexState
	for j := i; j < len(lines); j++ {
		st.scan(lines[j])
		if st.done() {
			return j + 1
		}
	}
	return len(lines)
}

// lexState tracks just enough TOML lexing (strings, comments, bracket depth) to
// find where a statement ends.
type lexState struct {
	depth   int
	inBasic bool // """ multi-line basic string
	inLit   bool // ''' multi-line literal string
}

func (s *lexState) done() bool { return s.depth == 0 && !s.inBasic && !s.inLit }

func (s *lexState) scan(line string) { //nolint:gocyclo // a flat lexer state machine over TOML string forms; splitting it hides the states
	if s.depth == 0 && !s.inBasic && !s.inLit && strings.HasPrefix(strings.TrimSpace(line), "[") && !strings.Contains(line, "=") {
		return // a table header carries no value
	}
	inStr, inLitStr := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case s.inBasic:
			if c == '\\' {
				i++
			} else if strings.HasPrefix(line[i:], `"""`) {
				s.inBasic = false
				i += 2
			}
		case s.inLit:
			if strings.HasPrefix(line[i:], `'''`) {
				s.inLit = false
				i += 2
			}
		case inStr:
			switch c {
			case '\\':
				i++
			case '"':
				inStr = false
			}
		case inLitStr:
			if c == '\'' {
				inLitStr = false
			}
		case strings.HasPrefix(line[i:], `"""`):
			s.inBasic = true
			i += 2
		case strings.HasPrefix(line[i:], `'''`):
			s.inLit = true
			i += 2
		case c == '"':
			inStr = true
		case c == '\'':
			inLitStr = true
		case c == '#':
			return
		case c == '[' || c == '{':
			s.depth++
		case c == ']' || c == '}':
			if s.depth > 0 {
				s.depth--
			}
		}
	}
}

// headerPath splits "[a.b]" or "[[a.b]]" into its key path.
func headerPath(trimmed string) []string {
	t := strings.TrimSpace(stripComment(trimmed))
	t = strings.TrimPrefix(t, "[")
	t = strings.TrimPrefix(t, "[")
	t = strings.TrimSuffix(t, "]")
	t = strings.TrimSuffix(t, "]")
	return splitDotted(t)
}

// keyPath returns the dotted key of a "key = value" line.
func keyPath(trimmed string) []string {
	idx := strings.Index(trimmed, "=")
	if idx < 0 {
		return nil
	}
	return splitDotted(trimmed[:idx])
}

func stripComment(line string) string {
	inStr, inLit := false, false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case inStr:
			switch c {
			case '\\':
				i++
			case '"':
				inStr = false
			}
		case inLit:
			if c == '\'' {
				inLit = false
			}
		case c == '"':
			inStr = true
		case c == '\'':
			inLit = true
		case c == '#':
			return line[:i]
		}
	}
	return line
}

func splitDotted(s string) []string {
	var parts []string
	var cur strings.Builder
	inStr, inLit := false, false
	flush := func() {
		parts = append(parts, strings.TrimSpace(cur.String()))
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr:
			switch {
			case c == '\\' && i+1 < len(s):
				cur.WriteByte(s[i+1])
				i++
			case c == '"':
				inStr = false
			default:
				cur.WriteByte(c)
			}
		case inLit:
			if c == '\'' {
				inLit = false
			} else {
				cur.WriteByte(c)
			}
		case c == '"':
			inStr = true
		case c == '\'':
			inLit = true
		case c == '.':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return parts
}

func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && equalPath(path[:len(prefix)], prefix)
}

const lintTable = "lint"

// isOldRatchetKey reports the pre-v5 spellings of [lint.ratchet]: [lint.budget]
// (4.0) and its replacement [lint.tolerate] (later 4.x).
func isOldRatchetKey(k string) bool { return k == "budget" || k == "tolerate" }

var (
	versionValue = regexp.MustCompile(`^(\s*version\s*=\s*)(?:"[^"]*"|'[^']*'|\d[0-9.]*)(.*)$`)
	budgetHeader = regexp.MustCompile(`^(\s*\[\[?\s*lint\s*\.\s*)(?:budget|tolerate)\b`)
	budgetKey    = regexp.MustCompile(`^(\s*)(?:budget|tolerate)(\s*[.=])`)
	lintBudgetKV = regexp.MustCompile(`^(\s*lint\s*\.\s*)(?:budget|tolerate)\b`)
)

// rootKey finds the root-level statement for key, or -1.
func (d *tomlDoc) rootKey(key string) int {
	for i, s := range d.stmts {
		if s.header {
			return -1
		}
		if len(s.key) > 0 && s.key[0] == key {
			return i
		}
	}
	return -1
}

// setVersion sets the root `version` key and reports the previous value ("" if
// the key was missing, in which case it is inserted).
func (d *tomlDoc) setVersion(to string) (old string, changed bool) {
	i := d.rootKey("version")
	if i < 0 {
		d.insertRootLine("version = " + tomlQuote(to))
		return "", true
	}
	s := d.stmts[i]
	line := d.lines[s.start]
	m := versionValue.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	old = strings.Trim(strings.TrimSpace(strings.SplitN(strings.SplitN(line, "=", 2)[1], "#", 2)[0]), `"' `)
	if old == to {
		return old, false
	}
	d.lines[s.start] = m[1] + tomlQuote(to) + m[2]
	return old, true
}

// insertRootLine inserts a line after the last root-level statement.
func (d *tomlDoc) insertRootLine(lines ...string) {
	at := 0
	for _, s := range d.stmts {
		if s.header {
			break
		}
		at = s.end
	}
	d.insertLines(at, lines...)
}

func (d *tomlDoc) insertLines(at int, newLines ...string) {
	out := make([]string, 0, len(d.lines)+len(newLines))
	out = append(out, d.lines[:at]...)
	out = append(out, newLines...)
	out = append(out, d.lines[at:]...)
	d.lines = out
	d.stmts = parseTOMLDoc(d.text()).stmts
}

// ratchetSpellings lists, in document order, the distinct spellings of the lint
// ratchet table in use: "budget", "tolerate" and "ratchet" (any of the header,
// dotted-in-table and dotted-at-root forms).
func (d *tomlDoc) ratchetSpellings() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range d.stmts {
		name := ratchetSpelling(s)
		if (isOldRatchetKey(name) || name == "ratchet") && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// ratchetSpelling is the name that a statement gives the table below [lint]
// in its header, dotted-in-table or dotted-at-root form, or "".
func ratchetSpelling(s stmt) string {
	switch {
	case s.header && len(s.path) >= 2 && s.path[0] == lintTable:
		return s.path[1]
	case !s.header && len(s.table) == 1 && s.table[0] == lintTable && len(s.key) > 0:
		return s.key[0]
	case !s.header && len(s.table) == 0 && len(s.key) > 1 && s.key[0] == lintTable:
		return s.key[1]
	}
	return ""
}

// renameLintBudget renames `[lint.budget]` and `[lint.tolerate]` (and dotted spellings) to
// `[lint.ratchet]` and reports how many lines changed.
func (d *tomlDoc) renameLintBudget() int { //nolint:gocyclo // the three spellings of the table (header, dotted-in-table, dotted-at-root) in one place
	n := 0
	for _, s := range d.stmts {
		line := d.lines[s.start]
		switch {
		case s.header && len(s.path) >= 2 && s.path[0] == lintTable && isOldRatchetKey(s.path[1]):
			d.lines[s.start] = budgetHeader.ReplaceAllString(line, "${1}ratchet")
		case !s.header && len(s.table) == 1 && s.table[0] == lintTable && len(s.key) > 0 && isOldRatchetKey(s.key[0]):
			d.lines[s.start] = budgetKey.ReplaceAllString(line, "${1}ratchet${2}")
		case !s.header && len(s.table) == 0 && len(s.key) > 1 && s.key[0] == lintTable && isOldRatchetKey(s.key[1]):
			d.lines[s.start] = lintBudgetKV.ReplaceAllString(line, "${1}ratchet")
		default:
			continue
		}
		if d.lines[s.start] != line {
			n++
		}
	}
	return n
}

// tableHas reports whether the table at path has a key (or dotted key) k.
func (d *tomlDoc) tableHas(path []string, k string) bool {
	for _, s := range d.stmts {
		if !s.header && equalPath(s.table, path) && len(s.key) > 0 && s.key[0] == k {
			return true
		}
	}
	return false
}

// hasTable reports whether a `[path]` header or a root `path = ...` /
// `path.x = ...` key exists.
func (d *tomlDoc) hasTable(path []string) bool {
	for _, s := range d.stmts {
		if s.header && hasPrefix(s.path, path) {
			return true
		}
		if !s.header && len(s.table) == 0 && hasPrefix(s.key, path) {
			return true
		}
	}
	return false
}

// addToTable inserts a line at the end of the `[path]` table's own statements.
func (d *tomlDoc) addToTable(path []string, line string) bool {
	hdr := -1
	for i, s := range d.stmts {
		if s.header && !s.array && equalPath(s.path, path) {
			hdr = i
			break
		}
	}
	if hdr < 0 {
		return false
	}
	at := d.stmts[hdr].end
	for _, s := range d.stmts[hdr+1:] {
		if s.header {
			break
		}
		at = s.end
	}
	d.insertLines(at, line)
	return true
}

// appendBlock appends text at the end of the file, separated by a blank line.
func (d *tomlDoc) appendBlock(block string) {
	d.lines = append(d.lines, "")
	d.lines = append(d.lines, strings.Split(strings.TrimRight(block, "\n"), "\n")...)
	d.stmts = parseTOMLDoc(d.text()).stmts
}

// replaceAll applies a literal replacement to every line outside comments and
// reports how many lines changed.
func (d *tomlDoc) replaceAll(oldStr, newStr string) int {
	n := 0
	for i, l := range d.lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") || !strings.Contains(l, oldStr) {
			continue
		}
		d.lines[i] = strings.ReplaceAll(l, oldStr, newStr)
		n++
	}
	return n
}

// presetsStmt is the root `presets` statement, or -1.
func (d *tomlDoc) presetsStmt() int { return d.rootKey("presets") }

// renamePreset renames a built-in preset in the root `presets` array and
// reports how many lines changed.
func (d *tomlDoc) renamePreset(oldName, newName string) int {
	i := d.presetsStmt()
	if i < 0 {
		return 0
	}
	n := 0
	for l := d.stmts[i].start; l < d.stmts[i].end; l++ {
		line := d.lines[l]
		for _, q := range []string{`"`, `'`} {
			line = strings.ReplaceAll(line, q+oldName+q, q+newName+q)
		}
		if line != d.lines[l] {
			d.lines[l] = line
			n++
		}
	}
	return n
}

// dropPreset removes a built-in preset from the root `presets` array and
// reports how many lines changed.
func (d *tomlDoc) dropPreset(name string) int {
	i := d.presetsStmt()
	if i < 0 {
		return 0
	}
	n := 0
	for l := d.stmts[i].start; l < d.stmts[i].end; l++ {
		line := d.lines[l]
		for _, q := range []string{`"`, `'`} {
			item := q + name + q
			line = strings.Replace(line, item+", ", "", 1)
			line = strings.Replace(line, ", "+item, "", 1)
			line = strings.Replace(line, item+",", "", 1)
			line = strings.Replace(line, item, "", 1)
		}
		if line != d.lines[l] {
			d.lines[l] = line
			n++
		}
	}
	return n
}

// dropArrayTable removes every [[name]] array-of-table block (the header and the
// key/value lines that belong to it) and returns how many blocks it removed. It
// is how migrate v5 drops a v4 `[[skills]]` array, which v5 rejects: skills are
// discovered from .ai-rulez/skills/<name>/SKILL.md.
func (d *tomlDoc) dropArrayTable(name string) int {
	drop := map[int]bool{}
	blocks := 0
	for i, s := range d.stmts {
		if !s.header || !s.array || len(s.path) != 1 || s.path[0] != name {
			continue
		}
		blocks++
		for l := s.start; l < s.end; l++ {
			drop[l] = true
		}
		for k := i + 1; k < len(d.stmts); k++ {
			n := d.stmts[k]
			if n.header {
				break
			}
			if len(n.table) == 1 && n.table[0] == name {
				for l := n.start; l < n.end; l++ {
					drop[l] = true
				}
			}
		}
	}
	if blocks == 0 {
		return 0
	}
	kept := make([]string, 0, len(d.lines))
	for l, line := range d.lines {
		if !drop[l] {
			kept = append(kept, line)
		}
	}
	d.lines = kept
	d.stmts = parseTOMLDoc(strings.Join(kept, "\n")).stmts
	return blocks
}
