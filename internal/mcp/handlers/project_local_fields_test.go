package handlers

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Every field update_config can apply must have a local overlay mapping, and no
// mapping may name a field update_config does not apply.
func TestLocalFields_CoverEveryUpdateConfigField(t *testing.T) {
	// Arrange: one value per field applyConfigUpdates understands.
	args := map[string]any{
		"name": "n", "description": "d", "builtins": []any{"go"}, "gitignore": true,
		"default_effort": "high", "default_effort_by_preset": map[string]any{"claude": "low"},
		"rules_mode": "native", "rules_mode_by_preset": map[string]any{"claude": "native"},
	}

	// Act
	updated, err := applyConfigUpdates(&config.Config{}, newRequestWithArgs(args))

	// Assert
	require.NoError(t, err)
	sort.Strings(updated)
	var mapped []string
	for field, fn := range localFields {
		assert.NotNil(t, fn, field)
		mapped = append(mapped, field)
	}
	sort.Strings(mapped)
	assert.Equal(t, mapped, updated)
	for _, field := range updated {
		_, _, _, ok := localFieldValue(&config.Config{}, field)
		assert.True(t, ok, field)
	}
	_, _, _, ok := localFieldValue(&config.Config{}, "no_such_field")
	assert.False(t, ok)
}

func TestAddIncludeHandler_DefaultsMergeStrategy(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	res, err := AddIncludeHandler(t.Context(), newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "inc", "source": "https://example.com/o/r.git", "local": true,
	}))

	require.NoError(t, err)
	assert.False(t, res.IsError, textOf(t, res))
}
