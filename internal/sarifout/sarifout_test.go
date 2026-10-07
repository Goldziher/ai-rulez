package sarifout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArtifactURI(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantURI  string
		wantBase string
	}{
		{"relative", "docs/a.md", "docs/a.md", SrcRoot},
		{"windows separators", `docs\a.md`, "docs/a.md", SrcRoot},
		{"spaces and hashes are escaped", "my docs/a#b.md", "my%20docs/a%23b.md", SrcRoot},
		{"absolute is a file uri", "/etc/x y", "file:///etc/x%20y", ""},
		{"a colon in a top-level file name is not a scheme", "notes:v1.md", "notes%3Av1.md", SrcRoot},
		{"a colon in a nested name is not a scheme", "docs/a:b.md", "docs/a%3Ab.md", SrcRoot},
		{"a scheme is kept as a file uri path", "C:/x", "file:///C:/x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, base := ArtifactURI(tt.in)

			assert.Equal(t, tt.wantURI, uri)
			assert.Equal(t, tt.wantBase, base)
		})
	}
}

func TestLevel(t *testing.T) {
	assert.Equal(t, "error", Level("error"))
	assert.Equal(t, "warning", Level("warning"))
	assert.Equal(t, "note", Level("info"))
	assert.Equal(t, "note", Level(""))
}
