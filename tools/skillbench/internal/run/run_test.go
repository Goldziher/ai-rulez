package run

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/cache"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/dataset"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

func TestShardsPartitionThePopulation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	bin := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '{\"findings\":[]}'\n"), 0o700); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	tool, err := scanner.New("ai-rulez", bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	m := &dataset.Materialized{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		m.Samples = append(m.Samples, dataset.Sample{ID: id})
		if err := os.Mkdir(filepath.Join(dir, id), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]int{}
	for shard := range 2 {
		out := filepath.Join(t.TempDir(), "v.jsonl")
		o := Options{Samples: m, Dir: dir, Out: out, Tool: tool, Shard: shard, Total: 2, Jobs: 2}
		if err := Shard(t.Context(), o); err != nil {
			t.Fatal(err)
		}
		records, err := Read(out)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			seen[r.ID]++
		}
	}
	if len(seen) != 5 {
		t.Fatalf("seen = %v", seen)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("%s scanned %d times", id, n)
		}
	}
}

func TestMaxErrorsStopsABrokenTool(t *testing.T) {
	tool, err := scanner.New("ai-rulez", filepath.Join(t.TempDir(), "missing"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o750); err != nil {
		t.Fatal(err)
	}
	m := &dataset.Materialized{Samples: []dataset.Sample{{ID: "a"}}}
	out := filepath.Join(t.TempDir(), "v.jsonl")
	o := Options{Samples: m, Dir: dir, Out: out, Tool: tool, Total: 1, Jobs: 1}
	if err := Shard(t.Context(), o); err == nil {
		t.Fatal("a tool that fails on every sample must not score as finding nothing")
	}
	o.MaxErrors = -1
	if err := Shard(t.Context(), o); err != nil {
		t.Fatalf("a negative limit keeps the fixed denominators: %v", err)
	}
}

func TestShardRange(t *testing.T) {
	if err := Shard(t.Context(), Options{Samples: &dataset.Materialized{}, Shard: 2, Total: 2}); err == nil {
		t.Fatal("an out-of-range shard must fail")
	}
}

// fixture makes a fake tool that logs each scan and reports the given findings, plus samples.
func fixture(t *testing.T, findings string, ids ...string) (*scanner.Tool, *dataset.Materialized, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	root := t.TempDir()
	calls := filepath.Join(root, "calls")
	script := "#!/bin/sh\necho x >> " + calls + "\necho '{\"findings\":" + findings + "}'\n"
	bin := filepath.Join(root, "tool")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	tool, err := scanner.New("ai-rulez", bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	m := &dataset.Materialized{}
	for _, id := range ids {
		m.Samples = append(m.Samples, dataset.Sample{ID: id})
		if err := os.Mkdir(filepath.Join(dir, id), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id, "SKILL.md"), []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return tool, m, dir, calls
}

func scans(t *testing.T, calls string) int {
	t.Helper()
	data, err := os.ReadFile(calls)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(data) / 2
}

func store(t *testing.T, dir string) *cache.Store {
	t.Helper()
	s, err := cache.Open(dir, cache.Key{Tool: "ai-rulez", Version: "1.0.0", Adapter: "a", Environment: "e"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCacheAnswersUnchangedSamples(t *testing.T) {
	tool, m, dir, calls := fixture(t, "[]", "a", "b", "c")
	cacheDir := t.TempDir()
	out := filepath.Join(t.TempDir(), "v.jsonl")
	o := Options{Samples: m, Dir: dir, Out: out, Tool: tool, Total: 1, Jobs: 2}

	o.Cache = store(t, cacheDir)
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if n := scans(t, calls); n != 3 {
		t.Fatalf("first run scanned %d, want 3", n)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	o.Cache, o.Verify = store(t, cacheDir), 1
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if n := scans(t, calls); n != 4 {
		t.Fatalf("second run scanned %d in total, want 3 plus 1 recheck", n)
	}
	second, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("cached output must match scanned output byte for byte")
	}
}

func TestChangedSampleIsRescanned(t *testing.T) {
	tool, m, dir, calls := fixture(t, "[]", "a", "b")
	cacheDir := t.TempDir()
	o := Options{Samples: m, Dir: dir, Out: filepath.Join(t.TempDir(), "v.jsonl"), Tool: tool, Total: 1, Jobs: 1}
	o.Cache = store(t, cacheDir)
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "SKILL.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	o.Cache = store(t, cacheDir)
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if n := scans(t, calls); n != 3 {
		t.Fatalf("scanned %d, want 2 plus the changed one", n)
	}
}

func TestRecheckFailsOnDifference(t *testing.T) {
	tool, m, dir, _ := fixture(t, "[]", "a")
	cacheDir := t.TempDir()
	o := Options{Samples: m, Dir: dir, Out: filepath.Join(t.TempDir(), "v.jsonl"), Tool: tool, Total: 1, Jobs: 1}
	s := store(t, cacheDir)
	digest, err := cache.HashDir(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	s.Put(digest, scanner.Verdict{Flagged: true, MaxSeverity: "high", Rules: []string{"R"}, Findings: 1})
	o.Cache, o.Verify = s, 1
	if err := Shard(t.Context(), o); err == nil {
		t.Fatal("a verdict that does not reproduce must fail the run")
	}
}

func TestCachedOnlyRefusesAMiss(t *testing.T) {
	tool, m, dir, calls := fixture(t, "[]", "a")
	o := Options{Samples: m, Dir: dir, Out: filepath.Join(t.TempDir(), "v.jsonl"), Tool: tool, Total: 1, Jobs: 1,
		Cache: store(t, t.TempDir()), CachedOnly: true}
	if err := Shard(t.Context(), o); err == nil {
		t.Fatal("a miss must fail in cached-only mode")
	}
	if n := scans(t, calls); n != 0 {
		t.Fatalf("cached-only scanned %d", n)
	}
}

func TestRecheckToleratesFindingDrift(t *testing.T) {
	tool, m, dir, _ := fixture(t, "[]", "a")
	s := store(t, t.TempDir())
	digest, err := cache.HashDir(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	s.Put(digest, scanner.Verdict{MaxSeverity: "low", Rules: []string{"R"}, Findings: 1})
	var log strings.Builder
	o := Options{Samples: m, Dir: dir, Out: filepath.Join(t.TempDir(), "v.jsonl"), Tool: tool, Total: 1, Jobs: 1,
		Cache: s, Verify: 1, Log: &log}
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "1 with changed findings") {
		t.Fatalf("log = %q", log.String())
	}
}

func TestCachedErrorIsReplayedOrRetried(t *testing.T) {
	tool, m, dir, calls := fixture(t, "[]", "a")
	cacheDir := t.TempDir()
	s := store(t, cacheDir)
	digest, err := cache.HashDir(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	s.Put(digest, scanner.Verdict{Error: "timeout"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "v.jsonl")
	o := Options{Samples: m, Dir: dir, Out: out, Tool: tool, Total: 1, Jobs: 1, Cache: store(t, cacheDir), CachedOnly: true, MaxErrors: -1}
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	records, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].Error != "timeout" || scans(t, calls) != 0 {
		t.Fatalf("cached-only must replay the error: %+v, %d scans", records[0], scans(t, calls))
	}
	o.Cache, o.CachedOnly = store(t, cacheDir), false
	if err := Shard(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if scans(t, calls) != 1 {
		t.Fatalf("a full run must retry the error, %d scans", scans(t, calls))
	}
	if v, _ := store(t, cacheDir).Get(digest); v.Error != "" {
		t.Fatalf("the answer must replace the error: %+v", v)
	}
}
