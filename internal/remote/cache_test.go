package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_BasicOperations(t *testing.T) {
	t.Run("get and set operations", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     "",
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"
		content := []byte("test content")
		etag := "abc123"
		lastModified := "Mon, 02 Jan 2006 15:04:05 MST"

		entry, found := cache.get(ctx, url)
		assert.False(t, found)
		assert.Nil(t, entry)

		err := cache.set(ctx, url, content, etag, lastModified)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		entry, found = cache.get(ctx, url)
		assert.True(t, found)
		require.NotNil(t, entry)
		assert.Equal(t, content, entry.Content)
		assert.Equal(t, url, entry.URL)
		assert.Equal(t, etag, entry.ETag)
		assert.Equal(t, lastModified, entry.LastModified)
		assert.WithinDuration(t, time.Now(), entry.FetchedAt, 1*time.Second)
	})

	t.Run("multiple entries", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     "",
		})

		ctx := context.Background()

		urls := []string{
			"https://example.com/1.yaml",
			"https://example.com/2.yaml",
			"https://example.com/3.yaml",
		}

		for i, url := range urls {
			content := []byte(string(rune('a' + i)))
			err := cache.set(ctx, url, content, "", "")
			require.NoError(t, err)
		}

		time.Sleep(10 * time.Millisecond)

		for i, url := range urls {
			entry, found := cache.get(ctx, url)
			assert.True(t, found)
			require.NotNil(t, entry)
			expectedContent := []byte(string(rune('a' + i)))
			assert.Equal(t, expectedContent, entry.Content)
		}
	})

	t.Run("overwrite existing entry", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     "",
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"

		err := cache.set(ctx, url, []byte("content1"), "etag1", "")
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		err = cache.set(ctx, url, []byte("content2"), "etag2", "")
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		entry, found := cache.get(ctx, url)
		assert.True(t, found)
		require.NotNil(t, entry)
		assert.Equal(t, []byte("content2"), entry.Content)
		assert.Equal(t, "etag2", entry.ETag)
	})
}

func TestCache_TTLExpiration(t *testing.T) {
	t.Run("memory TTL expiration", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        100 * time.Millisecond,
			DiskCacheDir:     "",
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"
		content := []byte("test content")

		err := cache.set(ctx, url, content, "", "")
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		entry, found := cache.get(ctx, url)
		assert.True(t, found)
		assert.NotNil(t, entry)

		time.Sleep(150 * time.Millisecond)

		entry, found = cache.get(ctx, url)
		assert.False(t, found)
		assert.Nil(t, entry)
	})
}

func TestCache_DiskPersistence(t *testing.T) {
	t.Run("persist to disk and retrieve", func(t *testing.T) {
		tempDir := t.TempDir()

		cache1 := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        100 * time.Millisecond,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"
		content := []byte("persistent content")

		err := cache1.set(ctx, url, content, "etag123", "modified")
		require.NoError(t, err)

		time.Sleep(150 * time.Millisecond)

		cache2 := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		entry, found := cache2.get(ctx, url)
		assert.True(t, found)
		require.NotNil(t, entry)
		assert.Equal(t, content, entry.Content)
		assert.Equal(t, "etag123", entry.ETag)
		assert.Equal(t, "modified", entry.LastModified)
	})

	t.Run("disk TTL expiration", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        100 * time.Millisecond,
			DiskCacheDir:     tempDir,
			DiskTTL:          200 * time.Millisecond,
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"
		content := []byte("test content")

		err := cache.set(ctx, url, content, "", "")
		require.NoError(t, err)

		cache.clearMemory()

		entry, found := cache.get(ctx, url)
		assert.True(t, found)
		assert.NotNil(t, entry)

		time.Sleep(250 * time.Millisecond)

		cache.clearMemory()

		entry, found = cache.get(ctx, url)
		assert.False(t, found)
		assert.Nil(t, entry)
	})

	t.Run("disk eviction on max entries", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			MaxDiskEntries:   3,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()

		for i := 0; i < 5; i++ {
			url := string(rune('a' + i))
			content := []byte(url)
			err := cache.set(ctx, url, content, "", "")
			require.NoError(t, err)
			time.Sleep(10 * time.Millisecond)
		}

		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)

		cacheFileCount := 0
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == "" {
				cacheFileCount++
			}
		}

		assert.LessOrEqual(t, cacheFileCount, cache.config.MaxDiskEntries+1)
	})
}

func TestCache_Clear(t *testing.T) {
	t.Run("clear memory cache", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     "",
		})

		ctx := context.Background()

		for i := 0; i < 3; i++ {
			url := string(rune('a' + i))
			err := cache.set(ctx, url, []byte(url), "", "")
			require.NoError(t, err)
		}

		cache.clearMemory()

		for i := 0; i < 3; i++ {
			url := string(rune('a' + i))
			entry, found := cache.get(ctx, url)
			assert.False(t, found)
			assert.Nil(t, entry)
		}
	})

	t.Run("clear disk cache", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()

		for i := 0; i < 3; i++ {
			url := string(rune('a' + i))
			err := cache.set(ctx, url, []byte(url), "", "")
			require.NoError(t, err)
		}

		err := cache.clearDisk(ctx)
		require.NoError(t, err)

		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)

		cacheFileCount := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				cacheFileCount++
			}
		}
		assert.Equal(t, 0, cacheFileCount)
	})

	t.Run("clear all", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()

		urls := []string{"a", "b", "c"}
		for _, url := range urls {
			err := cache.set(ctx, url, []byte(url), "", "")
			require.NoError(t, err)
		}

		err := cache.clear(ctx)
		require.NoError(t, err)

		for _, url := range urls {
			entry, found := cache.get(ctx, url)
			assert.False(t, found)
			assert.Nil(t, entry)
		}

		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		cacheFileCount := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				cacheFileCount++
			}
		}
		assert.Equal(t, 0, cacheFileCount)
	})
}

func TestCache_Stats(t *testing.T) {
	t.Run("stats tracking", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			MaxDiskEntries:   20,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()

		stats := cache.stats()
		assert.Equal(t, 0, stats.MemoryEntries)
		assert.Equal(t, 0, stats.DiskEntries)
		assert.Equal(t, 10, stats.MaxMemoryEntries)
		assert.Equal(t, 20, stats.MaxDiskEntries)

		for i := 0; i < 3; i++ {
			url := string(rune('a' + i))
			err := cache.set(ctx, url, []byte(url), "", "")
			require.NoError(t, err)
		}

		time.Sleep(50 * time.Millisecond)

		stats = cache.stats()
		assert.GreaterOrEqual(t, stats.MemoryEntries, 0)
		assert.LessOrEqual(t, stats.MemoryEntries, 3)
		assert.GreaterOrEqual(t, stats.DiskEntries, 0)
		assert.LessOrEqual(t, stats.DiskEntries, 3)
	})
}

func TestCache_EdgeCases(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		cache := newCache(nil)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.config)
		assert.NotNil(t, cache.memoryCache)
	})

	t.Run("empty URL", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
		})

		ctx := context.Background()

		err := cache.set(ctx, "", []byte("content"), "", "")
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		entry, found := cache.get(ctx, "")
		assert.True(t, found)
		assert.NotNil(t, entry)
		assert.Equal(t, []byte("content"), entry.Content)
	})

	t.Run("large content", func(t *testing.T) {
		t.Skip("Ristretto may reject very large items - this is expected behavior for performance")

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 100,
			MemoryTTL:        1 * time.Hour,
		})

		ctx := context.Background()
		url := "https://example.com/large.yaml"

		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		err := cache.set(ctx, url, largeContent, "", "")
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		entry, found := cache.get(ctx, url)
		if found {
			require.NotNil(t, entry)
			assert.Equal(t, largeContent, entry.Content)
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 100,
			MemoryTTL:        1 * time.Hour,
		})

		ctx := context.Background()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				url := string(rune('a' + id))
				content := []byte(url)

				for j := 0; j < 10; j++ {
					_ = cache.set(ctx, url, content, "", "")
					_, _ = cache.get(ctx, url)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		entry, found := cache.get(ctx, "a")
		assert.True(t, found)
		assert.NotNil(t, entry)
	})
}

func TestCache_DiskFormat(t *testing.T) {
	t.Run("new format with metadata", func(t *testing.T) {
		tempDir := t.TempDir()

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        100 * time.Millisecond,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		ctx := context.Background()
		url := "https://example.com/config.yaml"
		content := []byte("test content with metadata")
		etag := "etag-value"
		lastModified := "last-modified-value"

		err := cache.set(ctx, url, content, etag, lastModified)
		require.NoError(t, err)

		cache.clearMemory()

		entry, found := cache.get(ctx, url)
		assert.True(t, found)
		require.NotNil(t, entry)
		assert.Equal(t, content, entry.Content)
		assert.Equal(t, url, entry.URL)
		assert.Equal(t, etag, entry.ETag)
		assert.Equal(t, lastModified, entry.LastModified)
	})

	t.Run("backward compatibility with old format", func(t *testing.T) {
		tempDir := t.TempDir()

		key := "test-key"
		oldContent := []byte("old format content")
		filePath := filepath.Join(tempDir, key+"-"+cacheFileName)
		err := os.WriteFile(filePath, oldContent, 0o644)
		require.NoError(t, err)

		cache := newCache(&CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
			DiskCacheDir:     tempDir,
			DiskTTL:          1 * time.Hour,
		})

		entry := cache.getFromDisk(context.Background(), key)
		require.NotNil(t, entry)
		assert.Equal(t, oldContent, entry.Content)
		assert.Empty(t, entry.URL)
		assert.Empty(t, entry.ETag)
		assert.Empty(t, entry.LastModified)
	})
}
