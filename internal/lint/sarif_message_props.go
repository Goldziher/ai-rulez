package lint

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	// maxMessageProperties bounds how many message_properties an entry may list.
	maxMessageProperties = 16
	// maxMessagePropertyPath bounds the length of one message_properties entry.
	maxMessagePropertyPath = 128
	// locationSnippetPath is the message_properties entry for the standard SARIF
	// region snippet of the first location.
	locationSnippetPath = "location.snippet"
)

var messagePropertyPath = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
})

// messagePropertiesProblems lists what is wrong with an entry's message_properties.
func messagePropertiesProblems(props []string) []string {
	var problems []string
	if len(props) > maxMessageProperties {
		problems = append(problems, fmt.Sprintf("message_properties lists %d paths, at most %d are allowed", len(props), maxMessageProperties))
	}
	for _, p := range props {
		if len(p) > maxMessagePropertyPath || !messagePropertyPath().MatchString(p) {
			problems = append(problems, fmt.Sprintf("message_properties entry %q is not a dotted property path such as \"evidence\" or \"location.snippet\" (letters, digits, _ and -, at most %d characters)", p, maxMessagePropertyPath))
		}
	}
	return problems
}

// messageExtra renders the values the message_properties paths select from a
// SARIF result as "name: value; name: value". Values are sanitized like a
// message; a missing, empty or object-valued property is skipped.
func messageExtra(res sarifResult, paths []string) string {
	var parts []string
	for _, path := range paths {
		var raw string
		if path == locationSnippetPath {
			if len(res.Locations) > 0 {
				raw = res.Locations[0].PhysicalLocation.Region.Snippet.Text
			}
		} else {
			raw = propertyText(lookupPath(res.Properties, path))
		}
		raw = cleanScannerText(clipRunes(raw, maxScannerMessageLimit))
		if raw == "" {
			continue
		}
		name := path[strings.LastIndexByte(path, '.')+1:]
		parts = append(parts, name+": "+raw)
	}
	return strings.Join(parts, "; ")
}

// lookupPath follows a dotted path through nested JSON objects.
func lookupPath(m map[string]any, path string) any {
	var cur any = m
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		if cur, ok = obj[key]; !ok {
			return nil
		}
	}
	return cur
}

// propertyText is the text of a scalar JSON value, or of an array of scalars
// joined by ", ". Objects, nested arrays and null give "".
func propertyText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		items := make([]string, 0, len(x))
		for _, e := range x {
			if _, nested := e.([]any); nested {
				continue
			}
			if s := propertyText(e); s != "" {
				items = append(items, s)
			}
		}
		return strings.Join(items, ", ")
	}
	return ""
}

// clipRunes cuts s to at most limit runes, without an ellipsis.
func clipRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}
