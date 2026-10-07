package jsonmerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/samber/oops"
	"github.com/tailscale/hujson"
)

// This file merges owned keys into JSONC documents (comments and trailing
// commas), which VS Code, Zed, OpenCode and Kilo all write. The strict-JSON path
// in jsonmerge.go re-renders the whole root object and so cannot keep comments;
// here the document is held as a hujson syntax tree, which represents every byte
// of whitespace and comments, and only the owned members are edited in place.
// Everything the edit does not touch packs back byte for byte.

// jsoncEditor carries the formatting the document already uses.
type jsoncEditor struct {
	indent  string
	newline string
}

// parseJSONCRoot parses a JSONC document whose root must be an object.
func parseJSONCRoot(doc string) (hujson.Value, *hujson.Object, error) {
	root, err := hujson.Parse([]byte(doc))
	if err != nil {
		return hujson.Value{}, nil, err
	}
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return hujson.Value{}, nil, errNotJSONObject
	}
	return root, obj, nil
}

func newJSONCEditor(doc string, obj *hujson.Object) jsoncEditor {
	return jsoncEditor{indent: jsoncIndent(obj), newline: detectLineEnding(doc)}
}

// applyJSONC is Apply for a document that is not strict JSON. strictErr is the
// strict parser's complaint, reported when the document is not JSONC either.
func applyJSONC(path, existing string, owned []OwnedKey, strictErr error) (Result, error) {
	root, obj, err := parseJSONCRoot(existing)
	if err != nil {
		if errors.Is(err, errNotJSONObject) {
			strictErr = err
		}
		return Result{}, parseFailure(path, strictErr)
	}

	ed := newJSONCEditor(existing, obj)
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 {
			continue
		}
		if err := ed.setPath(obj, segs, key, 1); err != nil {
			return Result{}, oops.With("path", path).Wrapf(err, "merge owned keys into JSON settings document")
		}
	}
	body := string(root.Pack())
	before, after, err := jsoncTrees(existing, body)
	if err == nil {
		err = VerifyOwned(after, owned)
	}
	if err != nil {
		return Result{}, oops.With("path", path).
			Hint("ai-rulez could not merge its keys into this JSON document faithfully; the file was left untouched").
			Wrapf(err, "merged JSON settings document does not hold the owned values")
	}
	if err := CheckPreservedApply(before, after, owned); err != nil {
		return Result{}, oops.With("path", path).
			Hint("ai-rulez could not merge its keys into this JSON document without altering the rest of it; the file was left untouched").
			Wrapf(err, "merged JSON settings document does not preserve the existing content")
	}
	partial := hasUnownedJSONC(obj, ownedPaths(owned)) || jsoncHasComments(&root) || HasUserElements(after, owned)
	return Result{Body: body, PartiallyOwned: partial, Claims: AnnotateClaims(claimsFor(owned), before, existing)}, nil
}

// jsoncTrees decodes two JSONC documents into generic Go values.
func jsoncTrees(before, after string) (b, a map[string]any, err error) {
	if b, err = jsoncTree(before); err != nil {
		return nil, nil, err
	}
	if a, err = jsoncTree(after); err != nil {
		return nil, nil, err
	}
	return b, a, nil
}

// jsoncTree decodes a JSONC document into generic Go values.
func jsoncTree(doc string) (map[string]any, error) {
	standard, err := hujson.Standardize([]byte(doc))
	if err != nil {
		return nil, err
	}
	return DecodeTree(string(standard))
}

// unmergeJSONC is UnmergeDocument for a document that is not strict JSON.
func unmergeJSONC(path, existing string, claims []Claim, strictErr error) (Unmerged, error) {
	root, obj, err := parseJSONCRoot(existing)
	if err != nil {
		if errors.Is(err, errNotJSONObject) {
			strictErr = err
		}
		return Unmerged{}, parseFailure(path, strictErr)
	}

	ed := newJSONCEditor(existing, obj)
	changed, mismatched := unmergeJSONCClaims(ed, obj, claims)
	kept := keptJSONCPaths(obj, mismatched)
	if !changed {
		return Unmerged{Kept: kept}, nil
	}
	body := RestoreFinalNewline(claims, string(root.Pack()))
	before, after, err := jsoncTrees(existing, body)
	if err == nil {
		err = CheckPreservedUnmerge(before, after, claims)
	}
	if err != nil {
		return Unmerged{}, oops.With("path", path).
			Hint("ai-rulez could not remove its keys from this JSON document without altering the rest of it; the file was left untouched").
			Wrapf(err, "unmerged JSON settings document does not preserve the existing content")
	}
	empty := len(obj.Members) == 0 && !jsoncHasComments(&root)
	return Unmerged{Body: body, Changed: true, Empty: empty, Kept: kept}, nil
}

// unmergeJSONCClaims removes every claim from obj, the claims that own the whole
// document last, and returns the paths whose guard rejected the member.
func unmergeJSONCClaims(ed jsoncEditor, obj *hujson.Object, claims []Claim) (changed bool, mismatched [][]string) {
	for _, alone := range []bool{false, true} {
		for i := range claims {
			claim := &claims[i]
			if len(claim.Path) == 0 || claim.Alone != alone || (alone && len(obj.Members) != 1) {
				continue
			}
			did, mismatch := ed.unmergeClaim(obj, claim.Path, *claim)
			changed = changed || did
			if mismatch {
				mismatched = append(mismatched, claim.Path)
			}
		}
	}
	return changed, mismatched
}

// keptJSONCPaths is the distinct mismatched paths that are still present in obj.
func keptJSONCPaths(obj *hujson.Object, mismatched [][]string) [][]string {
	var kept [][]string
	seen := map[string]bool{}
	for _, claimPath := range mismatched {
		key := strings.Join(claimPath, "\x00")
		if !seen[key] && jsoncPresent(obj, claimPath) {
			seen[key] = true
			kept = append(kept, claimPath)
		}
	}
	return kept
}

// unmergeClaim removes the member (or array elements) the claim addresses,
// dropping an ancestor object the removal leaves empty unless it still holds a
// comment. mismatch reports a member whose value the claim's guard rejected.
func (ed jsoncEditor) unmergeClaim(obj *hujson.Object, path []string, claim Claim) (changed, mismatch bool) {
	idx := jsoncFind(obj, path[0])
	if idx < 0 {
		return false, false
	}
	if len(path) == 1 {
		return ed.unmergeLeaf(obj, idx, claim)
	}
	child, ok := obj.Members[idx].Value.Value.(*hujson.Object)
	if !ok {
		return false, false
	}
	did, mismatch := ed.unmergeClaim(child, path[1:], claim)
	if !did {
		return false, mismatch
	}
	// path[0] is the member at depth len(claim.Path)-len(path)+1 of the claim's path.
	if len(child.Members) == 0 && !hasComment(child.AfterExtra) &&
		!claim.IsPreexisting(claim.Path[:len(claim.Path)-len(path)+1]) {
		ed.removeAt(obj, idx)
	}
	return true, false
}

// unmergeLeaf applies a claim to the member at obj.Members[idx].
func (ed jsoncEditor) unmergeLeaf(obj *hujson.Object, idx int, claim Claim) (changed, mismatch bool) {
	value := &obj.Members[idx].Value
	if !claim.Matches(standardRaw(*value)) {
		return false, true
	}
	if !claim.HasElements() {
		ed.removeAt(obj, idx)
		return true, false
	}
	arr, ok := value.Value.(*hujson.Array)
	if !ok {
		return false, false
	}
	matcher := claim.NewElementMatcher()
	var claimed []int
	for i := range arr.Elements {
		if matcher.TakeRaw(standardRaw(arr.Elements[i])) {
			claimed = append(claimed, i)
		}
	}
	// A claim with no element names an empty array ai-rulez created itself (see
	// OwnedKey.Created): once it is still empty, it goes.
	removed := len(claimed) > 0 || (len(arr.Elements) == 0 && len(claim.ElementDigests()) == 0)
	for n := len(claimed) - 1; n >= 0; n-- {
		i := claimed[n]
		removeSlot(elementSlots(arr), i, &arr.AfterExtra)
		arr.Elements = slices.Delete(arr.Elements, i, i+1)
	}
	if removed && len(arr.Elements) == 0 && !hasComment(arr.AfterExtra) && !claim.IsPreexisting(claim.Path) {
		ed.removeAt(obj, idx)
	}
	return removed, false
}

// jsoncPresent reports whether the key path addresses a member of the document.
func jsoncPresent(obj *hujson.Object, path []string) bool {
	idx := jsoncFind(obj, path[0])
	if idx < 0 {
		return false
	}
	if len(path) == 1 {
		return true
	}
	child, ok := obj.Members[idx].Value.Value.(*hujson.Object)
	return ok && jsoncPresent(child, path[1:])
}

// setPath writes the owned key into obj, descending along path so only the
// addressed member changes; depth is the nesting depth of obj's members.
func (ed jsoncEditor) setPath(obj *hujson.Object, path []string, key OwnedKey, depth int) error {
	head, rest := path[0], path[1:]
	if len(rest) == 0 {
		return ed.setLeaf(obj, head, key, depth)
	}

	idx := jsoncFind(obj, head)
	if idx < 0 {
		if key.Remove {
			return nil
		}
		if err := ed.insertMember(obj, head, map[string]any{}, depth); err != nil {
			return err
		}
		idx = len(obj.Members) - 1
	}
	child, err := jsoncChildObject(obj, idx, head)
	if err != nil {
		return err
	}
	if err := ed.setPath(child, rest, key, depth+1); err != nil {
		return err
	}
	if key.Remove && len(child.Members) == 0 && !hasComment(child.AfterExtra) {
		ed.removeAt(obj, idx)
	}
	return nil
}

// setLeaf applies the owned key to the member named head of obj: it removes it
// (all copies, or those a guard accepts), merges its entries into it, or sets it.
func (ed jsoncEditor) setLeaf(obj *hujson.Object, head string, key OwnedKey, depth int) error {
	switch {
	case key.Remove && key.RemoveIf != nil:
		// A guarded removal takes only a value the guard accepts: one the
		// user has since changed is theirs and stays.
		for i := len(obj.Members) - 1; i >= 0; i-- {
			if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.String() == head &&
				key.RemoveIf(standardRaw(obj.Members[i].Value)) {
				ed.removeAt(obj, i)
			}
		}
		return nil
	case key.Remove:
		for idx := jsoncFind(obj, head); idx >= 0; idx = jsoncFind(obj, head) {
			ed.removeAt(obj, idx)
		}
		return nil
	case key.Members:
		return ed.mergeMembers(obj, head, key.Value, depth)
	default:
		return ed.setMember(obj, head, key.Value, depth)
	}
}

// mergeMembers writes each entry of value into the object at head, replacing the
// entry of the same name in place and appending the others in name order.
func (ed jsoncEditor) mergeMembers(obj *hujson.Object, head string, value any, depth int) error {
	names, entries, ok := memberEntries(value)
	idx := jsoncFind(obj, head)
	if !ok {
		if idx >= 0 {
			return nil
		}
		return ed.setMember(obj, head, value, depth)
	}
	if idx < 0 {
		if err := ed.insertMember(obj, head, map[string]any{}, depth); err != nil {
			return err
		}
		idx = len(obj.Members) - 1
	}
	child, err := jsoncChildObject(obj, idx, head)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := ed.setMember(child, name, entries[name], depth+1); err != nil {
			return err
		}
	}
	return nil
}

// jsoncChildObject returns the object value of obj.Members[idx], refusing a
// value that is not an object for the same reason childObjectMembers does.
func jsoncChildObject(obj *hujson.Object, idx int, head string) (*hujson.Object, error) {
	child, ok := obj.Members[idx].Value.Value.(*hujson.Object)
	if !ok {
		return nil, oops.
			With("key", head).
			Hint("ai-rulez merges its keys into an object and will not replace a non-object value it cannot preserve").
			Errorf("existing key %q is not a JSON object", head)
	}
	return child, nil
}

// setMember replaces the value of the member named name, keeping its name,
// the comments around its value and its position, or appends it. Duplicate
// members of the same name collapse into the first.
func (ed jsoncEditor) setMember(obj *hujson.Object, name string, value any, depth int) error {
	idx := jsoncFind(obj, name)
	if idx < 0 {
		return ed.insertMember(obj, name, value, depth)
	}
	// An array the document already holds keeps its elements, spacing and
	// comments: new elements are added after them, and an array that already says
	// what ai-rulez wants is left alone.
	if arr, ok := obj.Members[idx].Value.Value.(*hujson.Array); ok {
		if done, err := ed.extendArray(arr, value, depth); err != nil {
			return fmt.Errorf("marshal owned key %q: %w", name, err)
		} else if done {
			for dup := jsoncFindFrom(obj, name, idx+1); dup >= 0; dup = jsoncFindFrom(obj, name, idx+1) {
				ed.removeAt(obj, dup)
			}
			return nil
		}
	}
	rendered, err := ed.renderValue(value, depth, !jsoncMultiline(obj))
	if err != nil {
		return fmt.Errorf("marshal owned key %q: %w", name, err)
	}
	old := obj.Members[idx].Value
	rendered.BeforeExtra = old.BeforeExtra
	rendered.AfterExtra = old.AfterExtra
	obj.Members[idx].Value = rendered
	for dup := jsoncFindFrom(obj, name, idx+1); dup >= 0; dup = jsoncFindFrom(obj, name, idx+1) {
		ed.removeAt(obj, dup)
	}
	return nil
}

// extendArray edits arr in place when value is arr's own elements followed by
// new ones (or exactly arr's elements), reporting whether it did. Anything else,
// such as an element dropped or reordered, is left to the caller to replace.
func (ed jsoncEditor) extendArray(arr *hujson.Array, value any, depth int) (bool, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err //nolint:wrapcheck // the caller names the key
	}
	want, ok := decodeList(encoded)
	if !ok || len(arr.Elements) == 0 || len(want) < len(arr.Elements) {
		return false, nil
	}
	for i := range arr.Elements {
		have, haveOK := decodeValue(standardRaw(arr.Elements[i]))
		if !haveOK || !reflect.DeepEqual(have, want[i]) {
			return false, nil
		}
	}
	last := arr.Elements[len(arr.Elements)-1]
	for _, element := range want[len(arr.Elements):] {
		rendered, err := ed.renderValue(element, depth+1, true)
		if err != nil {
			return false, err
		}
		rendered.BeforeExtra = wsOnly(last.BeforeExtra)
		if last.AfterExtra != nil {
			// The array ends in a trailing comma: the new last element keeps one.
			rendered.AfterExtra = hujson.Extra{}
		}
		arr.Elements = append(arr.Elements, rendered)
	}
	return true, nil
}

// insertMember appends a member at the end of obj, matching the layout of the
// members already there.
func (ed jsoncEditor) insertMember(obj *hujson.Object, name string, value any, depth int) error {
	n := len(obj.Members)
	inline := n > 0 && !jsoncMultiline(obj)
	rendered, err := ed.renderValue(value, depth, inline)
	if err != nil {
		return fmt.Errorf("marshal owned key %q: %w", name, err)
	}
	nameBytes, err := json.Marshal(name)
	if err != nil {
		return fmt.Errorf("marshal key %q: %w", name, err)
	}
	member := hujson.ObjectMember{Name: hujson.Value{Value: hujson.Literal(nameBytes)}, Value: rendered}
	member.Value.BeforeExtra = hujson.Extra(" ")

	switch {
	case n == 0:
		after := bytes.TrimRight(obj.AfterExtra, " \t\r\n")
		member.Name.BeforeExtra = append(slices.Clone(after), ed.newline+ed.pad(obj, depth)...)
		obj.AfterExtra = hujson.Extra(ed.newline + strings.Repeat(ed.indent, depth-1))
	case inline:
		last := obj.Members[n-1]
		member.Name.BeforeExtra = wsOnly(last.Name.BeforeExtra)
		member.Value.BeforeExtra = wsOnly(last.Value.BeforeExtra)
		if last.Value.AfterExtra != nil {
			member.Value.AfterExtra = hujson.Extra{}
		} else if hasComment(obj.AfterExtra) {
			// A comment before the closing brace belonged to the old last member.
			obj.Members[n-1].Value.AfterExtra = hujson.Extra(slices.Clone(obj.AfterExtra))
			obj.AfterExtra = nil
		}
	default:
		trail, own := splitTrail(obj.AfterExtra)
		if !hasComment(trail) {
			trail = nil
		}
		member.Name.BeforeExtra = append(slices.Clone(trail), ed.newline+ed.pad(obj, depth)...)
		obj.AfterExtra = hujson.Extra(slices.Clone(own))
		if obj.Members[n-1].Value.AfterExtra != nil {
			member.Value.AfterExtra = hujson.Extra{}
		}
	}
	obj.Members = append(obj.Members, member)
	return nil
}

// renderValue renders an owned value as a JSONC value, indented as a member at
// depth (or compact for an inline object).
func (ed jsoncEditor) renderValue(value any, depth int, inline bool) (hujson.Value, error) {
	var encoded []byte
	var err error
	if inline {
		encoded, err = json.Marshal(value)
	} else {
		encoded, err = json.MarshalIndent(value, strings.Repeat(ed.indent, depth), ed.indent)
	}
	if err != nil {
		return hujson.Value{}, err
	}
	if ed.newline != "\n" {
		encoded = bytes.ReplaceAll(encoded, []byte("\n"), []byte(ed.newline))
	}
	return hujson.Parse(encoded)
}

// pad is the indentation for a new member of obj: what a sibling already uses,
// else the document's indent repeated to depth.
func (ed jsoncEditor) pad(obj *hujson.Object, depth int) string {
	for i := len(obj.Members) - 1; i >= 0; i-- {
		before := obj.Members[i].Name.BeforeExtra
		if nl := bytes.LastIndexByte(before, '\n'); nl >= 0 && len(bytes.TrimLeft(before[nl+1:], " \t")) == 0 {
			return string(before[nl+1:])
		}
	}
	return strings.Repeat(ed.indent, depth)
}

// removeAt deletes obj.Members[idx], keeping the comments around it.
func (jsoncEditor) removeAt(obj *hujson.Object, idx int) {
	removeSlot(memberSlots(obj), idx, &obj.AfterExtra)
	obj.Members = slices.Delete(obj.Members, idx, idx+1)
}

// jsoncFind returns the index of the first member named name, or -1.
func jsoncFind(obj *hujson.Object, name string) int {
	return jsoncFindFrom(obj, name, 0)
}

func jsoncFindFrom(obj *hujson.Object, name string, from int) int {
	for i := from; i < len(obj.Members); i++ {
		lit, ok := obj.Members[i].Name.Value.(hujson.Literal)
		if ok && lit.String() == name {
			return i
		}
	}
	return -1
}

// jsoncMultiline reports whether obj spreads over several lines.
func jsoncMultiline(obj *hujson.Object) bool {
	if bytes.IndexByte(obj.AfterExtra, '\n') >= 0 {
		return true
	}
	for i := range obj.Members {
		member := &obj.Members[i]
		if bytes.IndexByte(member.Name.BeforeExtra, '\n') >= 0 {
			return true
		}
	}
	return false
}

// jsoncIndent infers one indentation level from the first top-level member that
// starts its own line, falling back to defaultJSONIndent.
func jsoncIndent(obj *hujson.Object) string {
	for i := range obj.Members {
		member := &obj.Members[i]
		before := member.Name.BeforeExtra
		nl := bytes.LastIndexByte(before, '\n')
		if nl < 0 {
			continue
		}
		if lead := before[nl+1:]; len(lead) > 0 && len(bytes.TrimLeft(lead, " \t")) == 0 {
			return string(lead)
		}
	}
	return defaultJSONIndent
}

// wsOnly returns e when it is plain whitespace, else a single space.
func wsOnly(e hujson.Extra) hujson.Extra {
	if hasComment(e) {
		return hujson.Extra(" ")
	}
	return slices.Clone(e)
}

// hasUnownedJSONC is hasUnownedPaths over a syntax tree.
func hasUnownedJSONC(obj *hujson.Object, paths [][]string) bool {
	roots := make(map[string][][]string, len(paths))
	for _, path := range paths {
		roots[path[0]] = append(roots[path[0]], path[1:])
	}
	for i := range obj.Members {
		member := &obj.Members[i]
		rems, ok := roots[memberName(member)]
		if !ok {
			return true
		}
		var nested [][]string
		whole := false
		for _, rem := range rems {
			if len(rem) == 0 {
				whole = true
				break
			}
			nested = append(nested, rem)
		}
		if whole {
			continue
		}
		child, isObject := member.Value.Value.(*hujson.Object)
		if !isObject || hasUnownedJSONC(child, nested) {
			return true
		}
	}
	return false
}

// jsoncHasComments reports whether the document carries any comment. A comment
// is the consumer's, so a document holding one is theirs even when every key in
// it is ai-rulez's.
func jsoncHasComments(root *hujson.Value) bool {
	for v := range root.All() {
		if hasComment(v.BeforeExtra) || hasComment(v.AfterExtra) {
			return true
		}
		switch composite := v.Value.(type) {
		case *hujson.Object:
			if hasComment(composite.AfterExtra) {
				return true
			}
		case *hujson.Array:
			if hasComment(composite.AfterExtra) {
				return true
			}
		}
	}
	return false
}

// standardRaw returns v as strict JSON, without the surrounding extras.
func standardRaw(v hujson.Value) []byte {
	clone := v.Clone()
	clone.BeforeExtra, clone.AfterExtra = nil, nil
	clone.Standardize()
	return clone.Pack()
}

// jsoncSlot is one object member or array element as seen by removeSlot: first
// is the value whose BeforeExtra precedes the item, last the one whose
// AfterExtra sits before the item's comma (a non-nil AfterExtra on the final
// item is what emits a trailing comma).
type jsoncSlot struct {
	first, last *hujson.Value
}

func memberSlots(obj *hujson.Object) []jsoncSlot {
	slots := make([]jsoncSlot, len(obj.Members))
	for i := range obj.Members {
		slots[i] = jsoncSlot{first: &obj.Members[i].Name, last: &obj.Members[i].Value}
	}
	return slots
}

func elementSlots(arr *hujson.Array) []jsoncSlot {
	slots := make([]jsoncSlot, len(arr.Elements))
	for i := range arr.Elements {
		slots[i] = jsoncSlot{first: &arr.Elements[i], last: &arr.Elements[i]}
	}
	return slots
}

// removeSlot rewrites the extras around slots[i] so the item can be deleted
// without taking neighboring comments or the trailing-comma style with it.
// Comments on the line after the previous item stay with it; comments on the
// removed item's own line go with it; comments on their own lines above it stay.
// tail is the container's AfterExtra. The caller deletes the item afterwards.
func removeSlot(slots []jsoncSlot, i int, tail *hujson.Extra) {
	trail, own := splitTrail(slots[i].first.BeforeExtra)
	if !hasComment(trail) {
		trail = nil
	}

	if i < len(slots)-1 {
		next := slots[i+1].first
		_, nextOwn := splitTrail(next.BeforeExtra)
		next.BeforeExtra = keepComments(trail, own, nextOwn)
		return
	}

	_, tailOwn := splitTrail(*tail)
	if bytes.IndexByte(*tail, '\n') < 0 && !hasComment(*tail) {
		// A single-line object: the space before its closing brace is its own.
		tailOwn = *tail
	}
	hadTrailing := slots[i].last.AfterExtra != nil
	merged := keepComments(trail, own, tailOwn)
	if hasComment(own) {
		merged = append(slices.Clone(trail), trimToLastNewline(own)...)
		merged = append(merged, tailOwn...)
	}

	if i == 0 {
		if !hasComment(merged) {
			merged = nil
		}
		*tail = merged
		return
	}

	prev := slots[i-1].last
	switch {
	case hadTrailing && prev.AfterExtra == nil:
		prev.AfterExtra = hujson.Extra{}
	case !hadTrailing && prev.AfterExtra != nil:
		merged = append(slices.Clone(prev.AfterExtra), merged...)
		prev.AfterExtra = nil
	}
	*tail = merged
}

// keepComments joins what survives between two items once the one between them
// is gone: the previous item's trailing comment, then either the removed item's
// own-line comments or, when it had none, the following item's leading space.
func keepComments(trail, own, nextOwn []byte) []byte {
	out := slices.Clone(trail)
	if hasComment(own) {
		return append(out, own...)
	}
	return append(out, nextOwn...)
}

// trimToLastNewline drops the indentation of the line the removed item stood on,
// keeping everything up to (not including) its last newline.
func trimToLastNewline(own []byte) []byte {
	if nl := bytes.LastIndexByte(own, '\n'); nl >= 0 && len(bytes.TrimSpace(own[nl:])) == 0 {
		return bytes.TrimRight(own[:nl], "\r")
	}
	return bytes.TrimRight(own, " \t")
}

// splitTrail splits an Extra into the part on the same line as whatever precedes
// it (a trailing comment) and the rest, which starts at the first newline.
func splitTrail(e []byte) (trail, own []byte) {
	pos := 0
	for pos < len(e) {
		end := tokenEnd(e, pos)
		if isComment(e[pos:end]) {
			pos = end
			continue
		}
		if nl := bytes.IndexByte(e[pos:end], '\n'); nl >= 0 {
			break
		}
		pos = end
	}
	return e[:pos], e[pos:]
}

// hasComment reports whether the Extra contains a comment.
func hasComment(e []byte) bool {
	return bytes.Contains(e, []byte("//")) || bytes.Contains(e, []byte("/*"))
}

func isComment(token []byte) bool {
	return len(token) >= 2 && token[0] == '/' && (token[1] == '/' || token[1] == '*')
}

// tokenEnd returns the end of the whitespace run or comment starting at pos.
func tokenEnd(e []byte, pos int) int {
	switch {
	case bytes.HasPrefix(e[pos:], []byte("//")):
		// A line comment ends before its line break, CR included, so a CRLF
		// document's comments do not swallow the CR.
		if nl := bytes.IndexByte(e[pos:], '\n'); nl >= 0 {
			end := pos + nl
			if end > pos && e[end-1] == '\r' {
				end--
			}
			return end
		}
		return len(e)
	case bytes.HasPrefix(e[pos:], []byte("/*")):
		if end := bytes.Index(e[pos+2:], []byte("*/")); end >= 0 {
			return pos + 2 + end + 2
		}
		return len(e)
	default:
		end := pos + 1
		for end < len(e) && strings.IndexByte(" \t\r\n", e[end]) >= 0 {
			end++
		}
		return end
	}
}

// memberName is the name of an object member as written, or "" when it is not a literal.
func memberName(member *hujson.ObjectMember) string {
	if lit, ok := member.Name.Value.(hujson.Literal); ok {
		return lit.String()
	}
	return ""
}

// decodeList decodes data as a JSON array; ok is false when it is not one.
func decodeList(data []byte) (list []any, ok bool) {
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, false
	}
	return list, true
}

// decodeValue decodes data as any JSON value; ok is false when it is not valid JSON.
func decodeValue(data []byte) (value any, ok bool) {
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, false
	}
	return value, true
}
