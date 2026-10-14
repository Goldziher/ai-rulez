package mcp

import (
	"fmt"
	"math"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

// maxIntArg bounds a numeric tool argument read as an int. It is the largest
// value every platform's int holds, so a request means the same on all of them.
const maxIntArg = math.MaxInt32

// intArg reads a non-negative whole-number tool argument. JSON numbers arrive
// as float64, and converting a NaN, an infinity or a value beyond the int range
// is implementation-defined in Go, so those, fractions and negatives are
// rejected with an error naming the argument instead of being converted.
func intArg(req *handlers.ToolRequest, key string, def int) (int, error) {
	v := req.GetNumber(key, float64(def))
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return 0, fmt.Errorf("%s must be a finite number", key)
	case v != math.Trunc(v):
		return 0, fmt.Errorf("%s must be a whole number, got %v", key, v)
	case v < 0 || v > maxIntArg:
		return 0, fmt.Errorf("%s must be between 0 and %d, got %v", key, maxIntArg, v)
	}
	return int(v), nil
}

// limitArg reads a count argument where zero or below means "use def", the
// semantics find_skill, search_skills and budget_bytes always had. A NaN, an
// infinity, a fraction or a value beyond the int range is rejected like intArg.
func limitArg(req *handlers.ToolRequest, key string, def int) (int, error) {
	if v := req.GetNumber(key, float64(def)); v <= 0 {
		return def, nil
	}
	return intArg(req, key, def)
}
