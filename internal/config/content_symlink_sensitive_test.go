package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSensitiveTarget(t *testing.T) {
	root := "/proj"
	tests := []struct {
		rel       string
		sensitive bool
	}{
		{".git/config", true},
		{".Git/config", true},
		{".GIT/HEAD", true},
		{"sub/.gIt/hooks/x", true},
		{".gitignore", false},
		{"docs/guide.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got := sensitiveTarget(root, root+"/"+tt.rel)
			assert.Equal(t, tt.sensitive, got != "", got)
		})
	}
}
