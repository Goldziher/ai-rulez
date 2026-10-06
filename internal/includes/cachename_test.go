package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeCacheName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"shared", "shared"},
		{"my include/v2", "my_include_v2"},
		{"..", "include"},
		{"", "include"},
		{".hidden.", "hidden"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, SafeCacheName(tt.in))
		})
	}
}

func TestCacheDirMatches(t *testing.T) {
	tests := []struct {
		name string
		seg  string
		inc  string
		want bool
	}{
		{"exact", "shared-0123456789ab", "shared", true},
		{"name needing escaping", "my_include-0123456789ab", "my include", true},
		{"longer name sharing the prefix", "shared-extra-0123456789ab", "shared", false},
		{"short hash", "shared-0123456789a", "shared", false},
		{"uppercase hash", "shared-0123456789AB", "shared", false},
		{"no dash", "shared0123456789ab", "shared", false},
		{"other name", "other-0123456789ab", "shared", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CacheDirMatches(tt.seg, tt.inc))
		})
	}
}
