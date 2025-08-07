package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const cacheFileName = ".ai_rulez_cache"

// CacheEntry represents a cached remote resource
type CacheEntry struct {
	Content      []byte
	URL          string
	FetchedAt    time.Time
	ETag         string
	LastModified string
}

// CacheConfig configures the caching behavior
type CacheConfig struct {
	// L1 Cache (Memory)
	MaxMemoryEntries int           // Maximum entries in memory cache
	MemoryTTL        time.Duration // Time-to-live for memory cache entries

	// L2 Cache (Disk)
	DiskCacheDir   string        // Directory for disk cache
	MaxDiskEntries int           // Maximum entries in disk cache
	DiskTTL        time.Duration // Time-to-live for disk cache entries

	// L3 Cache (HTTP)
	RespectHTTPCache bool          // Whether to respect HTTP cache headers (ETag, Last-Modified)
	DefaultTTL       time.Duration // Default TTL when no HTTP cache headers present
}

// DefaultCacheConfig returns sensible default cache configuration
func DefaultCacheConfig() *CacheConfig {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir not accessible
		homeDir = "."
	}
	cacheDir := filepath.Join(homeDir, ".cache", "ai-rulez", "remote")

	return &CacheConfig{
		// L1: 50 entries, 5 minutes
		MaxMemoryEntries: 50,
		MemoryTTL:        5 * time.Minute,

		// L2: 500 entries, 24 hours
		DiskCacheDir:   cacheDir,
		MaxDiskEntries: 500,
		DiskTTL:        24 * time.Hour,

		// L3: Respect HTTP headers with 1 hour default
		RespectHTTPCache: true,
		DefaultTTL:       1 * time.Hour,
	}
}

// Cache implements a multi-level cache for remote resources
type Cache struct {
	config *CacheConfig

	// L1: Memory cache (fastest)
	memoryMux   sync.RWMutex
	memoryCache map[string]*CacheEntry

	// L2: Disk cache (persistent)
	diskMux sync.RWMutex
}

// NewCache creates a new multi-level cache
func NewCache(config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &Cache{
		config:      config,
		memoryCache: make(map[string]*CacheEntry),
	}

	// Ensure disk cache directory exists
	if config.DiskCacheDir != "" {
		if err := os.MkdirAll(config.DiskCacheDir, 0o755); err != nil { //nolint:staticcheck
			// Ignore error - directory creation will be retried during cache operations
		}
	}

	return cache
}

// Get retrieves content from cache, checking L1 (memory) first, then L2 (disk)
func (c *Cache) Get(ctx context.Context, url string) (*CacheEntry, bool) {
	key := c.generateKey(url)

	// L1: Check memory cache first
	if entry := c.getFromMemory(key); entry != nil {
		if c.isMemoryEntryValid(entry) {
			return entry, true
		}
		// Remove expired entry from memory
		c.removeFromMemory(key)
	}

	// L2: Check disk cache
	if entry := c.getFromDisk(ctx, key); entry != nil {
		if c.isDiskEntryValid(entry) {
			// Promote to memory cache
			c.setInMemory(key, entry)
			return entry, true
		}
		// Remove expired entry from disk
		if err := c.removeFromDisk(ctx, key); err != nil { //nolint:staticcheck
			// Ignore error - it's just cleanup, disk removal is not critical
		}
	}

	return nil, false
}

// Set stores content in both L1 (memory) and L2 (disk) caches
func (c *Cache) Set(ctx context.Context, url string, content []byte, etag, lastModified string) error {
	key := c.generateKey(url)

	entry := &CacheEntry{
		Content:      content,
		URL:          url,
		FetchedAt:    time.Now(),
		ETag:         etag,
		LastModified: lastModified,
	}

	// Store in both L1 and L2
	c.setInMemory(key, entry)
	return c.setOnDisk(ctx, key, entry)
}

// generateKey creates a cache key from URL using SHA256 hash
func (c *Cache) generateKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// L1 Memory Cache Implementation

func (c *Cache) getFromMemory(key string) *CacheEntry {
	c.memoryMux.RLock()
	defer c.memoryMux.RUnlock()

	entry, exists := c.memoryCache[key]
	if !exists {
		return nil
	}

	// Return a copy to prevent race conditions
	entryCopy := *entry
	contentCopy := make([]byte, len(entry.Content))
	copy(contentCopy, entry.Content)
	entryCopy.Content = contentCopy

	return &entryCopy
}

func (c *Cache) setInMemory(key string, entry *CacheEntry) {
	c.memoryMux.Lock()
	defer c.memoryMux.Unlock()

	// Check if we need to evict old entries
	if len(c.memoryCache) >= c.config.MaxMemoryEntries {
		c.evictOldestMemoryEntry()
	}

	// Store a copy to prevent external modifications
	entryCopy := *entry
	contentCopy := make([]byte, len(entry.Content))
	copy(contentCopy, entry.Content)
	entryCopy.Content = contentCopy

	c.memoryCache[key] = &entryCopy
}

func (c *Cache) removeFromMemory(key string) {
	c.memoryMux.Lock()
	defer c.memoryMux.Unlock()
	delete(c.memoryCache, key)
}

func (c *Cache) isMemoryEntryValid(entry *CacheEntry) bool {
	return time.Since(entry.FetchedAt) < c.config.MemoryTTL
}

func (c *Cache) evictOldestMemoryEntry() {
	// Find the oldest entry
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.memoryCache {
		if oldestKey == "" || entry.FetchedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.FetchedAt
		}
	}

	if oldestKey != "" {
		delete(c.memoryCache, oldestKey)
	}
}

// L2 Disk Cache Implementation

func (c *Cache) getFromDisk(ctx context.Context, key string) *CacheEntry {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	c.diskMux.RLock()
	defer c.diskMux.RUnlock()

	filePath := filepath.Join(c.config.DiskCacheDir, key+"-"+cacheFileName)

	// Check if file exists and get its modification time
	info, err := os.Stat(filePath)
	if err != nil {
		return nil // File doesn't exist or can't be accessed
	}

	// Read the cached content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	// Create cache entry with file modification time as fetch time
	// Note: We don't have the original URL in the simple disk cache
	entry := &CacheEntry{
		Content:   content,
		URL:       "", // URL not stored in simple disk implementation
		FetchedAt: info.ModTime(),
	}

	return entry
}

func (c *Cache) setOnDisk(ctx context.Context, key string, entry *CacheEntry) error {
	if c.config.DiskCacheDir == "" {
		return nil // Disk caching disabled
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	// Ensure directory exists - if this fails, disk caching won't work
	// but we shouldn't fail the entire cache operation
	if err := os.MkdirAll(c.config.DiskCacheDir, 0o755); err != nil {
		// Directory creation failed - disk caching won't work but don't fail
		return nil
	}

	// Check if we need to evict old entries
	if err := c.evictOldDiskEntries(); err != nil {
		// Log error but don't fail the cache operation
		// In a real implementation, you might want to use a logger here
		_ = err
	}

	filePath := filepath.Join(c.config.DiskCacheDir, key+"-"+cacheFileName)

	// Write content to disk - if this fails, disk caching won't work but don't fail
	if err := os.WriteFile(filePath, entry.Content, 0o644); err != nil {
		// Write failed - disk caching won't work but don't fail the cache operation
		return nil
	}

	return nil
}

func (c *Cache) removeFromDisk(ctx context.Context, key string) error {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	filePath := filepath.Join(c.config.DiskCacheDir, key+"-"+cacheFileName)
	return os.Remove(filePath)
}

func (c *Cache) isDiskEntryValid(entry *CacheEntry) bool {
	return time.Since(entry.FetchedAt) < c.config.DiskTTL
}

func (c *Cache) evictOldDiskEntries() error {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	entries, err := os.ReadDir(c.config.DiskCacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	// If we're under the limit, no need to evict
	if len(entries) < c.config.MaxDiskEntries {
		return nil
	}

	// Collect file info for all cache files
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var cacheFiles []fileInfo
	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), cacheFileName) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			cacheFiles = append(cacheFiles, fileInfo{
				path:    filepath.Join(c.config.DiskCacheDir, entry.Name()),
				modTime: info.ModTime(),
			})
		}
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(cacheFiles)-1; i++ {
		for j := i + 1; j < len(cacheFiles); j++ {
			if cacheFiles[i].modTime.After(cacheFiles[j].modTime) {
				cacheFiles[i], cacheFiles[j] = cacheFiles[j], cacheFiles[i]
			}
		}
	}

	// Remove oldest files until we're under the limit
	entriesToRemove := len(cacheFiles) - c.config.MaxDiskEntries + 1
	for i := 0; i < entriesToRemove && i < len(cacheFiles); i++ {
		if err := os.Remove(cacheFiles[i].path); err != nil {
			continue
		}
	}

	return nil
}

// ClearMemory clears the L1 memory cache
func (c *Cache) ClearMemory() {
	c.memoryMux.Lock()
	defer c.memoryMux.Unlock()
	c.memoryCache = make(map[string]*CacheEntry)
}

// ClearDisk clears the L2 disk cache
func (c *Cache) ClearDisk(ctx context.Context) error {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	entries, err := os.ReadDir(c.config.DiskCacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), cacheFileName) {
			filePath := filepath.Join(c.config.DiskCacheDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				continue
			}
		}
	}

	return nil
}

// Clear clears both L1 and L2 caches
func (c *Cache) Clear(ctx context.Context) error {
	c.ClearMemory()
	return c.ClearDisk(ctx)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.memoryMux.RLock()
	memoryEntries := len(c.memoryCache)
	c.memoryMux.RUnlock()

	diskEntries := 0
	if c.config.DiskCacheDir != "" {
		if entries, err := os.ReadDir(c.config.DiskCacheDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.Contains(entry.Name(), cacheFileName) {
					diskEntries++
				}
			}
		}
	}

	return CacheStats{
		MemoryEntries:    memoryEntries,
		DiskEntries:      diskEntries,
		MaxMemoryEntries: c.config.MaxMemoryEntries,
		MaxDiskEntries:   c.config.MaxDiskEntries,
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	MemoryEntries    int
	DiskEntries      int
	MaxMemoryEntries int
	MaxDiskEntries   int
}
