package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitLocationNormalizesTheHostName(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantHost string
		wantSegs []string
	}{
		{"trailing dot is the same host", "https://GitHub.com./Evil/repo", "github.com", []string{"evil", "repo"}},
		{"trailing dot with a port", "https://github.com.:8443/o/r", "github.com", []string{"o", "r"}},
		{"scp-like trailing dot", "git@github.com.:o/r.git", "github.com", []string{"o", "r"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			host, segs, ok := splitLocation(tt.source)
			// Assert
			require.True(t, ok)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantSegs, segs)
		})
	}
}
