package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// GitignoreSink collects the managed .gitignore patterns of a recursive
// `generate` run so they land in one block at the repository toplevel instead
// of one .gitignore beside every root (#316). Each root adds the patterns it
// manages, expressed relative to itself; the sink prefixes them with the root's
// path so they mean the same from the toplevel. It writes as patterns arrive,
// so a root's machine-local outputs are ignored before that root writes them,
// and rewrites the block once at the end so patterns of a removed root or
// output do not linger.
type GitignoreSink struct {
	mu       sync.Mutex
	toplevel string
	patterns map[string]bool
}

// NewGitignoreSink returns a sink managing toplevel/.gitignore. The caller must
// have established toplevel is the work tree root; an empty value disables it.
func NewGitignoreSink(toplevel string) *GitignoreSink {
	if toplevel == "" {
		return &GitignoreSink{patterns: map[string]bool{}} // inert: Add and Write no-op
	}
	return &GitignoreSink{toplevel: filepath.Clean(toplevel), patterns: map[string]bool{}}
}

// TopLevel is the repository root whose .gitignore the sink manages.
func (s *GitignoreSink) TopLevel() string { return s.toplevel }

// Add records the patterns one root manages, expressed relative to that root,
// and writes them into the toplevel block right away. prefix is the root's path
// relative to the toplevel ("" for the toplevel itself). Recursive roots add
// their patterns concurrently, so the sink serializes them.
func (s *GitignoreSink) Add(log logger.Logger, prefix string, patterns []string) error {
	if s == nil || s.toplevel == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prefixed := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = joinGitignorePrefix(prefix, p)
		if p == "" {
			continue
		}
		s.patterns[p] = true
		prefixed = append(prefixed, p)
	}
	if len(prefixed) == 0 {
		return nil
	}
	sort.Strings(prefixed)
	if err := gitignore.EnsureEntries(log, s.toplevel, prefixed); err != nil {
		return oops.With("path", filepath.Join(s.toplevel, ".gitignore")).Wrapf(err, "write the shared .gitignore block")
	}
	return nil
}

// Collect records the patterns one root would manage, like Add, but writes
// nothing: the read-only half of a recursive `generate --check`.
func (s *GitignoreSink) Collect(prefix string, patterns []string) {
	if s == nil || s.toplevel == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range patterns {
		if p = joinGitignorePrefix(prefix, p); p != "" {
			s.patterns[p] = true
		}
	}
}

// Drift reports whether Write would change the toplevel block, given the
// patterns Collect gathered from every root. The path is relative to the toplevel.
func (s *GitignoreSink) Drift(log logger.Logger) (Drift, bool) {
	if s == nil || s.toplevel == "" || gitignore.IsSymlink(s.toplevel) {
		return Drift{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	patterns := make([]string, 0, len(s.patterns))
	for p := range s.patterns {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	data, err := gitutil.ReadIgnoreFileOrEmpty(log, filepath.Join(s.toplevel, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		return Drift{}, false
	}
	return managedBlockDrift(".gitignore", string(data), dropUserPatterns(patterns, string(data)))
}

// Write rewrites the toplevel managed block to exactly the patterns this run
// collected, dropping what an earlier run left behind (a removed root or
// output) and removing the block when nothing is left. It runs once, after
// every root has been processed.
func (s *GitignoreSink) Write(log logger.Logger) error {
	if s == nil || s.toplevel == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	patterns := make([]string, 0, len(s.patterns))
	for p := range s.patterns {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)

	if gitignore.IsSymlink(s.toplevel) {
		return gitignore.ReplaceViaExclude(log, s.toplevel, patterns) //nolint:wrapcheck // already contextual
	}
	path := filepath.Join(s.toplevel, ".gitignore")
	data, err := gitutil.ReadIgnoreFileOrEmpty(log, path)
	if err != nil && !os.IsNotExist(err) {
		return oops.With("path", path).Wrapf(err, "read .gitignore")
	}
	content := string(data)

	kept := dropUserPatterns(patterns, content)
	if len(kept) == 0 {
		if !strings.Contains(content, gitignore.BeginMarker) && !strings.Contains(content, gitignore.OldHeader) {
			return nil
		}
		return dropManagedBlock(path, content) //nolint:wrapcheck // already contextual
	}
	if err := gitutil.WriteFileAtomic(path, []byte(gitignoreWithBlock(content, kept)), 0o644); err != nil {
		return oops.With("path", path).Wrapf(err, "write .gitignore")
	}
	return nil
}

// joinGitignorePrefix makes a root-relative pattern relative to the toplevel.
// A pattern already anchored at the root ("/x") keeps its meaning below the
// prefix.
func joinGitignorePrefix(prefix, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" || prefix == "." {
		return pattern
	}
	return prefix + "/" + strings.TrimPrefix(pattern, "/")
}

// managedGitignorePath is the .gitignore file this run manages: the shared
// toplevel block of a recursive run, else the file beside the config.
func (g *Generator) managedGitignorePath() string {
	if g.gitignoreSink != nil {
		return filepath.Join(g.gitignoreSink.TopLevel(), ".gitignore")
	}
	return filepath.Join(g.config.BaseDir, ".gitignore")
}

// SetGitignoreSink makes this generator collect its managed patterns into sink
// instead of writing a .gitignore beside the config. prefix is the root's path
// relative to the sink's toplevel ("" for the toplevel root itself).
func (g *Generator) SetGitignoreSink(sink *GitignoreSink, prefix string) {
	g.gitignoreSink, g.gitignorePrefix = sink, prefix
}
