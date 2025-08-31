package testutil

import "fmt"

// ConvertYAMLToJSON is a test helper that mimics the internal convertYAMLToJSON function
// It converts YAML-like structures (map[any]any) to JSON-like structures (map[string]any)
func ConvertYAMLToJSON(i any) any {
	switch x := i.(type) {
	case map[any]any:
		m2 := map[string]any{}
		for k, v := range x {
			m2[fmt.Sprintf("%v", k)] = ConvertYAMLToJSON(v)
		}
		return m2
	case []any:
		for idx, v := range x {
			x[idx] = ConvertYAMLToJSON(v)
		}
	}
	return i
}
