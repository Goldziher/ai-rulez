package remote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	assert.Equal(t, 50, config.MaxMemoryEntries)
	assert.Equal(t, 5*time.Minute, config.MemoryTTL)
	assert.Equal(t, 500, config.MaxDiskEntries)
	assert.Equal(t, 24*time.Hour, config.DiskTTL)
	assert.True(t, config.RespectHTTPCache)
	assert.Equal(t, 1*time.Hour, config.DefaultTTL)

	// Check cache directory is set
	assert.Contains(t, config.DiskCacheDir, "ai-rulez")
	assert.Contains(t, config.DiskCacheDir, "remote")
}

func TestNewCache(t *testing.T) {
	t.Run("with_nil_config", func(t *testing.T) {
		cache := NewCache(nil)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.config)
		assert.NotNil(t, cache.memoryCache)
	})

	t.Run("with_custom_config", func(t *testing.T) {
		config := &CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Minute,
		}
		cache := NewCache(config)
		assert.Equal(t, config, cache.config)
	})
}

func TestCache_MemoryOperations(t *testing.T) {
	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 2,
		MemoryTTL:        1 * time.Hour,
	})
	ctx := context.Background()

	t.Run("set_and_get", func(t *testing.T) {
		url := "https://example.com/test"
		content := []byte("test content")

		err := cache.Set(ctx, url, content, "etag123", "last-modified")
		require.NoError(t, err)

		entry, found := cache.Get(ctx, url)
		require.True(t, found)
		assert.Equal(t, content, entry.Content)
		assert.Equal(t, url, entry.URL)
		assert.Equal(t, "etag123", entry.ETag)
		assert.Equal(t, "last-modified", entry.LastModified)
	})

	t.Run("miss", func(t *testing.T) {
		entry, found := cache.Get(ctx, "https://notfound.com")
		assert.False(t, found)
		assert.Nil(t, entry)
	})

	t.Run("memory_eviction", func(t *testing.T) {
		// Fill cache to capacity
		cache.Set(ctx, "https://example.com/1", []byte("content1"), "", "")
		cache.Set(ctx, "https://example.com/2", []byte("content2"), "", "")

		// This should evict the oldest entry
		cache.Set(ctx, "https://example.com/3", []byte("content3"), "", "")

		// First entry should be evicted
		_, found1 := cache.Get(ctx, "https://example.com/1")
		assert.False(t, found1)

		// Others should still be there
		_, found2 := cache.Get(ctx, "https://example.com/2")
		_, found3 := cache.Get(ctx, "https://example.com/3")
		assert.True(t, found2)
		assert.True(t, found3)
	})
}

func TestCache_TTLExpiration(t *testing.T) {
	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 10,
		MemoryTTL:        50 * time.Millisecond, // Very short TTL
		DiskTTL:          100 * time.Millisecond,
	})
	ctx := context.Background()

	url := "https://example.com/expire"
	content := []byte("expiring content")

	// Set content
	err := cache.Set(ctx, url, content, "", "")
	require.NoError(t, err)

	// Should be available immediately
	entry, found := cache.Get(ctx, url)
	require.True(t, found)
	assert.Equal(t, content, entry.Content)

	// Wait for memory TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should not be in memory anymore, but might be on disk
	// Force check by clearing memory first to test disk behavior
	cache.ClearMemory()

	// After clearing memory and TTL expiration, should not be found
	entry, found = cache.Get(ctx, url)
	assert.False(t, found)
	assert.Nil(t, entry)
}

func TestCache_DiskOperations(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 1, // Small memory to force disk usage
		MemoryTTL:        1 * time.Hour,
		DiskCacheDir:     tmpDir,
		MaxDiskEntries:   3,
		DiskTTL:          1 * time.Hour,
	})
	ctx := context.Background()

	t.Run("disk_persistence", func(t *testing.T) {
		url := "https://example.com/disk"
		content := []byte("disk content")

		// Set content
		err := cache.Set(ctx, url, content, "etag", "last-mod")
		require.NoError(t, err)

		// Clear memory to force disk read
		cache.ClearMemory()

		// Should still be available from disk
		entry, found := cache.Get(ctx, url)
		require.True(t, found)
		assert.Equal(t, content, entry.Content)
		// Note: URL, ETag and LastModified are not persisted in simple disk cache
	})

	t.Run("disk_eviction", func(t *testing.T) {
		// Fill disk cache beyond capacity
		for i := 0; i < 5; i++ {
			url := fmt.Sprintf("https://example.com/disk%d", i)
			content := []byte(fmt.Sprintf("content%d", i))
			err := cache.Set(ctx, url, content, "", "")
			require.NoError(t, err)

			// Small delay to ensure different modification times
			time.Sleep(10 * time.Millisecond)
		}

		// Check that we don't exceed max disk entries
		stats := cache.Stats()
		assert.LessOrEqual(t, stats.DiskEntries, 3)
	})

	t.Run("cache_directory_creation", func(t *testing.T) {
		nonExistentDir := filepath.Join(tmpDir, "nested", "cache")
		_ = NewCache(&CacheConfig{
			DiskCacheDir: nonExistentDir,
		})

		// Directory should be created
		_, err := os.Stat(nonExistentDir)
		assert.NoError(t, err)
	})
}

func TestCache_ClearOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache-clear-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cache := NewCache(&CacheConfig{
		DiskCacheDir: tmpDir,
	})
	ctx := context.Background()

	// Add some content
	cache.Set(ctx, "https://example.com/1", []byte("content1"), "", "")
	cache.Set(ctx, "https://example.com/2", []byte("content2"), "", "")

	t.Run("clear_memory", func(t *testing.T) {
		cache.ClearMemory()

		// Memory should be empty, but disk should still have content
		stats := cache.Stats()
		assert.Equal(t, 0, stats.MemoryEntries)
		assert.Greater(t, stats.DiskEntries, 0)
	})

	t.Run("clear_disk", func(t *testing.T) {
		err := cache.ClearDisk(ctx)
		require.NoError(t, err)

		stats := cache.Stats()
		assert.Equal(t, 0, stats.DiskEntries)
	})

	t.Run("clear_all", func(t *testing.T) {
		// Add content again
		cache.Set(ctx, "https://example.com/3", []byte("content3"), "", "")

		err := cache.Clear(ctx)
		require.NoError(t, err)

		stats := cache.Stats()
		assert.Equal(t, 0, stats.MemoryEntries)
		assert.Equal(t, 0, stats.DiskEntries)
	})
}

func TestCache_Stats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache-stats-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 5,
		MaxDiskEntries:   10,
		DiskCacheDir:     tmpDir,
	})
	ctx := context.Background()

	// Initially empty
	stats := cache.Stats()
	assert.Equal(t, 0, stats.MemoryEntries)
	assert.Equal(t, 0, stats.DiskEntries)
	assert.Equal(t, 5, stats.MaxMemoryEntries)
	assert.Equal(t, 10, stats.MaxDiskEntries)

	// Add some content
	cache.Set(ctx, "https://example.com/1", []byte("content1"), "", "")
	cache.Set(ctx, "https://example.com/2", []byte("content2"), "", "")

	stats = cache.Stats()
	assert.Equal(t, 2, stats.MemoryEntries)
	assert.Equal(t, 2, stats.DiskEntries)
}

func TestCache_GenerateKey(t *testing.T) {
	cache := NewCache(nil)

	key1 := cache.generateKey("https://example.com/test")
	key2 := cache.generateKey("https://example.com/test")
	key3 := cache.generateKey("https://example.com/different")

	// Same URL should generate same key
	assert.Equal(t, key1, key2)

	// Different URLs should generate different keys
	assert.NotEqual(t, key1, key3)

	// Keys should be hex encoded SHA256 hashes (64 characters)
	assert.Len(t, key1, 64)
	assert.Regexp(t, "^[0-9a-f]+$", key1)
}

func TestCache_ExtendedConcurrentAccess(t *testing.T) {
	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 100,
		MemoryTTL:        1 * time.Hour,
	})
	ctx := context.Background()

	// Test concurrent read/write operations
	done := make(chan bool, 10)

	// Start multiple goroutines doing cache operations
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 100; j++ {
				url := fmt.Sprintf("https://example.com/concurrent/%d/%d", id, j)
				content := []byte(fmt.Sprintf("content-%d-%d", id, j))

				// Set
				err := cache.Set(ctx, url, content, "", "")
				assert.NoError(t, err)

				// Get
				entry, found := cache.Get(ctx, url)
				if found {
					assert.Equal(t, content, entry.Content)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCache_ExtendedErrorHandling(t *testing.T) {
	t.Run("disabled_disk_cache", func(t *testing.T) {
		cache := NewCache(&CacheConfig{
			DiskCacheDir:     "", // Disabled
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
		})
		ctx := context.Background()

		// Should work fine with memory-only cache
		err := cache.Set(ctx, "https://example.com/test", []byte("content"), "", "")
		assert.NoError(t, err)

		entry, found := cache.Get(ctx, "https://example.com/test")
		require.True(t, found)
		require.NotNil(t, entry)
		assert.Equal(t, []byte("content"), entry.Content)

		// Clear disk should not fail even when disabled
		err = cache.ClearDisk(ctx)
		assert.NoError(t, err)

		// Memory should still have the entry
		entry, found = cache.Get(ctx, "https://example.com/test")
		require.True(t, found)
		assert.Equal(t, []byte("content"), entry.Content)
	})

	t.Run("invalid_disk_path", func(t *testing.T) {
		// Skip this test on systems where /root might be accessible
		if os.Getuid() == 0 {
			t.Skip("Skipping test when running as root")
		}

		// Use a path that we can't write to
		cache := NewCache(&CacheConfig{
			DiskCacheDir:     "/root/no-permission",
			MaxMemoryEntries: 10,
			MemoryTTL:        1 * time.Hour,
		})
		ctx := context.Background()

		// Set should not fail even if disk write fails - memory cache should still work
		err := cache.Set(ctx, "https://example.com/test", []byte("content"), "", "")
		// Note: The current implementation doesn't return disk write errors
		// but memory caching should still work
		assert.NoError(t, err)

		// Should be available from memory cache
		entry, found := cache.Get(ctx, "https://example.com/test")
		require.True(t, found)
		assert.Equal(t, []byte("content"), entry.Content)
	})
}

// Benchmark tests to ensure cache performance is acceptable
func BenchmarkCache_MemoryOperations(b *testing.B) {
	cache := NewCache(&CacheConfig{
		MaxMemoryEntries: 1000,
		MemoryTTL:        1 * time.Hour,
	})
	ctx := context.Background()

	content := []byte("benchmark content for testing cache performance")

	b.ResetTimer()
	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			url := fmt.Sprintf("https://example.com/bench/%d", i%100)
			cache.Set(ctx, url, content, "", "")
		}
	})

	b.Run("Get", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 100; i++ {
			url := fmt.Sprintf("https://example.com/bench/%d", i)
			cache.Set(ctx, url, content, "", "")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			url := fmt.Sprintf("https://example.com/bench/%d", i%100)
			cache.Get(ctx, url)
		}
	})
}

// TestCache_ComprehensiveOperations provides extensive cache testing
func TestCache_ComprehensiveOperations(t *testing.T) {
	t.Run("cache_expiration_scenarios", func(t *testing.T) {
		config := &CacheConfig{
			MaxMemoryEntries: 10,
			MemoryTTL:        50 * time.Millisecond,
			DiskCacheDir:     "",
			MaxDiskEntries:   0,
			DiskTTL:          time.Hour,
			RespectHTTPCache: false,
			DefaultTTL:       time.Hour,
		}
		cache := NewCache(config)
		ctx := context.Background()

		url := "https://example.com/config.yaml"
		content := []byte("expiring content")

		// Store in cache
		err := cache.Set(ctx, url, content, "", "")
		require.NoError(t, err)

		// Should be available immediately
		entry, found := cache.Get(ctx, url)
		assert.True(t, found)
		assert.NotNil(t, entry)

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		// Should be expired
		entry, found = cache.Get(ctx, url)
		assert.False(t, found)
		assert.Nil(t, entry)
	})

	t.Run("multiple_url_isolation", func(t *testing.T) {
		cache := NewCache(nil)
		ctx := context.Background()

		testData := map[string][]byte{
			"https://example.com/config1.yaml": []byte("config 1 content"),
			"https://example.com/config2.yaml": []byte("config 2 content"),
			"https://other.com/config.yaml":    []byte("other config content"),
		}

		// Store all entries
		for url, content := range testData {
			err := cache.Set(ctx, url, content, "", "")
			require.NoError(t, err)
		}

		// Retrieve and verify isolation
		for expectedURL, expectedContent := range testData {
			entry, found := cache.Get(ctx, expectedURL)
			assert.True(t, found, "URL %s should be found", expectedURL)
			require.NotNil(t, entry)
			assert.Equal(t, expectedContent, entry.Content)
			assert.Equal(t, expectedURL, entry.URL)
		}
	})

	t.Run("error_handling_scenarios", func(t *testing.T) {
		t.Run("nil_config_default", func(t *testing.T) {
			cache := NewCache(nil)
			assert.NotNil(t, cache)
			assert.NotNil(t, cache.config)

			ctx := context.Background()
			url := "https://example.com/nil-config.yaml"
			content := []byte("nil config content")

			err := cache.Set(ctx, url, content, "", "")
			assert.NoError(t, err)

			entry, found := cache.Get(ctx, url)
			assert.True(t, found)
			assert.NotNil(t, entry)
		})

		t.Run("empty_content_handling", func(t *testing.T) {
			cache := NewCache(nil)
			ctx := context.Background()

			url := "https://example.com/empty.yaml"
			emptyContent := []byte("")

			err := cache.Set(ctx, url, emptyContent, "", "")
			require.NoError(t, err)

			entry, found := cache.Get(ctx, url)
			assert.True(t, found)
			require.NotNil(t, entry)
			assert.Equal(t, emptyContent, entry.Content)
			assert.Len(t, entry.Content, 0)
		})
	})
}
