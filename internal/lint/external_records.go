package lint

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// ScanRecord is what the result cache holds for one staged scanner over the
// current content: the input of the [[scan]] records of ai-rulez.lock.
type ScanRecord struct {
	Scanner string
	// Version is the scanner's --version line at scan time.
	Version string
	// Tree is the digest of the content staged for the scanner now.
	Tree string
	// Cached is set when the cache holds a result for Tree; the fields below are
	// meaningful only then.
	Cached bool
	// Findings counts the reported results (suppressed ones excluded).
	Findings int
	// MaxSeverity is "error", "warning" or "info" ("" for no finding).
	MaxSeverity string
	// Pass is false when a finding reaches the fail_on threshold (error by default).
	Pass bool
}

// ScanRecords reads the scanner result cache for every staged egress = false
// scanner of cfg and returns one record each, in scanner order. It starts no
// program: a scanner that was not run on the current content has Cached false.
func ScanRecords(cfg *config.Config, tree *Tree, so Options, opts ...Option) ([]ScanRecord, error) {
	counter, err := tokens.New("")
	if err != nil {
		return nil, fmt.Errorf("token counter: %w", err)
	}
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}, counter: counter, opts: so, cwd: so.Cwd,
		deps: map[string]map[string]struct{}{}, names: map[string][]string{}}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	baseAbs, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	r.baseRel = tree.Rel(baseAbs)
	if r.baseRel == "." {
		r.baseRel = ""
	}
	r.resolveSettings()
	r.collect()
	return r.scanRecords(baseAbs), nil
}

func (r *runner) scanRecords(root string) []ScanRecord {
	var out []ScanRecord
	cache := r.opts.Scanner.Cache
	threshold := SeverityError
	if t, ok := ParseSeverity(policyOf(&r.lc).failOn); ok && t != SeverityOff {
		threshold = t
	}
	for _, sc := range r.scanners() {
		if len(sc.Inputs) == 0 || sc.Egress == nil || *sc.Egress || len(sc.allProblems()) > 0 {
			continue
		}
		want := map[string]bool{}
		for _, in := range sc.Inputs {
			want[in] = true
		}
		files := r.stageFiles(want, sc.Layout)
		if len(files) == 0 {
			continue
		}
		rec := ScanRecord{Scanner: sc.Name, Tree: digestStage(files)}
		binary := lookExecutable(sc.Command[0], root)
		if key, ok := scanKeyFor(sc, binary, rec.Tree, r.opts.Scanner.ShowSuppressed, r.isolationKey()); ok {
			if hit, found := r.cacheGet(sc, cache, key); found {
				rec.Cached, rec.Version = true, hit.Version
				rec.Findings, rec.MaxSeverity, rec.Pass = summarize(sc, hit, threshold)
			}
		}
		out = append(out, rec)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Scanner < out[j].Scanner })
	return out
}

// summarize counts a cached result's findings and finds the highest severity.
func summarize(sc resolvedScanner, hit cachedScan, threshold Severity) (count int, maxSeverity string, pass bool) {
	pass = true
	top := SeverityOff
	for _, f := range fromCached(hit.Findings) {
		if f.Suppressed {
			continue
		}
		count++
		band := scannerBand(sc.SeverityMap, f)
		if limit, ok := parseBand(sc.MaxSeverity); ok && band > limit {
			band = limit
		}
		sev := bandSeverity(band)
		if top == SeverityOff || sev.AtLeast(top) {
			top = sev
		}
		if sev.AtLeast(threshold) {
			pass = false
		}
	}
	if top != SeverityOff {
		maxSeverity = string(top)
	}
	return count, maxSeverity, pass
}
