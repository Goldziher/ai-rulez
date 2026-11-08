// Package run executes one scanner over one shard of a track and writes verdicts.
package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/cache"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/dataset"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

// Record is one line of a verdict file. It holds no sample text and no timing, so identical
// behavior gives identical bytes.
type Record struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	scanner.Verdict
}

// Options select the work.
type Options struct {
	Samples      *dataset.Materialized
	Dir          string
	Out          string
	Tool         *scanner.Tool
	Shard, Total int
	Jobs         int
	// Cache supplies verdicts for unchanged samples and keeps new ones. Nil scans everything.
	Cache *cache.Store
	// CachedOnly fails on a miss instead of scanning, for tools that are not installed here.
	CachedOnly bool
	// Verify rescans this many cache hits. A changed verdict fails the run; changed findings are logged.
	Verify int
	// MaxErrors fails the run when more samples than this end in an error, so a tool that cannot start
	// is not published as a tool that found nothing. Negative disables it.
	MaxErrors int
	// Log reports what the cache did. Nil is silent.
	Log io.Writer
}

// planned is the work one shard needs after the cache has answered what it can.
type planned struct {
	out     []Record
	digests []string
	scan    []int
	check   map[int]bool
	hits    int
}

// moved compares a recheck with its cached verdict. A failed recheck proves nothing.
func moved(fresh, cached scanner.Verdict) (verdict, findings bool) {
	if fresh.Error != "" {
		return false, false
	}
	return fresh.Flagged != cached.Flagged, !reflect.DeepEqual(fresh, cached)
}

// Shard scans every sample of the shard and writes sorted verdicts to Options.Out.
func Shard(ctx context.Context, o Options) (err error) {
	if o.Total < 1 || o.Shard < 0 || o.Shard >= o.Total {
		return fmt.Errorf("shard %d of %d is out of range", o.Shard, o.Total)
	}
	if o.Jobs < 1 {
		o.Jobs = runtime.NumCPU()
	}
	todo := shardIDs(o)
	base, err := os.MkdirTemp("", "skillbench-work-")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(base)) }()

	p, err := plan(todo, o)
	if err != nil {
		return err
	}
	out := p.out
	diff, drift, err := scanAll(ctx, o, todo, p, base)
	if err != nil {
		return err
	}
	if len(diff) > 0 {
		slices.Sort(diff)
		return fmt.Errorf("%s: %d of %d cached verdicts did not reproduce, first %s: the tool is not deterministic here, or the key misses something",
			o.Tool.Name, len(diff), len(p.check), diff[0])
	}
	if err := saveCache(o, p); err != nil {
		return err
	}
	if failed := countErrors(out); o.MaxErrors >= 0 && failed > o.MaxErrors {
		return fmt.Errorf("%s: %d of %d samples ended in an error, the limit is %d: the tool is not working here",
			o.Tool.Name, failed, len(out), o.MaxErrors)
	}
	if o.Log != nil {
		if _, err := fmt.Fprintf(o.Log, "%s: %d cached, %d scanned, %d rechecked, %d with changed findings\n",
			o.Tool.Name, p.hits, len(p.scan)-len(p.check), len(p.check), drift); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
	}
	return Write(o.Out, out)
}

// scanAll runs the planned scans on o.Jobs workers. A recheck compares with the cached verdict and
// returns the samples whose verdict moved, and how many kept their verdict with other findings.
func scanAll(ctx context.Context, o Options, todo []string, p *planned, base string) (diff []string, drift int, err error) {
	var (
		wg   sync.WaitGroup
		lock sync.Mutex
		next = make(chan int)
	)
	for w := range o.Jobs {
		work := filepath.Join(base, fmt.Sprint(w))
		if err := os.MkdirAll(work, 0o750); err != nil {
			close(next)
			wg.Wait()
			return nil, 0, fmt.Errorf("create %s: %w", work, err)
		}
		wg.Go(func() {
			for i := range next {
				v := o.Tool.Scan(ctx, filepath.Join(o.Dir, todo[i]), work)
				if p.check[i] {
					verdict, findings := moved(v, p.out[i].Verdict)
					lock.Lock()
					if verdict {
						diff = append(diff, todo[i])
					} else if findings {
						drift++
					}
					lock.Unlock()
					continue
				}
				p.out[i] = Record{ID: todo[i], Tool: o.Tool.Name, Verdict: v}
			}
		})
	}
	for _, i := range p.scan {
		next <- i
	}
	close(next)
	wg.Wait()
	return diff, drift, nil
}

// saveCache stores what a full run scanned. A cached-only run never writes.
func saveCache(o Options, p *planned) error {
	if o.Cache == nil || o.CachedOnly {
		return nil
	}
	for i, d := range p.digests {
		if !p.check[i] {
			o.Cache.Put(d, p.out[i].Verdict)
		}
	}
	return o.Cache.Save()
}

func countErrors(records []Record) int {
	n := 0
	for i := range records {
		if records[i].Error != "" {
			n++
		}
	}
	return n
}

// plan answers from the cache, and chooses what still has to run. A cached error is replayed by a
// cached-only run and retried by any other. Rechecks follow digest order, so the choice does not depend
// on sample ids and needs no seed.
func plan(todo []string, o Options) (*planned, error) {
	p := &planned{out: make([]Record, len(todo)), check: map[int]bool{}}
	if o.Cache == nil {
		for i := range todo {
			p.scan = append(p.scan, i)
		}
		return p, nil
	}
	p.digests = make([]string, len(todo))
	var hits []int
	for i, id := range todo {
		digest, err := cache.HashDir(filepath.Join(o.Dir, id))
		if err != nil {
			return nil, err
		}
		p.digests[i] = digest
		v, ok := o.Cache.Get(digest)
		switch {
		case ok && (v.Error == "" || o.CachedOnly):
			p.out[i] = Record{ID: id, Tool: o.Tool.Name, Verdict: v}
			hits = append(hits, i)
		case o.CachedOnly:
			return nil, fmt.Errorf("%s: no cached verdict for %s: run the full benchmark for this version once", o.Tool.Name, id)
		default:
			p.scan = append(p.scan, i)
		}
	}
	p.hits = len(hits)
	if !o.CachedOnly {
		slices.SortFunc(hits, func(a, b int) int { return strings.Compare(p.digests[a], p.digests[b]) })
		for _, i := range hits[:min(o.Verify, len(hits))] {
			p.check[i] = true
			p.scan = append(p.scan, i)
		}
	}
	return p, nil
}

// Write stores records as JSON lines.
func Write(path string, records []Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path) //nolint:gosec // the operator names the output
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for i := range records {
		if err := enc.Encode(records[i]); err != nil {
			return errors.Join(fmt.Errorf("encode %s: %w", path, err), f.Close())
		}
	}
	if err := w.Flush(); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", path, err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

// Read loads records from a verdict file.
func Read(path string) ([]Record, error) {
	f, err := os.Open(path) //nolint:gosec // the operator names the input
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var out []Record
	dec := json.NewDecoder(bufio.NewReader(f))
	for dec.More() {
		var r Record
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// shardIDs lists the samples of this shard, every Total-th from Shard.
func shardIDs(o Options) []string {
	var todo []string
	for i, s := range o.Samples.Samples {
		if i%o.Total == o.Shard {
			todo = append(todo, s.ID)
		}
	}
	return todo
}
