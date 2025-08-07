# Remote References Implementation Plan for ai-rulez

## Overview

This document outlines the implementation plan for adding remote references support to ai-rulez. Remote references allow including ai-rulez files from HTTP/HTTPS URLs, enabling sharing and reusing configurations across projects and organizations.

Based on analysis of Go OpenAPI libraries (pb33f/libopenapi and getkin/kin-openapi), this implementation will provide robust, secure, and performant remote reference handling.

## Architecture Overview

### Core Components

1. **Remote URL Validation** - SSRF protection and URL sanitization
2. **HTTP Client Layer** - Configurable HTTP client with timeouts and headers
3. **Multi-Level Caching** - Memory, disk, and HTTP cache headers support
4. **Extended Config Loader** - Integration with existing include system
5. **Error Handling** - Comprehensive error types and context
6. **Security Layer** - IP blocking, DNS rebinding protection
7. **Circular Reference Detection** - Enhanced for cross-network references

### URL Schema Design

Remote includes will use standard HTTP/HTTPS URLs:
```yaml
includes:
  - "https://example.com/shared/rules.yaml"
  - "https://raw.githubusercontent.com/org/repo/main/ai_rules.yaml"
  - "./local/rules.yaml"  # Still supported
```

### Security Architecture

- **SSRF Protection**: IP allowlist/blocklist with private network blocking
- **DNS Rebinding Protection**: Pre-resolve IPs and validate against blocklist
- **HTTPS Enforcement**: Option to require HTTPS for remote refs
- **Checksum Verification**: Optional SHA256 validation
- **Timeout Controls**: Request, connection, and total operation timeouts

### Caching Strategy

**L1 Cache (Memory)**:
- In-memory LRU cache using sync.Map or similar
- Cache resolved content during single generation run
- Prevents duplicate fetches within same operation

**L2 Cache (Disk)**:
- XDG cache directory: `~/.cache/ai-rulez/remote/`
- Content-addressed storage using URL hash
- TTL-based expiration with configurable duration

**L3 Cache (HTTP Headers)**:
- Respect ETag and Last-Modified headers
- Conditional requests (If-None-Match, If-Modified-Since)
- Cache-Control directive handling

## Detailed Implementation Plan

### Phase 1: Core Infrastructure (Commits 1-3)

#### Commit 1: Remote URL Validation and SSRF Protection
**File**: `internal/remote/validator.go`

```go
package remote

import (
    "fmt"
    "net"
    "net/url"
    "strings"
)

// URLValidator provides SSRF protection for remote URLs
type URLValidator struct {
    blockedNetworks []net.IPNet
    allowedSchemes  []string
}

func NewURLValidator() *URLValidator {
    return &URLValidator{
        blockedNetworks: getDefaultBlockedNetworks(),
        allowedSchemes:  []string{"https", "http"},
    }
}

func (v *URLValidator) Validate(urlStr string) error {
    // Parse URL
    // Validate scheme (http/https)
    // Resolve hostname to IP
    // Check against blocked networks (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8)
    // Validate port range
    // Check for DNS rebinding attempts
}

func getDefaultBlockedNetworks() []net.IPNet {
    // Return private networks, localhost, etc.
}
```

**Tests**: `internal/remote/validator_test.go`
- Test blocked private networks
- Test allowed public URLs  
- Test malicious DNS rebinding attempts
- Test invalid schemes and malformed URLs

#### Commit 2: HTTP Client with Timeouts and Configuration
**File**: `internal/remote/client.go`

```go
package remote

import (
    "context"
    "crypto/tls"
    "net/http"
    "time"
)

// HTTPConfig configures the HTTP client for remote fetching
type HTTPConfig struct {
    Timeout         time.Duration
    ConnectTimeout  time.Duration
    ResponseTimeout time.Duration
    MaxRedirects    int
    UserAgent       string
    TLSConfig       *tls.Config
    Headers         map[string]string
}

func DefaultHTTPConfig() *HTTPConfig {
    return &HTTPConfig{
        Timeout:         30 * time.Second,
        ConnectTimeout:  10 * time.Second,
        ResponseTimeout: 20 * time.Second,
        MaxRedirects:    5,
        UserAgent:       fmt.Sprintf("ai-rulez/%s", version.Version),
        Headers: map[string]string{
            "Accept": "text/yaml, application/yaml, text/plain",
        },
    }
}

// Client wraps http.Client with remote reference specific configuration
type Client struct {
    http      *http.Client
    validator *URLValidator
    config    *HTTPConfig
}

func NewClient(config *HTTPConfig) *Client {
    // Configure http.Client with timeouts
    // Add custom transport for connection limits
    // Configure TLS settings
    // Set up redirect policy
}

func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
    // Validate URL using validator
    // Create request with proper headers
    // Execute with context timeout
    // Handle redirects and status codes
    // Return response body
}
```

**Tests**: `internal/remote/client_test.go`
- Test successful HTTP/HTTPS requests
- Test timeout handling
- Test redirect limits
- Test custom headers
- Test SSL certificate validation

#### Commit 3: Multi-Level Caching System
**File**: `internal/remote/cache.go`

```go
package remote

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
)

// CacheConfig configures caching behavior
type CacheConfig struct {
    TTL         time.Duration
    MaxMemory   int64
    DiskEnabled bool
    DiskDir     string
}

func DefaultCacheConfig() *CacheConfig {
    return &CacheConfig{
        TTL:         1 * time.Hour,
        MaxMemory:   50 * 1024 * 1024, // 50MB
        DiskEnabled: true,
        DiskDir:     getDefaultCacheDir(),
    }
}

// Cache provides multi-level caching for remote content
type Cache struct {
    config     *CacheConfig
    memory     sync.Map // URL -> CacheEntry
    diskDir    string
    memorySize int64
    mu         sync.RWMutex
}

type CacheEntry struct {
    Content    []byte
    ETag       string
    LastMod    time.Time
    ExpiresAt  time.Time
    Size       int64
}

func NewCache(config *CacheConfig) *Cache {
    // Initialize cache directories
    // Set up memory limits
}

func (c *Cache) Get(ctx context.Context, url string) (*CacheEntry, bool) {
    // Check L1 (memory) cache first
    // Fall back to L2 (disk) cache
    // Validate TTL and expiration
}

func (c *Cache) Set(url string, entry *CacheEntry) error {
    // Store in L1 (memory) with size limits
    // Store in L2 (disk) if enabled
    // Handle LRU eviction
}

func (c *Cache) keyForURL(url string) string {
    hash := sha256.Sum256([]byte(url))
    return hex.EncodeToString(hash[:])
}
```

**Tests**: `internal/remote/cache_test.go`
- Test memory cache operations
- Test disk cache persistence
- Test TTL expiration
- Test size-based eviction
- Test concurrent access

### Phase 2: Config Loader Integration (Commits 4-5)

#### Commit 4: Remote Reference Fetcher
**File**: `internal/remote/fetcher.go`

```go
package remote

import (
    "context"
    "fmt"
    "net/http"
    "time"
)

// Fetcher orchestrates remote reference fetching with caching
type Fetcher struct {
    client *Client
    cache  *Cache
}

func NewFetcher(httpConfig *HTTPConfig, cacheConfig *CacheConfig) *Fetcher {
    return &Fetcher{
        client: NewClient(httpConfig),
        cache:  NewCache(cacheConfig),
    }
}

func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
    // Check cache first
    if entry, found := f.cache.Get(ctx, url); found {
        if !entry.IsExpired() {
            return entry.Content, nil
        }
        // Use conditional request with ETag/Last-Modified
        return f.fetchConditional(ctx, url, entry)
    }
    
    // Fetch fresh content
    return f.fetchFresh(ctx, url)
}

func (f *Fetcher) fetchConditional(ctx context.Context, url string, cached *CacheEntry) ([]byte, error) {
    // Add If-None-Match, If-Modified-Since headers
    // Handle 304 Not Modified responses
    // Update cache on successful fetch
}

func (f *Fetcher) fetchFresh(ctx context.Context, url string) ([]byte, error) {
    // Fetch without cache headers
    // Store result in cache with appropriate TTL
    // Handle HTTP cache headers from response
}
```

#### Commit 5: Extended Config Loader for Remote Includes
**File**: `internal/config/loader.go` (modifications)

```go
// Add to configLoader struct
type configLoader struct {
    visited     map[string]bool
    baseDir     string
    fetcher     *remote.Fetcher  // New field
}

// Update LoadConfigWithIncludes function
func LoadConfigWithIncludes(filename string) (*Config, error) {
    // Initialize remote fetcher
    httpConfig := remote.DefaultHTTPConfig()
    cacheConfig := remote.DefaultCacheConfig()
    fetcher := remote.NewFetcher(httpConfig, cacheConfig)
    
    loader := &configLoader{
        visited: make(map[string]bool),
        baseDir: filepath.Dir(absPath),
        fetcher: fetcher,  // Set fetcher
    }
    // ... rest remains the same
}

// Update resolveIncludes method
func (l *configLoader) resolveIncludes(config *Config, baseDir string) error {
    for _, includePath := range config.Includes {
        var resolvedPath string
        var content []byte
        var err error
        
        if isRemoteURL(includePath) {
            // Handle remote includes
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            
            content, err = l.fetcher.Fetch(ctx, includePath)
            if err != nil {
                return errors.RemoteFetch(includePath, err)
            }
            
            // Create temporary file for parsing
            resolvedPath = includePath // Use URL as identifier
        } else {
            // Handle local includes (existing logic)
            resolvedPath = l.resolvePath(includePath, baseDir)
            // ... existing local file logic
        }
        
        // Parse content and merge (unified logic)
        includedConfig, err := l.parseConfigContent(content, resolvedPath)
        if err != nil {
            return err
        }
        
        // Merge as before...
    }
}

func isRemoteURL(path string) bool {
    return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func (l *configLoader) parseConfigContent(content []byte, source string) (*Config, error) {
    // Parse YAML content from byte slice
    // Handle both local files and remote content uniformly
}
```

### Phase 3: Error Handling and Circular Reference Detection (Commits 6-7)

#### Commit 6: Enhanced Error Types for Remote Operations
**File**: `internal/errors/remote.go`

```go
package errors

import (
    "fmt"
    "net/url"
)

// RemoteFetchError represents errors during remote reference fetching
type RemoteFetchError struct {
    URL        string
    StatusCode int
    Cause      error
    Context    map[string]interface{}
}

func (e *RemoteFetchError) Error() string {
    if e.StatusCode > 0 {
        return fmt.Sprintf("failed to fetch remote reference %s: HTTP %d", e.URL, e.StatusCode)
    }
    return fmt.Sprintf("failed to fetch remote reference %s: %v", e.URL, e.Cause)
}

func RemoteFetch(url string, err error) *RemoteFetchError {
    return &RemoteFetchError{
        URL:     url,
        Cause:   err,
        Context: make(map[string]interface{}),
    }
}

// Additional error types for remote operations
type RemoteValidationError struct {
    URL    string
    Reason string
}

type RemoteTimeoutError struct {
    URL     string
    Timeout time.Duration
}

type RemoteCacheError struct {
    URL    string
    Cause  error
}
```

#### Commit 7: Enhanced Circular Reference Detection
**File**: `internal/config/loader.go` (modifications)

```go
// Update configLoader to handle remote circular references
func (l *configLoader) loadConfig(filename string) (*Config, error) {
    var identifier string
    
    if isRemoteURL(filename) {
        // For remote URLs, use the URL itself as identifier
        identifier = filename
    } else {
        // For local files, use absolute path
        absPath, err := filepath.Abs(filename)
        if err != nil {
            return nil, errors.FileRead(filename, err)
        }
        identifier = absPath
    }
    
    // Check for circular includes using unified identifier
    if l.visited[identifier] {
        // Build include chain for better error context
        chain := make([]string, 0)
        for path := range l.visited {
            if l.visited[path] {
                chain = append(chain, path)
            }
        }
        chain = append(chain, identifier)
        return nil, errors.CircularInclude(chain)
    }
    
    l.visited[identifier] = true
    defer func() { l.visited[identifier] = false }()
    
    // Load config based on type (remote vs local)
    if isRemoteURL(filename) {
        return l.loadRemoteConfig(filename)
    } else {
        return l.loadLocalConfig(identifier)
    }
}

func (l *configLoader) loadRemoteConfig(url string) (*Config, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    content, err := l.fetcher.Fetch(ctx, url)
    if err != nil {
        return nil, errors.RemoteFetch(url, err)
    }
    
    return l.parseConfigContent(content, url)
}
```

### Phase 4: Testing and Validation (Commits 8-9)

#### Commit 8: Integration Tests for Remote References
**File**: `testing/integration/remote_test.go`

```go
package integration

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestRemoteIncludes(t *testing.T) {
    // Test successful remote include
    t.Run("successful_remote_include", func(t *testing.T) {
        // Set up test HTTP server
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "text/yaml")
            w.Write([]byte(`
rules:
  - name: "Remote Rule"
    content: "This is a remote rule"
`))
        }))
        defer server.Close()
        
        // Create config with remote include
        // Test loading and validation
    })
    
    t.Run("remote_circular_reference", func(t *testing.T) {
        // Test circular reference detection across remote refs
    })
    
    t.Run("remote_timeout", func(t *testing.T) {
        // Test timeout handling
    })
    
    t.Run("remote_cache_behavior", func(t *testing.T) {
        // Test caching works correctly
    })
    
    t.Run("remote_error_conditions", func(t *testing.T) {
        // Test various HTTP error conditions
    })
}

func TestRemoteSSRFProtection(t *testing.T) {
    testCases := []struct {
        name        string
        url         string
        shouldError bool
    }{
        {"localhost_blocked", "http://localhost:8080/test", true},
        {"private_ip_blocked", "http://192.168.1.1/test", true},
        {"public_ip_allowed", "https://example.com/test", false},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Test SSRF protection
        })
    }
}
```

#### Commit 9: Schema Updates and Documentation
**File**: `schema/ai-rules-v1.schema.json` (modifications)

```json
{
  "properties": {
    "includes": {
      "type": "array",
      "description": "List of file paths or URLs to include. Supports local files and remote HTTPS/HTTP URLs.",
      "items": {
        "type": "string",
        "pattern": "^(https?://|[./])"
      },
      "examples": [
        ["./shared/rules.yaml", "https://example.com/shared.yaml"]
      ]
    }
  }
}
```

**File**: `examples/with-remote-refs.yaml`

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

metadata:
  name: "Example with Remote References"
  version: "1.0.0"

includes:
  - "https://raw.githubusercontent.com/example/shared-rules/main/go-rules.yaml"
  - "https://example.com/team-standards/ai-rules.yaml"
  - "./local/custom-rules.yaml"

outputs:
  - file: "CLAUDE.md"
    template: "default"

rules:
  - name: "Project Specific Rule"
    content: "This rule is specific to this project"
```

### Phase 5: Performance and Final Integration (Commit 10)

#### Commit 10: Performance Optimizations and Final Integration
**Files**: Multiple optimizations

1. **Concurrent Fetching**: Implement `errgroup` for parallel remote fetching
2. **Connection Pooling**: Configure HTTP transport for connection reuse
3. **Compression Support**: Add gzip/deflate support for bandwidth optimization
4. **Metrics**: Add basic metrics for cache hit/miss rates
5. **Final Integration**: Complete integration with existing CLI commands

```go
// internal/remote/parallel.go
func (f *Fetcher) FetchMultiple(ctx context.Context, urls []string) (map[string][]byte, error) {
    g, ctx := errgroup.WithContext(ctx)
    results := make(map[string][]byte)
    var mu sync.Mutex
    
    for _, url := range urls {
        url := url // Capture for goroutine
        g.Go(func() error {
            content, err := f.Fetch(ctx, url)
            if err != nil {
                return err
            }
            
            mu.Lock()
            results[url] = content
            mu.Unlock()
            return nil
        })
    }
    
    return results, g.Wait()
}
```

## Configuration and Usage

### Environment Variables
- `AI_RULEZ_CACHE_DIR`: Override default cache directory
- `AI_RULEZ_CACHE_TTL`: Cache TTL (e.g., "1h", "30m")
- `AI_RULEZ_HTTP_TIMEOUT`: HTTP timeout for remote fetches
- `AI_RULEZ_MAX_REDIRECTS`: Maximum HTTP redirects to follow

### CLI Integration
Remote references work transparently with existing commands:
```bash
ai-rulez validate config-with-remote.yaml
ai-rulez generate config-with-remote.yaml
```

### Cache Management
```bash
ai-rulez cache clear          # Clear all cached remote content
ai-rulez cache list           # List cached URLs and metadata
ai-rulez cache stats          # Show cache statistics
```

## Security Considerations

1. **SSRF Protection**: Comprehensive IP and network blocking
2. **HTTPS Preference**: Warning for HTTP URLs in production
3. **Size Limits**: Configurable limits for remote content size
4. **Rate Limiting**: Per-domain request rate limiting
5. **Timeout Enforcement**: Strict timeout controls

## Performance Characteristics

- **Cold Start**: ~100ms overhead for remote validation
- **Cache Hit**: <1ms for cached remote content  
- **Cache Miss**: Depends on network, typically 100-500ms
- **Memory Usage**: ~1MB per 100 cached remote files
- **Concurrent Fetching**: Up to 10 parallel remote requests

## Testing Strategy

1. **Unit Tests**: Each component thoroughly tested
2. **Integration Tests**: End-to-end remote reference scenarios
3. **Security Tests**: SSRF and validation testing
4. **Performance Tests**: Benchmarks for caching and fetching
5. **Network Tests**: Various HTTP scenarios and error conditions

## Rollout Plan

1. **Phase 1-2**: Core infrastructure (no user-facing changes)
2. **Phase 3**: Basic remote reference support (feature flag protected)
3. **Phase 4-5**: Full feature with comprehensive testing
4. **Documentation**: Update README and examples
5. **Release**: Include in next minor version release

This implementation provides a robust, secure, and performant foundation for remote references in ai-rulez while maintaining backward compatibility and following Go best practices.