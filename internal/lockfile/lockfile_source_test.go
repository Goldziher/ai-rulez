package lockfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoversToleratesLegacyFileSources(t *testing.T) {
	tests := []struct {
		name, recorded, configured string
		want                       bool
	}{
		{"identical", "file://../increpo", "file://../increpo", true},
		{"legacy absolute", "file:///work/increpo", "file://../increpo", true},
		{"legacy deep relative", "file://../../../increpo", "file://../increpo", true},
		{"different directory", "file://../other", "file://../increpo", false},
		{"https differs", "https://a/b", "https://a/c", false},
		{"git+ mismatch", "git+file://../x", "file://../x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Source: tt.recorded}
			assert.Equal(t, tt.want, e.Covers(Want{Source: tt.configured}))
		})
	}
}
