// Command skillbench benchmarks agent-skill scanners on a pinned public dataset, reproducibly.
//
//	skillbench fetch       download the pinned dataset and verify its digests
//	skillbench materialize write one SKILL.md per sample of a track, outside any repository
//	skillbench run         scan a shard with one tool and write verdicts
//	skillbench cache-import  seed the verdict cache from a finished run
//	skillbench score       merge verdicts into summary.json and report.md
//	skillbench render      write the published table into README and docs
//	skillbench versions    record the installed tool versions
//
// See website/src/content/docs/benchmarks.md.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/cache"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/dataset"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/render"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/run"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/score"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := dispatch(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "skillbench:", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: skillbench fetch|materialize|run|score|render [flags]")
	}
	commands := map[string]func(context.Context, []string) error{
		"fetch": fetch, "materialize": materialize, "run": runShard, "score": scoreRuns, "render": renderDocs,
		"versions": versions, "cache-import": cacheImport,
	}
	cmd, ok := commands[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q", args[0])
	}
	return cmd(ctx, args[1:])
}

func protocolFlag(fs *flag.FlagSet) *string {
	return fs.String("protocol", "protocol.toml", "the protocol file")
}

func fetch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	proto := protocolFlag(fs)
	data := fs.String("data", "", "directory for the dataset files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := dataset.LoadProtocol(*proto)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	if err := dataset.Fetch(ctx, p, *data, client); err != nil {
		return err
	}
	return dataset.FetchClean(ctx, p, *data, client)
}

func materialize(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("materialize", flag.ContinueOnError)
	proto := protocolFlag(fs)
	data := fs.String("data", "", "directory holding the dataset files")
	track := fs.String("track", dataset.TrackDev, "test, dev, full or clean")
	out := fs.String("out", "", "new directory for the samples, outside any repository")
	every := fs.Int("every", 1, "keep one sample in N; a fixed-count track must use 1")
	allow := fs.Bool("allow-holdout", false, "read a holdout track; every read must be logged in bench/skills/HOLDOUT.md")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := dataset.LoadProtocol(*proto)
	if err != nil {
		return err
	}
	if t, ok := p.Tracks[*track]; ok && t.Holdout && !*allow {
		return fmt.Errorf("track %s is the holdout: pass --allow-holdout and log the read", *track)
	}
	materializer := dataset.Materialize
	if *track == dataset.TrackClean {
		materializer = dataset.MaterializeClean
	}
	m, err := materializer(p, *data, *track, *out, *every)
	if err != nil {
		return err
	}
	if err := dataset.WriteSamples(filepath.Join(*out, "samples.json"), m); err != nil {
		return err
	}
	fmt.Printf("%s: %d samples, population %s, dropped %+v\n", *track, len(m.Samples), m.Digest, m.Dropped)
	return nil
}

func runShard(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dir := fs.String("samples", "", "the materialized directory")
	name := fs.String("tool", "", "ai-rulez-threat, ai-rulez, skill-scanner or skillspector")
	bin := fs.String("bin", "", "the tool's executable (default: its name on PATH)")
	out := fs.String("out", "", "verdict file to write")
	shard := fs.Int("shard", 0, "this shard")
	total := fs.Int("shards", 1, "total shards")
	jobs := fs.Int("jobs", 0, "parallel scans (default: CPUs)")
	timeout := fs.Duration("timeout", 2*time.Minute, "limit per sample")
	cacheDir := fs.String("cache", "", "verdict cache directory (default: no cache)")
	version := fs.String("tool-version", "", "the tool's version, part of the cache key")
	environment := fs.String("environment", "", "architecture, network and image digest, part of the cache key")
	cachedOnly := fs.Bool("cached-only", false, "fail on a cache miss instead of scanning")
	verify := fs.Int("verify", 0, "rescan this many cache hits and fail if a verdict moved")
	maxErrors := fs.Int("max-errors", -1, "fail when more samples than this end in an error (negative: no limit)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := dataset.ReadSamples(filepath.Join(*dir, "samples.json"))
	if err != nil {
		return err
	}
	tool, err := scanner.New(*name, *bin, *timeout)
	if err != nil {
		return err
	}
	if !tool.Supported(ctx) {
		fmt.Fprintf(os.Stderr, "%s: this release has no such gate; the row is left out\n", tool.Name)
		return nil
	}
	o := run.Options{
		Samples: m, Dir: *dir, Out: *out, Tool: tool, Shard: *shard, Total: *total, Jobs: *jobs,
		CachedOnly: *cachedOnly, Verify: *verify, MaxErrors: *maxErrors, Log: os.Stderr,
	}
	if *cacheDir != "" {
		if !*cachedOnly {
			if err := checkVersion(ctx, tool, *version); err != nil {
				return err
			}
		}
		if o.Cache, err = openCache(*cacheDir, tool, *version, *environment); err != nil {
			return err
		}
	}
	return run.Shard(ctx, o)
}

func openCache(dir string, tool *scanner.Tool, version, environment string) (*cache.Store, error) {
	return cache.Open(dir, cache.Key{
		Tool: tool.Name, Version: version, Adapter: tool.AdapterDigest(), Environment: environment,
	})
}

// checkVersion stops a cache key from claiming a version the installed tool is not.
func checkVersion(ctx context.Context, tool *scanner.Tool, want string) error {
	got, err := tool.Version(ctx)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s is %s, the cache key says %s", tool.Name, got, want)
	}
	return nil
}

// cacheImport seeds the cache from a verdict file, so a finished scan never has to be repeated.
func cacheImport(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("cache-import", flag.ContinueOnError)
	dir := fs.String("samples", "", "the materialized directory the verdicts were scanned from")
	verdicts := fs.String("verdicts", "", "verdict file")
	name := fs.String("tool", "", "ai-rulez-threat, ai-rulez, skill-scanner or skillspector")
	cacheDir := fs.String("cache", "", "verdict cache directory")
	version := fs.String("tool-version", "", "the tool's version, part of the cache key")
	environment := fs.String("environment", "", "architecture, network and image digest, part of the cache key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tool, err := scanner.New(*name, "", time.Minute)
	if err != nil {
		return err
	}
	store, err := openCache(*cacheDir, tool, *version, *environment)
	if err != nil {
		return err
	}
	records, err := run.Read(*verdicts)
	if err != nil {
		return err
	}
	for i := range records {
		if records[i].Tool != *name {
			return fmt.Errorf("%s holds verdicts of %s, not %s", *verdicts, records[i].Tool, *name)
		}
		digest, err := cache.HashDir(filepath.Join(*dir, records[i].ID))
		if err != nil {
			return err
		}
		store.Put(digest, records[i].Verdict)
	}
	if err := store.Save(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s: imported %d verdicts, cache now holds %d\n", *name, len(records), store.Len())
	return nil
}

func scoreRuns(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	proto := protocolFlag(fs)
	samples := fs.String("samples", "", "samples.json")
	in := fs.String("runs", "", "directory with one subdirectory of verdict files per run")
	out := fs.String("out", "", "directory for summary.json and verdicts.jsonl")
	versionsPath := fs.String("versions", "", "JSON map of tool to version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := dataset.LoadProtocol(*proto)
	if err != nil {
		return err
	}
	m, err := dataset.ReadSamples(*samples)
	if err != nil {
		return err
	}
	versions, err := readVersions(*versionsPath)
	if err != nil {
		return err
	}
	runs, first, err := readRuns(*in)
	if err != nil {
		return err
	}
	s, err := score.Compute(p, m, runs, versions)
	if err != nil {
		return err
	}
	if err := publish(*out, s, first); err != nil {
		return err
	}
	fmt.Print(render.Table(s))
	return nil
}

func readVersions(path string) (map[string]string, error) {
	versions := map[string]string{}
	if path == "" {
		return versions, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the file
	if err != nil {
		return nil, fmt.Errorf("read versions: %w", err)
	}
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("parse versions: %w", err)
	}
	return versions, nil
}

// readRuns loads every run directory, in name order, and returns the first run's records as well.
func readRuns(dir string) ([]score.Run, []run.Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read runs: %w", err)
	}
	var runs []score.Run
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(dir, e.Name(), "*.jsonl"))
		if err != nil {
			return nil, nil, fmt.Errorf("list run %s: %w", e.Name(), err)
		}
		slices.Sort(files)
		var r score.Run
		for _, f := range files {
			records, err := run.Read(f)
			if err != nil {
				return nil, nil, err
			}
			r.Records = append(r.Records, records...)
		}
		runs = append(runs, r)
	}
	if len(runs) == 0 {
		return nil, nil, fmt.Errorf("no runs under %s", dir)
	}
	return runs, runs[0].Records, nil
}

// publish writes the summary and the sorted verdicts of the scored run.
func publish(dir string, s *score.Summary, records []run.Record) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), append(data, '\n'), 0o644); err != nil { //nolint:gosec // published
		return fmt.Errorf("write summary: %w", err)
	}
	slices.SortStableFunc(records, func(a, b run.Record) int {
		return cmp.Or(strings.Compare(a.Tool, b.Tool), strings.Compare(a.ID, b.ID))
	})
	return run.Write(filepath.Join(dir, "verdicts.jsonl"), records)
}

func versions(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("versions", flag.ContinueOnError)
	out := fs.String("out", "", "file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	found := map[string]string{}
	for _, name := range scanner.Names {
		tool, err := scanner.New(name, "", time.Minute)
		if err != nil {
			return err
		}
		if !tool.Supported(ctx) {
			continue
		}
		v, err := tool.Version(ctx)
		if err != nil {
			return err
		}
		found[name] = v
	}
	data, err := json.MarshalIndent(found, "", "  ")
	if err != nil {
		return fmt.Errorf("encode versions: %w", err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil { //nolint:gosec // published
		return fmt.Errorf("write versions: %w", err)
	}
	return nil
}

func renderDocs(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	summary := fs.String("summary", "", "summary.json")
	check := fs.Bool("check", false, "fail if a file is stale; write nothing")
	name := fs.String("block", render.DefaultBlock, "the marked block to replace: <!-- NAME:start --> to <!-- NAME:end -->")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := render.Load(*summary)
	if err != nil {
		return err
	}
	block := render.Table(s)
	for _, path := range fs.Args() {
		if err := render.File(path, *name, block, *check); err != nil {
			return err
		}
	}
	return nil
}
