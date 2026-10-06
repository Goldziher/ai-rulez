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
		{"file url inside the project", "/work/proj", "file:///work/proj/vendor/shared", "file://./vendor/shared"},
		{"file url beside the project", "/work/proj", "file:///work/shared", "file://../shared"},
		{"file url equal to the project", "/work/proj", "file:///work/proj", "file://."},
		{"relative file url unchanged", "/work/proj", "file://./vendor/shared", "file://./vendor/shared"},
		{"git+file prefix kept", "/work/proj", "git+file:///work/proj/x", "git+file://./x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, lockSource(tt.base, tt.source))
		})
	}
}
