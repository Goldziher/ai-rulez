package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/watch"
)

// Live refresh of the repository index. While a discovering server runs, a
// debounced file watcher (internal/watch, fsnotify) over the project tells it
// when something that can change the index appears, changes or goes: the server
// then rebuilds its catalog, which re-reads only the files whose size or
// modification time changed (the index cache) and re-scans only content it has
// no clean verdict for, and announces resources/list_changed so a client asks
// again. The poll of the skill roots and the configuration stays: it covers what
// the watcher cannot (a limit on watched directories, a network file system).

const (
	// uriIndex is the resource that changes whenever the index does; its
	// registration is what makes the SDK send resources/list_changed.
	uriIndex = resourceScheme + "index"

	metaIndexGeneration = "ai-rulez/index-generation"
)

// IndexWatch tells Server.Watch to follow the project tree with a file watcher.
type IndexWatch struct {
	// Root is the project root to watch.
	Root string
	// Skip lists project-relative directories whose changes are not index
	// changes (the configuration directory, which the poll covers).
	Skip []string
	// Debounce is the quiet period after the last change before a refresh
	// (0 selects 300 ms).
	Debounce time.Duration
}

// isConventionFile reports whether a file name is an instruction file or a skill.
func isConventionFile(name string) bool {
	switch name {
	case "AGENTS.md", "CLAUDE.md", "GEMINI.md", "copilot-instructions.md", "SKILL.md":
		return true
	}
	return false
}

// isConventionDir reports whether a directory name holds skills, rules, agents
// or commands.
func isConventionDir(name string) bool {
	switch name {
	case ".claude", ".agents", ".codex", ".cursor", ".github", "skills":
		return true
	}
	return false
}

// isIgnoredDir reports whether the watcher never enters a directory: build
// output and dependency trees hold many directories and no convention files.
func isIgnoredDir(name string) bool {
	switch name {
	case "vendor", "target", "dist", "build", "__pycache__", ".venv", ".next", ".gradle", ".tox", ".ai-rulez-cache":
		return true
	}
	return false
}

// relevantIndexChange reports whether a change at a project-relative, slash
// separated path can change the index: a convention file, anything below a
// convention directory, or a directory (which may have come with files in it).
func relevantIndexChange(rel string, isDir bool) bool {
	if isDir {
		return true
	}
	segs := strings.Split(rel, "/")
	if isConventionFile(segs[len(segs)-1]) {
		return true
	}
	for _, seg := range segs[:len(segs)-1] {
		if isConventionDir(seg) {
			return true
		}
	}
	return false
}

// indexWatchIgnore filters the paths the watcher follows.
func indexWatchIgnore(root string, skip []string) func(string) bool {
	return func(p string) bool {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return false
		}
		rel = filepath.ToSlash(rel)
		for _, s := range skip {
			if rel == s || strings.HasPrefix(rel, s+"/") {
				return true
			}
		}
		for _, seg := range strings.Split(rel, "/") {
			if isIgnoredDir(seg) {
				return true
			}
		}
		return false
	}
}

// runIndexWatch watches the project until ctx ends and sends on refresh when an
// index change settles.
func runIndexWatch(ctx context.Context, iw *IndexWatch, log logger.Logger, refresh chan<- struct{}) {
	root := iw.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	debug := func(msg string, kv ...any) { log.Debug(msg, kv...) }
	warn := func(msg string, kv ...any) { log.Warn(msg, kv...) }
	err := watch.Watch(ctx, watch.Options{
		Targets:  func() []watch.Target { return []watch.Target{{Path: root}} },
		Ignore:   indexWatchIgnore(root, iw.Skip),
		Debounce: iw.Debounce,
		Log:      debug,
		Warn:     warn,
		Run: func(_ context.Context, triggers []string) error {
			for _, p := range triggers {
				rel, err := filepath.Rel(root, p)
				if err != nil || strings.HasPrefix(rel, "..") {
					continue
				}
				info, statErr := os.Lstat(p)
				isDir := statErr == nil && info.IsDir()
				if statErr != nil && !strings.Contains(filepath.Base(p), ".") {
					isDir = true // a vanished path without an extension may have been a directory
				}
				if relevantIndexChange(filepath.ToSlash(rel), isDir) {
					select {
					case refresh <- struct{}{}:
					default: // a refresh is already waiting
					}
					return nil
				}
			}
			return nil
		},
	})
	if err != nil {
		log.Warn("Could not watch the repository for changes; the index refreshes on the poll interval only", "error", err.Error())
	}
}

// registerIndexResource registers the index resource of a server that indexed
// the repository.
func (s *Server) registerIndexResource() {
	if s.cat().repository() == nil {
		return
	}
	s.announceIndex()
}

// announceIndex (re-)registers the index resource with a new generation, which
// the SDK turns into one debounced resources/list_changed per client.
func (s *Server) announceIndex() {
	gen := s.indexGen.Add(1)
	s.mcpServer.AddResource(&sdkmcp.Resource{
		URI: uriIndex, Name: "index", Title: "Repository Index", MIMEType: mimeJSON,
		Description: "The state of the repository index: its generation, item and scope counts. Its generation changes, and resources/list_changed is sent, when the index is refreshed.",
		Meta:        sdkmcp.Meta{metaIndexGeneration: gen},
	}, s.readIndexResource)
}

func (s *Server) readIndexResource(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	v := s.cat().repository()
	if v == nil {
		return nil, resourceNotFound(req.Params.URI)
	}
	doc := map[string]any{
		"generation": s.indexGen.Load(), "items": len(v.index.Items()), "scopes": len(v.index.Scopes()),
	}
	if rep := s.cat().Discovery(); rep != nil && rep.Index != nil {
		doc["found"], doc["truncated"], doc["cache"] = rep.Index.Found, rep.Index.Truncated, rep.Index.Cache
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err //nolint:wrapcheck // a fixed shape
	}
	return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{{URI: uriIndex, MIMEType: mimeJSON, Text: string(data)}}}, nil
}

// indexSignature identifies what the index holds, so a refresh that changed
// nothing announces nothing.
func (v *repoView) signature() string {
	if v == nil {
		return ""
	}
	var b strings.Builder
	for _, it := range v.index.Items() {
		b.WriteString(string(it.Kind) + "\x00" + it.Scope + "\x00" + it.Name + "\x00" + it.Hash + "\x00" + it.Description + "\n")
	}
	return b.String()
}
