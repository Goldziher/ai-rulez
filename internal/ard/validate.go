package ard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"github.com/samber/oops"
)

// Query count bounds of spec section 4.2 ("SHOULD contain 2-5 examples").
const (
	MinQueries = 2
	MaxQueries = 5
)

// Validate checks an ard.json manifest. Schema violations against the vendored
// ArdManifest definition are errors; the discovery constraints of spec section
// D.2 add an error per identifier whose publisher is not an FQDN or that two
// entries share, and a warning per entry whose representativeQueries is
// missing or outside 2-5. The error is non-nil only when data is not JSON or
// the schema cannot be compiled. Findings are sorted.
func Validate(data []byte) ([]Finding, error) {
	v, err := loadValidators()
	if err != nil {
		return nil, err
	}
	var doc any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, oops.Wrapf(err, "parsing the ARD manifest")
	}
	if dec.More() {
		return nil, oops.Errorf("parsing the ARD manifest: data after the JSON document")
	}
	findings := v.schemaFindings(doc)
	findings = append(findings, discoveryFindings(doc)...)
	sortFindings(findings)
	return findings, nil
}

// schemaFindings validates the manifest against ArdManifest. When entries is an
// array (the only other thing ArdManifest constrains), each entry is validated
// against ArdEntry on its own, so a finding names its entry and a JSON pointer
// from the manifest root: the validator reports locations relative to the
// subschema a $ref enters.
func (v *validators) schemaFindings(doc any) []Finding {
	root := asMap(doc)
	entries, isArray := root["entries"].([]any)
	if !isArray {
		if res := v.manifest.Validate(doc); !res.IsValid() {
			_, present := root["entries"]
			return listFindings(res.ToList(), "", "", present)
		}
		return nil
	}
	var out []Finding
	for i, raw := range entries {
		if res := v.entry.Validate(raw); !res.IsValid() {
			entry := asMap(raw)
			ident := asString(entry["identifier"])
			out = append(out, listFindings(res.ToList(), fmt.Sprintf("/entries/%d", i), ident, true)...)
		}
	}
	return out
}

// aggregateKeywords only repeat that a subschema failed; the subschema's own
// error says why, so they are dropped.
var aggregateKeywords = map[string]bool{"$ref": true, "allOf": true, "items": true, "properties": true}

// urlXorData replaces the errors of the schema's only oneOf, url-xor-data
// (spec section 4.3), whose branch errors read as nonsense on their own.
const urlXorData = "exactly one of url or data is required (spec section 4.3)"

// listFindings turns a hierarchical validation result into findings under
// prefix. It descends only into failed nodes, never into a "not" subschema
// (which is meant to fail) or the oneOf branches, which it reports as one
// url-xor-data finding. When entries is absent the validator also reports a
// type error for the missing value; entriesPresent=false drops it, as the
// "required" error already says it.
func listFindings(list *jsonschema.List, prefix, ident string, entriesPresent bool) []Finding {
	var out []Finding
	seen := map[string]bool{}
	emit := func(loc, keyword, msg string) {
		path := prefix + loc
		if path == "" {
			path = "/"
		}
		key := path + "\x00" + keyword + "\x00" + msg
		if !seen[key] {
			seen[key] = true
			out = append(out, Finding{Rule: RuleSchema, Severity: SeverityError, Identifier: ident,
				Path: path, Message: keyword + ": " + msg})
		}
	}
	var walk func(l jsonschema.List)
	walk = func(l jsonschema.List) {
		if l.Valid || strings.HasSuffix(l.EvaluationPath, "/not") || isOneOfBranch(l.EvaluationPath) {
			return
		}
		if !entriesPresent && l.InstanceLocation == "/entries" {
			return
		}
		for keyword, msg := range l.Errors {
			switch {
			case keyword == "oneOf":
				emit(l.InstanceLocation, keyword, urlXorData)
			case !aggregateKeywords[keyword]:
				emit(l.InstanceLocation, keyword, msg)
			}
		}
		for _, d := range l.Details {
			walk(d)
		}
	}
	walk(*list)
	if len(out) == 0 { // every error was an aggregate: keep one so the failure shows
		emit("", "schema", "does not match the ARD schema")
	}
	return out
}

func isOneOfBranch(evaluationPath string) bool {
	return strings.Contains(evaluationPath, "/oneOf/")
}

// discoveryFindings applies the checks of spec section D.2 that the schema
// leaves to conformance tooling.
func discoveryFindings(doc any) []Finding {
	root := asMap(doc)
	entries := asSlice(root["entries"])
	var out []Finding
	seen := map[string]bool{}
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("/entries/%d", i)
		ident := asString(entry["identifier"])
		if ident != "" {
			if _, err := ParseIdentifier(ident); err != nil {
				out = append(out, Finding{Rule: RuleIdentifier, Severity: SeverityError, Identifier: ident,
					Path: path + "/identifier", Message: err.Error()})
			}
			if seen[ident] {
				out = append(out, Finding{Rule: RuleIdentifier, Severity: SeverityError, Identifier: ident,
					Path: path + "/identifier", Message: "identifier is used by more than one entry"})
			}
			seen[ident] = true
		}
		if f, bad := queryFinding(entry["representativeQueries"]); bad {
			f.Identifier, f.Path = ident, path+"/representativeQueries"
			out = append(out, f)
		}
	}
	return out
}

func queryFinding(raw any) (Finding, bool) {
	if raw == nil {
		return Finding{Rule: RuleQueries, Severity: SeverityWarning,
			Message: "no representativeQueries: registries build their search index from them, " +
				"so the entry will not be found by search"}, true
	}
	list, ok := raw.([]any)
	if !ok || (len(list) >= MinQueries && len(list) <= MaxQueries) {
		return Finding{}, false // a non-array is a schema error already
	}
	return Finding{Rule: RuleQueries, Severity: SeverityWarning,
		Message: fmt.Sprintf("%d representativeQueries; the spec recommends %d to %d",
			len(list), MinQueries, MaxQueries)}, true
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func sortFindings(f []Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		if f[i].Path != f[j].Path {
			return f[i].Path < f[j].Path
		}
		if f[i].Rule != f[j].Rule {
			return f[i].Rule < f[j].Rule
		}
		return f[i].Message < f[j].Message
	})
}
