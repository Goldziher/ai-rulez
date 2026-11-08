package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func ptr(s string) *string { return &s }

// fixture writes a small dataset and a protocol that pins it.
func fixture(t *testing.T) (*Protocol, string) {
	t.Helper()
	dir := t.TempDir()
	rows := []row{
		{ID: "a1", Label: "1", Text: ptr("alpha"), Source: "S1", Evidence: "static", Family: ptr("F1"), NormHash: "h1"},
		{ID: "a2", Label: "1", Text: ptr("bravo"), Source: "S2", Evidence: "static", Family: ptr("F1"), NormHash: "h2"},
		{ID: "a3", Label: "0", Text: ptr("charlie"), Source: "S1", Evidence: "static", NormHash: "h3"},
		{ID: "a4", Label: "0", Text: ptr("delta"), Source: "S2", Evidence: "static", NormHash: "h4"},
		{ID: "a5", Label: "1", Source: "S2", Evidence: "static", NormHash: "h5"},
		{ID: "a6", Label: "1", Text: ptr("echo"), Source: "S3", Evidence: "static", NormHash: "h6"},
		{ID: "a7", Label: "1", Text: ptr("foxtrot"), Source: "S2", Evidence: "static", NormHash: "h7"},
	}
	splits := []splitRow{
		{"a1", "test"}, {"a2", "train"}, {"a3", "test"}, {"a4", "validation"}, {"a5", "train"}, {"a6", "excluded"}, {"a7", "train"},
	}
	primary := filepath.Join(dir, "primary.parquet")
	if err := parquet.WriteFile(primary, rows); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "splits"), 0o750); err != nil {
		t.Fatal(err)
	}
	split := filepath.Join(dir, "splits", "source_disjoint.parquet")
	if err := parquet.WriteFile(split, splits); err != nil {
		t.Fatal(err)
	}
	return &Protocol{
		Dataset: Source{Repo: "x/y", Revision: "r", Files: []File{
			{Path: "primary.parquet", SHA256: digest(primary)},
			{Path: "splits/source_disjoint.parquet", SHA256: digest(split)},
		}},
		Tracks: map[string]Track{"test": {Malicious: 1, Benign: 1, Holdout: true}, "dev": {}, "full": {}},
	}, dir
}

func TestMaterializeDev(t *testing.T) {
	p, data := fixture(t)
	out := t.TempDir()
	m, err := Materialize(p, data, TrackDev, out, 1)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range m.Samples {
		ids = append(ids, s.ID)
	}
	// a2 shares family F1 with the test row a1, so tuning on it would leak the holdout.
	if got := strings.Join(ids, ","); got != "a4,a7" {
		t.Fatalf("dev = %s", got)
	}
	if m.Dropped.Overlap != 1 || m.Dropped.Withheld != 1 || m.Dropped.Excluded != 1 || m.Dropped.Other != 2 {
		t.Fatalf("dropped = %+v", m.Dropped)
	}
}

func TestMaterializeTestIsByteExact(t *testing.T) {
	p, data := fixture(t)
	out := t.TempDir()
	m, err := Materialize(p, data, TrackTest, out, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Samples) != 2 {
		t.Fatalf("test = %+v", m.Samples)
	}
	got, err := os.ReadFile(filepath.Join(out, "a1", "SKILL.md"))
	if err != nil || string(got) != "alpha" {
		t.Fatalf("a1 = %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(out, "a1", "SKILL.md"))
	if err != nil || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("sample mode = %v, %v", info.Mode(), err)
	}
	if m.Samples[0].Cluster != "F1" || m.Samples[1].Cluster != "h3" {
		t.Fatalf("clusters fall back to the hash: %+v", m.Samples)
	}
}

func TestMaterializeFixedCounts(t *testing.T) {
	p, data := fixture(t)
	p.Tracks["test"] = Track{Malicious: 5, Benign: 1}
	if _, err := Materialize(p, data, TrackTest, t.TempDir(), 1); err == nil {
		t.Fatal("a changed population must fail")
	}
}

func TestThinning(t *testing.T) {
	p, data := fixture(t)
	m, err := Materialize(p, data, TrackDev, t.TempDir(), 2)
	if err != nil || len(m.Samples) != 1 {
		t.Fatalf("thinned = %+v, %v", m, err)
	}
	if _, err := Materialize(p, data, TrackTest, t.TempDir(), 2); err == nil {
		t.Fatal("a fixed population cannot be thinned")
	}
}

func TestPopulationDigestIsStable(t *testing.T) {
	p, data := fixture(t)
	a, err := Materialize(p, data, TrackTest, t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Materialize(p, data, TrackTest, t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest || len(a.Digest) != sha256.Size*2 {
		t.Fatalf("digests %s and %s", a.Digest, b.Digest)
	}
}

func TestMaterializeRefusesRepository(t *testing.T) {
	p, data := fixture(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(p, data, TrackTest, filepath.Join(repo, "x", "y"), 1); err == nil {
		t.Fatal("samples must not land inside a repository")
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	p, data := fixture(t)
	if err := os.WriteFile(filepath.Join(data, "primary.parquet"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(p, data, TrackTest, t.TempDir(), 1); err == nil {
		t.Fatal("a changed file must fail")
	}
}

func TestFetchVerifiesDigests(t *testing.T) {
	sum := sha256.Sum256([]byte("body"))
	want := hex.EncodeToString(sum[:])
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	p := &Protocol{Dataset: Source{Files: []File{{Path: "sub/f", SHA256: want}}}}
	if err := fetchFrom(t.Context(), p, dir, srv.Client(), srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if err := fetchFrom(t.Context(), p, dir, srv.Client(), srv.URL+"/"); err != nil || hits != 1 {
		t.Fatalf("a verified file is not downloaded again: hits %d, %v", hits, err)
	}
	bad := &Protocol{Dataset: Source{Files: []File{{Path: "g", SHA256: strings.Repeat("0", 64)}}}}
	if err := fetchFrom(t.Context(), bad, dir, srv.Client(), srv.URL+"/"); err == nil {
		t.Fatal("a wrong digest must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "g")); !os.IsNotExist(err) {
		t.Fatal("a rejected file must not stay on disk")
	}
}

func TestLoadShippedProtocol(t *testing.T) {
	p, err := LoadProtocol("../../protocol.toml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Tracks[TrackTest].Malicious != 839 || p.Tracks[TrackTest].Benign != 545 || !p.Tracks[TrackTest].Holdout {
		t.Fatalf("test track = %+v", p.Tracks[TrackTest])
	}
	if p.Tracks[TrackDev].Holdout {
		t.Fatal("dev must be readable")
	}
}
