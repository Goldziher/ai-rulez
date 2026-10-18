package lint

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// cachedFile is the result of reading one file once per run. ok is false when
// the file could not be read, so a caller does not stat it again either.
type cachedFile struct {
	raw string
	ok  bool
}

// fileContent returns the raw text of abs, reading it at most once per run. A
// read that failed is remembered too: the run does not retry it per caller.
func (r *runner) fileContent(abs string) (string, bool) {
	if c, hit := r.fileCache[abs]; hit {
		return c.raw, c.ok
	}
	data, err := os.ReadFile(abs) //nolint:gosec // a content file of the project
	c := cachedFile{ok: err == nil}
	if err == nil {
		c.raw = string(data)
	}
	if r.fileCache == nil {
		r.fileCache = map[string]cachedFile{}
	}
	r.fileCache[abs] = c
	return c.raw, c.ok
}

// scanKey identifies one per-file scan: the file and the severity forced onto
// its findings. Imported content scans at its own trust level, so the same text
// under a different forced severity is a different scan.
type scanKey struct {
	abs      string
	forceSev Severity
}

// scanOutcome is the result of scanning one text in isolation: the findings it
// produced and the per-file state the run reuses afterwards. The findings are
// already filtered by severity, ignore paths, inline ignores and example
// regions, so replaying them appends exactly what the sequential scan would.
type scanOutcome struct {
	findings []Finding
	attempts map[string]map[string]bool
	doc      doc
	example  map[int]bool
	st       *scanText
}

// scanTask is one text the parallel pre-pass scans: the file, its raw text and
// the key that identifies the scan.
type scanTask struct {
	key scanKey
	raw string
}

// prefetchScans runs the per-file security scan of the run across
// runtime.GOMAXPROCS(0) workers, once per distinct file, and stores the result
// keyed by scanKey. securityScan replays the stored outcome instead of scanning
// the text again, so the findings, their order and every side effect (inline
// ignores, example regions, suppression attempts, the split text) are identical
// to a sequential scan. A text the pre-pass did not cover is scanned directly.
//
// The pre-pass is bounded: a worker pool over a fixed task list, each worker
// reusing one scratch runner, so memory does not grow with the file count
// beyond the outcomes the run keeps anyway.
func (r *runner) prefetchScans() {
	if r.opts.noScanPrefetch || !r.selected(unitOf("security-scan", AnalyzerSecurity)) {
		return // the scan unit does not run under this selection, or a test asked for the sequential path
	}
	tasks := r.scanTasks()
	if len(tasks) < 2 {
		return // one file is not worth a pool
	}
	// A file reached by more than one item is scanned once and replayed at each
	// call site, so shared content is not scanned again per item or per root.
	uniq := tasks[:0]
	seen := make(map[scanKey]bool, len(tasks))
	for _, t := range tasks {
		if seen[t.key] {
			continue
		}
		seen[t.key] = true
		uniq = append(uniq, t)
	}
	tasks = uniq
	out := make([]*scanOutcome, len(tasks))
	workers := runtime.GOMAXPROCS(0)
	if workers > len(tasks) {
		workers = len(tasks)
	}
	var wg sync.WaitGroup
	var next atomic.Int64
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := r.scanScratch()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(tasks) {
					return
				}
				out[i] = scratch.runScanTask(tasks[i])
			}
		}()
	}
	wg.Wait()
	r.scanResults = make(map[scanKey]*scanOutcome, len(tasks))
	for i, t := range tasks {
		r.scanResults[t.key] = out[i]
	}
}

// scanTasks lists the texts the run will security-scan, in the order the calls
// happen: the owned items and their resources, then the imported items. The raw
// text is read here, once per file, and cached for the item loop to reuse.
func (r *runner) scanTasks() []scanTask {
	var tasks []scanTask
	add := func(abs, raw string, force Severity) {
		if abs == "" {
			return
		}
		tasks = append(tasks, scanTask{key: scanKey{abs: abs, forceSev: force}, raw: raw})
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.owned {
			continue
		}
		raw := it.cf.Content
		if !it.isDoc {
			data, ok := r.fileContent(it.abs)
			if !ok {
				continue
			}
			raw = data
		}
		add(it.abs, raw, "")
		if it.itemDir == "" {
			continue
		}
		for j := range it.cf.Resources {
			res := it.cf.Resources[j]
			if !scannableResource(res) {
				continue
			}
			add(filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath)), string(res.Content), "")
		}
	}
	force, on := r.importLevel()
	if !on {
		return tasks
	}
	for i := range r.items {
		it := &r.items[i]
		if it.owned || !filepath.IsAbs(it.abs) {
			continue
		}
		data, ok := r.fileContent(it.abs)
		if !ok {
			continue
		}
		add(it.abs, data, importForceFor(it.cf.Trust, force))
	}
	return tasks
}

// scannableResource reports whether scanResources reads a skill or command
// resource: markdown resources are items of their own, and the scan only reads
// bounded UTF-8 text.
func scannableResource(res config.SkillResource) bool {
	if strings.HasSuffix(strings.ToLower(res.RelPath), ".md") || len(res.Content) > 512*1024 {
		return false
	}
	return utf8.Valid(res.Content) && bytes.IndexByte(res.Content, 0) < 0
}

// scanScratch returns a runner for the parallel pre-pass: it shares the
// read-only run settings and the tree, and gets its own mutable caches, so a
// worker never touches the main runner's findings or maps.
func (r *runner) scanScratch() *runner {
	sev := make(map[string]Severity, len(r.sev))
	maps.Copy(sev, r.sev)
	return &runner{
		cfg: r.cfg, lc: r.lc, tree: r.tree, opts: r.opts,
		baseRel: r.baseRel, cwd: r.cwd, host: r.host,
		sev: sev, ignore: r.ignore, ignorePaths: r.ignorePaths, ignoreRules: r.ignoreRules, allow: r.allow,
		exampleGlobs: r.exampleGlobs, protected: r.protected, noInline: r.noInline,
		noInlineIgnore: r.noInlineIgnore, sel: r.sel,
		skills: r.skills, commands: r.commands, agents: r.agents, rules: r.rules, contexts: r.contexts,
		names: r.names, docs: map[string]doc{},
	}
}

// runScanTask scans one text on a scratch runner and returns its outcome. The
// scratch state is reset between tasks, so a worker reuses one runner without
// one task seeing another's findings.
func (r *runner) runScanTask(t scanTask) *scanOutcome {
	r.forceSev = t.key.forceSev
	r.findings = r.findings[:0]
	r.docs = map[string]doc{}
	r.scanTexts = nil
	r.exampleCache = nil
	r.attempts = nil
	r.units = nil
	r.cur = nil
	r.securityScanDirect(t.key.abs, t.raw)
	out := &scanOutcome{
		findings: append([]Finding(nil), r.findings...),
		attempts: r.attempts,
		doc:      r.docs[t.key.abs],
		example:  r.exampleCache[t.key.abs],
	}
	if e, ok := r.scanTexts[t.key.abs]; ok {
		out.st = e.t
	}
	return out
}
