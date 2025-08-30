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

type CacheEntry struct {
	Content      []byte
	URL          string
	FetchedAt    time.Time
	ETag         string
	LastModified string
}

type CacheConfig struct {
	MaxMemoryEntries int
	MemoryTTL        time.Duration

	DiskCacheDir   string
	MaxDiskEntries int
	DiskTTL        time.Duration

	RespectHTTPCache bool
	DefaultTTL       time.Duration
}

func DefaultCacheConfig() *CacheConfig {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	cacheDir := filepath.Join(homeDir, ".cache", "ai-rulez", "remote")

	return &CacheConfig{
		MaxMemoryEntries: 50,
		MemoryTTL:        5 * time.Minute,

		DiskCacheDir:   cacheDir,
		MaxDiskEntries: 500,
		DiskTTL:        24 * time.Hour,

		RespectHTTPCache: true,
		DefaultTTL:       1 * time.Hour,
	}
}

type Cache struct {
	config      *CacheConfig
	memoryMux   sync.RWMutex
	memoryCache map[string]*CacheEntry
	diskMux     sync.RWMutex
}

func NewCache(config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &Cache{
		config:      config,
		memoryCache: make(map[string]*CacheEntry),
	}

	if config.DiskCacheDir != "" {
		_ = os.MkdirAll(config.DiskCacheDir, 0o755)
	}

	return cache
}

func (c *Cache) Get(ctx context.Context, url string) (*CacheEntry, bool) {
	key := c.generateKey(url)

	if entry := c.getFromMemory(key); entry != nil {
		if c.isMemoryEntryValid(entry) {
			return entry, true
		}
		c.removeFromMemory(key)
	}

	if entry := c.getFromDisk(ctx, key); entry != nil {
		if c.isDiskEntryValid(entry) {
			c.setInMemory(key, entry)
			return entry, true
		}
		_ = c.removeFromDisk(ctx, key)
	}

	return nil, false
}

func (c *Cache) Set(ctx context.Context, url string, content []byte, etag, lastModified string) error {
	key := c.generateKey(url)

	entry := &CacheEntry{
		Content:      content,
		URL:          url,
		FetchedAt:    time.Now(),
		ETag:         etag,
		LastModified: lastModified,
	}

	c.setInMemory(key, entry)
	return c.setOnDisk(ctx, key, entry)
}

func (c *Cache) generateKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

func (c *Cache) getFromMemory(key string) *CacheEntry {
	c.memoryMux.RLock()
	defer c.memoryMux.RUnlock()

	entry, exists := c.memoryCache[key]
	if !exists {
		return nil
	}

	entryCopy := *entry
	contentCopy := make([]byte, len(entry.Content))
	copy(contentCopy, entry.Content)
	entryCopy.Content = contentCopy

	return &entryCopy
}

func (c *Cache) setInMemory(key string, entry *CacheEntry) {
	c.memoryMux.Lock()
	defer c.memoryMux.Unlock()

	if len(c.memoryCache) >= c.config.MaxMemoryEntries {
		c.evictOldestMemoryEntry()
	}

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

func (c *Cache) getFromDisk(_ context.Context, key string) *CacheEntry {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	c.diskMux.RLock()
	defer c.diskMux.RUnlock()

	filePath := filepath.Join(c.config.DiskCacheDir, key+"-"+cacheFileName)

	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	entry := &CacheEntry{
		Content:   content,
		URL:       "",
		FetchedAt: info.ModTime(),
	}

	return entry
}

func (c *Cache) setOnDisk(_ context.Context, key string, entry *CacheEntry) error {
	if c.config.DiskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	if err := os.MkdirAll(c.config.DiskCacheDir, 0o755); err != nil {
		return nil
	}

	if err := c.evictOldDiskEntries(); err != nil {
		_ = err
	}

	filePath := filepath.Join(c.config.DiskCacheDir, key+"-"+cacheFileName)

	if err := os.WriteFile(filePath, entry.Content, 0o644); err != nil {
		return nil
	}

	return nil
}

func (c *Cache) removeFromDisk(_ context.Context, key string) error {
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

	if len(entries) < c.config.MaxDiskEntries {
		return nil
	}

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

	for i := 0; i < len(cacheFiles)-1; i++ {
		for j := i + 1; j < len(cacheFiles); j++ {
			if cacheFiles[i].modTime.After(cacheFiles[j].modTime) {
				cacheFiles[i], cacheFiles[j] = cacheFiles[j], cacheFiles[i]
			}
		}
	}

	entriesToRemove := len(cacheFiles) - c.config.MaxDiskEntries + 1
	for i := 0; i < entriesToRemove && i < len(cacheFiles); i++ {
		if err := os.Remove(cacheFiles[i].path); err != nil {
			continue
		}
	}

	return nil
}

func (c *Cache) ClearMemory() {
	c.memoryMux.Lock()
	defer c.memoryMux.Unlock()
	c.memoryCache = make(map[string]*CacheEntry)
}

func (c *Cache) ClearDisk(_ context.Context) error {
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

func (c *Cache) Clear(ctx context.Context) error {
	c.ClearMemory()
	return c.ClearDisk(ctx)
}

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

type CacheStats struct {
	MemoryEntries    int
	DiskEntries      int
	MaxMemoryEntries int
	MaxDiskEntries   int
}
