package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJudgeChangedArgumentIsABaseRevision(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"absent", map[string]any{}, ""},
		{"a revision", map[string]any{"changed": "origin/main"}, "origin/main"},
		{"a revision with blanks", map[string]any{"changed": "  main "}, "main"},
		{"true means the uncommitted work", map[string]any{"changed": true}, "HEAD"},
		{"false is off", map[string]any{"changed": false}, ""},
		{"HEAD", map[string]any{"changed": "HEAD"}, "HEAD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := NewToolRequest(nil, tt.args)

			// Act
			got := changedBase(req)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
