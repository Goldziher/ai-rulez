package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// unsupportedSchemaKey reports whether a keyword is left out of the provider-safe subset: Bedrock's
// strict structured output, Gemini and the OpenAI strict mode each reject some of them. Their limits
// are enforced in Go after parsing.
func unsupportedSchemaKey(k string) bool {
	switch k {
	case "$schema", "$id", "$comment", "title", "examples", "default",
		"minItems", "maxItems", "uniqueItems", "pattern", "format",
		"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "multipleOf":
		return true
	}
	return false
}

func loadSchema(name string) map[string]any {
	data, err := assets.ReadFile("schemas/" + name)
	if err != nil {
		panic("judge: missing embedded schema " + name) // a build-time invariant
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		panic("judge: embedded schema " + name + " is not JSON: " + err.Error())
	}
	return m
}

// JSON Schema type names the rewriting needs.
const (
	typeString  = "string"
	typeNumber  = "number"
	typeBoolean = "boolean"
	keyEnum     = "enum"
	keyDefs     = "$defs"
	keyType     = "type"
)

// obj returns m[key] as an object. The embedded schemas are fixed, so a miss is a build-time bug.
func obj(m map[string]any, key string) map[string]any {
	v, ok := m[key].(map[string]any)
	if !ok {
		panic("judge: an embedded schema has no object " + key)
	}
	return v
}

// candidateCodeProperty sets the code property of a candidate schema to the given codes.
func candidateCodeProperty(candidate map[string]any, codes []string) {
	obj(candidate, "properties")["code"] = map[string]any{keyType: typeString, keyEnum: stringsToAny(codes)}
}

// reviewerSchema returns the output schema of a category reviewer: the shared reviewer schema with
// the code enum narrowed to the codes of the category (the rubric failure code is allowed everywhere,
// for rubric criteria), in the provider-safe subset.
func reviewerSchema(cat Category) map[string]any {
	s := loadSchema("reviewer-output.schema.json")
	codes := append([]string(nil), CodesOf(cat)...)
	if cat != CategoryGovernance {
		codes = append(codes, codeRubricFailure)
	}
	candidateCodeProperty(obj(obj(s, keyDefs), "candidate"), codes)
	return ProviderSafeSchema(s)
}

// adjudicatorSchema returns the output schema of the adjudicator, with the escalation candidate
// inlined from the reviewer schema.
func adjudicatorSchema() map[string]any {
	reviewer := loadSchema("reviewer-output.schema.json")
	s := loadSchema("adjudicator-output.schema.json")
	// An escalation is a reviewer candidate; its $refs (location, step) resolve against the reviewer's $defs.
	s[keyDefs] = reviewer[keyDefs]
	cand := obj(obj(s, keyDefs), "candidate")
	obj(obj(s, "properties"), "escalations")["items"] = cand
	table := codeTable()
	codes := make([]string, 0, len(table))
	for i := range table {
		codes = append(codes, table[i].Code)
	}
	candidateCodeProperty(cand, codes)
	return ProviderSafeSchema(s)
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// ProviderSafeSchema rewrites a JSON Schema into the subset every supported provider accepts: local
// $refs are inlined (no $defs), keywords the strict modes reject are removed, an enum or const gets
// a type, and objects stay closed. The input is not modified.
func ProviderSafeSchema(in map[string]any) map[string]any {
	var defs map[string]any
	if d, ok := in[keyDefs].(map[string]any); ok {
		defs = d
	}
	out, ok := rewriteSchema(in, defs, 0).(map[string]any)
	if !ok {
		return nil
	}
	delete(out, keyDefs)
	return out
}

const maxSchemaDepth = 24

func rewriteSchema(node any, defs map[string]any, depth int) any {
	if depth > maxSchemaDepth {
		return node
	}
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
			if target, found := defs[strings.TrimPrefix(ref, "#/$defs/")]; found {
				return rewriteSchema(target, defs, depth+1)
			}
		}
		return rewriteObject(n, defs, depth)
	case []any:
		out := make([]any, len(n))
		for i, e := range n {
			out[i] = rewriteSchema(e, defs, depth+1)
		}
		return out
	}
	return node
}

// rewriteObject rewrites one schema object: it drops the unsupported keywords, keeps the names of
// "properties" (a property may be called "format" or "title"), and types an enum.
func rewriteObject(n, defs map[string]any, depth int) map[string]any {
	out := make(map[string]any, len(n))
	for k, v := range n {
		if k == keyDefs || unsupportedSchemaKey(k) {
			continue
		}
		if props, ok := v.(map[string]any); ok && k == "properties" {
			rewritten := make(map[string]any, len(props))
			for pk, pv := range props {
				rewritten[pk] = rewriteSchema(pv, defs, depth+1)
			}
			out[k] = rewritten
			continue
		}
		out[k] = rewriteSchema(v, defs, depth+1)
	}
	if c, ok := out["const"]; ok {
		delete(out, "const")
		out[keyEnum] = []any{c}
	}
	if values, hasEnum := out[keyEnum]; hasEnum {
		if _, typed := out[keyType]; !typed {
			out[keyType] = enumType(values)
		}
	}
	return out
}

func enumType(enum any) string {
	if list, ok := enum.([]any); ok && len(list) > 0 {
		switch list[0].(type) {
		case float64:
			return typeNumber
		case bool:
			return typeBoolean
		}
	}
	return typeString
}

// SchemaSHA256 hashes a schema canonically (sorted keys), for the run provenance and the cache key.
func SchemaSHA256(schema map[string]any) string {
	sum := sha256.Sum256(jsonText(schema))
	return hex.EncodeToString(sum[:])
}

// format wraps a schema as a structured-output request.
func format(name string, schema map[string]any) *llm.JSONSchemaFormat {
	return &llm.JSONSchemaFormat{Name: name, Schema: schema}
}

// decodeStrict decodes exactly one JSON object into dst: unknown fields, trailing data and a code
// fence around the object are rejected or stripped the way the model layer does.
func decodeStrict(text string, dst any) error {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	if text == "" {
		return errors.New("the reply is empty")
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("the reply is not the requested JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("the reply has data after the JSON object")
	}
	return nil
}

// ParseReviewerOutput decodes and shape-checks a reviewer reply for a category. Per-candidate
// evidence is checked later by the Verifier; this checks only what the schema would have enforced.
func ParseReviewerOutput(text string, cat Category, maxFindings int) (ReviewerOutput, error) {
	var out ReviewerOutput
	if err := decodeStrict(text, &out); err != nil {
		return out, err
	}
	if out.Findings == nil {
		out.Findings = []Candidate{}
	}
	if len(out.Findings) > maxFindings*2 {
		return out, fmt.Errorf("the reply holds %d findings; at most %d", len(out.Findings), maxFindings)
	}
	seen := map[string]bool{}
	for i := range out.Findings {
		id := out.Findings[i].ID
		if seen[id] {
			return out, fmt.Errorf("finding id %q is used twice", id)
		}
		seen[id] = true
	}
	for _, c := range out.CriteriaResults {
		switch c.Result {
		case CriterionPass, CriterionFail, CriterionNotApplicable:
		default:
			return out, fmt.Errorf("criterion %q has result %q; use pass, fail or not_applicable", c.RubricID, c.Result)
		}
	}

	return out, nil
}

// ParseAdjudicatorOutput decodes an adjudicator reply.
func ParseAdjudicatorOutput(text string) (AdjudicatorOutput, error) {
	var out AdjudicatorOutput
	if err := decodeStrict(text, &out); err != nil {
		return out, err
	}
	return out, nil
}

// sortedKeys is a helper for deterministic iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
