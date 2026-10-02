package rulefiles

import (
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
// case-insensitively because common filesystems do. The zero value is not
// usable; create one with NewRegistry.
type Registry struct {
	owner map[string]string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{owner: map[string]string{}}
}

func (r *Registry) claim(t Target, it Item) error {
	name := FileName(t, it)
	key := strings.ToLower(path.Join(t.Dir, name))
	if prev, dup := r.owner[key]; dup {
		return oops.With("file", name, "preset", t.Preset).
			Hint("rename one of the two sources so they map to different file names").
			Errorf("rule files collide on %q: %s and %s", name, prev, it.File.Path)
	}
	r.owner[key] = it.File.Path
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
		return nil, rules, context, nil
	}
	if reg == nil {
		reg = NewRegistry()
	}

	add := func(cf config.ContentFile, kind Kind) error {
		it, err := newItem(*t, cf, kind, scope)
		if err != nil {
			return err
		}
		if err := reg.claim(*t, it); err != nil {
			return err
		}
		files = append(files, it)
		return nil
	}

	for _, r := range rules {
		if routing == RoutingAll || routing == RoutingEverything || isScoped(r) {
			if err := add(r, KindRule); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		inlineRules = append(inlineRules, r)
	}
	for _, c := range context {
		if routing == RoutingEverything || isScoped(c) {
			if err := add(c, KindContext); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		inlineContext = append(inlineContext, c)
	}
	return files, inlineRules, inlineContext, nil
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

// scopeActivation prefixes glob activations with the scope path. Always-on
// items (and glob items without globs) become a glob over the whole scope;
// auto and manual items keep their mode. Globs containing ".." segments are
// rejected because they would escape the scope.
func scopeActivation(act config.Activation, prefix string) (config.Activation, error) {
	switch act.Mode {
	case config.ActivationAuto, config.ActivationManual:
		return act, nil
	case config.ActivationGlob:
		if len(act.Globs) > 0 {
			break
		}
		fallthrough
	default:
		act.Mode = config.ActivationGlob
		act.Globs = []string{prefix + "/**"}
		return act, nil
	}
	globs := make([]string, len(act.Globs))
	for i, g := range act.Globs {
		neg := strings.HasPrefix(g, "!")
		g = strings.TrimLeft(strings.TrimPrefix(g, "!"), "/")
		for _, seg := range strings.Split(g, "/") {
			if seg == ".." {
				return act, oops.Errorf("glob %q escapes the scope with \"..\"", act.Globs[i])
			}
		}
		globs[i] = path.Join(prefix, g)
		if neg {
			globs[i] = "!" + globs[i]
		}
	}
	act.Globs = globs
	return act, nil
}
