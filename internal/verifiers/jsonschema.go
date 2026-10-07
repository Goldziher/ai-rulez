package verifiers

// JSON-schema types used to build the structured-output schemas of the llm
// predicate and of suggest.
const (
	jsonObject  = "object"
	jsonArray   = "array"
	jsonString  = "string"
	jsonInteger = "integer"
	jsonBoolean = "boolean"

	jsonTypeKey = "type"
)

// schemaField is one named property of an object schema.
type schemaField struct {
	name   string
	schema map[string]any
}

// schemaOfType is the schema of a scalar of the given JSON type.
func schemaOfType(typ string) map[string]any {
	return map[string]any{jsonTypeKey: typ}
}

// schemaEnumOf is the schema of a string restricted to the given values.
func schemaEnumOf(values ...string) map[string]any {
	return map[string]any{jsonTypeKey: jsonString, "enum": values}
}

// schemaArrayOf is the schema of an array whose items follow items.
func schemaArrayOf(items map[string]any) map[string]any {
	return map[string]any{jsonTypeKey: jsonArray, "items": items}
}

// schemaObjectOf is the schema of an object whose every field is required, in
// the order given.
func schemaObjectOf(fields ...schemaField) map[string]any {
	props := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))
	for _, f := range fields {
		props[f.name] = f.schema
		required = append(required, f.name)
	}
	return map[string]any{jsonTypeKey: jsonObject, "properties": props, "required": required}
}
