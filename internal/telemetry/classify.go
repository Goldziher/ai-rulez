package telemetry

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zeebo/blake3"
)

// Item identifies a loaded instruction file.
type Item struct {
	Kind string
	ID   string
	// Path is relative to the repository root with forward slashes; empty when the
	// file is outside the repository.
	Path string
}

var (
	ruleFilePattern  = regexp.MustCompile(`(?:^|/)\.claude/rules/(.+)\.md$`)
	agentFilePattern = regexp.MustCompile(`(?:^|/)\.claude/agents/(.+)\.md$`)
)

// maxDigestBytes bounds the file a digest is computed over.
const maxDigestBytes = 2 << 20

// ClassifyInstruction maps the file_path of an InstructionsLoaded event to an
// item. A file under .claude/rules/ is a rule whose id is its path below that
// directory without the extension (ai-rulez writes a rule to a file named after
// it, so this is the source rule's name); every other file (CLAUDE.md,
// CLAUDE.local.md, AGENTS.md, an @-included file) is a context item identified
// by its repo-relative path, or by its base name when it lives outside the
// repository (a user-level file: the home directory never appears in an id).
func ClassifyInstruction(root, cwd, filePath string) Item {
	abs := filePath
	if !filepath.IsAbs(abs) {
		base := cwd
		if base == "" {
			base = root
		}
		abs = filepath.Join(base, abs)
	}
	abs = filepath.Clean(abs)
	rel, inside := relativeTo(root, abs)

	probe := filepath.ToSlash(abs)
	if inside {
		probe = rel
	}
	if match := ruleFilePattern.FindStringSubmatch(probe); len(match) > 1 && validID(match[1]) {
		item := Item{Kind: KindRule, ID: match[1]}
		if inside {
			item.Path = rel
		}
		return item
	}
	if inside && validID(rel) {
		return Item{Kind: KindContext, ID: rel, Path: rel}
	}
	return Item{Kind: KindContext, ID: filepath.Base(abs)}
}

func validID(id string) bool { return idPattern.MatchString(id) && !hasDotDot(id) }

// relativeTo returns path relative to root (forward slashes) and whether it is
// inside root. Symlinks are resolved on both sides so /var vs /private/var
// does not make an inside file look outside.
func relativeTo(root, path string) (string, bool) {
	if root == "" {
		return "", false
	}
	for _, pair := range [][2]string{{root, path}, {resolve(root), resolve(path)}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

func resolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	// The file may not exist (yet): resolve its directory.
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	return path
}

// FileDigest returns "blake3:<hex>" of a file inside the repository, "" when it is
// unreadable or large. The content is hashed and discarded.
func FileDigest(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDigestBytes {
		return ""
	}
	data, err := os.ReadFile(path) //nolint:gosec // the instruction file the harness just loaded
	if err != nil {
		return ""
	}
	sum := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(sum[:])
}

// Catalog lists the items ai-rulez generated, from its generated manifest, so a
// report can name the ones never loaded.
type Catalog struct {
	Rules    []string
	Agents   []string
	Contexts []string
}

// LoadCatalog reads <root>/<config dir>/.generated-manifest.json. A missing
// manifest yields an empty catalog and no error: the project may not have been
// generated yet.
func LoadCatalog(root, configDirName string) (*Catalog, error) {
	data, err := os.ReadFile(filepath.Join(root, configDirName, ".generated-manifest.json")) //nolint:gosec // project manifest
	if err != nil {
		if os.IsNotExist(err) {
			return &Catalog{}, nil
		}
		return nil, err
	}
	var manifest struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return CatalogFromFiles(manifest.Files), nil
}

// CatalogFromFiles classifies generated file paths (forward slashes, repo-relative).
func CatalogFromFiles(files []string) *Catalog {
	c := &Catalog{}
	for _, file := range files {
		switch {
		case ruleFilePattern.MatchString(file):
			c.Rules = append(c.Rules, ruleFilePattern.FindStringSubmatch(file)[1])
		case agentFilePattern.MatchString(file):
			c.Agents = append(c.Agents, agentFilePattern.FindStringSubmatch(file)[1])
		case file == "CLAUDE.md" || file == "AGENTS.md" || file == "GEMINI.md" || strings.HasSuffix(file, "/CLAUDE.md") || strings.HasSuffix(file, "/AGENTS.md"):
			c.Contexts = append(c.Contexts, file)
		}
	}
	sort.Strings(c.Rules)
	sort.Strings(c.Agents)
	sort.Strings(c.Contexts)
	return c
}
