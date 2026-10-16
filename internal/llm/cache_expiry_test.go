package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chatOnce(t *testing.T, c Client) ChatResponse {
	t.Helper()
	resp, err := c.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	require.NoError(t, err)
	return resp
}

func TestCacheExpiresOldEntries(t *testing.T) {
	// Arrange
	root := t.TempDir()
	opts := Options{ConfigDir: filepath.Join(root, "repo", ".ai-rulez"), CacheDir: filepath.Join(root, "cache"), SecretPath: filepath.Join(root, "cfg", "key"), CacheMaxAge: 24 * time.Hour}
	f := NewFake()
	m := Wrap(f, allowed(Config{Model: "gpt-4o-mini"}), opts)
	chatOnce(t, m)
	files := entryFiles(t, opts.CacheDir)
	require.Len(t, files, 1)

	// Act
	fresh := chatOnce(t, m)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(files[0], old, old))
	expired := chatOnce(t, m)

	// Assert
	assert.True(t, fresh.Cached)
	assert.False(t, expired.Cached, "an entry past CacheMaxAge is a miss")
}

func TestCacheReadOnlyServesHitsAndStoresNothing(t *testing.T) {
	// Arrange
	root := t.TempDir()
	opts := Options{ConfigDir: filepath.Join(root, "repo", ".ai-rulez"), CacheDir: filepath.Join(root, "cache"), SecretPath: filepath.Join(root, "cfg", "key")}
	writer := Wrap(NewFake(), allowed(Config{Model: "gpt-4o-mini"}), opts)
	chatOnce(t, writer)
	opts.CacheReadOnly = true
	reader := Wrap(NewFake(), allowed(Config{Model: "gpt-4o-mini"}), opts)

	// Act
	hit := chatOnce(t, reader)
	_, err := reader.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "another"}}})
	require.NoError(t, err)

	// Assert
	assert.True(t, hit.Cached)
	assert.Len(t, entryFiles(t, opts.CacheDir), 1, "a read-only cache stores no new entry")
}
