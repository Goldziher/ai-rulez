package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

const cacheFileName = ".ai_rulez_cache"

type cacheEntry struct {
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

func defaultCacheConfig() *CacheConfig {
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
	config       *CacheConfig
	memoryCache  *ristretto.Cache[string, *cacheEntry]
	diskMux      sync.RWMutex
	diskCacheDir string
	diskTTL      time.Duration
}

func newCache(config *CacheConfig) *Cache {
	if config == nil {
		config = defaultCacheConfig()
	}

	estimatedCostPerEntry := int64(10 * 1024)
	ristrettoCache, err := ristretto.NewCache(&ristretto.Config[string, *cacheEntry]{
		NumCounters: int64(config.MaxMemoryEntries * 10),
		MaxCost:     int64(config.MaxMemoryEntries) * estimatedCostPerEntry,
		BufferItems: 64,
		Metrics:     true,
	})
	if err != nil {
		//nolint:errcheck // Fallback cache creation, errors ignored
		ristrettoCache, _ = ristretto.NewCache(&ristretto.Config[string, *cacheEntry]{
			NumCounters: 1000,
			MaxCost:     100 * estimatedCostPerEntry,
			BufferItems: 64,
		})
	}

	cache := &Cache{
		config:       config,
		memoryCache:  ristrettoCache,
		diskCacheDir: config.DiskCacheDir,
		diskTTL:      config.DiskTTL,
	}

	if config.DiskCacheDir != "" {
		//nolint:errcheck // Directory creation errors are non-critical for cache
		_ = os.MkdirAll(config.DiskCacheDir, 0o755)
	}

	return cache
}

func (c *Cache) get(ctx context.Context, url string) (*cacheEntry, bool) {
	key := c.generateKey(url)

	if entry, found := c.memoryCache.Get(key); found {
		return entry, true
	}

	if entry := c.getFromDisk(ctx, key); entry != nil {
		if c.isDiskEntryValid(entry) {
			cost := int64(len(entry.Content) + 256)
			c.memoryCache.SetWithTTL(key, entry, cost, c.config.MemoryTTL)
			c.memoryCache.Wait()
			return entry, true
		}
		//nolint:errcheck // Cleanup errors are non-critical
		_ = c.removeFromDisk(ctx, key)
	}

	return nil, false
}

func (c *Cache) set(ctx context.Context, url string, content []byte, etag, lastModified string) error {
	key := c.generateKey(url)

	entry := &cacheEntry{
		Content:      content,
		URL:          url,
		FetchedAt:    time.Now(),
		ETag:         etag,
		LastModified: lastModified,
	}

	cost := int64(len(entry.Content) + 256)
	success := c.memoryCache.SetWithTTL(key, entry, cost, c.config.MemoryTTL)
	if success {
		c.memoryCache.Wait()
	}

	return c.setOnDisk(ctx, key, entry)
}

func (c *Cache) generateKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

func readFileBuffered(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	bufferSize := 8192
	if stat.Size() > 64*1024 {
		bufferSize = 32768
	}

	reader := bufio.NewReaderSize(file, bufferSize)
	return io.ReadAll(reader)
}

func writeBufferedCacheEntry(filePath string, metadataBytes, content []byte) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	totalSize := len(metadataBytes) + len(content) + 16
	bufferSize := 8192
	if totalSize > 64*1024 {
		bufferSize = 32768
	}

	writer := bufio.NewWriterSize(file, bufferSize)
	defer func() { _ = writer.Flush() }()

	if _, err := writer.Write(metadataBytes); err != nil {
		return err
	}
	if _, err := writer.WriteString("\n---CONTENT---\n"); err != nil {
		return err
	}
	if _, err := writer.Write(content); err != nil {
		return err
	}

	return writer.Flush()
}

func (c *Cache) getFromDisk(_ context.Context, key string) *cacheEntry {
	if c.diskCacheDir == "" {
		return nil
	}

	c.diskMux.RLock()
	defer c.diskMux.RUnlock()

	filePath := filepath.Join(c.diskCacheDir, key+"-"+cacheFileName)

	data, err := readFileBuffered(filePath)
	if err != nil {
		return nil
	}

	var metadata struct {
		URL          string    `json:"url"`
		FetchedAt    time.Time `json:"fetched_at"`
		ETag         string    `json:"etag"`
		LastModified string    `json:"last_modified"`
		ContentSize  int       `json:"content_size"`
	}

	boundary := []byte("\n---CONTENT---\n")
	if idx := bytes.Index(data, boundary); idx > 0 {
		metadataBytes := data[:idx]
		contentBytes := data[idx+len(boundary):]

		if err := json.Unmarshal(metadataBytes, &metadata); err == nil {
			return &cacheEntry{
				Content:      contentBytes,
				URL:          metadata.URL,
				FetchedAt:    metadata.FetchedAt,
				ETag:         metadata.ETag,
				LastModified: metadata.LastModified,
			}
		}
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}

	return &cacheEntry{
		Content:   data,
		URL:       "",
		FetchedAt: info.ModTime(),
	}
}

func (c *Cache) setOnDisk(_ context.Context, key string, entry *cacheEntry) error {
	if c.diskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	if err := os.MkdirAll(c.diskCacheDir, 0o755); err != nil {
		return nil
	}

	if err := c.evictOldDiskEntries(); err != nil {
		_ = err
	}

	filePath := filepath.Join(c.diskCacheDir, key+"-"+cacheFileName)

	metadata := struct {
		URL          string    `json:"url"`
		FetchedAt    time.Time `json:"fetched_at"`
		ETag         string    `json:"etag"`
		LastModified string    `json:"last_modified"`
		ContentSize  int       `json:"content_size"`
	}{
		URL:          entry.URL,
		FetchedAt:    entry.FetchedAt,
		ETag:         entry.ETag,
		LastModified: entry.LastModified,
		ContentSize:  len(entry.Content),
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return os.WriteFile(filePath, entry.Content, 0o644)
	}

	return writeBufferedCacheEntry(filePath, metadataBytes, entry.Content)
}

func (c *Cache) removeFromDisk(_ context.Context, key string) error {
	if c.diskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	filePath := filepath.Join(c.diskCacheDir, key+"-"+cacheFileName)
	return os.Remove(filePath)
}

func (c *Cache) isDiskEntryValid(entry *cacheEntry) bool {
	return time.Since(entry.FetchedAt) < c.diskTTL
}

func (c *Cache) evictOldDiskEntries() error {
	if c.diskCacheDir == "" {
		return nil
	}

	entries, err := os.ReadDir(c.diskCacheDir)
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
				path:    filepath.Join(c.diskCacheDir, entry.Name()),
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
		//nolint:errcheck // File cleanup errors are non-critical
		_ = os.Remove(cacheFiles[i].path)
	}

	return nil
}

func (c *Cache) clearMemory() {
	c.memoryCache.Clear()
}

func (c *Cache) clearDisk(_ context.Context) error {
	if c.diskCacheDir == "" {
		return nil
	}

	c.diskMux.Lock()
	defer c.diskMux.Unlock()

	entries, err := os.ReadDir(c.diskCacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), cacheFileName) {
			filePath := filepath.Join(c.diskCacheDir, entry.Name())
			//nolint:errcheck // File cleanup errors are non-critical
			_ = os.Remove(filePath)
		}
	}

	return nil
}

func (c *Cache) clear(ctx context.Context) error {
	c.clearMemory()
	return c.clearDisk(ctx)
}

func (c *Cache) stats() cacheStats {
	metrics := c.memoryCache.Metrics

	diskEntries := 0
	if c.diskCacheDir != "" {
		if entries, err := os.ReadDir(c.diskCacheDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.Contains(entry.Name(), cacheFileName) {
					diskEntries++
				}
			}
		}
	}

	memoryEntries := 0
	if metrics != nil {
		keysAdded := metrics.KeysAdded()
		keysEvicted := metrics.KeysEvicted()
		if keysAdded >= keysEvicted {
			memoryEntries = int(keysAdded - keysEvicted) //nolint:gosec // Conversion is safe in this context
		}
	}

	return cacheStats{
		MemoryEntries:    memoryEntries,
		DiskEntries:      diskEntries,
		MaxMemoryEntries: c.config.MaxMemoryEntries,
		MaxDiskEntries:   c.config.MaxDiskEntries,
	}
}

type cacheStats struct {
	MemoryEntries    int
	DiskEntries      int
	MaxMemoryEntries int
	MaxDiskEntries   int
}
