package rulefiles

import (
	"crypto/sha1" //nolint:gosec // not security relevant: a stable short name
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// Routing decides which items become rule files.
type Routing int

// Routing modes.
const (
	RoutingAll        Routing = iota // split: every rule and scoped context is a file
	RoutingScopedOnly                // inline mode with a rules dir: only path-scoped items are files
	RoutingNone                      // no rules dir: everything is inline
	RoutingEverything                // every rule and every context item is a file
	RoutingNonAlways                 // every rule that is not always-on and every path-scoped context item is a file
)

// WithoutAlwaysOn adapts a routing to a preset whose root file is replaced by
// the shared AGENTS.md (the agents_md flag): always-on rules and context live
// there, so only items with a narrower activation still become rule files.
// Inline and none stay as they are, since they create no files for always-on
// items either.
func WithoutAlwaysOn(r Routing) Routing {
	if r == RoutingAll || r == RoutingEverything {
		return RoutingNonAlways
	}
	return r
}

// RoutingFor maps the configured rules mode ("split" or "inline") to a Routing.
func RoutingFor(mode string, hasRulesDir bool) Routing {
	switch {
	case !hasRulesDir:
		return RoutingNone
	case mode == "split":
		return RoutingAll
	default:
		return RoutingScopedOnly
	}
}

// ScopeInfo places items of a monorepo scope into the root rules folder.
// The zero value means no scope.
type ScopeInfo struct {
	Slug   string // file-name qualifier
	Prefix string // path prefix applied to globs
}

// Registry detects rule files that would land on the same path across several
// Plan calls (the root plan and one per scope). Paths compare
// case-insensitively because common filesystems do, and a source claiming a
// path it already owns is not a collision. The zero value is not usable; create
// one with NewRegistry or RegistryFor.
type Registry struct {
	claims *config.PathClaims
	root   string // config directory; sources are hashed relative to it so ids do not depend on the checkout
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{claims: config.NewPathClaims()}
}

// NewRegistryFor returns an empty Registry for plans that do not share claims
// with a generation (machine-local rules). cfg only supplies the config
// directory that disambiguation suffixes are computed relative to.
func NewRegistryFor(cfg *config.Config) *Registry {
	r := NewRegistry()
	if cfg != nil {
		r.root = cfg.ConfigDir
	}
	return r
}

// source is the machine-independent identity of the file an item comes from.
// It is both the sort key that decides which colliding item keeps the plain
// name and the input of the suffix hash, so it must read the same on every
// machine. See logicalSource.
func (r *Registry) source(it Item) string {
	if it.File.Path == "" {
		return it.File.Name
	}
	return logicalSource(r.root, it.File.Path)
}

// logicalSource maps a content file path to an identity that does not depend on
// the checkout location, the user's home directory or the operating system:
//   - under the config directory: the slash-separated path relative to it
//     ("rules/api.md", "domains/web/rules/x.md"), also when one of the two
//     paths is relative;
//   - from an include (cached under ".../ai-rulez/includes/<name>/..."): "include:<name>/<path below its .ai-rulez/>";
//   - any other out-of-tree path: "include:<path below the last .ai-rulez/>", or
//     its last two segments when there is no such directory.
//
// A relative path with no config dir to compare against is returned as given
// (slashes normalized), it is already location-independent.
func logicalSource(root, owner string) string {
	if root != "" {
		if rel, ok := relUnder(root, owner); ok {
			return rel
		}
	}
	norm := strings.ReplaceAll(owner, "\\", "/")
	if len(norm) >= 2 && norm[1] == ':' && (norm[0]|0x20) >= 'a' && (norm[0]|0x20) <= 'z' {
		norm = norm[2:] // drive letter
	}
	if !strings.HasPrefix(norm, "/") && root == "" {
		return path.Clean(norm)
	}
	return outOfTreeIdentity(strings.FieldsFunc(norm, func(r rune) bool { return r == '/' }))
}

// outOfTreeIdentity is the include identity of a path outside the config dir,
// given its segments.
func outOfTreeIdentity(parts []string) string {
	last := func(name string, from int) int {
		for i := len(parts) - 1; i >= from; i-- {
			if parts[i] == name {
				return i
			}
		}
		return -1
	}
	tail := func(from int) string { return strings.Join(parts[from:], "/") }
	if i := last("includes", 1); i > 0 && i+1 < len(parts) && parts[i-1] == "ai-rulez" {
		rest := i + 2
		if j := last(".ai-rulez", i+2); j >= 0 {
			rest = j + 1
		}
		return "include:" + parts[i+1] + "/" + tail(rest)
	}
	if i := last(".ai-rulez", 0); i >= 0 && i+1 < len(parts) {
		return "include:" + tail(i+1)
	}
	return "include:" + tail(max(len(parts)-2, 0))
}

// relUnder returns target relative to root, in slash form, when target lies
// inside root. Either path may be relative; both are made absolute first.
func relUnder(root, target string) (string, bool) {
	absRoot, err1 := filepath.Abs(root)
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// disambiguate renames an item whose file name is taken: "<id>-<first 6 hex of
// sha1(source)>".
func disambiguate(it *Item, source string) {
	sum := sha1.Sum([]byte(source)) //nolint:gosec // not security relevant: a stable short name
	it.ID += "-" + hex.EncodeToString(sum[:])[:6]
}

// resolve claims the file of every item. Two items that map to the same path
// (names compare case-insensitively, and context files carry the "context-"
// prefix) are not an error: the one whose source sorts later gets a stable
// suffix, so the outcome does not depend on scan order. It is an error only
// when the suffixed name is taken too.
func (r *Registry) resolve(t Target, items []Item) error {
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return r.source(items[order[a]]) < r.source(items[order[b]])
	})
	for _, i := range order {
		it := &items[i]
		owner := r.source(*it)
		name := FileName(t, *it)
		if _, ok := r.claims.Claim(path.Join(t.Dir, name), owner); ok {
			continue
		}
		disambiguate(it, owner)
		renamed := FileName(t, *it)
		if prev, ok := r.claims.Claim(path.Join(t.Dir, renamed), owner); !ok {
			return oops.With("file", renamed, "preset", t.Preset).
				Hint("rename one of the two sources so they map to different file names").
				Errorf("rule files collide on %q: %s and %s", renamed, prev, owner)
		}
		warnCollisionOnce(t.Preset+" "+name+" "+owner, "rule files map to the same file name; "+
			"the later source was renamed (collide)", "preset", t.Preset, "file", name, "source", owner, "renamed", renamed)
	}
	return nil
}

var (
	collisionMu     sync.Mutex
	collisionWarned = map[string]struct{}{}
)

// warnCollisionOnce emits a warning once per generate run (ResetDowngrades
// starts a run), however many presets plan the same items.
func warnCollisionOnce(key, msg string, args ...any) {
	collisionMu.Lock()
	_, seen := collisionWarned[key]
	collisionWarned[key] = struct{}{}
	collisionMu.Unlock()
	if !seen {
		warnSink()(msg, args...)
	}
}

// Plan routes already ordered and deduplicated rules and context into rule
// files and inline remainders. t may be nil (no rules folder). Two items that
// map to the same file path (names are compared case-insensitively, and
// context files carry the "context-" prefix) are disambiguated by Registry.resolve.
// reg is shared by the caller across the root and scope plans of one
// target; nil uses a registry local to this call.
func Plan(rules, context []config.ContentFile, t *Target, routing Routing, scope ScopeInfo, reg *Registry,
) (files []Item, inlineRules, inlineContext []config.ContentFile, err error) {
	if t == nil || routing == RoutingNone {
		return nil, filterInlineFor(t, rules), filterInlineFor(t, context), nil
	}
	if reg == nil {
		reg = NewRegistry()
	}

	add := func(cf config.ContentFile, kind Kind) error {
		it, ok, err := planItem(*t, cf, kind, scope)
		if ok {
			files = append(files, it)
		}
		return err
	}

	place := func(cf config.ContentFile, kind Kind, asFile bool, inline *[]config.ContentFile) error {
		return routeItem(*t, cf, kind, scope, asFile, inline, add)
	}

	for _, r := range rules {
		asFile := ruleIsFile(routing, r) && !keepNegatedInline(*t, scope, routing, r, "rule")
		if err := place(r, KindRule, asFile, &inlineRules); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, c := range context {
		asFile := (routing == RoutingEverything || isScoped(c)) && !keepNegatedInline(*t, scope, routing, c, "context")
		if err := place(c, KindContext, asFile, &inlineContext); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := reg.resolve(*t, files); err != nil {
		return nil, nil, nil, err
	}
	return files, inlineRules, inlineContext, nil
}

// keepNegatedInline reports whether cf, a rule with only negated globs, stays
// in the root file. Such a rule has no scope a rules folder can express (scopes
// add their own positive glob). Where the preset has a root file it stays
// there, and under RoutingNonAlways the root content lives in the shared
// AGENTS.md (the agents_md flag), which carries it for every reader so the
// folder must not repeat it; otherwise it becomes an always-on file (see
// Frontmatter).
func keepNegatedInline(t Target, scope ScopeInfo, routing Routing, cf config.ContentFile, kind string) bool {
	rootFile := t.RootFile
	if routing == RoutingNonAlways {
		rootFile = "AGENTS.md"
	}
	if rootFile == "" || scope.Prefix != "" || !OnlyNegatedGlobs(cf) || !InlineAllowed(cf, t) {
		return false
	}
	WarnOnlyNegated(kind, cf, rootFile)
	return true
}

// routeItem writes cf as a rule file (asFile) or keeps it inline; targets may
// drop it from either.
func routeItem(t Target, cf config.ContentFile, kind Kind, scope ScopeInfo, asFile bool,
	inline *[]config.ContentFile, add func(config.ContentFile, Kind) error,
) error {
	fileOK := fileAllowed(cf, t, kind, scope)
	inlineOK := InlineAllowed(cf, t)
	switch {
	case asFile && fileOK, !asFile && !inlineOK && fileOK:
		// Targeted only at the rules folder: a file even in inline mode.
		return add(cf, kind)
	case inlineOK:
		// A root file shared with another preset (GEMINI.md) keeps items that
		// target that other preset, so it renders the same whoever writes it.
		*inline = append(*inline, cf)
	}
	return nil
}

// planItem builds the file of one item. ok is false, with a nil error, for an
// item that is skipped because its globs escape the scope.
func planItem(t Target, cf config.ContentFile, kind Kind, scope ScopeInfo) (it Item, ok bool, err error) {
	it, err = newItem(t, cf, kind, scope)
	if errors.Is(err, errEscapesScope) {
		warnSink()("rule file skipped: a glob escapes the scope with \"..\"",
			"scope", scope.Slug, "source", cf.Path, "error", err.Error())
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	if scope.Prefix != "" && (it.Activation.Mode == config.ActivationAuto || it.Activation.Mode == config.ActivationManual) &&
		warnScopeOnce(scope.Slug) {
		warnSink()("auto and manual rules of a scope are written to the root rules folder and are not limited to the scope",
			"scope", scope.Slug, "source", cf.Path)
	}
	return it, true, nil
}

// ruleIsFile reports whether routing sends the rule to a rule file.
func ruleIsFile(routing Routing, r config.ContentFile) bool {
	switch routing {
	case RoutingAll, RoutingEverything:
		return true
	case RoutingNonAlways:
		return !isAlwaysOn(r)
	default:
		return isScoped(r)
	}
}

func isAlwaysOn(cf config.ContentFile) bool {
	return cf.Metadata.ResolveActivation().Mode == config.ActivationAlways
}

func isScoped(cf config.ContentFile) bool {
	return cf.Metadata.ResolveActivation().Mode == config.ActivationGlob
}

func newItem(t Target, cf config.ContentFile, kind Kind, scope ScopeInfo) (Item, error) {
	act := cf.Metadata.ResolveActivation()
	id := ItemID(cf.Name)
	if prefix := cleanPrefix(scope.Prefix); prefix != "" {
		var err error
		if act, err = scopeActivation(act, prefix); err != nil {
			return Item{}, oops.With("source", cf.Path, "preset", t.Preset).Wrapf(err, "scope %q", scope.Slug)
		}
	}
	return Item{File: cf, Kind: kind, ID: ScopedID(t, scope.Slug, id), Activation: act}, nil
}

// ScopedID qualifies an id with a scope slug: "<slug>/<id>" for targets that
// discover rules recursively, "<slug>--<id>" for flat ones.
func ScopedID(t Target, slug, id string) string {
	if slug == "" {
		return id
	}
	if t.Recursive {
		return slug + "/" + id
	}
	return slug + "--" + id
}

func cleanPrefix(p string) string {
	p = strings.Trim(path.Clean(strings.ReplaceAll(p, "\\", "/")), "/")
	if p == "." {
		return ""
	}
	return p
}

// errEscapesScope marks a glob that would leave the scope directory.
var errEscapesScope = errors.New("glob escapes the scope")

// scopeActivation prefixes glob activations with the scope path, so globs of a
// scoped rule are relative to the scope root. Always-on items become a glob over
// the whole scope; a glob item without globs is left for the renderer to handle
// as in the root run; auto and manual items keep their mode. Globs with ".."
// segments (also inside braces) are rejected with errEscapesScope because they
// would escape the scope. When only negated globs remain, the whole scope is
// added so the rule still matches something.
func scopeActivation(act config.Activation, prefix string) (config.Activation, error) {
	switch act.Mode {
	case config.ActivationAuto, config.ActivationManual:
		return act, nil
	case config.ActivationGlob:
		if len(act.Globs) == 0 {
			return act, nil
		}
	default:
		act.Mode = config.ActivationGlob
		act.Globs = []string{prefix + "/**"}
		return act, nil
	}
	globs := make([]string, 0, len(act.Globs)+1)
	positive := false
	for _, g := range act.Globs {
		neg := strings.HasPrefix(g, "!")
		body := strings.TrimLeft(strings.TrimPrefix(g, "!"), "/")
		if escapesScope(body) {
			return act, fmt.Errorf("%w: %q", errEscapesScope, g)
		}
		joined := path.Join(prefix, body)
		if strings.HasSuffix(body, "/") {
			joined += "/"
		}
		if neg {
			joined = "!" + joined
		} else {
			positive = true
		}
		globs = append(globs, joined)
	}
	if !positive {
		globs = append([]string{prefix + "/**"}, globs...)
	}
	act.Globs = globs
	return act, nil
}

func escapesScope(glob string) bool {
	for _, expanded := range append(ExpandBraces(glob), glob) {
		for _, seg := range strings.Split(expanded, "/") {
			if seg == ".." {
				return true
			}
		}
	}
	return false
}
