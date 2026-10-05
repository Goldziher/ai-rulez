package tomlmerge

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// The scanner locates statements in a TOML document by byte span so the engine
// can replace, insert and remove exactly the ones ai-rulez owns and leave every
// other byte (comments, blank lines, ordering) alone. It assumes the document
// already parses as TOML (Apply checks that first), so it does not validate; it
// only has to find where each statement starts and ends.

type stmtKind int

const (
	kindKeyValue stmtKind = iota
	kindTable
	kindArrayTable
)

// statement is one table header or key/value pair.
type statement struct {
	kind stmtKind
	// path is the full key path: a header's own path, or a key/value's section
	// path followed by its (possibly dotted) key.
	path []string
	// start and end span the whole statement, from the start of its first line to
	// just after its newline, trailing comment included.
	start, end int
	// valStart and valEnd span a key/value's value text; unused for headers.
	valStart, valEnd int
}

// section is a header and the key/value statements that follow it; sections[0]
// of a document is the root, which has no header.
type section struct {
	header *statement
	stmts  []statement
	// bodyEnd is the end of the section's last statement.
	bodyEnd int
}

func (s section) path() []string {
	if s.header == nil {
		return nil
	}
	return s.header.path
}

type document struct {
	src        string
	sections   []section
	hasComment bool
}

var errScan = errors.New("unrecognized TOML syntax")

type scanner struct {
	src string
	doc *document
}

func scan(src string) (*document, error) {
	doc := &document{src: src, sections: []section{{}}}
	s := &scanner{src: src, doc: doc}
	pos := 0
	for pos < len(src) {
		lineStart := pos
		pos = s.skipInline(pos)
		if pos >= len(src) {
			break
		}
		var err error
		switch src[pos] {
		case '\n':
			pos++
		case '\r':
			pos = s.endOfLine(pos)
		case '#':
			doc.hasComment = true
			pos = s.endOfLine(pos)
		case '[':
			pos, err = s.header(lineStart, pos)
		default:
			pos, err = s.keyValue(lineStart, pos)
		}
		if err != nil {
			return nil, err
		}
	}
	return doc, nil
}

func (s *scanner) header(lineStart, pos int) (int, error) {
	kind := kindTable
	pos++
	if pos < len(s.src) && s.src[pos] == '[' {
		kind = kindArrayTable
		pos++
	}
	path, pos, err := s.key(pos)
	if err != nil {
		return 0, err
	}
	closer := "]"
	if kind == kindArrayTable {
		closer = "]]"
	}
	if !strings.HasPrefix(s.src[pos:], closer) {
		return 0, errScan
	}
	end := s.endOfStatement(pos + len(closer))
	stmt := statement{kind: kind, path: path, start: lineStart, end: end}
	s.doc.sections = append(s.doc.sections, section{header: &stmt, bodyEnd: end})
	return end, nil
}

func (s *scanner) keyValue(lineStart, pos int) (int, error) {
	rel, pos, err := s.key(pos)
	if err != nil {
		return 0, err
	}
	pos = s.skipInline(pos)
	if pos >= len(s.src) || s.src[pos] != '=' {
		return 0, errScan
	}
	pos = s.skipInline(pos + 1)
	valEnd, err := s.value(pos)
	if err != nil {
		return 0, err
	}
	end := s.endOfStatement(valEnd)
	cur := &s.doc.sections[len(s.doc.sections)-1]
	full := append(slices.Clone(cur.path()), rel...)
	cur.stmts = append(cur.stmts, statement{
		kind: kindKeyValue, path: full, start: lineStart, end: end, valStart: pos, valEnd: valEnd,
	})
	cur.bodyEnd = end
	return end, nil
}

// endOfStatement skips the rest of the line after a statement, noting a comment.
func (s *scanner) endOfStatement(pos int) int {
	pos = s.skipInline(pos)
	if pos < len(s.src) && s.src[pos] == '#' {
		s.doc.hasComment = true
	}
	return s.endOfLine(pos)
}

// endOfLine returns the index just after the newline at or following pos.
func (s *scanner) endOfLine(pos int) int {
	if nl := strings.IndexByte(s.src[pos:], '\n'); nl >= 0 {
		return pos + nl + 1
	}
	return len(s.src)
}

// skipInline skips spaces and tabs.
func (s *scanner) skipInline(pos int) int {
	for pos < len(s.src) && (s.src[pos] == ' ' || s.src[pos] == '\t') {
		pos++
	}
	return pos
}

// skipFiller skips whitespace, newlines and comments inside an array or inline
// table.
func (s *scanner) skipFiller(pos int) int {
	for pos < len(s.src) {
		switch s.src[pos] {
		case ' ', '\t', '\r', '\n':
			pos++
		case '#':
			s.doc.hasComment = true
			pos = s.endOfLine(pos)
		default:
			return pos
		}
	}
	return pos
}

// key parses a possibly dotted key starting at pos.
func (s *scanner) key(pos int) (parts []string, next int, err error) {
	parts = nil
	for {
		pos = s.skipInline(pos)
		part, next, err := s.keyPart(pos)
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, part)
		pos = s.skipInline(next)
		if pos < len(s.src) && s.src[pos] == '.' {
			pos++
			continue
		}
		return parts, pos, nil
	}
}

func (s *scanner) keyPart(pos int) (part string, next int, err error) {
	if pos >= len(s.src) {
		return "", 0, errScan
	}
	switch s.src[pos] {
	case '"':
		end, err := s.str(pos)
		if err != nil {
			return "", 0, err
		}
		raw := s.src[pos:end]
		if unquoted, err := strconv.Unquote(raw); err == nil {
			return unquoted, end, nil
		}
		return raw[1 : len(raw)-1], end, nil
	case '\'':
		end, err := s.str(pos)
		if err != nil {
			return "", 0, err
		}
		return s.src[pos+1 : end-1], end, nil
	}
	end := pos
	for end < len(s.src) && isBareKeyChar(s.src[end]) {
		end++
	}
	if end == pos {
		return "", 0, errScan
	}
	return s.src[pos:end], end, nil
}

func isBareKeyChar(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// value returns the end of the value starting at pos.
func (s *scanner) value(pos int) (int, error) {
	if pos >= len(s.src) {
		return 0, errScan
	}
	switch s.src[pos] {
	case '"', '\'':
		return s.str(pos)
	case '[':
		return s.collection(pos, ']', false)
	case '{':
		return s.collection(pos, '}', true)
	}
	end := pos
	for end < len(s.src) && !strings.ContainsRune(",]}#\r\n", rune(s.src[end])) {
		end++
	}
	for end > pos && (s.src[end-1] == ' ' || s.src[end-1] == '\t') {
		end--
	}
	if end == pos {
		return 0, errScan
	}
	return end, nil
}

// collection scans an array or inline table, returning the index after its
// closing bracket.
func (s *scanner) collection(pos int, closer byte, isTable bool) (int, error) {
	pos++
	for {
		pos = s.skipFiller(pos)
		if pos >= len(s.src) {
			return 0, errScan
		}
		if s.src[pos] == closer {
			return pos + 1, nil
		}
		if isTable {
			var err error
			if _, pos, err = s.key(pos); err != nil {
				return 0, err
			}
			pos = s.skipInline(pos)
			if pos >= len(s.src) || s.src[pos] != '=' {
				return 0, errScan
			}
			pos = s.skipInline(pos + 1)
		}
		end, err := s.value(pos)
		if err != nil {
			return 0, err
		}
		pos = s.skipFiller(end)
		if pos < len(s.src) && s.src[pos] == ',' {
			pos++
		}
	}
}

// str returns the end of the string literal starting at pos, in any of the four
// TOML forms.
func (s *scanner) str(pos int) (int, error) {
	quote := s.src[pos]
	basic := quote == '"'
	triple := strings.Repeat(string(quote), 3)
	if strings.HasPrefix(s.src[pos:], triple) {
		for i := pos + 3; i < len(s.src); {
			switch {
			case basic && s.src[i] == '\\':
				i += 2
			case strings.HasPrefix(s.src[i:], triple):
				i += 3
				for extra := 0; extra < 2 && i < len(s.src) && s.src[i] == quote; extra++ {
					i++
				}
				return i, nil
			default:
				i++
			}
		}
		return 0, errScan
	}
	for i := pos + 1; i < len(s.src); i++ {
		switch {
		case basic && s.src[i] == '\\':
			i++
		case s.src[i] == quote:
			return i + 1, nil
		case s.src[i] == '\n':
			return 0, errScan
		}
	}
	return 0, errScan
}

// hasPrefix reports whether path starts with prefix.
func hasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}

// occurrence is one place a key's value lives: a whole section or a key/value.
type occurrence struct {
	start, end int
	isSection  bool
	stmt       *statement
	sec        int
}

// occurrences lists everything that belongs to the value at key: every table
// whose path starts with it and every key/value (outside those tables) whose
// path starts with it, in source order.
func (d *document) occurrences(key []string) []occurrence {
	var found []occurrence
	removed := map[int]bool{}
	for i := 1; i < len(d.sections); i++ {
		sec := d.sections[i]
		if hasPrefix(sec.path(), key) {
			removed[i] = true
			found = append(found, occurrence{start: sec.header.start, end: sec.bodyEnd, isSection: true, sec: i})
		}
	}
	for i := range d.sections {
		if removed[i] {
			continue
		}
		for j := range d.sections[i].stmts {
			stmt := &d.sections[i].stmts[j]
			if hasPrefix(stmt.path, key) {
				found = append(found, occurrence{start: stmt.start, end: stmt.end, stmt: stmt, sec: i})
			}
		}
	}
	slices.SortFunc(found, func(a, b occurrence) int { return a.start - b.start })
	return found
}

// exactValue returns the key/value statement addressing exactly key.
func (d *document) exactValue(key []string) (*statement, bool) {
	for i := range d.sections {
		for j := range d.sections[i].stmts {
			if slices.Equal(d.sections[i].stmts[j].path, key) {
				return &d.sections[i].stmts[j], true
			}
		}
	}
	return nil, false
}

// tableSection returns the index of the [table] section whose path is exactly
// path, or 0 for the root when path is empty.
func (d *document) tableSection(path []string) (int, bool) {
	if len(path) == 0 {
		return 0, true
	}
	for i := 1; i < len(d.sections); i++ {
		h := d.sections[i].header
		if h.kind == kindTable && slices.Equal(h.path, path) {
			return i, true
		}
	}
	return 0, false
}

// lastSectionWithPrefix returns the index of the last table or array-of-tables
// section whose path starts with prefix.
func (d *document) lastSectionWithPrefix(prefix []string) (int, bool) {
	for i := len(d.sections) - 1; i >= 1; i-- {
		if hasPrefix(d.sections[i].path(), prefix) {
			return i, true
		}
	}
	return 0, false
}

// headerLead returns where the comment lines directly above section i's header
// begin (its header start when there are none).
func (d *document) headerLead(i int) int {
	pos := d.sections[i].header.start
	for pos > 0 {
		prev := strings.LastIndexByte(d.src[:pos-1], '\n') + 1
		if !strings.HasPrefix(strings.TrimLeft(d.src[prev:pos], " \t"), "#") {
			break
		}
		pos = prev
	}
	return pos
}

// blockedAncestor returns the length and kind of the first proper prefix of key
// that the document gives as a value (an inline table or an array) or as an
// array of tables, rather than as a plain table. A key below one of those is
// inside a value TOML does not let anything outside extend, or inside one of
// several elements, and ai-rulez cannot address it.
func (d *document) blockedAncestor(key []string) (n int, kind string, ok bool) {
	for n := 1; n < len(key); n++ {
		if stmt, found := d.exactValue(key[:n]); found && stmt.valStart < len(d.src) &&
			(d.src[stmt.valStart] == '{' || d.src[stmt.valStart] == '[') {
			return n, "an inline table or array", true
		}
		for i := 1; i < len(d.sections); i++ {
			if h := d.sections[i].header; h.kind == kindArrayTable && slices.Equal(h.path, key[:n]) {
				return n, "an array of tables", true
			}
		}
	}
	return 0, "", false
}
