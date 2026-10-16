package airulez

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualRootUsesAHostAbsolutePath(t *testing.T) {
	for _, tt := range []struct{ host, want string }{
		{"windows", "C:/.ai-rulez-virtual/0123456789abcdef"},
		{"linux", "/.ai-rulez-virtual/0123456789abcdef"},
		{"darwin", "/.ai-rulez-virtual/0123456789abcdef"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, virtualRootFor(tt.host, "0123456789abcdef"))
		})
	}
	root := NewMemWorkspace().Root()
	assert.True(t, filepath.IsAbs(root), "engine paths must never depend on the process working directory: %s", root)
}
