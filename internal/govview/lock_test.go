package govview

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

func TestFilterChanges(t *testing.T) {
	all := func() *contentlock.Diff {
		return &contentlock.Diff{Changes: []contentlock.Change{
			{Scope: contentlock.ScopeSource, Kind: "rule", ID: "a"},
			{Scope: contentlock.ScopeOutput, Path: "CLAUDE.md"},
			{Scope: contentlock.ScopeRemote, Kind: "include", ID: "shared"},
			{Scope: contentlock.ScopeRemote, Kind: "source", ID: "skills"},
			{Scope: contentlock.ScopeServed, Kind: "served", ID: "deploy"},
		}}
	}
	tests := []struct {
		kind    string
		want    int
		wantErr bool
	}{
		{"", 5, false},
		{"content", 2, false},
		{"include", 1, false},
		{"source", 1, false},
		{"served", 1, false},
		{"skill", 0, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			// Arrange
			diff := all()

			// Act
			err := FilterChanges(diff, tt.kind)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, diff.Changes, tt.want)
		})
	}
}

func TestLoadWithCacheFallback_NamesTheRetryFailure(t *testing.T) {
	calls := 0
	load := func(opts ...config.LoadOption) (*config.Config, error) {
		calls++
		if len(opts) == 0 {
			return nil, includes.ErrNotCached
		}
		return nil, errors.New("config.toml: bad syntax")
	}

	cfg, skipped, err := LoadWithCacheFallback(load)

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.False(t, skipped)
	assert.Equal(t, 2, calls)
	assert.ErrorIs(t, err, includes.ErrNotCached, "the first error stays the cause")
	assert.Contains(t, err.Error(), "bad syntax", "the retry failure is not hidden")
}

// With [lock] enforce off the resolver only warns about an uncached include, so
// the first load succeeds; the fallback must still notice and skip remote
// content instead of letting a later step re-resolve (and re-warn) it.
func TestLoadWithCacheFallback_SkipsRemoteWhenAnIncludeIsNotCached(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AI_RULEZ_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDir:   dir,
		ConfigDir: filepath.Join(dir, ".ai-rulez"),
		Includes:  []config.IncludeConfig{{Name: "shared", Source: "https://github.com/example/rules.git"}},
	}
	require.NotEmpty(t, includes.NotCached(cfg), "precondition: the include is not cached")
	var calls [][]config.LoadOption
	load := func(opts ...config.LoadOption) (*config.Config, error) {
		calls = append(calls, opts)
		return cfg, nil
	}

	// Act
	got, skipped, err := LoadWithCacheFallback(load)

	// Assert
	require.NoError(t, err)
	assert.Same(t, cfg, got)
	assert.True(t, skipped)
	require.Len(t, calls, 2)
	assert.Len(t, calls[1], 1, "the retry loads without remote includes")
}
