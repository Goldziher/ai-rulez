package mcp

import (
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// A serve view is one way of starting the skills server: the default view, a role
// (--role), a profile (--profile), a preset's rendering (--targets), with
// --include-static, and with extra --source arguments each select a different
// set of skills. ai-rulez.lock pins the served
// skills of each view under the view's key, so `lock --role backend` and
// `mcp --serve-skills --role backend --frozen` agree on what is pinned.

const (
	viewRole    = "role:"
	viewProfile = "profile:"
	viewTarget  = "targets:"
	viewSource  = "source:"
	viewStatic  = "static"
	viewSep     = "+"
)

// ViewKey names the view the setup serves: "" for the default view, otherwise
// the parts that select skills, in a fixed order, joined by "+":
// "role:backend+targets:cursor+static+source:cli-skills".
func (st *ServeSetup) ViewKey() string {
	var parts []string
	switch {
	case st.Role != "":
		parts = append(parts, viewRole+st.Role)
	case st.Profile != "":
		parts = append(parts, viewProfile+st.Profile)
	}
	if st.Preset != "" {
		parts = append(parts, viewTarget+st.Preset)
	}
	if st.IncludeStatic {
		parts = append(parts, viewStatic)
	}
	var sources []string
	for _, arg := range st.Sources {
		if spec, err := skillsource.ParseArg(arg); err == nil {
			sources = append(sources, viewSource+spec.Name)
		} else {
			sources = append(sources, viewSource+arg)
		}
	}
	sort.Strings(sources)
	return strings.Join(append(parts, sources...), viewSep)
}

// withView returns a copy of the setup that serves the view key names. ok is
// false for a key that cannot be rebuilt without the command line: one that names
// a --source, whose location the lock does not record.
func (st ServeSetup) withView(key string) (ServeSetup, bool) {
	st.Role, st.Profile, st.Preset, st.IncludeStatic, st.Sources = "", "", "", false, nil
	for _, part := range strings.Split(key, viewSep) {
		switch {
		case part == viewStatic:
			st.IncludeStatic = true
		case strings.HasPrefix(part, viewRole):
			st.Role = strings.TrimPrefix(part, viewRole)
		case strings.HasPrefix(part, viewProfile):
			st.Profile = strings.TrimPrefix(part, viewProfile)
		case strings.HasPrefix(part, viewTarget):
			st.Preset = strings.TrimPrefix(part, viewTarget)
		default:
			return st, false
		}
	}
	return st, key != ""
}

// namesRemovedRole reports whether view selects a role that is not in roles: a
// role deleted from the config, whose recorded pins `lock` drops rather than
// rebuilds, so a lock holding them is no reason to warn.
func namesRemovedRole(roles []string, view ServeSetup) bool {
	return view.Role != "" && !slices.Contains(roles, view.Role)
}

// recordedViews lists the non-default views the lock holds served pins for, in
// order.
func recordedViews(lock *lockfile.File) []string {
	if lock == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range lock.Served {
		if e.View != "" && !seen[e.View] {
			seen[e.View] = true
			out = append(out, e.View)
		}
	}
	sort.Strings(out)
	return out
}

// ViewKeySources lists the source names a view key refers to.
func ViewKeySources(key string) []string {
	var out []string
	for _, part := range strings.Split(key, viewSep) {
		if name, ok := strings.CutPrefix(part, viewSource); ok {
			out = append(out, name)
		}
	}
	return out
}
