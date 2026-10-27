package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	procrunner "github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
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
// Literals shared across the lint package.
const (
	extExe         = ".exe"
	dirGit         = ".git"
	cacheStateHit  = "hit"
	cacheStateUnrd = "unreadable"
)

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
	// Except drops the picked files below any of these directories (slash paths
	// relative to the tree top): the files the roots of a recursive run already
	// lint, so the rest can be linted once at the repository root.
	Except []string
	// ExceptFiles drops these files (slash paths relative to the tree top) from the files found by
	// convention: the outputs a generate run recorded, which are not hand-written content.
	ExceptFiles []string
	// QuietGenerated leaves out the notice for a file that carries the generated header: in a
	// configured project nearly every harness file does.
	QuietGenerated bool
	// Generated also collects the targets that carry the generated header, as
	// outputs to lint (Targets.Generated), instead of skipping them.
	Generated bool
	// Exclude lists globs (slash paths relative to the tree top) of files left out of the
	// targets, however they were picked: the way to keep an application's own SKILL.md out of
	// a run that has no configuration to say [lint] ignore_paths in (#457).
	Exclude []string
	// Lenient reports a named file that cannot be read or is over the size limit in
	// Targets.Skipped instead of failing the run, so one such file does not stop a list.
	Lenient bool
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
	// Plain holds the files a user named that are text but no content kind
	// (.env, a YAML file, ...): only the secret scan reads them.
	Plain []PlainFile
	// Skipped lists the files that were picked but not linted, with the reason,
	// so nothing a user asked for is dropped without a word.
	Skipped []SkippedTarget
	// GeneratedRels lists the picked files that carry the generated header, however
	// they were picked: a directory only counts them in SkippedInDirs.
	GeneratedRels []string
	// SkippedInDirs counts the files below a --path directory that are not
	// content (a directory lints the kinds it recognizes and says how many it left).
	SkippedInDirs int
	// skippedInDirNames names those files (slash paths), for the notice.
	skippedInDirNames []string
	// extra holds notices the caller adds (AddNotice).
	extra []string
}

// AddNotice appends a line to the notices, after the ones the resolution made.
func (t *Targets) AddNotice(line string) { t.extra = append(t.extra, line) }

// PlainFile is a text file that is scanned for secrets only.
type PlainFile struct {
	Abs, Rel string
	Data     []byte
}

// SkippedTarget is a picked file that was not linted.
type SkippedTarget struct {
	Rel, Reason string
}

// Notices returns the lines to show about what was not linted: each skipped file
// as "<rel>: not linted (<reason>)", a summary of the files left out of a
// directory and each glob that matched nothing.
func (t *Targets) Notices() []string {
	if t == nil {
		return nil
	}
	var out []string
	for _, s := range t.Skipped {
		out = append(out, fmt.Sprintf("%s: not linted (%s)", s.Rel, s.Reason))
	}
	if t.SkippedInDirs > 0 {
		out = append(out, fmt.Sprintf("%d file(s) below a --path directory were not linted (not content, or generated)%s: name a file with --path or --glob to scan it as text", t.SkippedInDirs, namedFiles(t.skippedInDirNames, maxNamedSkips)))
	}
	for _, g := range t.Unmatched {
		out = append(out, fmt.Sprintf("no file matches --glob %q (files git ignores are not matched)", g))
	}
	return append(out, t.extra...)
}

// maxNamedSkips caps how many skipped files a notice lists by name.
const maxNamedSkips = 10

// namedFiles renders ": a, b, c and N more" for the first max names, or "" without any.
func namedFiles(names []string, limit int) string {
	if len(names) == 0 {
		return ""
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if len(sorted) <= limit {
		return ": " + strings.Join(sorted, ", ")
	}
	return fmt.Sprintf(": %s and %d more", strings.Join(sorted[:limit], ", "), len(sorted)-limit)
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
	ctx = withSharedSkillGit(ctx, tree)
	cands := newCandidates(ctx, tree, spec.Detect)
	picked, err := cands.explicit(tree, spec, res)
	if err != nil {
		return nil, err
	}
	rels := picked.sorted(spec.Under)
	rels = dropUnder(rels, spec.Except)
	if spec.Detect {
		rels = append(rels, dropFiles(cands.conventional(spec.Under, picked), spec.ExceptFiles)...)
	}
	rels = dropMatching(rels, spec.Exclude)
	if len(rels) > maxTargetFiles {
		return nil, fmt.Errorf("%d files match --path, --glob or [lint] paths; the limit is %d: narrow the selection", len(rels), maxTargetFiles)
	}
	var unreadable []string
	for _, rel := range rels {
		out := loadTarget(ctx, tree, rel, picked[rel])
		switch {
		case out.loaded:
			res.Files = append(res.Files, out.file)
		case out.plain != nil:
			res.Plain = append(res.Plain, *out.plain)
		default:
			if out.fatal && picked[rel] == pickPath && !spec.Lenient {
				unreadable = append(unreadable, rel+" ("+out.reason+")")
			}
			res.recordSkip(spec, rel, picked[rel], out)
		}
	}
	if len(unreadable) > 0 {
		return nil, fmt.Errorf("cannot lint %s: a file named with --path must be a readable text file inside the limit", strings.Join(unreadable, "; "))
	}
	if spec.Generated {
		res.Generated = generatedTargets(tree, rels)
	}
	return res, nil
}

// recordSkip notes a picked file that was not linted, unless the spec lints or silences it: --outputs
// lints a generated file as an output, and a conventional file with the generated header is silent
// when QuietGenerated is set.
func (t *Targets) recordSkip(spec TargetSpec, rel string, how pick, out loadOutcome) {
	if out.generated {
		t.GeneratedRels = append(t.GeneratedRels, rel)
		if spec.Generated || (spec.QuietGenerated && how == 0) {
			return
		}
	}
	t.noteSkipped(rel, how, out)
}

// dropMatching returns rels without the files that match one of the globs.
func dropMatching(rels, globs []string) []string {
	matchers := compileGlobs(globs)
	if len(matchers) == 0 {
		return rels
	}
	kept := rels[:0:0]
	for _, rel := range rels {
		if !globsMatch(matchers, rel) {
			kept = append(kept, rel)
		}
	}
	return kept
}

// dropFiles returns rels without the files in except.
func dropFiles(rels, except []string) []string {
	if len(except) == 0 {
		return rels
	}
	skip := make(map[string]bool, len(except))
	for _, e := range except {
		skip[e] = true
	}
	kept := rels[:0:0]
	for _, rel := range rels {
		if !skip[rel] {
			kept = append(kept, rel)
		}
	}
	return kept
}

// withSharedSkillGit makes the skills loaded under the returned context share
// one set of git answers instead of asking git per skill.
func withSharedSkillGit(ctx context.Context, tree *Tree) context.Context {
	if !tree.Git {
		return ctx
	}
	return config.WithBundleSnapshots(ctx, tree.Top)
}

// generatedTargets returns the markdown files among rels that carry the
// generated header or have the OKF listing shape, which loadTarget leaves out as sources.
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
		if data, err := readSmallFile(abs); err == nil && (genmark.IsGeneratedOutput(rel, data) || isOKFListingTarget(rel, data)) {
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
		rels, isDir, err := c.expandPath(tree, spec.Cwd, p)
		if err != nil {
			return nil, err
		}
		how := pickPath
		if isDir {
			how = pickDir
		}
		for _, rel := range rels {
			picked.add(rel, how)
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
				picked.add(rel, pickGlob)
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
		if picked[rel] != 0 || (under != "" && !strings.HasPrefix(rel, under+"/")) {
			continue
		}
		if _, conventional := classifyPath(rel); conventional || isConventionalJSON(rel) {
			out = append(out, rel)
		}
	}
	return out
}

// pick says how a file was picked; the zero value is "not picked".
type pick uint8

const (
	// pickDir: below a --path directory.
	pickDir pick = iota + 1
	// pickGlob: matched by a --glob.
	pickGlob
	// pickPath: a file named with --path (or [lint] paths).
	pickPath
)

// picks is the set of files a path or glob named explicitly; a file picked by
// convention alone is not in it, so an unconventional markdown file is only
// taken when the user asked for it. A file picked several ways keeps the
// most direct one.
type picks map[string]pick

func (p picks) add(rel string, how pick) {
	if how > p[rel] {
		p[rel] = how
	}
}

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
	// A backup directory (.ai-rulez.bak-okf-<time>) holds old copies, not content.
	if rel == "" || c.set[rel] || walkutil.InBackupDir(rel) {
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
		if files, ok, err := gitutil.New(procrunner.FromContext(ctx)).ListFilesContext(ctx, tree.Top); ok && err == nil {
			for _, rel := range files {
				c.add(rel)
			}
			config.SeedBundleListing(ctx, files)
		}
	}
	if detect {
		for _, dir := range conventionDirs() {
			c.walk(tree.Top, dir)
		}
		if !tree.Git {
			c.walkNestedClaude(tree.Top)
		}
	}
	sort.Strings(c.rels)
	return c
}

// conventionDirs are the hidden top-level directories that hold agent content;
// the walk of a repository without git skips them as hidden.
func conventionDirs() []string {
	return []string{dirClaude, ".agents", ".cursor", dirPluginMeta, ".github", ".vscode"}
}

func (c *candidates) walk(top, dir string) {
	root := filepath.Join(top, dir)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if d.IsDir() {
			if p != root {
				// Tool state may contain independent repositories and linked worktrees.
				// Their instructions belong to that repository, not this project.
				if _, err := os.Lstat(filepath.Join(p, dirGit)); err == nil {
					return filepath.SkipDir
				}
				if d.Name() == dirGit {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if rel, rerr := filepath.Rel(top, p); rerr == nil {
			c.add(filepath.ToSlash(rel))
		}
		return nil
	})
}

// walkNestedClaude adds the files of every .claude directory below top, which a
// non-git walk skips as hidden. It skips the directories a project walk skips
// and stops once the candidates exceed maxTargetFiles.
func (c *candidates) walkNestedClaude(top string) {
	_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil || !d.IsDir() || p == top {
			return nil //nolint:nilerr // best effort
		}
		if len(c.rels) > maxTargetFiles {
			return filepath.SkipAll
		}
		if d.Name() == dirClaude {
			if rel, rerr := filepath.Rel(top, p); rerr == nil {
				c.walk(top, rel)
			}
			return filepath.SkipDir
		}
		if walkutil.ShouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		return nil
	})
}

// walkProject adds the files below dir (relative to top; "" is the top itself),
// skipping the directories a project walk skips (.git, dependency and build
// directories, hidden directories) below dir. dir itself is walked even when it
// is such a directory, since the user named it. The walk stops once the
// candidates exceed maxTargetFiles, which the caller reports.
func (c *candidates) walkProject(top, dir string) {
	root := filepath.Join(top, dir)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if d.IsDir() {
			if p != root && walkutil.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(c.rels) > maxTargetFiles {
			return filepath.SkipAll
		}
		if rel, rerr := filepath.Rel(top, p); rerr == nil {
			c.add(filepath.ToSlash(rel))
		}
		return nil
	})
}

// expandPath returns the files --path p selects: the file itself, or the
// candidates below a directory.
func (c *candidates) expandPath(tree *Tree, cwd, p string) (rels []string, isDir bool, err error) {
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
		return nil, false, fmt.Errorf("path %q is outside the project root %s: lint paths must stay inside it", p, tree.Top)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, false, fmt.Errorf("path %q does not exist", p)
	}
	rel = realCase(tree.Top, filepath.ToSlash(rel))
	if !info.IsDir() {
		return []string{rel}, false, nil
	}
	if rel == "." {
		rel = ""
	}
	prefix := rel
	if prefix != "" {
		prefix += "/"
	}
	if !tree.Git {
		// In git the candidates already hold the tracked and the untracked files git
		// does not ignore, so ignored dependency trees and .git stay out. Outside
		// git, walk the directory but skip what a project walk skips.
		c.walkProject(tree.Top, filepath.FromSlash(rel))
	}
	var out []string
	for _, f := range c.rels {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	return out, true, nil
}

// contained reports whether the file at rel stays inside the tree top once its
// symlinks are resolved.
func contained(tree *Tree, rel string) (abs string, ok bool) {
	abs = filepath.Join(tree.Top, filepath.FromSlash(rel))
	resolved := gitutil.Resolve(abs)
	r, err := filepath.Rel(tree.Top, resolved)
	return abs, err == nil && !safefs.RelEscapes(r)
}

// loadOutcome is what loading one picked file produced: a linted target, a text
// file only the secret scan reads, or the reason it was left out.
type loadOutcome struct {
	file   TargetFile
	loaded bool
	plain  *PlainFile
	// reason says why the file was not linted; kind how that is reported.
	reason string
	kind   skipKind
	// fatal marks a file that could not be read at all: named with --path it fails the run.
	fatal bool
	// generated marks a file that carries the generated header.
	generated bool
}

// skipKind groups the reasons a file is not linted by how loudly they are reported.
type skipKind uint8

const (
	// skipProblem: the file could not be read or lies outside the project;
	// reported whatever way it was picked.
	skipProblem skipKind = iota + 1
	// skipUnsupported: not content a run lints, or generated; reported for a
	// file that was named, counted for a file found below a directory.
	skipUnsupported
)

// noteSkipped records a picked file that was not linted: a problem is always
// reported, an unsupported file when it was named (--path file or --glob) and
// counted when it came from a directory. A file found by convention alone is
// not reported unless it could not be read or was left out as generated.
func (t *Targets) noteSkipped(rel string, how pick, out loadOutcome) {
	switch {
	case out.kind == skipProblem:
		t.Skipped = append(t.Skipped, SkippedTarget{Rel: rel, Reason: out.reason})
	case how == pickDir:
		t.SkippedInDirs++
		t.skippedInDirNames = append(t.skippedInDirNames, rel)
	case how >= pickGlob, out.generated:
		// A conventional file left out as generated is named too: the header is
		// text anyone can write, so the skip must be visible.
		t.Skipped = append(t.Skipped, SkippedTarget{Rel: rel, Reason: out.reason})
	}
}

func problem(reason string, fatal bool) loadOutcome {
	return loadOutcome{reason: reason, kind: skipProblem, fatal: fatal}
}

func unsupported(reason string) loadOutcome {
	return loadOutcome{reason: reason, kind: skipUnsupported}
}

// reasonTooLarge is the notice for a file over the read limit.
const reasonTooLarge = "larger than 2 MiB"

// loadTarget classifies and loads one file. A file that is not a lintable kind,
// is unreadable, leaves the project through a symlink, or is itself generated
// (outputs are linted with --outputs, not as sources) comes back with its reason.
// A text file the user named that is no content kind (.env, YAML, ...) comes back
// as a plain file for the secret scan.
func loadTarget(ctx context.Context, tree *Tree, rel string, how pick) loadOutcome {
	abs, ok := contained(tree, rel)
	if !ok {
		return problem("resolves outside the project", false)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return problem("cannot be read", true)
	}
	if !info.Mode().IsRegular() {
		return problem("not a regular file", true)
	}
	named := how >= pickGlob
	switch strings.ToLower(path.Ext(rel)) {
	case extJSON:
		return loadJSONTarget(tree, abs, rel, info.Size(), named)
	case extMD, extMDC:
		return loadMarkdownTarget(ctx, tree, abs, rel, info.Size(), how)
	}
	if !named {
		return unsupported("not markdown or JSON")
	}
	return loadPlainTarget(tree, abs, rel)
}

// loadPlainTarget reads a named file that is no content kind, so the secret scan
// can look at it. It reads the resolved path without following a link, and
// leaves out a file over the limit, a binary file and anything unreadable.
func loadPlainTarget(tree *Tree, abs, rel string) loadOutcome {
	resolved := gitutil.Resolve(abs)
	if r, err := filepath.Rel(tree.Top, resolved); err != nil || safefs.RelEscapes(r) {
		return problem("resolves outside the project", false)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return problem("not a regular file", true)
	}
	if info.Size() > maxReadBytes {
		return problem(reasonTooLarge, true)
	}
	data, _, err := safefs.ReadRegularKeepMode(resolved)
	if err != nil {
		return problem("cannot be read", true)
	}
	if isBinary(data) {
		return unsupported("binary file")
	}
	return loadOutcome{plain: &PlainFile{Abs: abs, Rel: rel, Data: data}}
}

// isBinary reports whether data looks like a binary file: a NUL byte in its first
// 8 KiB, which text in every encoding the scan understands does not hold.
func isBinary(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	return bytes.IndexByte(head, 0) >= 0
}

func loadMarkdownTarget(ctx context.Context, tree *Tree, abs, rel string, size int64, how pick) loadOutcome {
	kind, conventional := classifyPath(rel)
	if kind == "" { // a resource of a skill: the skill item carries it
		if how >= pickGlob {
			return loadPlainTarget(tree, abs, rel)
		}
		return unsupported("a resource of a skill")
	}
	if !conventional && how == 0 {
		return unsupported("not in a place an agent reads")
	}
	if size > maxReadBytes {
		return problem(reasonTooLarge, true)
	}
	data, err := readSmallFile(abs)
	if err != nil {
		return problem("cannot be read", true)
	}
	if genmark.IsGeneratedOutput(rel, data) || isOKFListingTarget(rel, data) {
		out := unsupported("generated file: lint it with --outputs")
		out.generated = true
		return out
	}
	f := TargetFile{Abs: abs, Rel: rel, Kind: kind}
	if kind == kindSkill && path.Base(rel) == fileSkill {
		skill, ok := config.LoadSkillDir(ctx, tree.Top, filepath.Dir(abs))
		if !ok {
			return problem("the skill could not be loaded", false)
		}
		f.Content = &skill
		f.Content.Path = abs
		return loadOutcome{file: f, loaded: true}
	}
	cf := config.ContentFileFromText(abs, string(data))
	f.Content = &cf
	return loadOutcome{file: f, loaded: true}
}

func isOKFListingTarget(rel string, data []byte) bool {
	switch strings.ToLower(path.Base(rel)) {
	case "index.md", "log.md":
		return config.IsOKFListing(data)
	}
	return false
}

func loadJSONTarget(tree *Tree, abs, rel string, size int64, named bool) loadOutcome {
	if size > maxReadBytes {
		return problem(reasonTooLarge, true)
	}
	data, err := readSmallFile(abs)
	if err != nil {
		return problem("cannot be read", true)
	}
	kind, key := sniffJSON(path.Base(rel), data)
	if kind == "" {
		if named {
			return loadPlainTarget(tree, abs, rel)
		}
		return unsupported("JSON that is no plugin, marketplace, hook or MCP file")
	}
	return loadOutcome{file: TargetFile{Abs: abs, Rel: rel, Kind: kind, MCPKey: key}, loaded: true}
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
	case keyCommands:
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
			nested := i > 0 && segs[i-1] == dirClaude // a .claude directory at any depth
			return k, nested || under(dirClaude) || under(".agents") || under(".cursor") || under(".ai-rulez") || under(".github")
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

// dropUnder removes the files below any of the directories dirs.
func dropUnder(rels, dirs []string) []string {
	if len(dirs) == 0 {
		return rels
	}
	return slices.DeleteFunc(rels, func(rel string) bool {
		return slices.ContainsFunc(dirs, func(dir string) bool { return strings.HasPrefix(rel, dir+"/") })
	})
}

// Candidates lists the files of the project a run can pick from: the tracked files, the untracked
// files git does not ignore and the files of the hidden agent directories (.claude, .agents, ...)
// that a walk outside git skips, at any depth. The paths are slash paths relative to the tree top.
func Candidates(ctx context.Context, tree *Tree) []string {
	return newCandidates(ctx, tree, true).list()
}
