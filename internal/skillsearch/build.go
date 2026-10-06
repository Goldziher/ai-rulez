package skillsearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/samber/oops"
)

// CodeTextWithheld is AR9D3: a skill whose embedded text looks like it holds a
// secret is not sent to the embedder. `search index` reports it.
const CodeTextWithheld = "AR9D3"

const (
	// DefaultBatchSize is how many texts one embed call carries.
	DefaultBatchSize = 64
	lockFile         = "index.lock"
	staleLockAge     = 10 * time.Minute
)

// SecretScanner reports whether text looks like it holds a credential, and
// what kind. It is injected so this package does not depend on the linter.
type SecretScanner func(text string) (kind string, found bool)

// BuildOptions configures PlanBuild and Build.
type BuildOptions struct {
	Config   Config
	Embedder Embedder
	// Old is the existing index; vectors are reused from it when it is compatible.
	Old *Index
	// Rebuild ignores Old and re-embeds everything.
	Rebuild bool
	// Only, when not empty, re-embeds just these ids; every other vector is reused from Old.
	Only      map[string]bool
	Scanner   SecretScanner
	BatchSize int
}

// Withheld is an item whose text was not sent.
type Withheld struct {
	ID   string
	Kind string
}

// Plan is what a build would do, computed without any call.
type Plan struct {
	Total     int
	Reused    int
	ToEmbed   int
	Bytes     int
	EstTokens int
	Withheld  []Withheld
	// Reason is why Old could not be reused ("" when it can or there is none).
	Reason string
	todo   []int
}

type work struct {
	text, digest string
}

func (o *BuildOptions) reusable() (*Index, string) {
	if o.Rebuild || o.Old == nil {
		return nil, ""
	}
	if reason := o.Old.Manifest.Reason(o.Embedder.Fingerprint(), o.Embedder.Model(), o.Config); reason != "" {
		return nil, reason
	}
	return o.Old, ""
}

// PlanBuild says what Build would embed.
func PlanBuild(items []Item, o *BuildOptions) *Plan {
	old, reason := o.reusable()
	p := &Plan{Total: len(items), Reason: reason}
	for i := range items {
		text := EmbedText(&items[i], o.Config)
		digest := TextDigest(text)
		if old != nil && !o.Only[items[i].ID] {
			if row := old.Row(items[i].Domain, items[i].ID); row >= 0 && old.Manifest.Items[row].TextDigest == digest {
				p.Reused++
				continue
			}
			// the same text under another skill id (a rename) is reused too
			if _, ok := old.byText(digest); ok {
				p.Reused++
				continue
			}
		}
		if o.Scanner != nil {
			if kind, found := o.Scanner(text); found {
				p.Withheld = append(p.Withheld, Withheld{items[i].ID, kind})
				continue
			}
		}
		p.ToEmbed++
		p.Bytes += len(text)
		p.EstTokens += llm.EstimateTokens(text)
		p.todo = append(p.todo, i)
	}
	return p
}

func (x *Index) byText(digest string) (int, bool) {
	for i := range x.Manifest.Items {
		if x.Manifest.Items[i].TextDigest == digest {
			return i, true
		}
	}
	return 0, false
}

// BuildResult is the outcome of Build. Index holds every vector that exists:
// reused, newly embedded, and, after a budget stop, the batches that finished.
type BuildResult struct {
	Index    *Index
	Plan     *Plan
	Embedded int
	Reused   int
	Calls    int
	Tokens   int
	CostUSD  float64
	// CostKnown is true when every embed call reported a price.
	CostKnown bool
	// Missing lists the items that have no vector (withheld, or not reached after an error).
	Missing []string
	// Err is why the build stopped early (the budget, the provider); Index is still the partial result.
	Err error
}

// buildRow is one vector with its manifest row.
type buildRow struct {
	item ManifestItem
	vec  []float32
}

// builder holds the state of one Build.
type builder struct {
	items []Item
	o     *BuildOptions
	m     Manifest
	rows  map[int]buildRow // by item index
	res   *BuildResult
}

// Build embeds what changed and returns the new index. It never writes: the
// caller writes Index (also when Err is set, so finished batches are kept).
func Build(ctx context.Context, items []Item, o *BuildOptions) (*BuildResult, error) {
	if o.Embedder == nil {
		return nil, oops.Errorf("no embedding provider is configured")
	}
	plan := PlanBuild(items, o)
	old, _ := o.reusable()
	cfg := o.Config.Resolved()
	b := &builder{items: items, o: o, rows: map[int]buildRow{}, res: &BuildResult{Plan: plan, CostKnown: true}}
	b.m = Manifest{
		DocTemplateVersion: DocTemplateVersion, Provider: o.Embedder.Fingerprint(), Model: o.Embedder.Model(),
		DType: cfg.DType, Fields: cfg.Fields, IndexBody: cfg.IndexBody,
	}
	if cfg.IndexBody {
		b.m.BodyChars = cfg.BodyChars
	}
	if old != nil {
		b.m.Dims = old.Manifest.Dims
	}
	b.reuse(old)
	batch := o.BatchSize
	if batch <= 0 {
		batch = cfg.BatchSize
	}
	for start := 0; start < len(plan.todo) && b.res.Err == nil; start += batch {
		if err := ctx.Err(); err != nil {
			b.res.Err = err
			break
		}
		if err := b.embed(ctx, plan.todo[start:min(start+batch, len(plan.todo))]); err != nil {
			return nil, err
		}
	}
	return b.finish()
}

// reuse takes the vectors of unchanged skills from the old index: by id and text
// digest, or by text digest alone for a renamed skill.
func (b *builder) reuse(old *Index) {
	if old == nil {
		return
	}
	for i := range b.items {
		it := &b.items[i]
		if b.o.Only[it.ID] {
			continue
		}
		digest := TextDigest(EmbedText(it, b.o.Config))
		src := old.Row(it.Domain, it.ID)
		if src < 0 || old.Manifest.Items[src].TextDigest != digest {
			var ok bool
			if src, ok = old.byText(digest); !ok {
				continue
			}
		}
		b.rows[i] = buildRow{ManifestItem{ID: it.ID, Domain: it.Domain, ItemDigest: it.Digest, TextDigest: digest}, old.Vector(src)}
	}
	b.res.Reused = len(b.rows)
}

// embed sends one batch. A provider error ends the build early and is kept in
// Err with what finished; a malformed answer aborts the whole build.
func (b *builder) embed(ctx context.Context, chunk []int) error {
	texts := make([]string, len(chunk))
	for i, idx := range chunk {
		texts[i] = EmbedText(&b.items[idx], b.o.Config)
	}
	emb, err := b.o.Embedder.Embed(ctx, texts)
	if err != nil {
		b.res.Err = err
		return nil
	}
	b.res.Calls++
	b.res.Tokens += emb.Tokens
	b.res.CostUSD += emb.CostUSD
	b.res.CostKnown = b.res.CostKnown && emb.CostKnown
	if len(emb.Vectors) != len(chunk) {
		return oops.Errorf("the embedder returned %d vectors for %d texts", len(emb.Vectors), len(chunk))
	}
	for i, idx := range chunk {
		v := emb.Vectors[i]
		if b.m.Dims == 0 {
			b.m.Dims = len(v)
		}
		if len(v) != b.m.Dims {
			return oops.Errorf("the embedder returned %d dimensions for %q, earlier vectors have %d: the model changed mid-build, nothing was written", len(v), b.items[idx].ID, b.m.Dims)
		}
		if err := finite(v); err != nil {
			return oops.Wrapf(err, "the embedder returned a bad vector for %q", b.items[idx].ID)
		}
		it := &b.items[idx]
		b.rows[idx] = buildRow{ManifestItem{ID: it.ID, Domain: it.Domain, ItemDigest: it.Digest, TextDigest: TextDigest(texts[i])}, v}
		b.res.Embedded++
	}
	return nil
}

// finish assembles the index from every vector there is.
func (b *builder) finish() (*BuildResult, error) {
	idxs := make([]int, 0, len(b.rows))
	for i := range b.rows {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	mi := make([]ManifestItem, len(idxs))
	vecs := make([][]float32, len(idxs))
	for k, i := range idxs {
		mi[k], vecs[k] = b.rows[i].item, b.rows[i].vec
	}
	for i := range b.items {
		if _, ok := b.rows[i]; !ok {
			b.res.Missing = append(b.res.Missing, b.items[i].ID)
		}
	}
	if len(mi) == 0 {
		if b.res.Err != nil {
			return b.res, nil
		}
		return nil, oops.Errorf("nothing to index: no skill has text that can be embedded")
	}
	x, err := NewIndex(b.m, mi, vecs)
	if err != nil {
		return nil, err
	}
	if err := x.Quantize(); err != nil {
		return nil, err
	}
	b.res.Index = x
	return b.res, nil
}

// ErrLocked means another `search index` run holds the index directory.
var ErrLocked = errors.New("another 'search index' run holds index.lock")

// lockHeartbeat is how often a held lock refreshes its modification time, so a
// build that outlives staleLockAge is not taken over while it still runs.
const lockHeartbeat = staleLockAge / 5

// Lock takes the advisory lock of an index directory (clock nil is the wall
// clock). The returned function releases it, and removes the file only while it
// is still this run's. A held lock keeps its modification time fresh; one that
// has not been touched for ten minutes belongs to a dead run and is taken over.
func Lock(dir string, clock ambient.Clock) (release func(), err error) {
	return lockWith(dir, clock, lockHeartbeat)
}

func lockWith(dir string, clock ambient.Clock, heartbeat time.Duration) (release func(), err error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, oops.Wrapf(err, "create the search index directory")
	}
	path := filepath.Join(dir, lockFile)
	token := fmt.Sprintf("%d %d\n", os.Getpid(), clock.Now().UnixNano())
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // project-controlled path
		if err == nil {
			_, _ = f.WriteString(token) //nolint:errcheck // informational
			_ = f.Close()               //nolint:errcheck // informational
			return holdLock(path, token, clock, heartbeat), nil
		}
		if !os.IsExist(err) {
			return nil, oops.Wrapf(err, "lock the search index")
		}
		info, statErr := os.Stat(path)
		if statErr == nil && clock.Now().Sub(info.ModTime()) < staleLockAge {
			return nil, oops.Hint("Wait for it to finish, or remove " + path + " if no run is active").Wrap(ErrLocked)
		}
		_ = os.Remove(path) //nolint:errcheck // a stale lock; the retry reports a real failure
	}
	return nil, ErrLocked
}

// holdLock keeps path fresh until the returned release runs.
func holdLock(path, token string, clock ambient.Clock, heartbeat time.Duration) func() {
	owned := func() bool {
		b, err := os.ReadFile(path) //nolint:gosec // project-controlled path
		return err == nil && string(b) == token
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if owned() {
					now := clock.Now()
					_ = os.Chtimes(path, now, now) //nolint:errcheck // the next beat retries
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
			if owned() {
				_ = os.Remove(path) //nolint:errcheck // best effort
			}
		})
	}
}
