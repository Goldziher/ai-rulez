package generator

import (
	"encoding/json"
	"os"
	"reflect"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// unionOutputs combines two renders of one merged document by different presets
// (Copilot and Zoo Code both own keys of .vscode/settings.json; Copilot and
// Copilot CLI both feed .github/hooks/ai-rulez.json): the owned keys of both are
// merged once, so neither preset's keys replace the other's. ok is false when the
// outputs do not describe a union (a plain file, other protections, other
// documents) or two keys of one path cannot be reconciled; the caller then
// reports the difference as a conflict.
func unionOutputs(read jsonmerge.Reader, kept, other config.OutputFile) (config.OutputFile, bool) {
	if read == nil {
		read = os.ReadFile
	}
	a, b := kept.Merge, other.Merge
	if a == nil || b == nil || a.Path != b.Path || !sameMergeFormat(a.Format, b.Format) ||
		kept.Sensitive != other.Sensitive || kept.LocalOnly != other.LocalOnly {
		return kept, false
	}
	owned, ok := unionOwned(a.Owned, b.Owned)
	if !ok {
		return kept, false
	}
	union := kept
	union.Merge = &config.MergeSource{Path: a.Path, Format: a.Format, Owned: owned}
	if a.Format == config.MergeFormatOwnedHooks {
		body, err := settings.RenderOwnedHooks(owned)
		if err != nil {
			return kept, false
		}
		union.Content = body
		return union, true
	}
	result, err := docmerge.ApplyWith(read, a.Path, docmerge.Format(a.Format), owned)
	if err != nil {
		return kept, false
	}
	union.Content = result.Body
	union.PartiallyOwned = result.PartiallyOwned
	union.MergeClaims = result.Claims
	return union, true
}

// sameMergeFormat reports whether two renders use one document syntax: JSON and
// JSONC are one engine.
func sameMergeFormat(a, b string) bool {
	jsonLike := func(f string) bool { return f == string(docmerge.FormatJSON) || f == string(docmerge.FormatJSONC) }
	return a == b || (jsonLike(a) && jsonLike(b))
}

// unionOwned merges two lists of owned keys. A key both lists hold is kept once
// when identical, and its elements are combined when it is an array each side
// extends; any other disagreement is not reconcilable.
func unionOwned(a, b []jsonmerge.OwnedKey) ([]jsonmerge.OwnedKey, bool) {
	out := append([]jsonmerge.OwnedKey(nil), a...)
	for _, key := range b {
		index := -1
		for i := range out {
			if reflect.DeepEqual(out[i].Segments(), key.Segments()) {
				index = i
				break
			}
		}
		switch {
		case index < 0:
			out = append(out, key)
		case reflect.DeepEqual(out[index], key):
		default:
			combined, ok := unionArrayKey(out[index], key)
			if !ok {
				return nil, false
			}
			out[index] = combined
		}
	}
	return out, true
}

// unionArrayKey combines two keys of one array: the elements of the first, then
// those of the second it lacks. Both must treat the array alike (whole value, or
// consumer array with owned elements).
func unionArrayKey(a, b jsonmerge.OwnedKey) (jsonmerge.OwnedKey, bool) {
	if a.Members || b.Members || a.Remove || b.Remove || a.Alone != b.Alone || (a.Elements == nil) != (b.Elements == nil) {
		return a, false
	}
	value, okA := unionElements(a.Value, b.Value)
	if !okA {
		return a, false
	}
	combined := a
	combined.Value = value
	if a.Elements != nil {
		elements, ok := unionElements(a.Elements, b.Elements)
		if !ok {
			return a, false
		}
		list, isList := elements.([]any)
		if !isList {
			list = nil
		}
		combined.Elements = list
	}
	return combined, true
}

// unionElements appends the elements of y that x does not hold, compared as JSON.
// It keeps the element type of x so the engines render it as before.
func unionElements(x, y any) (any, bool) {
	switch xs := x.(type) {
	case []any:
		ys, ok := y.([]any)
		if !ok {
			return nil, false
		}
		out := append([]any(nil), xs...)
		for _, element := range ys {
			if !containsJSONValue(out, element) {
				out = append(out, element)
			}
		}
		return out, true
	case []json.RawMessage:
		ys, ok := y.([]json.RawMessage)
		if !ok {
			return nil, false
		}
		out := append([]json.RawMessage(nil), xs...)
		for _, element := range ys {
			if !containsJSONValue(rawAsAny(out), element) {
				out = append(out, element)
			}
		}
		return out, true
	}
	return nil, false
}

func rawAsAny(list []json.RawMessage) []any {
	out := make([]any, len(list))
	for i, raw := range list {
		out[i] = raw
	}
	return out
}

func containsJSONValue(list []any, value any) bool {
	want := canonicalJSON(value)
	for _, item := range list {
		if canonicalJSON(item) == want {
			return true
		}
	}
	return false
}

// canonicalJSON is the comparison form of a value: its JSON text with object keys
// sorted.
func canonicalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var generic any
	if json.Unmarshal(raw, &generic) != nil {
		return string(raw)
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		return string(raw)
	}
	return string(canonical)
}
