package rulefiles

import (
	"errors"
	"fmt"
	"path"
	"strings"

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
)

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
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{claims: config.NewPathClaims()}
}

func (r *Registry) claim(t Target, it Item) error {
	name := FileName(t, it)
	owner := it.File.Path
	if owner == "" {
		owner = it.File.Name
	}
	if prev, ok := r.claims.Claim(path.Join(t.Dir, name), owner); !ok {
		return oops.With("file", name, "preset", t.Preset).
			Hint("rename one of the two sources so they map to different file names").
			Errorf("rule files collide on %q: %s and %s", name, prev, owner)
	}
	return nil
}

// Plan routes already ordered and deduplicated rules and context into rule
// files and inline remainders. t may be nil (no rules folder). Two items that
// map to the same file path (names are compared case-insensitively, and
// context files carry the "context-" prefix) are an error. reg is shared by the caller across the root and scope plans of one
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
		it, ok, err := planItem(*t, cf, kind, scope, reg)
		if ok {
			files = append(files, it)
		}
		return err
	}

	place := func(cf config.ContentFile, kind Kind, asFile bool, inline *[]config.ContentFile) error {
		return routeItem(*t, cf, kind, scope, asFile, inline, add)
	}

	for _, r := range rules {
		asFile := routing == RoutingAll || routing == RoutingEverything || isScoped(r)
		if err := place(r, KindRule, asFile, &inlineRules); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, c := range context {
		asFile := routing == RoutingEverything || isScoped(c)
		if err := place(c, KindContext, asFile, &inlineContext); err != nil {
			return nil, nil, nil, err
		}
	}
	return files, inlineRules, inlineContext, nil
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

// planItem builds and claims the file of one item. ok is false, with a nil
// error, for an item that is skipped because its globs escape the scope.
func planItem(t Target, cf config.ContentFile, kind Kind, scope ScopeInfo, reg *Registry) (it Item, ok bool, err error) {
	it, err = newItem(t, cf, kind, scope)
	if errors.Is(err, errEscapesScope) {
		warnSink()("rule file skipped: a glob escapes the scope with \"..\"",
			"scope", scope.Slug, "source", cf.Path, "error", err.Error())
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	if err := reg.claim(t, it); err != nil {
		return Item{}, false, err
	}
	return it, true, nil
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
