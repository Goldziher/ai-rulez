package tomlmerge

import (
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

// editor edits a document's source text one statement at a time, rescanning
// after every edit so offsets are never stale.
type editor struct {
	src     string
	newline string
}

func (e *editor) scan() (*document, error) {
	doc, err := scan(e.src)
	if err != nil {
		return nil, oops.Wrapf(err, "scan TOML document")
	}
	return doc, nil
}

// text converts rendered LF text to the document's line ending.
func (e *editor) text(s string) string {
	if e.newline == "\n" {
		return s
	}
	return strings.ReplaceAll(s, "\n", e.newline)
}

func (e *editor) apply(key OwnedKey, segs []string) error {
	switch {
	case key.Remove:
		return e.remove(segs, Claim{})
	case key.Members:
		return e.mergeMembers(segs, key.Value)
	default:
		return e.set(segs, key.Value)
	}
}

// mergeMembers writes each entry of value as its own key under key, leaving the
// entries the document already has under other names.
func (e *editor) mergeMembers(key []string, value any) error {
	names, entries, ok := jsonmerge.MemberEntries(value)
	if !ok {
		doc, err := e.scan()
		if err != nil {
			return err
		}
		if len(doc.occurrences(key)) > 0 {
			return nil
		}
		return e.set(key, value)
	}
	for _, name := range names {
		if err := e.set(append(slices.Clone(key), name), entries[name]); err != nil {
			return err
		}
	}
	return nil
}

func (e *editor) set(key []string, value any) error {
	doc, err := e.scan()
	if err != nil {
		return err
	}
	if n, kind, blocked := doc.blockedAncestor(key); blocked {
		return oops.
			With("key", strings.Join(key, ".")).
			Hint("ai-rulez cannot add a key below "+strings.Join(key[:n], ".")+", which the file writes as "+kind+
				"; write it as a plain [table] instead, or move the file aside").
			Errorf("owned key %s under %s, which is %s", strings.Join(key, "."), strings.Join(key[:n], "."), kind)
	}
	_, isTable := asTable(value)
	_, isTables := asTableArray(value)
	_, inlineNow := doc.exactValue(key)
	switch {
	case isTables && inlineNow:
		// An array the file keeps inline stays inline, not [[tables]].
		return e.setInline(key, value)
	case isTable || isTables:
		return e.setTable(key, value)
	default:
		return e.setInline(key, value)
	}
}

// setTable replaces the table (or array of tables) at key with the rendering of
// value, in the place of the first existing occurrence; with none, a new one is
// added after its siblings.
func (e *editor) setTable(key []string, value any) error {
	blocks, err := tableBlocks(key, value)
	if err != nil {
		return oops.With("key", strings.Join(key, ".")).Wrapf(err, "render TOML table")
	}
	text := strings.Join(blocks, "\n")

	doc, err := e.scan()
	if err != nil {
		return err
	}
	occ := doc.occurrences(key)
	if len(occ) == 0 {
		return e.insertBlock(key[:len(key)-1], text)
	}

	anchor := occ[0]
	for {
		doc, err = e.scan()
		if err != nil {
			return err
		}
		occ = doc.occurrences(key)
		if len(occ) == 0 {
			break
		}
		victim := occ[len(occ)-1]
		if anchor.isSection && victim.start == anchor.start {
			break
		}
		e.removeOccurrence(doc, victim)
	}
	if !anchor.isSection {
		return e.insertBlock(key[:len(key)-1], text)
	}
	e.src = e.src[:anchor.start] + e.text(text) + e.src[anchor.end:]
	return nil
}

// setInline sets the key/value pair at key, replacing the value in place so the
// key's own comments survive, or adding the pair to its table.
func (e *editor) setInline(key []string, value any) error {
	rendered, err := renderInline(value)
	if err != nil {
		return oops.With("key", strings.Join(key, ".")).Wrapf(err, "render TOML value")
	}

	for {
		doc, err := e.scan()
		if err != nil {
			return err
		}
		victim, found := lastNonExact(doc, key)
		if !found {
			break
		}
		e.removeOccurrence(doc, victim)
	}

	doc, err := e.scan()
	if err != nil {
		return err
	}
	if stmt, ok := doc.exactValue(key); ok {
		e.src = e.src[:stmt.valStart] + rendered + e.src[stmt.valEnd:]
		return nil
	}
	return e.insertKeyValue(doc, key, rendered)
}

// lastNonExact finds an occurrence under key that is not the key/value pair for
// key itself: a table or sub-key left over from when the value was another type.
func lastNonExact(doc *document, key []string) (occurrence, bool) {
	occ := doc.occurrences(key)
	for i := len(occ) - 1; i >= 0; i-- {
		if occ[i].isSection || len(occ[i].stmt.path) != len(key) {
			return occ[i], true
		}
	}
	return occurrence{}, false
}

// insertKeyValue adds name = value to the table that holds key, creating the
// table when the document has none.
func (e *editor) insertKeyValue(doc *document, key []string, rendered string) error {
	parent, name := key[:len(key)-1], key[len(key)-1]
	line := renderKey(name) + " = " + rendered + "\n"

	idx, ok := doc.tableSection(parent)
	if !ok {
		return e.insertBlock(parent[:len(parent)-1], "["+renderPath(parent)+"]\n"+line)
	}
	sec := doc.sections[idx]
	if idx == 0 && len(sec.stmts) == 0 {
		if len(doc.sections) > 1 {
			// A comment block at the very top of the file is the file's own header,
			// not the first table's; keep the new key below it.
			pos := doc.headerLead(1)
			if pos == 0 {
				pos = doc.sections[1].header.start
			}
			e.insertLineAt(pos, line, true)
			return nil
		}
		e.insertLineAt(len(e.src), line, false)
		return nil
	}
	e.insertLineAt(sec.bodyEnd, line, false)
	return nil
}

// insertLineAt inserts a key/value line at pos, optionally followed by a blank
// line separating it from the next header.
func (e *editor) insertLineAt(pos int, line string, blankAfter bool) {
	before, after := e.src[:pos], e.src[pos:]
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += e.newline
	}
	inserted := e.text(line)
	if blankAfter {
		inserted += e.newline
	}
	e.src = before + inserted + after
}

// insertBlock adds table blocks after the last section under parent (or the
// nearest ancestor of it that has one), else at the end of the document, set
// off by a blank line.
func (e *editor) insertBlock(parent []string, text string) error {
	doc, err := e.scan()
	if err != nil {
		return err
	}
	pos := len(e.src)
	for prefix := parent; len(prefix) > 0; prefix = prefix[:len(prefix)-1] {
		if i, ok := doc.lastSectionWithPrefix(prefix); ok {
			pos = doc.sections[i].bodyEnd
			break
		}
	}

	before, after := e.src[:pos], e.src[pos:]
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += e.newline
	}
	if strings.TrimSpace(before) != "" && !blankBefore(before, len(before)) {
		before += e.newline
	}
	e.src = before + e.text(text) + after
	return nil
}

// remove deletes everything at key, then any ancestor table the removal left
// without content.
func (e *editor) remove(key []string, claim Claim) error {
	for {
		doc, err := e.scan()
		if err != nil {
			return err
		}
		occ := doc.occurrences(key)
		if len(occ) == 0 {
			break
		}
		victim := occ[len(occ)-1]
		e.removeOccurrence(doc, victim)
	}
	return e.dropEmptyAncestors(key, claim)
}

// dropEmptyAncestors removes header-only tables above key that nothing is left
// in, so removing the last [mcp_servers.x] does not strand an empty [mcp_servers].
// A table the claim records as already in the file before ai-rulez wrote under it
// is the user's and stays.
func (e *editor) dropEmptyAncestors(key []string, claim Claim) error {
	for n := len(key) - 1; n >= 1; n-- {
		doc, err := e.scan()
		if err != nil {
			return err
		}
		idx, ok := doc.tableSection(key[:n])
		if !ok {
			continue
		}
		sec := doc.sections[idx]
		if claim.IsPreexisting(key[:n]) || len(sec.stmts) > 0 || len(doc.occurrences(key[:n])) != 1 {
			return nil
		}
		next := len(doc.src)
		if idx+1 < len(doc.sections) {
			next = doc.sections[idx+1].header.start
		}
		if strings.Contains(doc.src[sec.bodyEnd:next], "#") {
			return nil
		}
		e.removeSpan(sec.header.start, sec.bodyEnd, true, false)
	}
	return nil
}

// removeOccurrence deletes an occurrence found by doc.
func (e *editor) removeOccurrence(doc *document, occ occurrence) {
	// The only key of the root section was inserted above the first header with
	// a blank line after it; that line goes with it.
	soleRoot := !occ.isSection && occ.sec == 0 && len(doc.sections[0].stmts) == 1 && len(doc.sections) > 1
	e.removeSpan(occ.start, occ.end, occ.isSection, soleRoot)
}

// removeSpan deletes [start, end) along with the blank line that separated it
// from its neighbors, so removing what an earlier insert added restores the
// original bytes.
func (e *editor) removeSpan(start, end int, isSection, soleRoot bool) {
	afterLen := blankAfterLen(e.src, end)
	switch {
	case (start == 0 || soleRoot || blankBefore(e.src, start)) && afterLen > 0:
		end += afterLen
	case isSection && blankBefore(e.src, start):
		start = lineStartBefore(e.src, start)
	}
	e.src = e.src[:start] + e.src[end:]
}

// blankBefore reports whether the line ending just before pos is empty.
func blankBefore(src string, pos int) bool {
	if pos == 0 || src[pos-1] != '\n' {
		return false
	}
	return strings.TrimRight(src[lineStartBefore(src, pos):pos-1], "\r") == ""
}

// lineStartBefore returns where the line that ends just before pos begins.
func lineStartBefore(src string, pos int) int {
	return strings.LastIndexByte(src[:pos-1], '\n') + 1
}

// blankAfterLen returns the length of the blank line starting at pos, or 0.
func blankAfterLen(src string, pos int) int {
	switch {
	case strings.HasPrefix(src[pos:], "\r\n"):
		return 2
	case strings.HasPrefix(src[pos:], "\n"):
		return 1
	default:
		return 0
	}
}
