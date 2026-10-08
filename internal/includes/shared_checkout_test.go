package includes

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// Two loads of one remote at different commits share a cache directory: each
// fetch must see the checkout of its own commit, not the other's half-replaced one.
func TestConcurrentFetchesOfOneRemoteAtDifferentCommits(t *testing.T) {
	// Arrange
	isolateHome(t)
	remote := t.TempDir()
	testutil.Git(t, remote, "init", "-q", "-b", "main")
	rule := filepath.Join(remote, ".ai-rulez", "rules", "shared.md")
	writeTestFile(t, rule, "# Shared\n\nversion one\n")
	testutil.Git(t, remote, "add", "-A")
	testutil.Git(t, remote, "commit", "-qm", "one")
	first := testutil.Git(t, remote, "rev-parse", "HEAD")
	writeTestFile(t, rule, "# Shared\n\nversion two\n")
	testutil.Git(t, remote, "commit", "-qam", "two")
	second := testutil.Git(t, remote, "rev-parse", "HEAD")
	require.NotEqual(t, first, second)
	base := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	// Act
	const rounds = 2
	type outcome struct {
		err    error
		commit string
		body   string
	}
	results := make([][2]outcome, rounds)
	var wg sync.WaitGroup
	for i := range rounds {
		for j, commit := range []string{first, second} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				src, err := NewGitSource("shared", "file://"+filepath.ToSlash(remote), "", commit, base, nil, "")
				if err != nil {
					results[i][j].err = err
					return
				}
				src.state = newResolutionState()
				tree, err := src.Fetch(ctx)
				if err != nil {
					results[i][j].err = err
					return
				}
				o, _ := src.state.observedFor(base, lockfile.KindInclude, "shared")
				results[i][j] = outcome{commit: o.commit, body: tree.Rules[0].Content}
			}()
		}
	}
	wg.Wait()

	// Assert
	for i, pair := range results {
		for j, want := range []struct{ commit, body string }{{first, "version one"}, {second, "version two"}} {
			require.NoError(t, pair[j].err, "round %d fetch %d", i, j)
			assert.Equal(t, want.commit, pair[j].commit, "round %d fetch %d: the commit the fetch recorded", i, j)
			assert.Contains(t, pair[j].body, want.body, "round %d fetch %d: the content it returned", i, j)
		}
	}
}
