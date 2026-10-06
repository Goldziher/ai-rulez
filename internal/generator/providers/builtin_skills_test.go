package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuiltinRendersSkills(t *testing.T) {
	tests := []struct {
		preset string
		want   bool
	}{
		{"claude", true},
		{"gitlab-duo", false},
		{"cursor", true}, // a Go preset has no embedded spec
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			assert.Equal(t, tt.want, BuiltinRendersSkills(tt.preset))
		})
	}
}
