package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelevantIndexChange(t *testing.T) {
	tests := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{rel: "AGENTS.md", want: true},
		{rel: "pkg/api/CLAUDE.md", want: true},
		{rel: "pkg/GEMINI.md", want: true},
		{rel: ".github/copilot-instructions.md", want: true},
		{rel: ".claude/skills/x/SKILL.md", want: true},
		{rel: "pkg/.claude/rules/style.md", want: true},
		{rel: ".cursor/rules/ui.mdc", want: true},
		{rel: "pkg/skills/ui/references/notes.md", want: true},
		{rel: "pkg/new", isDir: true, want: true},
		{rel: "src/main.go", want: false},
		{rel: "README.md", want: false},
		{rel: "docs/guide.md", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			assert.Equal(t, tt.want, relevantIndexChange(tt.rel, tt.isDir))
		})
	}
}

// Windows reports a write on a directory whenever a file in it changes, so a
// directory event counts only when the directory can hold indexed content.
func TestDirMayChangeIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/main.go", "package main\n")
	writeFile(t, root, "pkg/new/deep/AGENTS.md", "# new\n")
	writeFile(t, root, "pkg/tool/.claude/rules/r.md", "rule\n")
	writeFile(t, root, "pkg/dep/vendor/x/AGENTS.md", "# vendored\n")
	writeFile(t, root, ".claude/skills/x/notes.txt", "notes\n")
	ignore := indexWatchIgnore(root, nil, nil)

	tests := []struct {
		rel  string
		want bool
	}{
		{rel: "src", want: false},
		{rel: "pkg/new", want: true},
		{rel: "pkg/tool", want: true},
		{rel: "pkg/dep", want: false},
		{rel: ".claude/skills/x", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			assert.Equal(t, tt.want, dirMayChangeIndex(filepath.Join(root, filepath.FromSlash(tt.rel)), tt.rel, ignore))
		})
	}
}

// watchedServer starts a discovering server whose only reload trigger is the
// file watcher: the poll interval is an hour.
func watchedServer(t *testing.T, root string, mutate ...func(*ServeSetup)) (*Server, *sdkmcp.ClientSession, *atomic.Int32) {
	t.Helper()
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", CacheDir: t.TempDir(), PollInterval: time.Hour, IndexWatchDebounce: 20 * time.Millisecond}
	for _, m := range mutate {
		m(setup)
	}
	srv, err := setup.NewServer(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.Watch(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	var changed atomic.Int32
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, &sdkmcp.ClientOptions{
		ResourceListChangedHandler: func(context.Context, *sdkmcp.ResourceListChangedRequest) { changed.Add(1) },
	})
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	if !setup.NoWatch {
		awaitIndexWatch(t, root, session)
		time.Sleep(3 * setup.IndexWatchDebounce) // let the announcements of the probe settle
		changed.Store(0)
	}
	return srv, session, &changed
}

// awaitIndexWatch returns once the file watcher is armed: Watch starts in its own goroutine, so a change
// made at once after the server is up could be missed. A probe instruction file appears, then goes.
func awaitIndexWatch(t *testing.T, root string, session *sdkmcp.ClientSession) {
	t.Helper()
	const probe = "zz-watch-probe"
	// A change made before the watcher is armed is lost for good, so the probe file is written again on
	// every poll until the index shows it.
	tick := 0
	eventually(t, "the watcher sees the probe file", func() bool {
		tick++
		writeFile(t, root, probe+"/AGENTS.md", fmt.Sprintf("# probe %d\n", tick))
		return contextCount(t, session, probe) == 1
	})
	require.NoError(t, os.RemoveAll(filepath.Join(root, probe)))
	eventually(t, "the watcher sees the probe go", func() bool { return contextCount(t, session, probe) == 0 })
}

// settled returns the counter once it has stopped moving: the watcher's own
// set-up (the probe file appearing and going, which on some platforms arrives as
// several events) can still rebuild after the server is up, and a test that
// asserts no rebuild has to count from after the last of those.
func settled(t *testing.T, counter *atomic.Int32) int32 {
	t.Helper()
	const quiet = 300 * time.Millisecond
	last, since := counter.Load(), time.Now()
	require.Eventually(t, func() bool {
		if now := counter.Load(); now != last {
			last, since = now, time.Now()
		}
		return time.Since(since) >= quiet
	}, 10*time.Second, 25*time.Millisecond, "the watcher settles")
	return last
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 10*time.Second, 25*time.Millisecond, what)
}

func contextCount(t *testing.T, s *sdkmcp.ClientSession, scope string) int {
	t.Helper()
	out, isErr := toolJSON(t, s, "find_context", map[string]any{"scope": scope})
	if isErr {
		return 0
	}
	results, _ := out["results"].([]any)
	return len(results)
}

func TestIndexWatch_ANewNestedInstructionFileIsPickedUpAndAnnounced(t *testing.T) {
	root := monorepoProject(t)
	_, session, changed := watchedServer(t, root)
	require.Zero(t, contextCount(t, session, "pkg/new"))

	writeFile(t, root, "pkg/new/AGENTS.md", "# New package\n\nBe careful.\n")

	eventually(t, "the new AGENTS.md is indexed", func() bool { return contextCount(t, session, "pkg/new") == 1 })
	eventually(t, "resources/list_changed is sent", func() bool { return changed.Load() > 0 })
}

func TestIndexWatch_ANewSkillDirectoryIsServedWithoutARestart(t *testing.T) {
	root := monorepoProject(t)
	srv, _, _ := watchedServer(t, root)
	_, served := srv.Catalog().Lookup("fresh")
	require.False(t, served)

	writeFile(t, root, "pkg/data/.claude/skills/fresh/SKILL.md", "---\nname: fresh\ndescription: A skill added while the server runs\n---\n\n# fresh\n")

	eventually(t, "the new skill is served", func() bool {
		_, ok := srv.Catalog().Lookup("fresh")
		return ok
	})
}

func TestIndexWatch_ARemovedInstructionFileLeavesTheIndex(t *testing.T) {
	root := monorepoProject(t)
	_, session, _ := watchedServer(t, root)
	before, _ := toolJSON(t, session, "find_context", map[string]any{"scope": "pkg/api", "kind": "instruction"})
	require.Len(t, before["results"], 1)

	require.NoError(t, os.Remove(filepath.Join(root, "pkg", "api", "AGENTS.md")))

	eventually(t, "the file is gone from the index", func() bool {
		out, _ := toolJSON(t, session, "find_context", map[string]any{"scope": "pkg/api", "kind": "instruction"})
		results, _ := out["results"].([]any)
		return len(results) == 0
	})
}

func TestIndexWatch_AnUnrelatedFileChangeDoesNotRebuild(t *testing.T) {
	root := monorepoProject(t)
	writeFile(t, root, "src/other.go", "package main\n") // the directory exists before the watch starts
	var rebuilds atomic.Int32
	watchedServer(t, root, func(s *ServeSetup) { s.afterLoad = func() { rebuilds.Add(1) } })
	base := settled(t, &rebuilds)

	writeFile(t, root, "src/main.go", "package main\n")
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, base, rebuilds.Load())
}

func TestIndexWatch_NoWatchTurnsItOff(t *testing.T) {
	root := monorepoProject(t)
	srv, session, _ := watchedServer(t, root, func(s *ServeSetup) { s.NoWatch = true })

	writeFile(t, root, "pkg/new/AGENTS.md", "# New package\n")
	time.Sleep(500 * time.Millisecond)

	assert.Zero(t, contextCount(t, session, "pkg/new"))
	assert.Nil(t, srv.serve.opts.IndexWatch)
}
