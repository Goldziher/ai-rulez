package schema

import "strings"

// enumMarker splits the library's enum message "Value <x> should be one of the
// allowed values: a, b" into the (secret) supplied value and the allowed list.
const enumMarker = " should be one of the allowed values: "

// safeMessagePrefixes are message shapes that name only types, counts or schema
// keys, never the supplied value.
var safeMessagePrefixes = []string{
	"Value is ",             // "Value is string but should be array": type names only
	"Required property",     // names the missing key
	"Additional propert",    // names the unexpected key
	"Item at index",         // position only
	"Array ",                // size constraints
	"Property ",             // names a key
	"Object ",               // size constraints
	"Value must be a ",      // type constraints
	"Value must not be nil", // no value
}

// redactSchemaValues rewrites validation messages so they never echo the value
// the user supplied (overlay files hold secrets). Enum errors keep the allowed
// values; messages of an unknown shape collapse to a generic one.
func redactSchemaValues(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = redactSchemaLine(line)
	}
	return out
}

func redactSchemaLine(line string) string {
	where, msg, ok := strings.Cut(line, ": ")
	if !ok {
		return "- invalid value"
	}
	// Nested errors read "- path: Item at index 0 does not match the schema - path.field.keyword: <message>";
	// only the last segment can carry a value.
	if idx := strings.Index(msg, " - "); idx >= 0 && strings.HasPrefix(msg, "Item at index") {
		head := msg[:idx]
		tail := redactSchemaLine("- " + strings.TrimPrefix(msg[idx+3:], "- "))
		return where + ": " + head + " " + tail
	}
	return where + ": " + redactMessage(msg)
}

func redactMessage(msg string) string {
	if before, allowed, found := strings.Cut(msg, enumMarker); found && strings.HasPrefix(before, "Value ") {
		return "Value is not one of the allowed values: " + allowed
	}
	for _, prefix := range safeMessagePrefixes {
		if strings.HasPrefix(msg, prefix) {
			return msg
		}
	}
	return "Value is invalid"
}
