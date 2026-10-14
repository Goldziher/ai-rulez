package lint

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Targets are files a run lints in addition to (or, in a repository without a
// configuration, instead of) the content tree of the configuration: files named
// with --path and --glob or [lint] paths, and the files detected by convention.
// Each file is classified by its name, its place and its content, so a
// marketplace or a skill kept in an unusual directory is still recognized.

// The kinds of a target that is not a content item: a JSON file an analyzer
// reads. The content kinds (kindRule, kindSkill, ...) are shared with the tree.
const (
	targetPlugin      = "plugin"
	targetMarketplace = "marketplace"
	targetHooks       = "hooks"
	targetMCP         = "mcp"
)

// maxTargetFiles bounds how many files one run takes as targets, so a glob such
// as "**" on a huge tree cannot make the run unbounded.
const maxTargetFiles = 20000

// TargetSpec names the files to lint.
type TargetSpec struct {
	// Paths are files or directories: absolute, or relative to Cwd.
	Paths []string
	// Globs match slash paths relative to the repository root.
	Globs []string
	// Detect adds the files that follow a convention (AGENTS.md, SKILL.md,
	// .claude/skills, .cursor/rules, plugin manifests, ...).
	Detect bool
	// Cwd resolves relative Paths; empty uses the tree top.
	Cwd string
	// Under keeps only the files below this directory (slash path relative to the
	// tree top), so each root of a recursive run lints its own files once. Empty
	// keeps every file.
	Under string
	// Generated also collects the targets that carry the generated header, as
	// outputs to lint (Targets.Generated), instead of skipping them.
	Generated bool
}

// Empty reports whether the spec names nothing.
func (s TargetSpec) Empty() bool { return len(s.Paths) == 0 && len(s.Globs) == 0 && !s.Detect }

// TargetFile is one classified target.
type TargetFile struct {
	// Abs is the absolute path, Rel the slash path relative to the tree top.
	Abs, Rel string
	// Kind is a content kind or one of the target kinds above.
	Kind string
	// Content is the parsed file of a content kind; for a skill it carries the
	// resources of the skill directory.
	Content *config.ContentFile
	// MCPKey is the JSON key that holds the servers of an MCP file.
	MCPKey string
}

// Targets is the resolved, classified set.
type Targets struct {
	Files []TargetFile
	// Generated holds the targets that carry the generated header, when the spec
	// asked for them (TargetSpec.Generated).
	Generated []OutputFile
	// Unmatched lists the globs that matched no file, for a notice.
	Unmatched []string
}

// Rels returns the tree-relative paths of the files.
func (t *Targets) Rels() []string {
	if t == nil {
		return nil
	}
	out := make([]string, len(t.Files))
	for i := range t.Files {
		out[i] = t.Files[i].Rel
	}
	return out
}

// ResolveTargets expands spec against the tree: every file must lie inside the
// tree top once symlinks are resolved, an explicit path that does not exist or
// leaves the project is an error, and a directory or glob takes the tracked and
// the untracked, not ignored files below it.
func ResolveTargets(ctx context.Context, tree *Tree, spec TargetSpec) (*Targets, error) {
	res := &Targets{}
	cands := newCandidates(ctx, tree, spec.Detect)
	picked, err := cands.explicit(tree, spec, res)
	if err != nil {
		return nil, err
	}
	rels := picked.sorted(spec.Under)
	if spec.Detect {
		rels = append(rels, cands.conventional(spec.Under, picked)...)
	}
	if len(rels) > maxTargetFiles {
		return nil, fmt.Errorf("%d files match --path, --glob or [lint] paths; the limit is %d: narrow the selection", len(rels), maxTargetFiles)
	}
	for _, rel := range rels {
		if f, ok := loadTarget(ctx, tree, rel, picked[rel]); ok {
			res.Files = append(res.Files, f)
		}
	}
	if spec.Generated {
		res.Generated = generatedTargets(tree, rels)
	}
	return res, nil
}

// generatedTargets returns the markdown files among rels that carry the
// generated header, which loadTarget leaves out as sources.
func generatedTargets(tree *Tree, rels []string) []OutputFile {
	var out []OutputFile
	for _, rel := range rels {
		if _, ok := classifyOutput(rel); !ok {
			continue
		}
		abs, inside := contained(tree, rel)
		if !inside {
			continue
		}
		if data, err := readSmallFile(abs); err == nil && carriesGeneratedBanner(data) {
			out = append(out, OutputFile{Abs: abs, Data: data})
		}
	}
	return out
}

// candidates is the set of files a directory or glob selects from.
type candidates struct {
	rels []string
	set  map[string]bool
}

// explicit expands --path and --glob; a glob that matches nothing is recorded in res.
func (c *candidates) explicit(tree *Tree, spec TargetSpec, res *Targets) (picks, error) {
	picked := picks{}
	for _, p := range spec.Paths {
		rels, err := c.expandPath(tree, spec.Cwd, p)
		if err != nil {
			return nil, err
		}
		for _, rel := range rels {
			picked[rel] = true
		}
	}
	for _, g := range spec.Globs {
		m, ok := newGlob(g)
		if !ok {
			return nil, fmt.Errorf("--glob %q is not a valid pattern", g)
		}
		hit := false
		for _, rel := range c.list() {
			if m.match(rel) {
				picked[rel] = true
				hit = true
			}
		}
		if !hit {
			res.Unmatched = append(res.Unmatched, g)
		}
	}
	return picked, nil
}

// conventional returns the candidates below under whose name and place follow a
// convention and that were not picked explicitly.
func (c *candidates) conventional(under string, picked picks) []string {
	var out []string
	for _, rel := range c.list() {
		if picked[rel] || (under != "" && !strings.HasPrefix(rel, under+"/")) {
			continue
		}
		if _, conventional := classifyPath(rel); conventional || isConventionalJSON(rel) {
			out = append(out, rel)
		}
	}
	return out
}

// picks is the set of files a path or glob named explicitly; a file picked by
// convention alone is not in it, so an unconventional markdown file is only
// taken when the user asked for it.
type picks map[string]bool

// sorted returns the picked files below under (every file when it is empty), in order.
func (p picks) sorted(under string) []string {
	rels := make([]string, 0, len(p))
	for rel := range p {
		if under == "" || strings.HasPrefix(rel, under+"/") {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	return rels
}

func (c *candidates) list() []string { return c.rels }

func (c *candidates) add(rel string) {
	if rel == "" || c.set[rel] {
		return
	}
	c.set[rel] = true
	c.rels = append(c.rels, rel)
}

// newCandidates lists the tracked files, the untracked files git does not
// ignore and, for a detection run, the files of the hidden directories a
// non-git walk skips (.claude, .agents, .cursor, ...).
func newCandidates(ctx context.Context, tree *Tree, detect bool) *candidates {
	c := &candidates{set: map[string]bool{}}
	for rel := range tree.files {
		c.add(rel)
	}
	if tree.Git {
		if files, ok, err := (gitutil.Git{}).ListFilesContext(ctx, tree.Top); ok && err == nil {
			for _, rel := range files {
				c.add(rel)
			}
		}
	}
	if detect {
		for _, dir := range conventionDirs() {
			c.walk(tree.Top, dir)
		}
	}
	sort.Strings(c.rels)
	return c
}

// conventionDirs are the hidden top-level directories that hold agent content;
// the walk of a repository without git skips them as hidden.
func conventionDirs() []string {
	return []string{".claude", ".agents", ".cursor", dirPluginMeta, ".github", ".vscode"}
}

func (c *candidates) walk(top, dir string) {
	root := filepath.Join(top, dir)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best effort
		}
		if rel, rerr := filepath.Rel(top, p); rerr == nil {
			c.add(filepath.ToSlash(rel))
		}
		return nil
	})
}

// expandPath returns the files --path p selects: the file itself, or the
// candidates below a directory.
func (c *candidates) expandPath(tree *Tree, cwd, p string) ([]string, error) {
	abs := p
	if !filepath.IsAbs(abs) {
		if cwd == "" {
			cwd = tree.Top
		}
		abs = filepath.Join(cwd, p)
	}
	abs = filepath.Clean(abs)
	resolved := gitutil.Resolve(abs)
	rel, err := filepath.Rel(tree.Top, resolved)
	if err != nil || safefs.RelEscapes(rel) {
		return nil, fmt.Errorf("path %q is outside the project root %s: lint paths must stay inside it", p, tree.Top)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("path %q does not exist", p)
	}
	rel = filepath.ToSlash(rel)
	if !info.IsDir() {
		return []string{rel}, nil
	}
	if rel == "." {
		rel = ""
	}
	prefix := rel
	if prefix != "" {
		prefix += "/"
	}
	c.walk(tree.Top, filepath.FromSlash(rel))
	var out []string
	for _, f := range c.rels {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	return out, nil
}

// contained reports whether the file at rel stays inside the tree top once its
// symlinks are resolved.
func contained(tree *Tree, rel string) (abs string, ok bool) {
	abs = filepath.Join(tree.Top, filepath.FromSlash(rel))
	resolved := gitutil.Resolve(abs)
	r, err := filepath.Rel(tree.Top, resolved)
	return abs, err == nil && !safefs.RelEscapes(r)
}

// loadTarget classifies and loads one file; ok is false for a file that is not
// a lintable kind, is unreadable, leaves the project through a symlink, or is
// itself generated (outputs are linted with --outputs, not as sources).
func loadTarget(ctx context.Context, tree *Tree, rel string, explicit bool) (TargetFile, bool) {
	abs, ok := contained(tree, rel)
	if !ok {
		return TargetFile{}, false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return TargetFile{}, false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case extJSON:
		return loadJSONTarget(abs, rel)
	case extMD, extMDC:
		return loadMarkdownTarget(ctx, tree, abs, rel, explicit)
	}
	return TargetFile{}, false
}

func loadMarkdownTarget(ctx context.Context, tree *Tree, abs, rel string, explicit bool) (TargetFile, bool) {
	kind, conventional := classifyPath(rel)
	if kind == "" || (!conventional && !explicit) {
		return TargetFile{}, false
	}
	data, err := readSmallFile(abs)
	if err != nil || carriesGeneratedBanner(data) {
		return TargetFile{}, false
	}
	f := TargetFile{Abs: abs, Rel: rel, Kind: kind}
	if kind == kindSkill && path.Base(rel) == fileSkill {
		skill, ok := config.LoadSkillDir(ctx, tree.Top, filepath.Dir(abs))
		if !ok {
			return TargetFile{}, false
		}
		f.Content = &skill
		f.Content.Path = abs
		return f, true
	}
	cf := config.ContentFileFromText(abs, string(data))
	f.Content = &cf
	return f, true
}

func loadJSONTarget(abs, rel string) (TargetFile, bool) {
	data, err := readSmallFile(abs)
	if err != nil {
		return TargetFile{}, false
	}
	kind, key := sniffJSON(path.Base(rel), data)
	if kind == "" {
		return TargetFile{}, false
	}
	return TargetFile{Abs: abs, Rel: rel, Kind: kind, MCPKey: key}, true
}

// isInstructionFile reports whether base is an instruction file of a harness;
// it is context, whatever directory it sits in.
func isInstructionFile(base string) bool {
	switch base {
	case "AGENTS.md", "CLAUDE.md", "GEMINI.md", "CLAUDE.local.md", "AGENTS.local.md":
		return true
	}
	return false
}

// segmentKind maps a directory name to the content kind of the markdown below it.
func segmentKind(dir string) (string, bool) {
	switch dir {
	case "rules":
		return kindRule, true
	case "agents":
		return kindAgent, true
	case "commands":
		return kindCommand, true
	case "checks":
		return kindCheck, true
	case "context":
		return kindContext, true
	}
	return "", false
}

// classifyPath names the content kind of a markdown file from its name and
// place. conventional is true when the place is one a harness or ai-rulez reads
// (AGENTS.md, SKILL.md, .claude/agents/, .cursor/rules/, ...); a markdown file
// anywhere else is context, which an explicit --path or --glob may still ask for.
func classifyPath(rel string) (kind string, conventional bool) {
	base := path.Base(rel)
	switch {
	case base == fileSkill:
		return kindSkill, true
	case base == "COMMAND.md":
		return kindCommand, true
	case isInstructionFile(base):
		return kindContext, true
	}
	segs := strings.Split(rel, "/")
	under := func(dir string) bool { return len(segs) > 1 && segs[0] == dir }
	for i := len(segs) - 2; i >= 0; i-- {
		if segs[i] == "skills" {
			return "", false // a resource of a skill: the skill item carries it
		}
		if k, ok := segmentKind(segs[i]); ok {
			return k, under(".claude") || under(".agents") || under(".cursor") || under(".ai-rulez") || under(".github")
		}
	}
	return kindContext, false
}

// isConventionalJSON reports whether rel is a JSON file an analyzer reads from
// a fixed place.
func isConventionalJSON(rel string) bool {
	dir, base := path.Dir(rel), path.Base(rel)
	switch {
	case path.Base(dir) == dirPluginMeta && (base == filePlugin || base == "marketplace.json"):
		return true
	case path.Base(dir) == keyHooks && base == fileHooks:
		return true
	case base == fileMCP || rel == ".cursor/mcp.json" || rel == ".vscode/mcp.json":
		return true
	}
	return rel == ".claude/settings.json"
}

// notManifestJSON reports whether base names a JSON file that carries a name and
// a version without being a plugin manifest.
func notManifestJSON(base string) bool {
	switch strings.ToLower(base) {
	case "package.json", "package-lock.json", "composer.json", "deno.json", "tsconfig.json", "manifest.json", "bower.json":
		return true
	}
	return false
}

// sniffJSON recognizes a plugin manifest, a marketplace, a hook file or an MCP
// file by its shape, so the place of the file does not matter. kind is "" for
// any other JSON. The shapes are the ones the analyzers read: a marketplace
// lists plugins (and names an owner or gives each a source), a plugin manifest
// has a name and a version, a hook file has a "hooks" object of matcher-group
// lists, an MCP file has an "mcpServers" (or, for VS Code, "servers") object.
func sniffJSON(base string, data []byte) (kind, mcpKey string) {
	if notManifestJSON(base) {
		return "", ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil {
		return "", ""
	}
	switch {
	case isMarketplaceShape(m):
		return targetMarketplace, ""
	case isPluginShape(m): // before hooks and servers: a manifest may declare them inline
		return targetPlugin, ""
	case isHooksShape(m):
		return targetHooks, ""
	case isObject(m[keyMCPServers]):
		return targetMCP, keyMCPServers
	case base == "mcp.json" && isObject(m["servers"]):
		return targetMCP, "servers"
	}
	return "", ""
}

func isObject(raw json.RawMessage) bool {
	var o map[string]json.RawMessage
	return raw != nil && json.Unmarshal(raw, &o) == nil
}

func isMarketplaceShape(m map[string]json.RawMessage) bool {
	var plugins []map[string]json.RawMessage
	raw, ok := m["plugins"]
	if !ok || json.Unmarshal(raw, &plugins) != nil {
		return false
	}
	if _, owner := m["owner"]; owner {
		return true
	}
	for _, p := range plugins {
		if _, hasSource := p["source"]; hasSource {
			return true
		}
	}
	return false
}

func isHooksShape(m map[string]json.RawMessage) bool {
	var byEvent map[string][]json.RawMessage
	raw, ok := m[keyHooks]
	return ok && json.Unmarshal(raw, &byEvent) == nil && len(byEvent) > 0
}

func isPluginShape(m map[string]json.RawMessage) bool {
	for _, k := range []string{"scripts", jsonKeyMain, "devDependencies", "exports", "bin", "engines", "plugins"} {
		if _, ok := m[k]; ok {
			return false
		}
	}
	name, hasName := m[keyName]
	_, hasVersion := m["version"]
	if !hasName || !hasVersion {
		return false
	}
	_, isString := rawString(name)
	return isString
}

// File names and extensions the classification keys on.
const (
	extJSON       = ".json"
	extMD         = ".md"
	extMDC        = ".mdc"
	fileSkill     = "SKILL.md"
	filePlugin    = "plugin.json"
	fileHooks     = "hooks.json"
	fileMCP       = ".mcp.json"
	dirPluginMeta = ".claude-plugin"
	jsonKeyMain   = "main"
)
