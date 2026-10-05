package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
)

// documentRel is the slash-separated path the previous run's ownership record
// is keyed by: the document relative to the base directory of the run.
func documentRel(cfg *config.Config, docPath string) string {
	if docPath == "" || cfg == nil {
		return ""
	}
	rel, err := filepath.Rel(cfg.BaseDir, docPath)
	if err != nil {
		return filepath.ToSlash(docPath)
	}
	return filepath.ToSlash(rel)
}

// readPath returns the raw value at path in the JSON or JSONC document at docPath,
// or nil when the document or the key is absent or the document cannot be parsed
// (the merge reports that itself). A strict document keeps its bytes; a JSONC one
// (comments, trailing commas, a BOM) is read through the tolerant decoder, so a
// commented config is not mistaken for an empty one.
func readPath(docPath string, path []string) json.RawMessage {
	if docPath == "" {
		return nil
	}
	data, err := os.ReadFile(docPath) //nolint:gosec // path is derived from the preset layout and the base directory
	if err != nil {
		return nil
	}
	if json.Valid(data) {
		return strictPath(data, path)
	}
	value, ok := jsonmerge.LookupTree(docTree(docPath), path)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

// strictPath walks a strict JSON document and returns the value at path, or nil.
func strictPath(data []byte, path []string) json.RawMessage {
	var node json.RawMessage = data
	for _, key := range path {
		var object map[string]json.RawMessage
		if json.Unmarshal(node, &object) != nil {
			return nil
		}
		next, ok := object[key]
		if !ok {
			return nil
		}
		node = next
	}
	return node
}

// equalJSON compares two raw JSON values structurally.
func equalJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func containsJSON(list []json.RawMessage, value json.RawMessage) bool {
	for _, item := range list {
		if equalJSON(item, value) {
			return true
		}
	}
	return false
}

// arrayKey builds the owned key for an array in which ai-rulez owns only the
// elements in ours and the consumer owns the rest. The written value is the
// consumer's elements (byte-for-byte, in their order) plus the elements of ours
// the array lacks, appended in declaration order; elements an earlier run added
// and this one no longer wants leave with it. Only the elements ai-rulez added or
// claimed earlier are recorded as its own, so an identical element the consumer
// wrote stays theirs on clean.
func arrayKey(cfg *config.Config, docPath string, path []string, ours []json.RawMessage) jsonmerge.OwnedKey {
	if !isJSONDocument(docPath) {
		return nativeArrayKey(cfg, docPath, path, ours)
	}
	var existing []json.RawMessage
	if raw := readPath(docPath, path); raw != nil {
		if json.Unmarshal(raw, &existing) != nil {
			existing = nil // a non-array value is the consumer's; Apply reports it
		}
	}
	var previous []json.RawMessage
	for _, claim := range cfg.Run.PreviousClaims(documentRel(cfg, docPath)) {
		if !equalPath(claim.Path, path) {
			continue
		}
		for _, element := range claim.Elements {
			if raw, err := json.Marshal(element); err == nil {
				previous = append(previous, raw)
			}
		}
	}

	value := make([]any, 0, len(existing)+len(ours))
	kept := make([]json.RawMessage, 0, len(existing)+len(ours))
	for _, element := range existing {
		if containsJSON(previous, element) && !containsJSON(ours, element) {
			continue
		}
		kept = append(kept, element)
		value = append(value, element)
	}
	claimed := make([]any, 0, len(ours))
	for _, element := range ours {
		switch {
		case !containsJSON(kept, element):
			kept = append(kept, element)
			value = append(value, element)
		case !containsJSON(previous, element):
			continue // identical to an element the consumer wrote: theirs
		}
		var generic any
		if json.Unmarshal(element, &generic) == nil {
			claimed = append(claimed, generic)
		}
	}
	return jsonmerge.OwnedKey{Path: path, Value: value, Elements: claimed}
}

func equalPath(a, b []string) bool { return slices.Equal(a, b) }
