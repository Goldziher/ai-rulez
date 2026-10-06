package includes

import (
	"context"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteTagDate(t *testing.T) {
	// Arrange: a lightweight and an annotated tag, made just now.
	t.Setenv("HOME", t.TempDir())
	repo := tagtest.New(t)
	repo.Write("a.txt", "a")
	repo.Commit("one")
	repo.Tag("v1.0.0")
	repo.AnnotatedTag("v1.1.0")
	repo.Tag("deploy/v2.0.0")

	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{name: "lightweight tag has the commit date", tag: "v1.0.0"},
		{name: "annotated tag has the tagger date", tag: "v1.1.0"},
		{name: "monorepo tag with a slash", tag: "deploy/v2.0.0"},
		{name: "unknown tag", tag: "v9.9.9", wantErr: true},
		{name: "option-like name is refused before git runs", tag: "--upload-pack=x", wantErr: true},
		{name: "refspec injection is refused", tag: "v1:refs/heads/main", wantErr: true},
		{name: "dot dot is refused", tag: "a..b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := RemoteTagDate(context.Background(), repo.URL, "", tt.tag)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.WithinDuration(t, time.Now(), got, 2*time.Minute)
		})
	}
}

func TestSafeTagRef(t *testing.T) {
	for _, ok := range []string{"v1.2.3", "deploy/v2.1.3", "release-1_0", "1.2.3+build"} {
		assert.True(t, safeTagRef(ok), ok)
	}
	for _, bad := range []string{"", "-x", "a b", "a:b", "a^b", "a~b", "a?b", "a*b", "a[b", `a\b`, "a..b", "a@{b", "a//b", "a.lock", "a.", "/a", "a/", "a\x01b"} {
		assert.False(t, safeTagRef(bad), bad)
	}
}
