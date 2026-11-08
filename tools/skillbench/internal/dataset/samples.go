package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// Split names in splits/source_disjoint.parquet.
const (
	splitTest     = "test"
	splitExcluded = "excluded"
)

// Track names.
const (
	TrackTest  = "test"
	TrackDev   = "dev"
	TrackFull  = "full"
	TrackClean = "clean"
)

// Sample is the metadata of one sample. It carries no skill text, so it is safe to publish.
type Sample struct {
	ID        string `json:"id"`
	Malicious bool   `json:"malicious"`
	Source    string `json:"source"`
	Evidence  string `json:"evidence"`
	// Cluster groups related samples for the bootstrap: the structural family, else the hash.
	Cluster string `json:"cluster"`
}

type row struct {
	ID       string  `parquet:"benchmark_id"`
	Label    string  `parquet:"label"`
	Text     *string `parquet:"skill_text,optional"`
	Source   string  `parquet:"source_id"`
	Evidence string  `parquet:"evidence_type"`
	Family   *string `parquet:"structural_family_id,optional"`
	NormHash string  `parquet:"normalized_hash"`
}

type splitRow struct {
	ID    string `parquet:"benchmark_id"`
	Split string `parquet:"split"`
}

// Materialized is a track written to disk.
type Materialized struct {
	Track   string   `json:"track"`
	Digest  string   `json:"population_sha256"`
	Dropped Dropped  `json:"dropped"`
	Samples []Sample `json:"samples"`
}

// Dropped accounts for every row that is not in the track.
type Dropped struct {
	Withheld int `json:"text_withheld"`
	Excluded int `json:"split_excluded"`
	Overlap  int `json:"overlaps_test"`
	Other    int `json:"other_partition"`
}

// Materialize writes one non-executable <id>/SKILL.md per sample of the track under dir, byte for
// byte, and returns the metadata. dir must be outside any git work tree: agents auto-load skills
// and indexers ingest them. every keeps one sample in N, in id order, and only on a track of free size.
func Materialize(p *Protocol, dataDir, track, dir string, every int) (*Materialized, error) {
	want, ok := p.Tracks[track]
	if !ok {
		return nil, fmt.Errorf("unknown track %q", track)
	}
	if every < 1 || (every > 1 && want.Malicious != 0) {
		return nil, fmt.Errorf("track %s cannot be thinned by %d", track, every)
	}
	if err := refuseRepository(dir); err != nil {
		return nil, err
	}
	if err := Verify(p, dataDir); err != nil {
		return nil, err
	}
	sel, dropped, err := loadTrack(dataDir, track)
	if err != nil {
		return nil, err
	}
	out := &Materialized{Track: track, Dropped: dropped}
	population := sha256.New()
	var malicious, benign int
	for i := range sel {
		if i%every != 0 {
			continue
		}
		r := &sel[i]
		if err := writeSample(dir, r.ID, []byte(*r.Text)); err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(*r.Text))
		population.Write(fmt.Appendf(nil, "%s %s %s\n", r.ID, r.Label, hex.EncodeToString(sum[:])))
		s := metadata(r)
		if s.Malicious {
			malicious++
		} else {
			benign++
		}
		out.Samples = append(out.Samples, s)
	}
	if want.Malicious != 0 && (malicious != want.Malicious || benign != want.Benign) {
		return nil, fmt.Errorf("track %s: %d malicious and %d benign, the protocol fixes %d and %d",
			track, malicious, benign, want.Malicious, want.Benign)
	}
	out.Digest = hex.EncodeToString(population.Sum(nil))
	return out, nil
}

// loadTrack reads the pinned files and returns the rows of a track in id order.
func loadTrack(dataDir, track string) ([]row, Dropped, error) {
	rows, err := readParquet[row](filepath.Join(dataDir, "primary.parquet"))
	if err != nil {
		return nil, Dropped{}, err
	}
	splits, err := readParquet[splitRow](filepath.Join(dataDir, "splits", "source_disjoint.parquet"))
	if err != nil {
		return nil, Dropped{}, err
	}
	split := make(map[string]string, len(splits))
	for _, s := range splits {
		split[s.ID] = s.Split
	}
	sel, dropped, err := selectTrack(rows, split, track)
	if err != nil {
		return nil, dropped, err
	}
	slices.SortFunc(sel, func(a, b row) int { return strings.Compare(a.ID, b.ID) })
	return sel, dropped, nil
}

func metadata(r *row) Sample {
	s := Sample{ID: r.ID, Malicious: r.Label == "1", Source: r.Source, Evidence: r.Evidence, Cluster: r.NormHash}
	if r.Family != nil && *r.Family != "" {
		s.Cluster = *r.Family
	}
	return s
}

// selectTrack picks the rows of a track and accounts for the rest. A dev row sharing a family or a
// hash with test is dropped, because tuning on it would leak the holdout.
func selectTrack(rows []row, split map[string]string, track string) ([]row, Dropped, error) {
	families, hashes := holdoutKeys(rows, split)
	var (
		sel []row
		d   Dropped
	)
	for i := range rows {
		r := &rows[i]
		s, ok := split[r.ID]
		if !ok {
			return nil, d, fmt.Errorf("%s has no split", r.ID)
		}
		overlap := hashes[r.NormHash] || (r.Family != nil && families[*r.Family])
		if counter := rejection(r, s, track, overlap, &d); counter != nil {
			*counter++
			continue
		}
		sel = append(sel, *r)
	}
	return sel, d, nil
}

// holdoutKeys collects what identifies a test row.
func holdoutKeys(rows []row, split map[string]string) (families, hashes map[string]bool) {
	families, hashes = map[string]bool{}, map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if split[r.ID] != splitTest {
			continue
		}
		hashes[r.NormHash] = true
		if r.Family != nil && *r.Family != "" {
			families[*r.Family] = true
		}
	}
	return families, hashes
}

// rejection returns the counter a row falls under, or nil when the track keeps it.
func rejection(r *row, split, track string, overlap bool, d *Dropped) *int {
	test := split == splitTest
	switch {
	case r.Text == nil:
		return &d.Withheld
	case track == TrackFull:
		return nil
	case split == splitExcluded:
		return &d.Excluded
	case track == TrackTest && test, track == TrackDev && !test && !overlap:
		return nil
	case track == TrackDev && !test:
		return &d.Overlap
	}
	return &d.Other
}

func writeSample(dir, id string, text []byte) error {
	if filepath.Base(id) != id || id == "" || id == "." || id == ".." {
		return fmt.Errorf("unsafe sample id %q", id)
	}
	sub := filepath.Join(dir, id)
	if err := os.MkdirAll(sub, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", sub, err)
	}
	path := filepath.Join(sub, "SKILL.md")
	if err := os.WriteFile(path, text, 0o440); err != nil { //nolint:gosec // non-executable by design
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// refuseRepository rejects a target inside a git work tree.
func refuseRepository(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return fmt.Errorf("refusing to materialize samples inside the git work tree %s", d)
		}
		if d == filepath.Dir(d) {
			return nil
		}
	}
}

func readParquet[T any](path string) ([]T, error) {
	f, err := os.Open(path) //nolint:gosec // verified against the protocol before this call
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	r := parquet.NewGenericReader[T](f)
	defer func() { _ = r.Close() }()
	out := make([]T, r.NumRows())
	n := 0
	for n < len(out) {
		c, err := r.Read(out[n:])
		n += c
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	if n != len(out) {
		return nil, fmt.Errorf("read %s: %d of %d rows", path, n, len(out))
	}
	return out, nil
}

// WriteSamples stores the metadata beside the samples, for run and score.
func WriteSamples(path string, m *Materialized) error {
	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return fmt.Errorf("encode samples: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o640); err != nil { //nolint:gosec // no secrets
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ReadSamples loads the metadata written by WriteSamples.
func ReadSamples(path string) (*Materialized, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the file
	if err != nil {
		return nil, fmt.Errorf("read samples: %w", err)
	}
	var m Materialized
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse samples: %w", err)
	}
	return &m, nil
}
