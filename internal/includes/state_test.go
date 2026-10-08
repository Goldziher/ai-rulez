package includes

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// TestResolutionStateIsPerLoad guards #290 item 2: what a load resolved must be
// kept on that load's config, so two loads in one process — two projects, or a
// refresh of one while another resolves — never share or clear each other's
// records. The state used to be a process-wide map that every refresh wiped.
func TestResolutionStateIsPerLoad(t *testing.T) {
	// Arrange: two loads, each resolving a same-named source at its own commit.
	a := &config.Config{BaseDir: "/proj-a"}
	b := &config.Config{BaseDir: "/proj-b"}

	// Act: resolve both in parallel.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		st := stateFor(a)
		st.record("/proj-a", lockfile.KindInclude, "kb", observed{commit: "aaaa", digest: "da"})
		st.recordTag("/proj-a", lockfile.KindInclude, "kb", tagInfo{tag: "v1"})
	}()
	go func() {
		defer wg.Done()
		st := stateFor(b)
		st.record("/proj-b", lockfile.KindInclude, "kb", observed{commit: "bbbb", digest: "db"})
	}()
	wg.Wait()

	// Assert: each load kept its own records; neither leaked into the other.
	assert.NotSame(t, stateFor(a), stateFor(b), "each load owns its resolution state")
	assert.Equal(t, "aaaa", ObservedCommit(a, lockfile.KindInclude, "kb"))
	assert.Equal(t, "bbbb", ObservedCommit(b, lockfile.KindInclude, "kb"))

	tag, ok := stateFor(a).tagFor("/proj-a", lockfile.KindInclude, "kb")
	assert.True(t, ok)
	assert.Equal(t, "v1", tag.tag)
	assert.Empty(t, stateFor(b).tags, "a's tag resolution did not leak into b")
}
