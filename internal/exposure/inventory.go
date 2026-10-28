// Package exposure models what Claude Code puts into the context of a session
// started at a repository root: the start-of-session load model (CLAUDE.md
// files, unconditional rules, the skill and agent listings, MCP server names
// and enabled plugins) and what waits on demand or for the user.
//
// The inventory reads the repository's file listing (git's, as the repository
// index does) and every native Claude Code artifact in it, applies the root
// project settings (skillOverrides, claudeMdExcludes, enabledPlugins) and the
// repository marketplace, and keeps the frontmatter parse status of each item
// instead of reading a broken block as an absent one.
//
// The load model follows Claude Code 2.1.306 as measured; docs/exposure.md
// describes it. A symlinked file is read
// through its link, as Claude Code reads it, when the target is a regular file
// inside the root; a target outside the root is not followed, and the artifact
// is kept unread with a problem. Symlinked directories are not walked (the file
// listing does not enter them). A file over repoindex.MaxFileBytes is kept
// unread the same way rather than dropped, its file size standing in for its
// text in TextBytes and Tokens. A UTF-8 byte order mark is stripped before the
// frontmatter is parsed and is not part of the text. Not modeled: `@path` imports inside
// CLAUDE.md and rules, whose text is not counted.
//
// Known gaps against Claude Code:
//   - CLAUDE.local.md is not read, though Claude Code loads it at the start of
//     a session (it is machine-local, usually ignored by git).
//   - A nested .mcp.json and a nested .claude/settings*.json (and the plugins
//     it enables) are listed with phase on_demand, but Claude Code reads only
//     the ones at the project root, so they never load. A reader of the
//     on-demand artifacts over-reports by them.
//   - .claude/rules/CLAUDE.md is a CLAUDE.md with scope ".claude/rules"; Claude
//     Code loads it as a rule.
//   - User and managed settings, ~/.claude/CLAUDE.md, and the agents, commands,
//     MCP servers and hooks of plugins are not counted; a plugin enabled from
//     another marketplace contributes only its key.
//   - The frontmatter of a CLAUDE.md is left out of its text; whether Claude
//     Code does the same is not verified.
//
// Tokens is EstimateTokens of the text's UTF-8 length: bytes / 3.9 rounded up
// (BytesPerToken), so a non-empty text is at least 1 and an empty one 0. It is
// an estimate per artifact, not a tokenizer count; a total over many short texts
// carries one rounding per text.
//
// Root scope: an artifact at the repository root has scope RootScope ("."),
// as in internal/repoindex. internal/areas names the root "" and accepts ".".
// AreaScope and Artifact.AreaSkill map an artifact to what the area
// classification reads, and Inventory.Classify applies it.
package exposure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

// SchemaInventory is the schema_version of an Inventory, whose JSON is
// described by schema/exposure-inventory.v1.schema.json. Like every JSON
// document of the CLI it is an integer, bumped on an incompatible change.
const SchemaInventory = 1

// BytesPerToken is the ratio behind EstimateTokens, the measured load model's
// (see docs/exposure.md).
const BytesPerToken = 3.9

// EstimateTokens is the token estimate of a text of n UTF-8 bytes: n / 3.9
// rounded up, 0 for an empty text.
func EstimateTokens(n int) int {
	if n <= 0 {
		return 0
	}
	// ceil(n / 3.9) in integers: ceil(10n / 39).
	return (n*10 + 38) / 39
}

// HarnessClaude is the only harness the load model describes.
const HarnessClaude = "claude"

// RootScope is the scope of the repository root, as in internal/repoindex.
const RootScope = repoindex.RootScope

// Kind is what an artifact is.
type Kind string

// The artifact kinds.
const (
	KindClaudeMD  Kind = "claude_md"
	KindRule      Kind = "rule"
	KindSkill     Kind = "skill"
	KindAgent     Kind = "agent"
	KindMCPServer Kind = "mcp_server"
	KindPlugin    Kind = "plugin"
	KindSettings  Kind = "settings"
)

// Source is where an artifact comes from.
type Source string

// The artifact sources.
const (
	// SourceFile is a file of the repository read in its own convention.
	SourceFile Source = "file"
	// SourceCommand is a .claude/commands file, listed as a skill.
	SourceCommand Source = "command"
	// SourcePlugin is a skill of a repository-marketplace plugin.
	SourcePlugin Source = "plugin"
)

// Visibility is how an artifact reaches the model.
type Visibility string

// The visibilities.
const (
	// VisibilityAlways is listed or loaded whenever its scope is.
	VisibilityAlways Visibility = "always"
	// VisibilityPathGated waits until a file its `paths:` globs match is touched.
	VisibilityPathGated Visibility = "path_gated"
	// VisibilityManualOnly is never shown to the model; only the user runs it.
	VisibilityManualOnly Visibility = "manual_only"
)

// Phase is when an artifact's text enters the context of a session started at
// the repository root.
type Phase string

// The phases.
const (
	// PhaseStart is in context before the session touches any file.
	PhaseStart Phase = "start"
	// PhaseOnDemand loads when the session touches a file in its scope or one its
	// `paths:` globs match.
	PhaseOnDemand Phase = "on_demand"
	// PhaseManual never reaches the model unless the user invokes it.
	PhaseManual Phase = "manual"
	// PhaseUnknown is a root rule or skill whose frontmatter does not parse while
	// its raw block names a gate (`paths:`, `disable-model-invocation`,
	// `user-invocable`), so whether it is gated cannot be known. A reader that
	// guards the start of a session counts it as start (fail closed), as
	// Inventory.Start does.
	PhaseUnknown Phase = "unknown"
)

// FrontmatterStatus is whether the frontmatter block of a markdown artifact
// decoded. An artifact whose block does not decode is read as if it had none.
type FrontmatterStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Artifact is one thing a session can see: a file, a listing line, a server or
// plugin name.
type Artifact struct {
	Kind   Kind   `json:"kind"`
	Source Source `json:"source"`
	// Name is the artifact's own name: the frontmatter name, else the convention's
	// (the skill directory, the command path joined by ":", the rule path below
	// rules/, the agent file stem, the CLAUDE.md path, the server or plugin key).
	Name string `json:"name"`
	// QualifiedName is the name a session lists: `plugin:skill` for a plugin
	// skill, the name otherwise.
	QualifiedName string `json:"qualified_name"`
	// Plugin is the plugin of a plugin skill, as the marketplace names it.
	Plugin string `json:"plugin,omitempty"`
	// PluginPath is the directory of that plugin, relative to the root and slash
	// separated (the marketplace source); the area classification owns a plugin
	// skill by it (see AreaScope).
	PluginPath string `json:"plugin_path,omitempty"`
	// Path is the file, relative to the root and slash separated.
	Path string `json:"path"`
	// Scope is the directory that owns the artifact (the parent of its .claude
	// directory, or of CLAUDE.md, or of .mcp.json); RootScope for the root.
	Scope      string     `json:"scope"`
	Visibility Visibility `json:"visibility"`
	Phase      Phase      `json:"phase"`
	// Paths are the `paths:` globs of the frontmatter, as written.
	Paths []string `json:"paths"`
	// ListingText is the listing line of a skill or agent: `name: description
	// when_to_use`, whitespace squashed, the description capped for a skill.
	ListingText string `json:"listing_text,omitempty"`
	// ListingChars is the length of ListingText in characters (code points).
	ListingChars int `json:"listing_chars"`
	// Truncated reports a skill whose description and when_to_use exceed the
	// listing cap.
	Truncated bool `json:"truncated"`
	// TextBytes, TextSHA256 and Tokens describe Text: its UTF-8 length, its
	// sha256 (hex) and its estimated token count (EstimateTokens: bytes / 3.9
	// rounded up). For a file over the read limit, kept unread, TextBytes is the
	// file size and Tokens its estimate, so a start count still sees it;
	// TextSHA256 stays the hash of the empty text.
	TextBytes  int    `json:"text_bytes"`
	TextSHA256 string `json:"text_sha256"`
	Tokens     int    `json:"tokens"`
	// LineOffset is the number of file lines before the first line of Text (the
	// frontmatter of a CLAUDE.md or rule), so line n of Text is file line
	// LineOffset+n.
	LineOffset int `json:"line_offset"`
	// Frontmatter is set for markdown artifacts.
	Frontmatter *FrontmatterStatus `json:"frontmatter,omitempty"`
	// Area and AreaSource are the owning area and how it was determined; empty
	// until Inventory.Classify fills them.
	Area       string       `json:"area,omitempty"`
	AreaSource areas.Source `json:"area_source,omitempty"`
	// Text is what the session sees of the artifact: the body of a CLAUDE.md or
	// rule, the listing line of a skill or agent, the name of a server or plugin,
	// nothing for a settings file. WithoutText drops it from an output.
	Text string `json:"text,omitempty"`
	// unreadBytes is the size of a file that was not read for its size, which
	// stands in for its text in TextBytes and Tokens.
	unreadBytes int
}

// Settings are the root project settings the load model applies,
// .claude/settings.local.json over .claude/settings.json.
type Settings struct {
	SkillOverrides   map[string]string `json:"skill_overrides"`
	ClaudeMdExcludes []string          `json:"claude_md_excludes"`
	// EnabledPlugins lists the enabledPlugins keys (`plugin@marketplace`) the
	// merged settings set to true, sorted.
	EnabledPlugins []string `json:"enabled_plugins"`
}

// Problem is a file the inventory could not read or decode. A settings file,
// .mcp.json or the marketplace is left out; a markdown artifact is read as far
// as it goes, or kept unread (empty text, phase from its path, the reason as
// its frontmatter error).
type Problem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Inventory is the result of Build.
type Inventory struct {
	SchemaVersion int           `json:"schema_version"`
	Harness       string        `json:"harness"`
	Root          string        `json:"root"`
	Settings      Settings      `json:"settings"`
	Plugins       PluginSummary `json:"plugins"`
	Artifacts     []Artifact    `json:"artifacts"`
	Problems      []Problem     `json:"problems"`
	// areaPrefix is the path of Root below Options.AreaBase, slash separated, ""
	// when Root is the base or lies elsewhere; Classify reads the scopes with it
	// in front (ClaimScope).
	areaPrefix string
}

// Options configure Build.
type Options struct {
	// Root is the repository root. Build makes it absolute (against the working
	// directory), since claudeMdExcludes globs are matched against the absolute
	// root joined with an artifact's path.
	Root string
	// Files, when set, is the file listing (slash separated, relative to Root)
	// instead of repoindex.ListFiles.
	Files []string
	// Plugins selects the marketplace plugins whose skills are listed; empty is
	// PluginsEnabled.
	Plugins PluginMode
	// Requested names plugins a session installs on top of the enabled ones
	// (PluginsEnabled only).
	Requested []string
	// AreaBase is the directory the path claims of an area map are relative
	// to: the project root of the configuration that holds the map. Empty is
	// Root. When Root lies below it, Classify matches the claims against the
	// scopes with Root's path below AreaBase in front, so `--root
	// services/billing` matches the claim `services/billing`.
	AreaBase string
}

// Build reads the inventory of the repository at opts.Root.
func Build(ctx context.Context, opts Options) (*Inventory, error) {
	if opts.Root == "" {
		return nil, oops.Errorf("build the exposure inventory: no repository root")
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, oops.Wrapf(err, "build the exposure inventory: resolve the repository root")
	}
	mode := opts.Plugins
	if mode == "" {
		mode = PluginsEnabled
	}
	if !mode.Valid() {
		return nil, oops.Errorf("build the exposure inventory: unknown plugin mode %q (use none, enabled or all)", mode)
	}
	files, capped := opts.Files, false
	if files == nil {
		files, capped = repoindex.ListFilesCapped(ctx, root)
	}
	files = withRootSettings(root, files)
	b := &builder{root: root, absRoot: filepath.ToSlash(root), realRoot: realPath(root), files: files}
	if capped {
		b.problems = append(b.problems, Problem{Path: ".", Error: fmt.Sprintf("the file walk stopped after %d files; artifacts past them are not counted", len(files))})
	}
	b.readSettings()
	for _, file := range walkOrder(files) {
		b.collect(file)
	}
	repo := b.applySettings()
	catalog := b.catalog()
	plugins := b.pluginSkills(catalog.selected(mode, opts.Requested))
	all := make([]Artifact, 0, len(repo)+len(plugins))
	all = append(append(all, repo...), plugins...)
	for i := range all {
		all[i] = finish(all[i])
	}
	return &Inventory{
		SchemaVersion: SchemaInventory,
		Harness:       HarnessClaude,
		Root:          b.absRoot,
		Settings:      b.rootSettings.summary(),
		Plugins:       catalog.summary(mode, opts.Requested),
		Artifacts:     all,
		Problems:      append([]Problem{}, b.problems...),
		areaPrefix:    areaPrefix(opts.AreaBase, root),
	}, nil
}

// areaPrefix is the path of root below base, slash separated; "" when base is
// empty or root, or root does not lie below it.
func areaPrefix(base, root string) string {
	if base == "" {
		return ""
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(abs, root)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Start lists the artifacts in context when the session starts, failing
// closed: an artifact of PhaseUnknown, which may or may not be gated, is
// listed with the PhaseStart ones. A reader that needs the certain start set
// filters on Phase itself.
func (inv *Inventory) Start() []Artifact {
	var out []Artifact
	for i := range inv.Artifacts {
		if p := inv.Artifacts[i].Phase; p == PhaseStart || p == PhaseUnknown {
			out = append(out, inv.Artifacts[i])
		}
	}
	return out
}

// AreaScope is the directory whose path claims own an artifact in
// internal/areas (Map.Claim): its Scope, except for a plugin skill, whose Scope
// is the root it loads at and which is owned by its plugin directory. The root
// stays RootScope ("."), which internal/areas reads as its own "".
func AreaScope(a *Artifact) string {
	if a.Source == SourcePlugin && a.PluginPath != "" {
		return a.PluginPath
	}
	return a.Scope
}

// ClaimScope is the AreaScope of an artifact as the path claims of the area
// map read it: relative to Options.AreaBase, which differs from AreaScope
// when the inventory's root lies below that base.
func (inv *Inventory) ClaimScope(a *Artifact) string {
	scope := AreaScope(a)
	switch {
	case inv.areaPrefix == "":
		return scope
	case scope == RootScope || scope == "":
		return inv.areaPrefix
	}
	return inv.areaPrefix + "/" + scope
}

// WithoutText is a copy of the inventory without the artifact texts, which an
// output includes only on request.
func (inv *Inventory) WithoutText() *Inventory {
	out := *inv
	out.Artifacts = make([]Artifact, len(inv.Artifacts))
	copy(out.Artifacts, inv.Artifacts)
	for i := range out.Artifacts {
		out.Artifacts[i].Text = ""
	}
	return &out
}

// builder holds the state of one Build.
type builder struct {
	root, absRoot string
	// realRoot is the root with its symlinks resolved, which a symlinked file's
	// target must lie below.
	realRoot string
	files    []string
	// settings holds every decoded .claude/settings*.json by path; a file that
	// does not decode is absent (and a problem).
	settings map[string]*settingsFile
	// rootSettings is the merged root project settings.
	rootSettings mergedSettings
	artifacts    []Artifact
	problems     []Problem
}

func (b *builder) problem(path, err string) {
	b.problems = append(b.problems, Problem{Path: path, Error: err})
}

// readStatus is what came of reading a listed file.
type readStatus int

const (
	// readOK is a file that was read.
	readOK readStatus = iota
	// readGone is a file that is not there (or a symlink to nothing): Claude
	// Code loads nothing from it either.
	readGone
	// readRefused is a file that exists but was not read: over the read limit,
	// a symlink to outside the root or to a directory, or not a regular file. It
	// is a problem, and a markdown artifact is kept unread.
	readRefused
)

// readResult is what read made of a listed file: its text, the status, why it
// was refused, and the size of a file refused for being over the read limit.
type readResult struct {
	text      string
	status    readStatus
	reason    string
	oversized int
}

// read reads a listed file as Claude Code does: a symlinked file is read
// through its link, provided the target is a regular file inside the root.
// Every outcome other than readOK and a plain missing file is a problem;
// reason says why a file was refused.
func (b *builder) read(rel string) readResult {
	abs := filepath.Join(b.root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	switch {
	case os.IsNotExist(err):
		return readResult{status: readGone}
	case err != nil:
		return b.refuse(rel, err.Error())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, terr := filepath.EvalSymlinks(abs)
		if terr != nil {
			if os.IsNotExist(terr) {
				b.problem(rel, "a symlink whose target does not exist")
				return readResult{status: readGone}
			}
			return b.refuse(rel, terr.Error())
		}
		if !b.inside(target) {
			return b.refuse(rel, "a symlink to "+filepath.ToSlash(target)+", outside the repository root; not followed")
		}
		if info, err = os.Stat(target); err != nil {
			return b.refuse(rel, err.Error())
		}
		abs = target
	} else if dir := realPath(filepath.Dir(abs)); !b.inside(dir) {
		return b.refuse(rel, "in a directory that resolves to "+filepath.ToSlash(dir)+", outside the repository root; not followed")
	}
	switch {
	case !info.Mode().IsRegular():
		return b.refuse(rel, "not a regular file")
	case info.Size() > repoindex.MaxFileBytes:
		r := b.refuse(rel, fmt.Sprintf("%d bytes, over the read limit of %d", info.Size(), repoindex.MaxFileBytes))
		r.oversized = int(info.Size())
		return r
	}
	data, err := os.ReadFile(abs) //nolint:gosec // a listed file below the root, or a symlink resolved inside it
	if err != nil {
		return b.refuse(rel, err.Error())
	}
	return readResult{text: decodeText(data), status: readOK}
}

func (b *builder) refuse(rel, reason string) readResult {
	b.problem(rel, reason)
	return readResult{status: readRefused, reason: reason}
}

// inside reports whether a resolved path lies below the resolved root.
func (b *builder) inside(target string) bool {
	rel, err := filepath.Rel(b.realRoot, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// realPath is p with its symlinks resolved, or p when they cannot be.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// withRootSettings adds the root project settings files that exist but are not
// listed: settings.local.json is machine-local and usually ignored by git, and
// Claude Code reads it all the same.
func withRootSettings(root string, files []string) []string {
	listed := map[string]bool{}
	for _, f := range files {
		listed[f] = true
	}
	out := files
	for _, name := range []string{settingsShared, settingsLocal} {
		rel := claudeDir + "/" + name
		if listed[rel] {
			continue
		}
		if info, err := os.Stat(filepath.Join(root, claudeDir, name)); err == nil && info.Mode().IsRegular() {
			out = append(append([]string(nil), out...), rel)
		}
	}
	return out
}

// finish derives the phase and the text measures of an artifact.
func finish(a Artifact) Artifact {
	if a.Paths == nil {
		a.Paths = []string{}
	}
	if a.QualifiedName == "" {
		a.QualifiedName = a.Name
	}
	if a.Phase == "" {
		a.Phase = phaseOf(&a)
	}
	a.ListingChars = utf8.RuneCountInString(a.ListingText)
	sum := sha256.Sum256([]byte(a.Text))
	a.TextBytes = len(a.Text)
	if a.Text == "" && a.unreadBytes > 0 {
		a.TextBytes = a.unreadBytes
	}
	a.TextSHA256 = hex.EncodeToString(sum[:])
	a.Tokens = EstimateTokens(a.TextBytes)
	return a
}

// phaseOf is the phase of an artifact whose visibility is settled.
func phaseOf(a *Artifact) Phase {
	switch {
	case a.Visibility == VisibilityManualOnly:
		return PhaseManual
	case a.Scope != RootScope || a.Visibility == VisibilityPathGated:
		return PhaseOnDemand
	}
	return PhaseStart
}

// pruned reports whether a file lies below a directory the load model never
// enters at any depth.
func pruned(file string) bool {
	segs := strings.Split(file, "/")
	for _, s := range segs[:len(segs)-1] {
		switch s {
		case ".git", "node_modules", ".venv", "__pycache__":
			return true
		}
	}
	return false
}

// walkOrder is the listing without pruned directories, in the order a sorted
// top-down walk visits it: the files of a directory before its subdirectories.
func walkOrder(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if f != "" && !pruned(f) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return walkLess(out[i], out[j]) })
	return out
}

func walkLess(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		aFile, bFile := i == len(as)-1, i == len(bs)-1
		if aFile != bFile {
			return aFile
		}
		return as[i] < bs[i]
	}
	return len(as) < len(bs)
}
