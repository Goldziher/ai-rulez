package schema_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/schema"
	"github.com/samber/oops"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeDependencyEnabledConfigSchema(t *testing.T) {
	for _, test := range []struct {
		name, field string
		value       any
		invalid     bool
	}{
		{name: "omitted"},
		{name: "enabled", field: "opencode_dependency_enabled", value: true},
		{name: "disabled", field: "opencode_dependency_enabled", value: false},
		{name: "string is not boolean", field: "opencode_dependency_enabled", value: "false", invalid: true},
		{name: "number is not boolean", field: "opencode_dependency_enabled", value: 0, invalid: true},
		{name: "null is not boolean", field: "opencode_dependency_enabled", value: nil, invalid: true},
		{name: "array is not boolean", field: "opencode_dependency_enabled", value: []any{}, invalid: true},
		{name: "unknown field", field: "opencode_dependencies_enabled", value: false, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			plugin := map[string]any{"name": "example", "version": "1.2.3", "description": "Example."}
			if test.field != "" {
				plugin[test.field] = test.value
			}
			data, err := json.Marshal(map[string]any{"version": "5.0", "name": "example", "plugin": plugin})
			require.NoError(t, err)
			err = schema.ValidateWithSchema(data)
			if test.invalid {
				require.Error(t, err)
				detail, ok := oops.AsOops(err)
				require.True(t, ok)
				require.Contains(t, fmt.Sprint(detail.Context()["errors"]), test.field)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
