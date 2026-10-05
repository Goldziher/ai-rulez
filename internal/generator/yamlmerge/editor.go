package yamlmerge

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

var (
	errFlowRoot = errors.New("the root mapping is written in flow style ({...}), which ai-rulez cannot edit in place")
	errNotMap   = errors.New("an existing key on the path is not a mapping")
)

// editor edits a document's source text one member at a time, reparsing after
// every edit so line numbers are never stale.
type editor struct {
	src     string
	newline string
	// unit is the document's indentation step, used for text ai-rulez renders.
	unit int
}

func newEditor(src string) *editor {
	return &editor{src: src, newline: detectLineEnding(src), unit: detectIndent(src)}
}

func detectLineEnding(doc string) string {
	if strings.Contains(doc, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// detectIndent infers the indentation step from the first nested block, falling
// back to two spaces.
func detectIndent(src string) int {
	prevIndent, prevOpens := 0, false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if prevOpens && indent > prevIndent && !strings.HasPrefix(trimmed, "- ") {
			return indent - prevIndent
		}
		prevIndent, prevOpens = indent, strings.HasSuffix(trimmed, ":")
	}
	return defaultIndent
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
		p, err := parse(e.src)
		if err != nil {
			return err
		}
		if _, present := jsonmerge.LookupTree(p.tree, key); present {
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

func (e *editor) set(path []string, value any) error {
	p, err := parse(e.src)
	if err != nil {
		return err
	}
	if p.root == nil {
		text, err := e.renderMember(0, path[0], chain(path[1:], value))
		if err != nil {
			return err
		}
		e.insertAt(len(e.src), text)
		return nil
	}
	return e.setIn(p.root, path, value)
}

// chain nests value under path as maps.
func chain(path []string, value any) any {
	for i := len(path) - 1; i >= 0; i-- {
		value = map[string]any{path[i]: value}
	}
	return value
}

func (e *editor) setIn(cur *yaml.Node, path []string, value any) error {
	if cur.Style&yaml.FlowStyle != 0 {
		return errFlowRoot
	}
	idx := findMember(cur, path[0])
	if idx < 0 {
		text, err := e.renderMember(memberIndent(cur), path[0], chain(path[1:], value))
		if err != nil {
			return err
		}
		_, end := e.memberSpan(cur, len(cur.Content)/2-1)
		e.insertAt(end, text)
		return nil
	}
	if len(path) == 1 {
		return e.replaceMember(cur, idx, path[0], value)
	}

	child := cur.Content[2*idx+1]
	switch {
	case child.Kind == yaml.MappingNode && child.Style&yaml.FlowStyle != 0:
		return e.editFlowMember(cur, idx, false, func(flow *yaml.Node) error { return setNode(flow, path[1:], value) })
	case child.Kind == yaml.MappingNode:
		return e.setIn(child, path[1:], value)
	case isNull(child) && child.Value == "":
		// A key with nothing after it keeps its line and any comments under it; the
		// new members go beneath them.
		text, err := e.renderMember(memberIndent(cur)+e.unit, path[1], chain(path[2:], value))
		if err != nil {
			return err
		}
		_, end := e.memberSpan(cur, idx)
		e.insertAt(end, text)
		return nil
	case isNull(child):
		return e.replaceMember(cur, idx, path[0], chain(path[1:], value))
	default:
		return oops.With("key", path[0]).Wrapf(errNotMap, "set %s", strings.Join(path, "."))
	}
}

// replaceMember rewrites the member at cur.Content[2*idx], keeping a trailing
// comment when a scalar is replaced by another single-line value.
func (e *editor) replaceMember(cur *yaml.Node, idx int, key string, value any) error {
	old := cur.Content[2*idx+1]
	if old.Kind == yaml.SequenceNode && old.Style&yaml.FlowStyle == 0 && e.replaceBlockSequence(cur, idx, key, old, value) {
		return nil
	}
	if old.Kind == yaml.SequenceNode && old.Style&yaml.FlowStyle != 0 {
		value = flowSequence(value)
	}
	text, err := e.renderMember(memberIndent(cur), key, value)
	if err != nil {
		return err
	}
	start, end := e.memberSpan(cur, idx)
	if old.Kind == yaml.ScalarNode && old.LineComment != "" {
		if body, single := strings.CutSuffix(text, e.newline); single && !strings.Contains(body, "\n") {
			text = body + " " + old.LineComment + e.newline
		}
	}
	e.src = e.src[:start] + text + e.src[end:]
	return nil
}

// flowSequence renders a slice in flow style ([a, b]), the style the array it
// replaces was written in; any other value is returned as it is.
func flowSequence(value any) any {
	if rv := reflect.ValueOf(value); rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return value
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil || node.Kind != yaml.SequenceNode {
		return value
	}
	node.Style |= yaml.FlowStyle
	return &node
}

// remove deletes the member at path, and any mapping above it the removal leaves
// empty, except one the user owns: a mapping the claim records as already there
// before ai-rulez, or one carrying a comment, is kept with whatever else it holds.
func (e *editor) remove(path []string, claim Claim) error {
	p, err := parse(e.src)
	if err != nil || p.root == nil {
		return err
	}
	return e.removeIn(p.root, path, nil, claim)
}

// removeIn removes path from cur; done is the path of cur itself.
func (e *editor) removeIn(cur *yaml.Node, path, done []string, claim Claim) error {
	if cur.Style&yaml.FlowStyle != 0 {
		return errFlowRoot
	}
	idx := findMember(cur, path[0])
	if idx < 0 {
		return nil
	}
	if len(path) == 1 {
		e.removeMember(cur, idx)
		return nil
	}
	child := cur.Content[2*idx+1]
	here := append(slices.Clone(done), path[0])
	switch {
	case child.Kind == yaml.MappingNode && child.Style&yaml.FlowStyle != 0:
		return e.editFlowMember(cur, idx, true, func(flow *yaml.Node) error {
			removeNode(flow, path[1:])
			return nil
		})
	case child.Kind != yaml.MappingNode:
		return nil
	case wouldEmpty(child, path[1:]) && e.droppable(cur, idx, path, done, claim):
		e.removeMember(cur, idx)
		return nil
	default:
		return e.removeIn(child, path[1:], here, claim)
	}
}

// droppable reports whether every mapping on the way from the member at
// cur.Content[2*idx] down to the leaf of path is ai-rulez's to remove with it.
func (e *editor) droppable(cur *yaml.Node, idx int, path, done []string, claim Claim) bool {
	node := cur
	for k := 0; k < len(path)-1; k++ {
		i := findMember(node, path[k])
		if i < 0 {
			return false
		}
		child := node.Content[2*i+1]
		if claim.IsPreexisting(append(slices.Clone(done), path[:k+1]...)) || child.Kind != yaml.MappingNode ||
			len(child.Content) == 0 || e.hasUserComment(node.Content[2*i], child.Content[0]) {
			return false
		}
		node = child
	}
	return true
}

// hasUserComment reports whether a comment sits on the line of a mapping's key
// or on the lines between it and its first key.
func (e *editor) hasUserComment(parentKey, childKey *yaml.Node) bool {
	lines := strings.Split(e.src, "\n")
	if parentKey.Line < 1 || parentKey.Line > len(lines) || strings.Contains(lines[parentKey.Line-1], "#") {
		return true
	}
	for i := parentKey.Line; i < childKey.Line-1 && i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
			return true
		}
	}
	return false
}

// wouldEmpty reports whether removing path from the mapping leaves it empty.
func wouldEmpty(m *yaml.Node, path []string) bool {
	idx := findMember(m, path[0])
	if idx < 0 || len(m.Content) != 2 {
		return false
	}
	if len(path) == 1 {
		return true
	}
	child := m.Content[2*idx+1]
	return child.Kind == yaml.MappingNode && child.Style&yaml.FlowStyle == 0 && wouldEmpty(child, path[1:])
}

// removeMember deletes a member's lines, with the blank line that separated it
// from its neighbors when it had one on each side.
func (e *editor) removeMember(cur *yaml.Node, idx int) {
	start, end := e.memberSpan(cur, idx)
	if blankBefore(e.src, start) {
		if n := blankAfterLen(e.src, end); n > 0 {
			end += n
		}
	}
	e.src = e.src[:start] + e.src[end:]
}

// editFlowMember re-renders the member at cur.Content[2*idx], whose value is a
// flow mapping, after mutate has changed its nodes. With dropIfEmpty, a mapping
// the mutation empties takes its member with it.
func (e *editor) editFlowMember(cur *yaml.Node, idx int, dropIfEmpty bool, mutate func(*yaml.Node) error) error {
	key, flow := cur.Content[2*idx], cur.Content[2*idx+1]
	if len(flow.Content) == 0 {
		flow.Style &^= yaml.FlowStyle
	}
	if err := mutate(flow); err != nil {
		return err
	}
	if dropIfEmpty && len(flow.Content) == 0 {
		e.removeMember(cur, idx)
		return nil
	}
	pair := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, flow}}
	rendered, err := marshal(pair, e.unit)
	if err != nil {
		return oops.Wrapf(err, "render YAML member")
	}
	start, end := e.memberSpan(cur, idx)
	e.src = e.src[:start] + e.indentText(rendered, memberIndent(cur)) + e.src[end:]
	return nil
}

func setNode(m *yaml.Node, path []string, value any) error {
	idx := findMember(m, path[0])
	if idx >= 0 && len(path) > 1 {
		child := m.Content[2*idx+1]
		switch {
		case child.Kind == yaml.MappingNode:
			if len(child.Content) == 0 {
				child.Style &^= yaml.FlowStyle
			}
			return setNode(child, path[1:], value)
		case !isNull(child):
			return oops.Wrapf(errNotMap, "set %s", strings.Join(path, "."))
		}
	}
	var node yaml.Node
	if err := node.Encode(chain(path[1:], value)); err != nil {
		return oops.Wrapf(err, "encode YAML value")
	}
	if idx >= 0 {
		m.Content[2*idx+1] = &node
		return nil
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]}, &node)
	return nil
}

func removeNode(m *yaml.Node, path []string) {
	idx := findMember(m, path[0])
	if idx < 0 {
		return
	}
	if len(path) > 1 {
		child := m.Content[2*idx+1]
		if child.Kind != yaml.MappingNode {
			return
		}
		removeNode(child, path[1:])
		if len(child.Content) > 0 {
			return
		}
	}
	m.Content = slices.Delete(m.Content, 2*idx, 2*idx+2)
}

// findMember returns the index of the pair whose key is name, or -1.
func findMember(m *yaml.Node, name string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == name {
			return i / 2
		}
	}
	return -1
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// memberIndent is the column, from zero, at which the mapping's keys start.
func memberIndent(m *yaml.Node) int {
	if len(m.Content) == 0 {
		return 0
	}
	return m.Content[0].Column - 1
}

// memberSpan returns the byte span of the member at index idx of a block
// mapping: from the start of its key's line through the last non-blank line that
// belongs to its value. Comments and blank lines after that stay out of the
// span, so they are never taken along with it. A comment-only line is never the
// end of the member and never ends it either, whatever its column: YAML lets a
// comment sit at column zero inside a block, so it is the next content line that
// says where the member stops. A multi-line flow value ({...} or [...]) runs
// through the line of its closing bracket, and a keep-chomped block scalar
// (|+, >+) through the blank lines that are part of its value.
func (e *editor) memberSpan(m *yaml.Node, idx int) (start, end int) {
	key, value := m.Content[2*idx], m.Content[2*idx+1]
	indent := key.Column - 1
	seqAtKeyIndent := value.Kind == yaml.SequenceNode && value.Style&yaml.FlowStyle == 0

	lines := strings.SplitAfter(e.src, "\n")
	offsets := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		offsets[i] = offsets[i-1] + len(lines[i-1])
	}
	startLine := key.Line - 1
	start = offsets[startLine]
	end = start + len(lines[startLine])
	for i := startLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && len(lines[i])-len(strings.TrimLeft(lines[i], " ")) <= indent) {
			continue
		}
		ind := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		isSeq := trimmed == "-" || strings.HasPrefix(trimmed, "- ")
		if ind <= indent && (!seqAtKeyIndent || ind != indent || !isSeq) {
			break
		}
		end = offsets[i] + len(lines[i])
	}

	return start, e.extendSpan(value, lines, offsets, end)
}

// extendSpan stretches a member's end over what its value owns beyond the lines
// the indentation scan found: the rest of a multi-line flow collection, or the
// blank lines of a keep-chomped block scalar.
func (e *editor) extendSpan(value *yaml.Node, lines []string, offsets []int, end int) int {
	if value.Line < 1 {
		return end
	}
	line := lines[value.Line-1]
	col := byteColumn(line, value.Column-1)
	switch {
	case value.Kind != yaml.ScalarNode && value.Style&yaml.FlowStyle != 0:
		if closer := flowEnd(e.src, offsets[value.Line-1]+col); closer > end {
			end = lineEnd(e.src, closer)
		}
	case value.Kind == yaml.ScalarNode && value.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 &&
		keepsTrailingLines(line, col):
		for end < len(e.src) && blankAfterLen(e.src, end) > 0 {
			end += blankAfterLen(e.src, end)
		}
	}
	return end
}

// byteColumn converts a column counted in characters to a byte offset in line.
func byteColumn(line string, column int) int {
	offset := 0
	for range column {
		if offset >= len(line) {
			break
		}
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
	}
	return offset
}

// lineEnd returns the offset just after the line holding pos.
func lineEnd(src string, pos int) int {
	if nl := strings.IndexByte(src[pos:], '\n'); nl >= 0 {
		return pos + nl + 1
	}
	return len(src)
}

// keepsTrailingLines reports whether the block scalar whose indicator starts at
// column of line has the keep chomping indicator (+).
func keepsTrailingLines(line string, column int) bool {
	if column >= len(line) {
		return false
	}
	header := strings.Fields(line[column:])
	return len(header) > 0 && strings.ContainsRune(header[0], '+')
}

// flowEnd returns the offset just after the bracket closing the flow collection
// that opens at from, skipping quoted strings and comments.
func flowEnd(src string, from int) int {
	depth := 0
	for i := from; i < len(src); i++ {
		switch c := src[i]; c {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '"', '\'':
			if opensQuote(src, i) {
				i = quotedEnd(src, i)
			}
		case '#':
			if i > 0 && (src[i-1] == ' ' || src[i-1] == '\t' || src[i-1] == '\n') {
				i = lineEnd(src, i) - 1
			}
		}
	}
	return len(src)
}

// opensQuote reports whether the quote at i starts a quoted scalar rather than
// being an apostrophe inside a plain one.
func opensQuote(src string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch src[j] {
		case ' ', '\t', '\n', '\r':
			continue
		case '[', '{', ',', ':', '?', '-':
			return true
		default:
			return false
		}
	}
	return true
}

// quotedEnd returns the index of the quote closing the one at i.
func quotedEnd(src string, i int) int {
	quote := src[i]
	for j := i + 1; j < len(src); j++ {
		switch {
		case quote == '"' && src[j] == '\\':
			j++
		case src[j] == quote:
			return j
		}
	}
	return len(src)
}

// renderMember renders key: value as block YAML at the given indentation, in
// the document's line ending.
func (e *editor) renderMember(indent int, key string, value any) (string, error) {
	rendered, err := marshal(map[string]any{key: value}, e.unit)
	if err != nil {
		return "", oops.With("key", key).Wrapf(err, "render YAML member")
	}
	return e.indentText(rendered, indent), nil
}

func (e *editor) indentText(text string, indent int) string {
	pad := strings.Repeat(" ", indent)
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = pad + line
		}
	}
	out := strings.Join(lines, "")
	if e.newline != "\n" {
		out = strings.ReplaceAll(out, "\n", e.newline)
	}
	return out
}

// insertAt inserts whole lines at pos, first ending the line before it.
func (e *editor) insertAt(pos int, text string) {
	before, after := e.src[:pos], e.src[pos:]
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += e.newline
	}
	e.src = before + text + after
}

// blankBefore reports whether the line ending just before pos is blank.
func blankBefore(src string, pos int) bool {
	if pos < 2 || src[pos-1] != '\n' {
		return false
	}
	start := strings.LastIndexByte(src[:pos-1], '\n') + 1
	return strings.TrimSpace(src[start:pos-1]) == ""
}

// blankAfterLen returns the length of the blank line starting at pos, or 0.
func blankAfterLen(src string, pos int) int {
	if pos >= len(src) {
		return 0
	}
	nl := strings.IndexByte(src[pos:], '\n')
	if nl < 0 || strings.TrimSpace(src[pos:pos+nl]) != "" {
		return 0
	}
	return nl + 1
}

// replaceBlockSequence rewrites the block sequence at cur.Content[2*idx] to hold
// value (a slice) one element at a time: an element that is unchanged keeps its
// source text, comments and key order included, and only changed or new elements
// are rendered. Re-rendering the whole list would sort every mapping's keys and
// drop the comments of elements the caller meant to leave alone, such as the
// groups a user wrote in a list ai-rulez shares with them. It returns false,
// with the document untouched, when the layout is not one it can edit this way
// (an element whose dash is not on its first line, a value that is not a list) or
// the result does not read back as value; the caller then renders the whole list.
func (e *editor) replaceBlockSequence(cur *yaml.Node, idx int, key string, old *yaml.Node, value any) bool {
	items, ok := sliceItems(value)
	if !ok || len(old.Content) == 0 {
		return false
	}
	start, end := e.memberSpan(cur, idx)
	starts, oldValues, dashIndent, ok := e.blockSequenceLayout(old, start)
	if !ok {
		return false
	}
	replaced, ok := e.renderBlockSequence(items, starts, oldValues, dashIndent, start, end)
	if !ok {
		return false
	}

	// The new member must read back as value on its own, before it goes in.
	var parsedMember map[string]any
	if err := yaml.Unmarshal([]byte(replaced), &parsedMember); err != nil ||
		jsonmerge.Digest(parsedMember[key]) != jsonmerge.Digest(value) {
		return false
	}
	e.src = e.src[:start] + replaced + e.src[end:]
	return true
}

// sliceItems returns the elements of a slice or array value.
func sliceItems(value any) ([]any, bool) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	items := make([]any, rv.Len())
	for i := range items {
		items[i] = rv.Index(i).Interface()
	}
	return items, true
}

func (e *editor) blockSequenceLayout(old *yaml.Node, start int) (starts []int, oldValues []any, dashIndent int, ok bool) {
	lines := strings.SplitAfter(e.src, "\n")
	offsets := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		offsets[i] = offsets[i-1] + len(lines[i-1])
	}
	starts = make([]int, len(old.Content))
	oldValues = make([]any, len(old.Content))
	dashIndent = -1
	for i, item := range old.Content {
		if item.Line < 1 || item.Line > len(lines) {
			return nil, nil, 0, false
		}
		line := lines[item.Line-1]
		prefix := strings.TrimRight(line[:min(byteColumn(line, item.Column-1), len(line))], " ")
		indent, isDash := strings.CutSuffix(prefix, "-")
		if !isDash || strings.TrimSpace(indent) != "" || (dashIndent >= 0 && len(indent) != dashIndent) {
			return nil, nil, 0, false
		}
		dashIndent = len(indent)
		starts[i] = offsets[item.Line-1]
		if starts[i] < start || (i > 0 && starts[i] <= starts[i-1]) {
			return nil, nil, 0, false
		}
		if err := item.Decode(&oldValues[i]); err != nil {
			return nil, nil, 0, false
		}
	}

	return starts, oldValues, dashIndent, true
}

func (e *editor) renderBlockSequence(items []any, starts []int, oldValues []any, dashIndent, start, end int) (string, bool) {
	spanEnd := func(i int) int {
		if i+1 < len(starts) {
			return starts[i+1]
		}
		return end
	}

	var b strings.Builder
	b.WriteString(e.src[start:starts[0]])
	used := make([]bool, len(oldValues))
	retained := 0
	for _, item := range items {
		chunk := ""
		for j := range oldValues {
			if !used[j] && jsonmerge.Digest(oldValues[j]) == jsonmerge.Digest(item) {
				used[j] = true
				retained++
				chunk = e.src[starts[j]:spanEnd(j)]
				break
			}
		}
		if chunk == "" {
			rendered, err := marshal([]any{item}, e.unit)
			if err != nil {
				return "", false
			}
			chunk = e.indentText(rendered, dashIndent)
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString(e.newline)
		}
		b.WriteString(chunk)
	}
	if retained == 0 {
		return "", false // nothing to keep: rendering the whole list is the same edit
	}
	if !strings.HasSuffix(b.String(), "\n") && end < len(e.src) {
		b.WriteString(e.newline)
	}
	return b.String(), true
}
