package skillsearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Index files, inside the index directory. The manifest is written last, so a
// manifest that validates against vectors.bin means both were written.
const (
	ManifestFile       = "manifest.json"
	VectorsFile        = "vectors.bin"
	IndexSchemaVersion = 1

	DTypeFloat32 = "float32"
	DTypeFloat16 = "float16"

	maxIndexRows    = 1_000_000
	maxVectorsBytes = 100 << 20
	maxManifestSize = 64 << 20
	maxDims         = 16384
)

// ErrNoIndex means there is no usable index: no files, or files that do not
// validate against each other. The ranking is then lexical.
var ErrNoIndex = errors.New("no search index")

// ManifestItem is the manifest row of one indexed skill. ItemDigest is the
// skill's lock digest at index time (for reporting drift); TextDigest is the
// digest of the exact embedded text, which keys vector reuse.
//
//nolint:tagliatelle // the manifest is snake_case
type ManifestItem struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Domain     string `json:"domain"`
	ItemDigest string `json:"item_digest"`
	TextDigest string `json:"text_digest"`
	Row        int    `json:"row"`
}

// Manifest describes an index. It is deterministic: items are sorted by
// (kind, domain, id) and nothing in it is a timestamp, so a committed index
// diffs cleanly and rebuilding from the same inputs gives the same bytes.
//
//nolint:tagliatelle // the manifest is snake_case
type Manifest struct {
	SchemaVersion      int            `json:"schema_version"`
	DocTemplateVersion int            `json:"doc_template_version"`
	Provider           string         `json:"provider"`
	Model              string         `json:"model"`
	Dims               int            `json:"dims"`
	DType              string         `json:"dtype"`
	Normalized         bool           `json:"normalized"`
	Fields             []string       `json:"fields"`
	IndexBody          bool           `json:"index_body"`
	BodyChars          int            `json:"body_chars,omitempty"`
	VectorsDigest      string         `json:"vectors_digest"`
	Items              []ManifestItem `json:"items"`
}

// Index is a loaded vector index: the manifest and one L2-normalised float32
// vector per row.
type Index struct {
	Manifest Manifest
	vecs     [][]float32
	byKey    map[string]int
}

func itemKey(kind, domain, id string) string { return kind + "\x00" + domain + "\x00" + id }

// NewIndex builds an index over rows. vectors[i] belongs to items[i]; they are
// normalised here. It fails on a ragged, empty-dimension, NaN or infinite vector.
func NewIndex(m Manifest, items []ManifestItem, vectors [][]float32) (*Index, error) {
	if len(items) != len(vectors) {
		return nil, oops.Errorf("search index: %d items but %d vectors", len(items), len(vectors))
	}
	type row struct {
		item ManifestItem
		vec  []float32
	}
	rows := make([]row, len(items))
	for i := range items {
		v := append([]float32(nil), vectors[i]...)
		if len(v) != m.Dims || m.Dims <= 0 {
			return nil, oops.Errorf("search index: vector of %q has %d dimensions, want %d", items[i].ID, len(v), m.Dims)
		}
		if err := finite(v); err != nil {
			return nil, oops.Wrapf(err, "search index: vector of %q", items[i].ID)
		}
		normalize(v)
		it := items[i]
		it.Kind = ItemKind
		rows[i] = row{it, v}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].item, rows[j].item
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		return a.ID < b.ID
	})
	idx := &Index{Manifest: m, byKey: map[string]int{}}
	idx.Manifest.SchemaVersion = IndexSchemaVersion
	idx.Manifest.Normalized = true
	idx.Manifest.Items = make([]ManifestItem, len(rows))
	for i, r := range rows {
		r.item.Row = i
		if _, dup := idx.byKey[itemKey(r.item.Kind, r.item.Domain, r.item.ID)]; dup {
			return nil, oops.Errorf("search index: %q is indexed twice", r.item.ID)
		}
		idx.Manifest.Items[i] = r.item
		idx.vecs = append(idx.vecs, r.vec)
		idx.byKey[itemKey(r.item.Kind, r.item.Domain, r.item.ID)] = i
	}
	if idx.Manifest.Fields == nil {
		idx.Manifest.Fields = []string{}
	}
	return idx, nil
}

func finite(v []float32) error {
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return errors.New("holds a NaN or infinite component")
		}
	}
	return nil
}

// normalize scales v to unit length. A vector that already is one (within float32
// rounding) is left alone, so reusing a stored vector never moves its bits.
func normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 || math.Abs(sum-1) < 1e-5 {
		return
	}
	inv := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

// Len is the number of indexed rows.
func (x *Index) Len() int { return len(x.vecs) }

// Row returns the row of an item, or -1.
func (x *Index) Row(domain, id string) int {
	if r, ok := x.byKey[itemKey(ItemKind, domain, id)]; ok {
		return r
	}
	return -1
}

// Vector returns a row's vector (do not modify).
func (x *Index) Vector(row int) []float32 { return x.vecs[row] }

// encodeVectors lays the rows out little-endian, row-major, in the manifest's dtype.
func (x *Index) encodeVectors() ([]byte, error) {
	width := 4
	if x.Manifest.DType == DTypeFloat16 {
		width = 2
	}
	buf := make([]byte, 0, len(x.vecs)*x.Manifest.Dims*width)
	for _, v := range x.vecs {
		for _, f := range v {
			switch x.Manifest.DType {
			case DTypeFloat16:
				h := float32ToFloat16(f)
				if h&0x7c00 == 0x7c00 {
					return nil, oops.Errorf("search index: component %v does not fit float16", f)
				}
				buf = binary.LittleEndian.AppendUint16(buf, h)
			default:
				buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(f))
			}
		}
	}
	return buf, nil
}

// Bytes returns the exact bytes WriteIndex writes: the manifest and the
// vectors. They are a pure function of the index.
func (x *Index) Bytes() (manifest, vectors []byte, err error) {
	if x.Manifest.DType == "" {
		x.Manifest.DType = DTypeFloat32
	}
	if x.Manifest.DType != DTypeFloat32 && x.Manifest.DType != DTypeFloat16 {
		return nil, nil, oops.Errorf("search index: unknown dtype %q", x.Manifest.DType)
	}
	vectors, err = x.encodeVectors()
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(vectors)
	x.Manifest.VectorsDigest = "sha256:" + hex.EncodeToString(sum[:])
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(x.Manifest); err != nil {
		return nil, nil, oops.Wrapf(err, "encode search manifest")
	}
	return buf.Bytes(), vectors, nil
}

// Quantize rounds the vectors to the stored dtype, so a freshly built index
// ranks exactly as the same index loaded from disk.
func (x *Index) Quantize() error {
	if x.Manifest.DType != DTypeFloat16 {
		return nil
	}
	for _, v := range x.vecs {
		for i, f := range v {
			h := float32ToFloat16(f)
			if h&0x7c00 == 0x7c00 {
				return oops.Errorf("search index: component %v does not fit float16", f)
			}
			v[i] = float16ToFloat32(h)
		}
	}
	return nil
}

// WriteIndex writes the index atomically: vectors first, the manifest last,
// each through a temporary file and a rename.
func WriteIndex(dir string, x *Index) error {
	manifest, vectors, err := x.Bytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return oops.Wrapf(err, "create the search index directory")
	}
	if err := writeAtomic(filepath.Join(dir, VectorsFile), vectors); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ManifestFile), manifest)
}

func writeAtomic(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return oops.Wrapf(err, "write %s", filepath.Base(path))
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name()) //nolint:errcheck // best effort cleanup of the temp file
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is the one to report
		return oops.Wrapf(err, "write %s", filepath.Base(path))
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close() //nolint:errcheck // the sync error is the one to report
		return oops.Wrapf(err, "sync %s", filepath.Base(path))
	}
	if err = tmp.Close(); err != nil {
		return oops.Wrapf(err, "write %s", filepath.Base(path))
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // a committed index must be readable
		return oops.Wrapf(err, "write %s", filepath.Base(path))
	}
	return oops.Wrapf(os.Rename(tmp.Name(), path), "write %s", filepath.Base(path))
}

// LoadIndex reads and validates an index. A missing directory or manifest is
// ErrNoIndex; so is any file that does not validate (truncated vectors, a digest
// mismatch, a NaN, a manifest over its caps): the ranking then falls back to
// lexical instead of trusting damaged data. Whatever is read only orders
// results; it never changes which bytes load_skill returns.
func LoadIndex(dir string) (*Index, error) {
	mpath := filepath.Join(dir, ManifestFile)
	info, err := os.Stat(mpath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoIndex
		}
		return nil, oops.Wrapf(err, "read the search manifest")
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestSize {
		return nil, fmt.Errorf("%w: manifest is not a regular file under %d bytes", ErrNoIndex, maxManifestSize)
	}
	raw, err := os.ReadFile(mpath) //nolint:gosec // the index directory is project-controlled
	if err != nil {
		return nil, oops.Wrapf(err, "read the search manifest")
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: manifest does not parse: %v", ErrNoIndex, err)
	}
	if err := m.check(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoIndex, err)
	}
	width := 4
	if m.DType == DTypeFloat16 {
		width = 2
	}
	want := len(m.Items) * m.Dims * width
	vpath := filepath.Join(dir, VectorsFile)
	vinfo, err := os.Stat(vpath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoIndex, err)
	}
	if !vinfo.Mode().IsRegular() || vinfo.Size() != int64(want) || want > maxVectorsBytes {
		return nil, fmt.Errorf("%w: %s is %d bytes, the manifest needs %d (limit %d)", ErrNoIndex, VectorsFile, vinfo.Size(), want, maxVectorsBytes)
	}
	data, err := os.ReadFile(vpath) //nolint:gosec // the index directory is project-controlled
	if err != nil {
		return nil, oops.Wrapf(err, "read the search vectors")
	}
	sum := sha256.Sum256(data)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != m.VectorsDigest {
		return nil, fmt.Errorf("%w: %s does not match the manifest digest", ErrNoIndex, VectorsFile)
	}
	x := &Index{Manifest: m, byKey: make(map[string]int, len(m.Items))}
	x.vecs = make([][]float32, len(m.Items))
	for i := range m.Items {
		v := make([]float32, m.Dims)
		off := i * m.Dims * width
		for j := range v {
			if width == 2 {
				v[j] = float16ToFloat32(binary.LittleEndian.Uint16(data[off+j*2:]))
			} else {
				v[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[off+j*4:]))
			}
		}
		if err := finite(v); err != nil {
			return nil, fmt.Errorf("%w: row %d %v", ErrNoIndex, i, err)
		}
		x.vecs[i] = v
		x.byKey[itemKey(m.Items[i].Kind, m.Items[i].Domain, m.Items[i].ID)] = i
	}
	return x, nil
}

func (m *Manifest) check() error {
	switch {
	case m.SchemaVersion != IndexSchemaVersion:
		return fmt.Errorf("manifest schema_version %d, this build reads %d", m.SchemaVersion, IndexSchemaVersion)
	case m.DType != DTypeFloat32 && m.DType != DTypeFloat16:
		return fmt.Errorf("manifest dtype %q is not float32 or float16", m.DType)
	case m.Dims <= 0 || m.Dims > maxDims:
		return fmt.Errorf("manifest dims %d out of range", m.Dims)
	case len(m.Items) > maxIndexRows:
		return fmt.Errorf("manifest holds %d rows, the limit is %d", len(m.Items), maxIndexRows)
	}
	seen := map[string]bool{}
	for i := range m.Items {
		it := &m.Items[i]
		if it.Row != i {
			return fmt.Errorf("manifest item %q has row %d, want %d", it.ID, it.Row, i)
		}
		k := itemKey(it.Kind, it.Domain, it.ID)
		if seen[k] {
			return fmt.Errorf("manifest lists %q twice", it.ID)
		}
		seen[k] = true
	}
	return nil
}

// VecHit is one vector-ranked row.
type VecHit struct {
	Row int
	Sim float64
}

// TopK returns up to k rows by cosine similarity to q (which must be
// normalised), best first, ties by manifest order. allow filters rows; nil
// allows all.
func (x *Index) TopK(q []float32, k int, allow func(row int) bool) []VecHit {
	if len(q) != x.Manifest.Dims || k <= 0 {
		return nil
	}
	hits := make([]VecHit, 0, len(x.vecs))
	for r, v := range x.vecs {
		if allow != nil && !allow(r) {
			continue
		}
		var dot float32
		for i, c := range v {
			dot += c * q[i]
		}
		hits = append(hits, VecHit{Row: r, Sim: float64(dot)})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Sim > hits[j].Sim })
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// Reason says why an index cannot be used for the given provider, model and
// config, "" when it can. Vectors from another model or embedded text are
// never mixed in.
func (m *Manifest) Reason(provider, model string, cfg Config) string {
	r := cfg.Resolved()
	switch {
	case m.DocTemplateVersion != DocTemplateVersion:
		return fmt.Sprintf("the text template changed (index %d, build %d)", m.DocTemplateVersion, DocTemplateVersion)
	case provider != "" && m.Provider != provider:
		return fmt.Sprintf("the provider changed (index %q, config %q)", m.Provider, provider)
	case model != "" && m.Model != model:
		return fmt.Sprintf("the model changed (index %q, config %q)", m.Model, model)
	case strings.Join(m.Fields, ",") != strings.Join(r.Fields, ","):
		return "the embedded fields changed"
	case m.IndexBody != r.IndexBody || (r.IndexBody && m.BodyChars != r.BodyChars):
		return "index_body or body_chars changed"
	}
	return ""
}

// Status compares an index with the current items.
type Status struct {
	Rows int `json:"rows"`
	// Current items have a vector for their present embedded text.
	Current int `json:"current"`
	// Stale items are indexed, but their embedded text changed since.
	Stale []string `json:"stale,omitempty"`
	// Missing items have no vector.
	Missing []string `json:"missing,omitempty"`
	// Orphaned rows belong to skills that are no longer served.
	Orphaned []string `json:"orphaned,omitempty"`
	// Drifted counts items whose digest changed but whose embedded text did not
	// (their vector is still valid).
	Drifted int `json:"drifted"`
}

// Check classifies every item against the index.
func (x *Index) Check(items []Item, cfg Config) Status {
	st := Status{Rows: x.Len()}
	present := map[string]bool{}
	for i := range items {
		it := &items[i]
		row := x.Row(it.Domain, it.ID)
		present[itemKey(ItemKind, it.Domain, it.ID)] = true
		if row < 0 {
			st.Missing = append(st.Missing, it.ID)
			continue
		}
		mi := x.Manifest.Items[row]
		if mi.TextDigest != TextDigest(EmbedText(it, cfg)) {
			st.Stale = append(st.Stale, it.ID)
			continue
		}
		st.Current++
		if it.Digest != "" && mi.ItemDigest != it.Digest {
			st.Drifted++
		}
	}
	for _, mi := range x.Manifest.Items {
		if !present[itemKey(mi.Kind, mi.Domain, mi.ID)] {
			st.Orphaned = append(st.Orphaned, mi.ID)
		}
	}
	sort.Strings(st.Stale)
	sort.Strings(st.Missing)
	sort.Strings(st.Orphaned)
	return st
}

// Fresh reports whether every item is current and no row is orphaned.
func (s Status) Fresh() bool { return len(s.Stale) == 0 && len(s.Missing) == 0 && len(s.Orphaned) == 0 }
