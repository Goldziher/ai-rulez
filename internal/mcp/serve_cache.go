package mcp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/samber/oops"
)

// The serve catalog cache. Building the served catalog renders every skill and
// scans it with the security rules, which on a large monorepo takes seconds; a
// server restart paid it again. The rendered catalog is a function of the files
// the serve build fingerprints (its content digest) and of the view's selection
// options, so it is cached on disk keyed by exactly those. A cache hit restores
// the catalog without rendering or scanning; a digest change invalidates it.
//
// The entry is authenticated with a per-user secret kept outside the cache, like
// the scanner result cache: a planted, edited, truncated or symlinked entry
// fails the check, counts as a miss and is rebuilt. Nothing is ever served from a
// partial or corrupt entry.
const (
	// serveCacheVersion is bumped when the entry format changes. Rendering that
	// changes what the catalog holds is caught by the binary version in the key.
	serveCacheVersion = 1
	// serveCacheFileName is the one entry file per project; its stored key says
	// which build it holds.
	serveCacheFileName = "catalog.json"
	// serveCacheSecretFile is the per-user secret, next to the other caches'.
	serveCacheSecretFile = "serve-cache.key"
	// maxServeCacheEntry bounds one entry's payload, on write and on read. A
	// catalog larger than this is not cached (the build still serves it).
	maxServeCacheEntry = 64 << 20
	// maxServeCacheFile bounds the file that carries a payload: the entry plus
	// its envelope, so a payload at the cap is still readable.
	maxServeCacheFile = maxServeCacheEntry + (1 << 16)
)

// serveCache is the on-disk serve catalog cache. The zero value and a nil
// pointer are disabled caches.
type serveCache struct {
	dir        string
	secretPath string
	// log receives the cache's debug reports; nil is the CLI's logger.
	log logger.Logger

	once   sync.Once
	secret []byte
	err    error
}

// newServeCache returns a cache rooted at dir, authenticated with the secret at
// secretPath (created on first use). Empty arguments give a disabled cache.
func newServeCache(log logger.Logger, dir, secretPath string) *serveCache {
	if dir == "" || secretPath == "" {
		return nil
	}
	return &serveCache{dir: dir, secretPath: secretPath, log: logger.Or(log)}
}

// path is the single entry file of the project.
func (c *serveCache) path() string { return filepath.Join(c.dir, serveCacheFileName) }

// key32 loads the per-user secret once.
func (c *serveCache) key32() ([]byte, error) {
	c.once.Do(func() {
		secret, err := llm.LoadSecretFile(c.secretPath)
		if err != nil {
			c.err = oops.Wrapf(err, "load the serve cache secret")
			return
		}
		c.secret = secret
	})
	return c.secret, c.err
}

// mac authenticates one entry: the version, the key and the payload.
func macEntry(secret []byte, key string, payload []byte) []byte {
	h := hmac.New(sha256.New, secret)
	fmt.Fprintf(h, "%d\x00%s\x00", serveCacheVersion, key) //nolint:errcheck,gosec // hash writes never fail
	h.Write(payload)                                       //nolint:errcheck,gosec // hash writes never fail
	return h.Sum(nil)
}

// load returns the entry stored for key, or false for a missing, stale, corrupt
// or unauthenticated entry.
func (c *serveCache) load(key string) (*serveCacheEntry, bool) {
	data, ok := c.read()
	if !ok {
		return nil, false
	}
	var file serveCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, false
	}
	if file.Version != serveCacheVersion || file.Key != key || len(file.Payload) == 0 {
		return nil, false
	}
	secret, err := c.key32()
	if err != nil {
		return nil, false
	}
	want, err := hex.DecodeString(file.MAC)
	if err != nil || !hmac.Equal(macEntry(secret, key, file.Payload), want) {
		c.log.Debug("Ignoring a serve catalog cache entry that failed its authentication")
		return nil, false
	}
	var entry serveCacheEntry
	if err := json.Unmarshal(file.Payload, &entry); err != nil {
		return nil, false
	}
	if entry.Version != serveCacheVersion || entry.Key != key || entry.Raw == nil || entry.Admitted == nil {
		return nil, false
	}
	return &entry, true
}

// read reads the entry file, refusing a symlinked or oversized one.
func (c *serveCache) read() ([]byte, bool) {
	info, err := os.Lstat(c.path())
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxServeCacheFile {
		return nil, false
	}
	data, err := os.ReadFile(c.path()) //nolint:gosec // the Lstat above rejected a non-regular file
	if err != nil {
		return nil, false
	}
	return data, true
}

// store writes the entry for key, atomically. An entry over the size cap is not
// written; the caller keeps serving the freshly built catalog.
func (c *serveCache) store(key string, entry *serveCacheEntry) error {
	entry.Version, entry.Key = serveCacheVersion, key
	payload, err := json.Marshal(entry)
	if err != nil {
		return oops.Wrapf(err, "encode the serve catalog cache entry")
	}
	if len(payload) > maxServeCacheEntry {
		c.log.Debug("Not caching the served catalog: the entry is over the size cap", "bytes", len(payload), "cap", maxServeCacheEntry)
		return nil
	}
	secret, err := c.key32()
	if err != nil {
		return err
	}
	file := serveCacheFile{
		Version: serveCacheVersion,
		Key:     key,
		Payload: payload,
		MAC:     hex.EncodeToString(macEntry(secret, key, payload)),
	}
	data, err := json.Marshal(file)
	if err != nil {
		return oops.Wrapf(err, "encode the serve catalog cache file")
	}
	return c.writeAtomic(data)
}

// writeAtomic writes data to a private temp file then renames it into place, so
// a concurrent reader never sees a half-written entry.
func (c *serveCache) writeAtomic(data []byte) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return oops.Wrapf(err, "create the serve cache directory")
	}
	tmp, err := os.CreateTemp(c.dir, ".catalog-*.tmp")
	if err != nil {
		return oops.Wrapf(err, "create a serve cache temp file")
	}
	name := tmp.Name()
	defer os.Remove(name) //nolint:errcheck // best-effort cleanup of the temp file
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is what matters
		return oops.Wrapf(err, "write the serve cache entry")
	}
	if err := tmp.Close(); err != nil {
		return oops.Wrapf(err, "close the serve cache temp file")
	}
	if err := os.Rename(name, c.path()); err != nil {
		return oops.Wrapf(err, "replace the serve cache entry")
	}
	return nil
}

// serveCacheFile is the authenticated envelope on disk.
type serveCacheFile struct {
	Version int             `json:"version"`
	Key     string          `json:"key"`
	Payload json.RawMessage `json:"payload"`
	MAC     string          `json:"mac"`
}

// serveCacheEntry is what one build caches: the pre-admission catalog (so
// admission can be judged again as approvals and signing keys age), the admitted
// catalog that is served, and what the build reported.
type serveCacheEntry struct {
	Version int    `json:"version"`
	Key     string `json:"key"`
	// Empty is the build's no-skills message, "" when something is served.
	Empty string `json:"empty,omitempty"`
	// Authored is the digest map of the authored skills, so a cache hit does not
	// re-digest them; nil when the project has no authored content.
	Authored []authoredItemDTO `json:"authored,omitempty"`
	// Raw is the catalog before admission; Admitted is what the server serves.
	Raw      *catalogDTO `json:"raw"`
	Admitted *catalogDTO `json:"admitted"`
}

// serveCacheKey is the content digest plus the view's selection options and what
// each skill source resolved to, so two builds of the same files under different
// profiles, or with a remote ref that moved, never share an entry.
func serveCacheKey(fingerprint string, st *ServeSetup, resolved []*skillsource.Resolved) string {
	h := sha256.New()
	fmt.Fprintf(h, "ai-rulez-serve-catalog\x00%d\x00", serveCacheVersion) //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, st.Version)                                         //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, "\x00")                                             //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, fingerprint)                                        //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, "\x00")                                             //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, st.ViewKey())                                       //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, "\x00")                                             //nolint:errcheck,gosec // hash writes never fail
	for _, res := range resolved {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%t\x01", res.Spec.Name, res.Dir, res.Commit, res.Digest, res.Pinned) //nolint:errcheck,gosec // hash writes never fail
	}
	writeKeyStrings(h, st.Filter.Domains)
	writeKeyStrings(h, st.Filter.Allow)
	writeKeyStrings(h, st.Filter.Deny)
	fmt.Fprintf(h, "frozen=%t\x00offline=%t\x00maxclone=%d", st.Frozen, st.Offline, st.MaxCloneBytes) //nolint:errcheck,gosec // hash writes never fail
	return hex.EncodeToString(h.Sum(nil))
}

func writeKeyStrings(h io.Writer, values []string) {
	for _, v := range values {
		io.WriteString(h, v)      //nolint:errcheck,gosec // hash writes never fail
		io.WriteString(h, "\x00") //nolint:errcheck,gosec // hash writes never fail
	}
	io.WriteString(h, "\x01") //nolint:errcheck,gosec // hash writes never fail
}

// serveCacheLocation is where the cache lives and which secret authenticates it.
// A test override (ServeCacheDir, or CacheDir) keeps both beside the test's other
// cache; otherwise the entry sits in the user cache, keyed by the project, and
// the secret in the user config directory, like the scanner result cache.
func (st *ServeSetup) serveCacheLocation(cfg *config.Config) (dir, secret string) {
	if st.ServeCacheDir != "" {
		return st.ServeCacheDir, filepath.Join(st.ServeCacheDir, "."+serveCacheSecretFile)
	}
	if st.CacheDir != "" {
		dir = filepath.Join(st.CacheDir, "serve")
		return dir, filepath.Join(dir, "."+serveCacheSecretFile)
	}
	base := cfg.ConfigDir
	if base == "" {
		base = st.workDir()
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		abs = base
	}
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = resolved // /var and /private/var name one project
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	dir, err = config.CacheDir("serve", hex.EncodeToString(sum[:8]))
	if err != nil {
		return "", ""
	}
	return dir, llm.UserSecretPath(serveCacheSecretFile)
}

// serveCache is the cache of the setup, or nil when one cannot be placed.
func (st *ServeSetup) serveCache(cfg *config.Config) *serveCache {
	dir, secret := st.serveCacheLocation(cfg)
	return newServeCache(st.logger(), dir, secret)
}

// cachePlan is where a build's catalog is cached and under which key, or nil
// when the build is not cached.
type cachePlan struct {
	cache *serveCache
	key   string
}

// cachePlan prepares a build's cache lookup: only a plain serving build is
// cached, and only when a cache can be placed and its content fingerprint is
// readable. nil means the build runs and is not cached.
func (st *ServeSetup) cachePlan(b *built, bo buildOptions) *cachePlan {
	if !st.cacheable(bo) {
		return nil
	}
	cache := st.serveCache(b.cfg)
	if cache == nil {
		return nil
	}
	roots, err := st.cacheFingerprint(b.cfg, b)
	if err != nil {
		return nil
	}
	return &cachePlan{cache: cache, key: serveCacheKey(roots, st, b.sources)}
}

// cacheable reports whether a build's catalog may be cached: only a plain
// serving build, whose inputs the fingerprint covers. A lock run's views, a
// refresh, a load with programmatic options (whose effect the fingerprint cannot
// see), a load that reuses another build's project or sources, and a build with
// an injected clock (tests, embedders) are not cached.
func (st *ServeSetup) cacheable(bo buildOptions) bool {
	return bo.admit && !bo.ignoreLock && !bo.refresh && bo.reuse == nil && bo.project == nil &&
		len(st.LoadOptions) == 0 && st.LockPolicy == nil && st.Clock == nil
}

// cacheFingerprint is the content digest the cache key is built on: the same
// fingerprint the watcher compares, over the configuration directory, the local
// includes, the policy files and the local skill sources.
func (st *ServeSetup) cacheFingerprint(cfg *config.Config, b *built) (string, error) {
	return fingerprint(st.watchRoots(b), st.usageFiles(cfg)...)
}

// restoreFromCache fills b from a cache entry. The admitted catalog is served as
// it was built; readmit judges admission again from the cached pre-admission
// catalog, so an approval or signing key that expired since the entry was
// written is still caught by the watcher's revalidation.
func (st *ServeSetup) restoreFromCache(b *built, entry *serveCacheEntry, denied map[string]string, bo buildOptions) {
	raw := dtoToCatalog(entry.Raw)
	b.catalog = dtoToCatalog(entry.Admitted)
	b.empty = entry.Empty
	admit := st.admitFunc(b, raw, denied, dtoToAuthored(entry.Authored), bo)
	b.readmit = func() *Catalog { return admit(logger.Discard()) }
}

// --- serialization -----------------------------------------------------------

// catalogDTO is the serializable form of a Catalog. Every field the server reads
// after a build is carried, so a restored catalog is indistinguishable from a
// freshly built one.
type catalogDTO struct {
	Profile string            `json:"profile,omitempty"`
	Preset  string            `json:"preset,omitempty"`
	Skills  []catalogSkillDTO `json:"skills"`
	Refused []refusalDTO      `json:"refused,omitempty"`
	Reports []scanReportDTO   `json:"reports,omitempty"`
}

type catalogSkillDTO struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	URI          string           `json:"uri"`
	Domain       string           `json:"domain,omitempty"`
	Source       string           `json:"source,omitempty"`
	Ref          string           `json:"ref,omitempty"`
	Pinned       bool             `json:"pinned,omitempty"`
	Keywords     []string         `json:"keywords,omitempty"`
	Triggers     []string         `json:"triggers,omitempty"`
	Delivery     string           `json:"delivery,omitempty"`
	Commit       string           `json:"commit,omitempty"`
	Trust        string           `json:"trust,omitempty"`
	Imported     bool             `json:"imported,omitempty"`
	LockDigest   string           `json:"lock_digest,omitempty"`
	Locked       bool             `json:"locked,omitempty"`
	Approved     bool             `json:"approved,omitempty"`
	Approvers    []string         `json:"approvers,omitempty"`
	ScanFindings int              `json:"scan_findings,omitempty"`
	Unscanned    []string         `json:"unscanned,omitempty"`
	ScanLevel    string           `json:"scan_level,omitempty"`
	Unscannable  []findingDTO     `json:"unscannable,omitempty"`
	Frontmatter  map[string]any   `json:"frontmatter,omitempty"`
	Files        []catalogFileDTO `json:"files"`
	Digest       string           `json:"digest"`
}

type catalogFileDTO struct {
	URI     string `json:"uri"`
	RelPath string `json:"rel_path"`
	Digest  string `json:"digest"`
	Size    int    `json:"size"`
	MIME    string `json:"mime"`
	Content []byte `json:"content"`
}

type findingDTO struct {
	Code     string        `json:"code"`
	Name     string        `json:"name"`
	Severity lint.Severity `json:"severity"`
	File     string        `json:"file"`
	Line     int           `json:"line"`
	Message  string        `json:"message"`
	Root     string        `json:"root,omitempty"`
}

type refusalDTO struct {
	Name   string `json:"name"`
	View   string `json:"view,omitempty"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type scanReportDTO struct {
	Skill    string       `json:"skill"`
	View     string       `json:"view,omitempty"`
	Level    string       `json:"level"`
	Findings []findingDTO `json:"findings,omitempty"`
	Unserved []string     `json:"unserved,omitempty"`
}

type authoredItemDTO struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Domain  string `json:"domain,omitempty"`
	Path    string `json:"path,omitempty"`
	Digest  string `json:"digest"`
	Owner   string `json:"owner,omitempty"`
	Version string `json:"version,omitempty"`
}

func findingToDTO(f *lint.Finding) findingDTO {
	return findingDTO{Code: f.Code, Name: f.Name, Severity: f.Severity, File: f.File, Line: f.Line, Message: f.Message, Root: f.Root}
}

func (d findingDTO) toFinding() lint.Finding {
	return lint.Finding{Code: d.Code, Name: d.Name, Severity: d.Severity, File: d.File, Line: d.Line, Message: d.Message, Root: d.Root}
}

// catalogToDTO flattens a catalog for the cache. Refusals are sorted by name so
// two builds of the same content serialize identically.
func catalogToDTO(c *Catalog) *catalogDTO {
	if c == nil {
		return nil
	}
	out := &catalogDTO{Profile: c.Profile, Preset: c.Preset}
	for _, s := range c.skills {
		out.Skills = append(out.Skills, skillToDTO(s))
	}
	for _, r := range c.Refusals() {
		out.Refused = append(out.Refused, refusalDTO(r))
	}
	for i := range c.reports {
		out.Reports = append(out.Reports, reportToDTO(&c.reports[i]))
	}
	return out
}

func skillToDTO(s *CatalogSkill) catalogSkillDTO {
	d := catalogSkillDTO{
		Name: s.Name, Description: s.Description, URI: s.URI, Domain: s.Domain, Source: s.Source,
		Ref: s.Ref, Pinned: s.Pinned, Keywords: s.Keywords, Triggers: s.Triggers, Delivery: s.Delivery,
		Commit: s.Commit, Trust: s.Trust, Imported: s.Imported, LockDigest: s.LockDigest, Locked: s.Locked,
		Approved: s.Approved, Approvers: s.Approvers, ScanFindings: s.ScanFindings, Unscanned: s.Unscanned,
		ScanLevel: s.scanLevel, Frontmatter: s.Frontmatter, Digest: s.Digest,
	}
	for i := range s.unscannable {
		d.Unscannable = append(d.Unscannable, findingToDTO(&s.unscannable[i]))
	}
	for i := range s.Files {
		f := &s.Files[i]
		d.Files = append(d.Files, catalogFileDTO{URI: f.URI, RelPath: f.RelPath, Digest: f.Digest, Size: f.Size, MIME: f.MIME, Content: f.Content})
	}
	return d
}

func reportToDTO(r *ScanReport) scanReportDTO {
	d := scanReportDTO{Skill: r.Skill, View: r.View, Level: r.Level, Unserved: r.Unserved}
	for i := range r.Findings {
		d.Findings = append(d.Findings, findingToDTO(&r.Findings[i]))
	}
	return d
}

// dtoToCatalog rebuilds a catalog, including the name and URI indexes the server
// resolves lookups through.
func dtoToCatalog(d *catalogDTO) *Catalog {
	if d == nil {
		return nil
	}
	c := &Catalog{
		Profile: d.Profile, Preset: d.Preset,
		byName: map[string]*CatalogSkill{}, byURI: map[string]*CatalogSkill{}, byFile: map[string]*CatalogFile{},
	}
	for i := range d.Skills {
		s := d.Skills[i].toSkill()
		c.skills = append(c.skills, s)
		c.byName[s.Name] = s
		c.byURI[s.URI] = s
		for j := range s.Files {
			c.byFile[s.Files[j].URI] = &s.Files[j]
		}
	}
	for _, r := range d.Refused {
		if c.refused == nil {
			c.refused = map[string]Refusal{}
		}
		c.refused[r.Name] = Refusal(r)
	}
	for i := range d.Reports {
		c.reports = append(c.reports, d.Reports[i].toReport())
	}
	return c
}

func (d catalogSkillDTO) toSkill() *CatalogSkill {
	s := &CatalogSkill{
		Name: d.Name, Description: d.Description, URI: d.URI, Domain: d.Domain, Source: d.Source,
		Ref: d.Ref, Pinned: d.Pinned, Keywords: d.Keywords, Triggers: d.Triggers, Delivery: d.Delivery,
		Commit: d.Commit, Trust: d.Trust, Imported: d.Imported, LockDigest: d.LockDigest, Locked: d.Locked,
		Approved: d.Approved, Approvers: d.Approvers, ScanFindings: d.ScanFindings, Unscanned: d.Unscanned,
		scanLevel: d.ScanLevel, Frontmatter: d.Frontmatter, Digest: d.Digest,
	}
	for i := range d.Unscannable {
		s.unscannable = append(s.unscannable, d.Unscannable[i].toFinding())
	}
	for i := range d.Files {
		f := &d.Files[i]
		s.Files = append(s.Files, CatalogFile{URI: f.URI, RelPath: f.RelPath, Digest: f.Digest, Size: f.Size, MIME: f.MIME, Content: f.Content})
	}
	return s
}

func (d scanReportDTO) toReport() ScanReport {
	r := ScanReport{Skill: d.Skill, View: d.View, Level: d.Level, Unserved: d.Unserved}
	for i := range d.Findings {
		r.Findings = append(r.Findings, d.Findings[i].toFinding())
	}
	return r
}

// authoredToDTO flattens the authored-skill digest map, sorted by key.
func authoredToDTO(items map[string]lockfile.Item) []authoredItemDTO {
	if len(items) == 0 {
		return nil
	}
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]authoredItemDTO, 0, len(keys))
	for _, k := range keys {
		it := items[k]
		out = append(out, authoredItemDTO{Key: k, Kind: it.Kind, ID: it.ID, Domain: it.Domain, Path: it.Path, Digest: it.Digest, Owner: it.Owner, Version: it.Version})
	}
	return out
}

func dtoToAuthored(items []authoredItemDTO) map[string]lockfile.Item {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]lockfile.Item, len(items))
	for i := range items {
		d := &items[i]
		out[d.Key] = lockfile.Item{Kind: d.Kind, ID: d.ID, Domain: d.Domain, Path: d.Path, Digest: d.Digest, Owner: d.Owner, Version: d.Version}
	}
	return out
}
