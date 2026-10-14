package agentssource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/utils"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
)

const (
	// nestedMaxDepth is how many directories below the project a nested
	// AGENTS.md is looked for.
	nestedMaxDepth = 8
	// nestedMaxEntries bounds the walk: a tree larger than this is read no further.
	nestedMaxEntries = 100000
	// nestedPrefix starts the name of a nested AGENTS.md item: agents-<dir slug>.
	nestedPrefix = "agents-"
	// nestedUnnamed is the slug utils.SanitizeName gives a name with nothing left.
	nestedUnnamed = "unnamed"
	// nestedHashBytes is how many bytes of the path hash are kept in a name (8 hex digits).
	nestedHashBytes = 4
)

// nestedFile is a nested AGENTS.md found by the walk.
type nestedFile struct {
	dir string // slash-separated, relative to the project
}

// findNested walks the project for AGENTS.md files in subdirectories. It does
// not follow symlinks, skips the directories every recursive walk skips
// (hidden ones, dependency and build trees), stops at a directory that is a
// project of its own (it owns its AGENTS.md), and is bounded in depth and size.
// The second result is false when the size bound cut the walk short.
func findNested(fsys fs.FS) (found []nestedFile, complete bool) {
	return nestedWalker{maxEntries: nestedMaxEntries}.find(fsys)
}

// NestedPaths lists the nested AGENTS.md files of the project at base (the ones
// the agents source reads as scoped context) as paths joined to base, in walk
// order. It is the same walk as Read's, without
// the git ignore filter: a watcher can watch a file git ignores at no cost.
func NestedPaths(base string) []string {
	found, _ := findNested(os.DirFS(base))
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, filepath.Join(base, filepath.FromSlash(f.dir), config.AgentsMDFilename))
	}
	return out
}

// nestedWalker is the walk findNested runs, with the size bound as a field so a
// test can lower it.
type nestedWalker struct {
	maxEntries int
}

func (w nestedWalker) find(fsys fs.FS) (found []nestedFile, complete bool) {
	entries := 0
	complete = true
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // best effort, read-only
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil //nolint:nilerr // an unreadable entry is skipped
		}
		entries++
		if entries > w.maxEntries {
			complete = false
			return fs.SkipAll
		}
		if d.IsDir() {
			if p == "." {
				return nil
			}
			if walkutil.ShouldSkipDir(d.Name()) || strings.Count(p, "/") >= nestedMaxDepth || ownsConfig(fsys, p) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == config.AgentsMDFilename && d.Type().IsRegular() && p != config.AgentsMDFilename {
			found = append(found, nestedFile{dir: path.Dir(p)})
		}
		return nil
	})
	return found, complete
}

// ownsConfig reports whether the directory dir holds its own ai-rulez
// configuration, which makes it a project and not a scope of this one.
func ownsConfig(fsys fs.FS, dir string) bool {
	for _, rel := range []string{
		config.AgentsDirName + "/" + config.AgentsConfigFilename,
		".ai-rulez/config.toml",
		".config/ai-rulez/config.toml",
	} {
		if info, err := fs.Lstat(fsys, path.Join(dir, rel)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// readNested reads the nested AGENTS.md files as context scoped to their
// directory (`<dir>/**`, the scope the generator gives any glob-scoped context).
// Files git ignores, generated files, empty files and files that only point
// elsewhere are left out, as for the root. taken holds the names in use; it is updated.
func readNested(ctx context.Context, cfg *config.Config, fsys fs.FS, taken map[string]bool) []config.ContentFile {
	return readNestedWith(ctx, cfg, fsys, taken, nestedWalker{maxEntries: nestedMaxEntries})
}

// readNestedWith is readNested with the walk to use.
func readNestedWith(ctx context.Context, cfg *config.Config, fsys fs.FS, taken map[string]bool, w nestedWalker) []config.ContentFile {
	found, complete := w.find(fsys)
	if !complete {
		cfg.Warn("stopped looking for nested AGENTS.md files: the project has more than a bounded number of entries",
			"source", config.SourceAgents)
	}
	if len(found) == 0 {
		return nil
	}
	slugs := nestedSlugs(found)
	ignored := ignoredNested(ctx, cfg, fsys, found)
	var out []config.ContentFile
	for _, f := range found {
		file := f.dir + "/" + config.AgentsMDFilename
		if ignored[file] {
			continue
		}
		sub, err := fs.Sub(fsys, f.dir)
		if err != nil {
			continue
		}
		md := importer.ReadAgentsMD(sub, importer.Options{})
		for _, note := range md.Notes {
			cfg.Warn(file+" "+note.Reason, "source", config.SourceAgents)
		}
		for i := range md.Items {
			md.Items[i].Name = slugs[f.dir]
			name := itemNames(md.Items[i:i+1], taken)[0]
			out = append(out, contextFile(cfg, name, &md.Items[i], file, f.dir))
		}
	}
	return out
}

// ignoredNested asks git which of the nested files .gitignore (and the other
// ignore sources) excludes. Ignore rules are git's: outside a repository nothing
// is ignored, and a root .gitignore there is reported as not applied.
func ignoredNested(ctx context.Context, cfg *config.Config, fsys fs.FS, found []nestedFile) map[string]bool {
	paths := make([]string, 0, len(found))
	for _, f := range found {
		paths = append(paths, f.dir+"/"+config.AgentsMDFilename)
	}
	git := gitutil.New(runner.FromContext(ctx))
	if !git.IsRepoContext(ctx, cfg.BaseDir) {
		if _, err := fs.Stat(fsys, ".gitignore"); err == nil {
			cfg.Warn(".gitignore is not applied to nested AGENTS.md files: the project is not a git repository, so every one is read",
				"source", config.SourceAgents)
		}
		return nil
	}
	ignored, err := git.IgnoredAmongContext(ctx, cfg.BaseDir, paths)
	if err != nil {
		cfg.Warn("could not ask git which nested AGENTS.md files are ignored; reading all of them",
			"source", config.SourceAgents, "error", err.Error())
		return nil
	}
	return ignored
}

// nestedSlugs names every nested directory found, before .gitignore is applied,
// so a name does not change with the ignore state or with unrelated directories:
// agents-<dir slug>, plus a short hash of the slash path when another directory
// sanitizes to the same slug or the slug is empty (a/b, a-b and "a b" all slug to a-b).
func nestedSlugs(found []nestedFile) map[string]string {
	slugOf := make(map[string]string, len(found))
	uses := map[string]int{}
	for _, f := range found {
		slug := utils.SanitizeName(strings.ReplaceAll(f.dir, "/", "-"))
		slugOf[f.dir] = slug
		uses[slug]++
	}
	out := make(map[string]string, len(found))
	for _, f := range found {
		slug := slugOf[f.dir]
		if uses[slug] > 1 || slug == nestedUnnamed {
			sum := sha256.Sum256([]byte(f.dir))
			slug += "-" + hex.EncodeToString(sum[:nestedHashBytes])
		}
		out[f.dir] = nestedPrefix + slug
	}
	return out
}
