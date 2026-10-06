package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLockSource(t *testing.T) {
	tests := []struct {
		name, base, source, want string
	}{
		{"https is only redacted", "/work/proj", "https://tok:x@github.com/o/r.git", "https://<redacted>@github.com/o/r.git"},
		{"ssh unchanged", "/work/proj", "git@github.com:o/r.git", "git@github.com:o/r.git"},
		{"file url is kept as written", "/work/proj", "file://../shared", "file://../shared"},
		{"absolute file url is kept as written", "/work/proj", "file:///work/shared", "file:///work/shared"},
		{"trailing slash is dropped", "/work/proj", "file://./vendor/shared/", "file://./vendor/shared"},
		{"git+file prefix kept", "/work/proj", "git+file://./x", "git+file://./x"},
		{"same at any depth", "/a/b/c/d", "file://../shared", "file://../shared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, lockSource(tt.base, tt.source))
		})
	}
}
