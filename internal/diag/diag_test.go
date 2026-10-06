package diag

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type recorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recorder) sink(msg string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg+fmt.Sprint(args...))
}

func TestCollector(t *testing.T) {
	tests := []struct {
		name string
		act  func(c *Collector)
		want []string
	}{
		{
			name: "a repeated warning is issued once",
			act:  func(c *Collector) { c.Warn("a"); c.Warn("a"); c.Warn("b") },
			want: []string{"a", "b"},
		},
		{
			name: "raise is never de-duplicated",
			act:  func(c *Collector) { c.Raise("a"); c.Raise("a") },
			want: []string{"a", "a"},
		},
		{
			name: "reset forgets what was issued",
			act:  func(c *Collector) { c.Warn("a"); c.Reset(); c.Warn("a") },
			want: []string{"a", "a"},
		},
		{
			name: "once is per namespace and key",
			act: func(c *Collector) {
				c.OnceWarn("scope", "x", "first")
				c.OnceWarn("scope", "x", "again")
				c.OnceWarn("other", "x", "other namespace")
			},
			want: []string{"first", "other namespace"},
		},
		{
			name: "downgrades are aggregated, sorted and cleared by flush",
			act: func(c *Collector) {
				c.RecordDowngrade("rule", "b", "auto")
				c.RecordDowngrade("rule", "a", "manual")
				c.RecordDowngrade("rule", "a", "manual")
				c.Flush()
				c.Flush()
			},
			want: []string{"2 rules/context items use an activation the target tool cannot express; they are loaded always:" +
				"itemsrule a, rule b"},
		},
		{
			name: "silence drops warnings and keeps them eligible for later",
			act: func(c *Collector) {
				restore := c.Silence()
				c.Warn("quiet")
				restore()
				c.Warn("quiet")
			},
			want: []string{"quiet"},
		},
		{
			name: "set sink redirects and restores",
			act: func(c *Collector) {
				restore := c.SetSink(func(string, ...any) {})
				c.Raise("dropped")
				restore()
				c.Raise("kept")
			},
			want: []string{"kept"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rec := &recorder{}
			c := New(rec.sink)

			// Act
			tt.act(c)

			// Assert
			assert.Equal(t, tt.want, rec.msgs)
		})
	}
}

func TestCollectorsOfDifferentRunsShareNothing(t *testing.T) {
	// Arrange
	var recs [2]recorder
	cs := [2]*Collector{New(recs[0].sink), New(recs[1].sink)}

	// Act: the same warning in both runs, one run resetting while the other renders.
	var wg sync.WaitGroup
	for i := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				cs[i].Reset()
				cs[i].Warn("same")
				cs[i].RecordDowngrade("rule", strings.Repeat("x", i+1), "auto")
				cs[i].Flush()
			}
		}()
	}
	wg.Wait()

	// Assert: each run saw exactly its own 200 warnings and 200 summaries.
	for i := range recs {
		assert.Len(t, recs[i].msgs, 400, "run %d", i)
	}
}

func TestNilCollectorStandsForTheProcessDefault(t *testing.T) {
	// Arrange
	rec := &recorder{}
	defer SetDefaultSink(rec.sink)()
	var c *Collector

	// Act
	c.Warn("a")
	c.Warn("a")
	c.Raise("b")
	c.RecordDowngrade("rule", "x", "auto")
	c.Flush()
	assert.True(t, c.Once("ns", "k"))
	assert.False(t, c.Once("ns", "k"))
	c.Reset()
	assert.True(t, c.Once("ns", "k"))
	restore := c.Silence()
	c.Raise("quiet")
	restore()

	// Assert
	assert.Equal(t, []string{"a", "b", "1 rules/context items use an activation the target tool cannot express; they are loaded always:itemsrule x"}, rec.msgs)
}

func TestStickyKeysSurviveAResetAndASinkChange(t *testing.T) {
	// Arrange
	c := New(func(string, ...any) {})

	// Act
	first := c.Sticky("k")
	c.Reset()
	restore := c.SetSink(func(string, ...any) {})
	second := c.Sticky("k")
	restore()

	// Assert: a clean plan and the clean that follows it raise an advice once.
	assert.True(t, first)
	assert.False(t, second)
	assert.True(t, c.Once("ns", "k"), "Once is cleared by Reset, Sticky is a separate set")
}

func TestCollectorWithoutASinkUsesTheDefaultSink(t *testing.T) {
	// Arrange
	rec := &recorder{}
	defer SetDefaultSink(rec.sink)()

	// Act
	New(nil).Raise("hello")

	// Assert
	assert.Equal(t, []string{"hello"}, rec.msgs)
}
