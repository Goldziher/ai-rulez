package includes

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestFetchMemo_RealFetchHappensOncePerSourceEvenConcurrently(t *testing.T) {
	// Arrange
	memo := &fetchMemo{}
	var real, again atomic.Int32
	var wg sync.WaitGroup

	// Act
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tree, err := memo.fetchTree("same-source",
				func() (*config.ContentTree, error) { real.Add(1); return &config.ContentTree{}, nil },
				func() (*config.ContentTree, error) { again.Add(1); return &config.ContentTree{}, nil })
			assert.NoError(t, err)
			assert.NotNil(t, tree)
		}()
	}
	wg.Wait()

	// Assert
	assert.EqualValues(t, 1, real.Load(), "the networked fetch runs once")
	assert.EqualValues(t, 19, again.Load(), "everyone else re-reads the cache")
}

func TestFetchMemo_ReplaysFailuresWithoutRetrying(t *testing.T) {
	memo := &fetchMemo{}
	var real, again int

	for i := 0; i < 3; i++ {
		_, err := memo.fetchSkill("s",
			func() (config.ContentFile, error) { real++; return config.ContentFile{}, assert.AnError },
			func() (config.ContentFile, error) { again++; return config.ContentFile{}, nil })
		require.ErrorIs(t, err, assert.AnError)
	}

	assert.Equal(t, 1, real)
	assert.Zero(t, again)
}

func TestMemoFor_IsSharedThroughTheConfig(t *testing.T) {
	first := &config.Config{}
	m1 := memoFor(first)
	second := &config.Config{IncludeMemo: first.IncludeMemo}

	assert.Same(t, m1, memoFor(first), "a config keeps its memo")
	assert.Same(t, m1, memoFor(second), "a baseline load given the memo reuses it")
	assert.NotSame(t, m1, memoFor(&config.Config{}), "unrelated configs do not share")
}

// Two views of one project that share a memo must not share content: editing one
// view's tree (as a renderer or the profile selection might) cannot reach the other.
func TestSharedMemo_GivesEachConsumerItsOwnTree(t *testing.T) {
	// Arrange
	base := t.TempDir()
	rules := filepath.Join(base, "inc", ".ai-rulez", "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "a.md"), []byte("---\npriority: high\n---\n\nbody\n"), 0o600))
	newCfg := func(memo any) *config.Config {
		return &config.Config{
			BaseDir:     base,
			IncludeMemo: memo,
			Content:     &config.ContentTree{Domains: map[string]*config.Domain{}},
			Includes:    []config.IncludeConfig{{Name: "inc", Source: filepath.Join(base, "inc")}},
		}
	}
	first := newCfg(nil)

	// Act
	firstTree, err := NewResolver(base, "").ResolveIncludes(context.Background(), first)
	require.NoError(t, err)
	require.NotEmpty(t, firstTree.Rules)
	firstTree.Rules[0].Content = "MUTATED"
	secondTree, err := NewResolver(base, "").ResolveIncludes(context.Background(), newCfg(first.IncludeMemo))
	require.NoError(t, err)

	// Assert
	require.NotEmpty(t, secondTree.Rules)
	assert.NotEqual(t, "MUTATED", secondTree.Rules[0].Content)
	assert.Contains(t, secondTree.Rules[0].Content, "body")
}
