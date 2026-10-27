package presets

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateDescription_RuneSafe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
	}{
		{"ascii within limit", "short", 20},
		{"ascii over limit", strings.Repeat("a", 50), 20},
		{"cyrillic over limit", strings.Repeat("Правило ", 30), 41},
		{"cyrillic odd boundary", strings.Repeat("ж", 40), 20},
		{"emoji over limit", strings.Repeat("🚀", 40), 21},
		{"emoji boundary", strings.Repeat("a🚀", 40), 22},
		{"group limit mixed", strings.Repeat("Тест 🚀, ", 40), scopedGroupDescriptionLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateDescription(tt.input, tt.limit)

			assert.True(t, utf8.ValidString(got), "invalid UTF-8: %q", got)
			assert.LessOrEqual(t, len(got), tt.limit)
			if len(tt.input) > tt.limit {
				assert.True(t, strings.HasSuffix(got, "..."))
			} else {
				assert.Equal(t, tt.input, got)
			}
		})
	}
}
