package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// warnLog collects the warnings of one run.
type warnLog struct {
	logger.Logger
	mu   sync.Mutex
	warn []string
}

func (r *warnLog) Warn(msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warn = append(r.warn, msg)
}

// concurrentProject is a project whose rules differ per index, so a leak between
// two runs shows in the output. A manual rule makes every run raise the
// downgrade warning, which the collector keeps per run.
func concurrentProject(t *testing.T, i int) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml":      fmt.Sprintf("version = \"4.0\"\nname = \"p%d\"\npresets = [\"claude\", \"cursor\", \"codex\"]\ngitignore = true\n", i),
		".ai-rulez/rules/own.md":     fmt.Sprintf("---\npriority: high\n---\n# Rule %d\n\nOnly project %d.\n", i, i),
		".ai-rulez/rules/manual.md":  "---\nactivation: manual\n---\n# Manual\n\nBody.\n",
		".ai-rulez/context/about.md": fmt.Sprintf("# About %d\n", i),
	}
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func generateWithLogger(t *testing.T, dir string, rec *warnLog) error {
	t.Helper()
	cfg, err := config.LoadConfig(t.Context(), dir, config.WithHost(ambient.Host{
		Env: ambient.MapEnv{Home: t.TempDir()}, Log: rec,
	}))
	if err != nil {
		return err //nolint:wrapcheck // test helper
	}
	return NewGenerator(cfg).Generate("")
}

func TestConcurrentGenerationsOnDifferentProjectsShareNothing(t *testing.T) {
	// Arrange: the expected output of each project, generated one at a time.
	const projects = 6
	want := make([]map[string]string, projects)
	wantWarn := make([][]string, projects)
	for i := range projects {
		dir := concurrentProject(t, i)
		rec := &warnLog{Logger: logger.Discard()}
		require.NoError(t, generateWithLogger(t, dir, rec))
		want[i], wantWarn[i] = planTree(t, dir), rec.warn
	}
	require.NotEmpty(t, wantWarn[0], "the manual rule must raise a downgrade warning")

	// Act: the same projects, generated concurrently and repeatedly in one process.
	dirs := make([]string, projects)
	recs := make([]*warnLog, projects)
	for i := range projects {
		dirs[i] = concurrentProject(t, i)
		recs[i] = &warnLog{Logger: logger.Discard()}
	}
	errs := make([]error, projects)
	var wg sync.WaitGroup
	for i := range projects {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = generateWithLogger(t, dirs[i], recs[i])
		}()
	}
	wg.Wait()

	// Assert: every run wrote exactly what it wrote alone and warned about its own project only.
	for i := range projects {
		require.NoError(t, errs[i], "project %d", i)
		assert.Equal(t, want[i], planTree(t, dirs[i]), "project %d output", i)
		assert.Equal(t, wantWarn[i], recs[i].warn, "project %d warnings", i)
		for _, w := range recs[i].warn {
			assert.False(t, strings.Contains(w, fmt.Sprintf("p%d", (i+1)%projects)), "project %d saw another project's warning %q", i, w)
		}
	}
}
