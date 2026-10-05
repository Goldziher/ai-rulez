package rulefiles

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMappedLines_CollapsesNewlines(t *testing.T) {
	// Arrange
	it := item("multi\nline", config.ActivationAuto, "first\r\nsecond\tthird\ninjected: key", "a/**\nb", "c")
	m := &ActivationMap{
		Format: MappedFormatLines,
		Auto: map[string]any{
			"instructions": "{description}",
			"name":         "{name}",
			"patterns":     "{globs_list}",
			"joined":       "{globs}",
		},
	}

	// Act
	fm, _ := mappedFrontmatter(m, it, config.ActivationAuto)
	out := MappedLines(map[string]any{"raw": "x\ny", "list": []string{"a\nb", "c"}, "n": 3})

	// Assert
	for _, line := range strings.Split(strings.TrimRight(MappedLines(fm), "\n"), "\n") {
		key, _, found := strings.Cut(line, ": ")
		require.True(t, found, "line %q must be key: value", line)
		assert.Contains(t, []string{"instructions", "name", "patterns", "joined"}, key, "injected line: %q", line)
	}
	assert.Equal(t, "list: a b, c\nn: 3\nraw: x y\n", out)
}

func TestValidateActivationKeyAndEmbeddedList(t *testing.T) {
	for key, valid := range map[string]bool{
		"apply": true, "_x-y": true, "a1": true, "": false, "1a": false, "a b": false, "a\nb": false, "a:b": false,
	} {
		assert.Equal(t, valid, ValidActivationKey(key), key)
	}
	assert.True(t, ActivationValueHasEmbeddedList("x {globs_list}"))
	assert.False(t, ActivationValueHasEmbeddedList("{globs_list}"))
	assert.False(t, ActivationValueHasEmbeddedList("{globs}"))
}
