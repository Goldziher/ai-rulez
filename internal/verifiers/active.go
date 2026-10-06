package verifiers

import (
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const defaultProfile = "default"

// activeSet is the content a run considers active: the tree of the selected
// profile or role, and a label for messages. A nil tree means everything is active.
type activeSet struct {
	tree  *config.ContentTree
	label string
}

// activeContent resolves the active profile or role the way `generate` does:
// the explicit --role, else the profile (--profile, else the configured
// default, else the built-in "default"). With no profiles defined every domain
// is active, so a project without profiles never has an inactive verifier.
func activeContent(cfg *config.Config, profile, role string) (activeSet, error) {
	if cfg.Content == nil {
		return activeSet{}, nil
	}
	if role != "" {
		flat, err := cfg.FlattenRole(role)
		if err != nil {
			return activeSet{}, oops.Wrap(err)
		}
		tree, err := cfg.FilterTreeForRole(cfg.Content, &flat.RoleConfig)
		if err != nil {
			return activeSet{}, oops.Wrap(err)
		}
		return activeSet{tree: tree, label: "role " + quote(role)}, nil
	}
	name := profile
	if name == "" {
		name = cfg.Default
	}
	if name == "" {
		name = defaultProfile
	}
	name = config.CanonicalProfile(name)
	label := "profile " + quote(name)
	if name == defaultProfile && !cfg.HasProfile(defaultProfile) {
		if len(cfg.Profiles) == 0 {
			return activeSet{}, nil
		}
		tree, err := cfg.SelectContentForDomains(cfg.Content, nil)
		return activeSet{tree: tree, label: label}, oops.Wrap(err)
	}
	if !cfg.HasProfile(name) {
		return activeSet{}, oops.With("profile", name).Hint("Use a profile defined under [profiles], or `default`.").
			Errorf("profile %q is not defined", name)
	}
	tree, err := cfg.SelectContentForProfile(cfg.Content, name)
	return activeSet{tree: tree, label: label}, oops.Wrap(err)
}

// inactive reports whether the item a spec enforces exists but is outside the
// active set. A target that does not exist at all is a declaration problem
// (AR9H2) found by LoadSpecs, not an inactive verifier.
func (a activeSet) inactive(cfg *config.Config, sp *Spec) (string, bool) {
	if a.tree == nil {
		return "", false
	}
	kind, id := sp.TargetKind()
	if _, ok := findTargetIn(a.tree, kind, id); ok {
		return "", false
	}
	if _, ok := findTargetIn(cfg.Content, kind, id); !ok {
		return "", false
	}
	return kind + " " + quote(id) + " is not in the active " + a.label, true
}
