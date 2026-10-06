package lint

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// The scanner result cache. A staged scanner run is a function of the scanner
// binary, its arguments and configuration, and the staged content, so an
// unchanged tree costs nothing. Entries hold normalised findings only, never the
// raw report (which can quote a secret the scanner matched), and are
// authenticated with a per-user secret kept outside the repository, like the
// LLM cache: a planted, edited, truncated or symlinked entry fails the check,
// counts as a miss and is removed. Scanners that can send content away
// (egress = true) are never cached: their answer depends on a remote service.

const (
	// scanCacheVersion is bumped when the entry format or the key changes.
	scanCacheVersion = 1
	// maxScanCacheEntry bounds one entry's size, on write and on read.
	maxScanCacheEntry = 4 << 20
	// scanCacheSecretFile is the per-user secret, next to the LLM cache's.
	scanCacheSecretFile = "scan-cache.key"
)

// ScanCache is the on-disk result cache. The zero value and a nil pointer are
// disabled caches.
type ScanCache struct {
	dir        string
	secretPath string

	once   sync.Once
	secret []byte
}

// NewScanCache returns a cache rooted at dir, authenticated with the secret at
// secretPath (created on first use). Empty arguments give a disabled cache.
func NewScanCache(dir, secretPath string) *ScanCache {
	if dir == "" || secretPath == "" {
		return nil
	}
	return &ScanCache{dir: dir, secretPath: secretPath}
}

// UserScanCache places the cache in the user's directories for the project
// whose configuration directory is configDir: ~/.cache/ai-rulez/scan/<project
// hash> and the secret in the user config directory. It returns nil when there
// is no home directory.
func UserScanCache(configDir string) *ScanCache {
	abs, err := filepath.Abs(configDir)
	if err != nil {
		abs = configDir
	}
	if real, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = real // /var and /private/var name one project
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	dir, err := config.CacheDir("scan", hex.EncodeToString(sum[:8]))
	if err != nil {
		return nil
	}
	return NewScanCache(dir, llm.UserSecretPath(scanCacheSecretFile))
}

func (c *ScanCache) key32() []byte {
	c.once.Do(func() {
		secret, err := llm.LoadSecretFile(c.secretPath)
		if err != nil {
			logger.Debug("Scanner result cache is off: no usable secret", "error", err)
			return
		}
		c.secret = secret
	})
	return c.secret
}

// cachedFinding is a finding as the cache keeps it (every field the mapping
// needs, which externalFinding does not all serialise).
type cachedFinding struct {
	File         string  `json:"file,omitempty"`
	Line         int     `json:"line,omitempty"`
	Severity     string  `json:"severity,omitempty"`
	Rule         string  `json:"rule,omitempty"`
	Message      string  `json:"message,omitempty"`
	Fingerprint  string  `json:"fingerprint,omitempty"`
	Score        float64 `json:"score,omitempty"`
	HasScore     bool    `json:"has_score,omitempty"`
	DefaultLevel string  `json:"default_level,omitempty"`
	HelpURI      string  `json:"help_uri,omitempty"`
	Suppressed   bool    `json:"suppressed,omitempty"`
}

// cachedScan is one cached run. File of each finding is the stage-relative path.
type cachedScan struct {
	Findings   []cachedFinding `json:"findings"`
	OutOfScope int             `json:"out_of_scope,omitempty"`
	FirstOut   string          `json:"first_out,omitempty"`
	// Version is the scanner's --version line at scan time ("" when unknown).
	Version string `json:"version,omitempty"`
}

func toCached(found []externalFinding) []cachedFinding {
	out := make([]cachedFinding, len(found))
	for i, f := range found {
		out[i] = cachedFinding{File: f.File, Line: f.Line, Severity: f.Severity, Rule: f.Rule, Message: f.Message,
			Fingerprint: f.Fingerprint, Score: f.Score, HasScore: f.HasScore, DefaultLevel: f.DefaultLevel,
			HelpURI: f.HelpURI, Suppressed: f.Suppressed}
	}
	return out
}

func fromCached(in []cachedFinding) []externalFinding {
	out := make([]externalFinding, len(in))
	for i, f := range in {
		out[i] = externalFinding{File: f.File, Line: f.Line, Severity: f.Severity, Rule: f.Rule, Message: f.Message,
			Fingerprint: f.Fingerprint, Score: f.Score, HasScore: f.HasScore, DefaultLevel: f.DefaultLevel,
			HelpURI: f.HelpURI, Suppressed: f.Suppressed}
	}
	return out
}

type scanCacheEntry struct {
	V       int             `json:"v"`
	MAC     string          `json:"mac"`
	Payload json.RawMessage `json:"payload"`
}

func (c *ScanCache) path(key string) string { return filepath.Join(c.dir, key[:2], key+".json") }

func (c *ScanCache) mac(secret []byte, key string, payload []byte) string {
	m := hmac.New(sha256.New, secret)
	for _, part := range [][]byte{[]byte("ai-rulez scan cache v1"), []byte(key), payload} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(part)))
		m.Write(n[:])
		m.Write(part)
	}
	return hex.EncodeToString(m.Sum(nil))
}

// get returns the entry for key. Anything that is not a regular, small, correctly
// authenticated file is a miss, and is removed.
func (c *ScanCache) get(key string) (cachedScan, bool) {
	var zero cachedScan
	if c == nil || len(key) < 2 {
		return zero, false
	}
	secret := c.key32()
	if len(secret) == 0 {
		return zero, false
	}
	p := c.path(key)
	fi, err := os.Lstat(p)
	if err != nil {
		return zero, false
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxScanCacheEntry {
		_ = os.Remove(p) //nolint:errcheck,gosec // a planted link or oversized file is dropped, never followed
		return zero, false
	}
	f, err := os.Open(p) //nolint:gosec // a hash under the cache directory
	if err != nil {
		return zero, false
	}
	defer f.Close() //nolint:errcheck // read only
	data, err := io.ReadAll(io.LimitReader(f, maxScanCacheEntry+1))
	if err != nil || len(data) > maxScanCacheEntry {
		return zero, false
	}
	var e scanCacheEntry
	var out cachedScan
	if json.Unmarshal(data, &e) != nil || e.V != scanCacheVersion ||
		!hmac.Equal([]byte(e.MAC), []byte(c.mac(secret, key, e.Payload))) || json.Unmarshal(e.Payload, &out) != nil {
		_ = os.Remove(p) //nolint:errcheck,gosec // a failed check is a miss and the entry is replaced
		return zero, false
	}
	return out, true
}

// put stores an entry; a failure only costs the next run its hit.
func (c *ScanCache) put(key string, v cachedScan) {
	if c == nil || len(key) < 2 {
		return
	}
	secret := c.key32()
	if len(secret) == 0 {
		return
	}
	payload, err := json.Marshal(v)
	if err != nil || len(payload) > maxScanCacheEntry/2 {
		return
	}
	data, err := json.Marshal(scanCacheEntry{V: scanCacheVersion, MAC: c.mac(secret, key, payload), Payload: payload})
	if err != nil {
		return
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".entry-*") // mode 0600
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil || os.Rename(tmp.Name(), p) != nil {
		_ = os.Remove(tmp.Name()) //nolint:errcheck,gosec // temp file
	}
}

// scanCacheKey is what a staged run's result depends on.
type scanCacheKeyInput struct {
	V       int      `json:"v"`
	Scanner string   `json:"scanner"`
	Binary  string   `json:"binary"`
	BinSize int64    `json:"bin_size"`
	BinTime int64    `json:"bin_mtime"`
	Command []string `json:"command"`
	Format  string   `json:"format"`
	Layout  string   `json:"layout,omitempty"`
	Inputs  []string `json:"inputs"`
	EnvPass []string `json:"env_pass,omitempty"`
	// SeverityMap is sorted "pattern=value" pairs.
	SeverityMap []string `json:"severity_map,omitempty"`
	MaxSeverity string   `json:"max_severity,omitempty"`
	Suppressed  bool     `json:"suppressed,omitempty"`
	Tree        string   `json:"tree"`
}

func (k scanCacheKeyInput) hash() string {
	data, _ := json.Marshal(k) //nolint:errcheck // plain strings and numbers
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// digestStage is the digest of the staged content: every path with its bytes.
func digestStage(files []stagedFile) string {
	h := sha256.New()
	for _, f := range files { // sorted by stageFiles
		fmt.Fprintf(h, "%d:%s\x00%d:", len(f.rel), f.rel, len(f.data))
		h.Write(f.data)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// scanKeyFor builds the key of one scanner over a staged tree. binary is the
// resolved executable ("" when it is not installed, which has no cached result).
// The scanner's identity is its binary file (path, size, modification time) and
// its command line; the version string is stored with the entry but is not part
// of the key, so computing a key never starts a program.
func scanKeyFor(sc resolvedScanner, binary, tree string, showSuppressed bool) (string, bool) {
	info, err := os.Stat(binary)
	if binary == "" || err != nil {
		return "", false
	}
	smap := make([]string, 0, len(sc.SeverityMap))
	for k, v := range sc.SeverityMap {
		smap = append(smap, k+"="+v)
	}
	sort.Strings(smap)
	return scanCacheKeyInput{
		V: scanCacheVersion, Scanner: sc.Name, Binary: binary, BinSize: info.Size(), BinTime: info.ModTime().UnixNano(),
		Command: sc.Command, Format: sc.Format, Layout: sc.Layout, Inputs: sc.Inputs,
		EnvPass: sc.EnvPass, SeverityMap: smap, MaxSeverity: sc.MaxSeverity, Suppressed: showSuppressed, Tree: tree,
	}.hash(), true
}
