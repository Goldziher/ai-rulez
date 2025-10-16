package handlers

import (
	"encoding/json"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolRequest wraps an MCP CallToolRequest and provides convenience accessors
// that mirror the helper methods from the legacy SDK.
type ToolRequest struct {
	raw       *mcp.CallToolRequest
	arguments map[string]any
}

func NewToolRequest(raw *mcp.CallToolRequest, input map[string]any) *ToolRequest {
	if input == nil {
		input = map[string]any{}
	}
	return &ToolRequest{
		raw:       raw,
		arguments: input,
	}
}

// Raw returns the underlying CallToolRequest.
func (r *ToolRequest) Raw() *mcp.CallToolRequest {
	return r.raw
}

// GetArguments returns the raw argument map.
func (r *ToolRequest) GetArguments() map[string]any {
	return r.arguments
}

// GetString returns the string value for key, or def if not present.
func (r *ToolRequest) GetString(key string, def string) string {
	if val, ok := r.arguments[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case json.Number:
			return v.String()
		}
	}
	return def
}

// GetBool returns the bool value for key, or def if not present.
func (r *ToolRequest) GetBool(key string, def bool) bool {
	if val, ok := r.arguments[key]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case string:
			switch v {
			case "true", "TRUE", "True", "1":
				return true
			case "false", "FALSE", "False", "0":
				return false
			}
		case float64:
			return v != 0
		case json.Number:
			f, err := v.Float64()
			if err == nil {
				return f != 0
			}
		}
	}
	return def
}

// GetNumber returns the numeric value for key, or def if not present.
func (r *ToolRequest) GetNumber(key string, def float64) float64 {
	if val, ok := r.arguments[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case uint:
			return float64(v)
		case uint64:
			return float64(v)
		case json.Number:
			f, err := v.Float64()
			if err == nil {
				return f
			}
		}
	}
	return def
}

// GetStringSlice returns a slice of strings for key, or def if not present.
func (r *ToolRequest) GetStringSlice(key string, def []string) []string {
	if val, ok := r.arguments[key]; ok {
		switch v := val.(type) {
		case []string:
			return append([]string(nil), v...)
		case []interface{}:
			res := make([]string, 0, len(v))
			for _, item := range v {
				if str, ok := item.(string); ok {
					res = append(res, str)
				}
			}
			return res
		}
	}
	if def == nil {
		return nil
	}
	return append([]string(nil), def...)
}

// GetFloat64Slice returns a slice of float64 for key, or def if not present.
func (r *ToolRequest) GetFloat64Slice(key string, def []float64) []float64 {
	if val, ok := r.arguments[key]; ok {
		switch v := val.(type) {
		case []float64:
			return append([]float64(nil), v...)
		case []interface{}:
			res := make([]float64, 0, len(v))
			for _, item := range v {
				if num, ok := toFloat64(item); ok {
					res = append(res, num)
				}
			}
			return res
		}
	}
	if def == nil {
		return nil
	}
	return append([]float64(nil), def...)
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f, true
		}
	}
	return 0, false
}
