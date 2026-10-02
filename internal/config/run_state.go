package config

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// ScopeRun describes the monorepo scope being generated. Rules of a scope are
// written into the root rules folders, so presets need the project root as well
// as the scope.
type ScopeRun struct {
	Path    string // scope path relative to the project root, slash form
	Slug    string // file-name qualifier derived from Path
	RootDir string // absolute project root
}

// PathClaims records which source claimed each output path during one
// generation, case-insensitively because common filesystems compare paths that
// way. Rule files of the root and of every scope land in the same rules folders,
// so one PathClaims per preset is shared by all of those runs.
type PathClaims struct {
	owner map[string]string
}

// NewPathClaims returns an empty PathClaims.
func NewPathClaims() *PathClaims {
	return &PathClaims{owner: map[string]string{}}
}

// Claim records owner as the source of outputPath. It reports the previous
// owner and false when a different source already claimed the path; claiming a
// path again for the same owner succeeds.
func (c *PathClaims) Claim(outputPath, owner string) (previous string, ok bool) {
	key := strings.ToLower(outputPath)
	if prev, dup := c.owner[key]; dup && prev != owner {
		return prev, false
	}
	c.owner[key] = owner
	return "", true
}

// RunState is the per-generation state shared by the root run and the scope
// runs. A config used by one run carries it in Config.Run.
type RunState struct {
	// Scope is the scope being generated; nil for the project root.
	Scope  *ScopeRun
	claims map[string]*PathClaims
}

// NewRunState returns the state of a generation, positioned at the project root.
func NewRunState() *RunState {
	return &RunState{claims: map[string]*PathClaims{}}
}

// ForScope returns the state for generating scope. It shares path claims with
// the receiver.
func (r *RunState) ForScope(scope *ScopeRun) *RunState {
	return &RunState{Scope: scope, claims: r.claims}
}

// ClaimsFor returns the PathClaims of a preset, creating it on first use.
func (r *RunState) ClaimsFor(preset string) *PathClaims {
	if r.claims == nil {
		r.claims = map[string]*PathClaims{}
	}
	claims, ok := r.claims[preset]
	if !ok {
		claims = NewPathClaims()
		r.claims[preset] = claims
	}
	return claims
}

// ValidateScopePath checks that a [[scopes]] path names a subdirectory of the
// project: relative, without ".." segments, and free of glob metacharacters
// because the path becomes a glob prefix and a file-name qualifier.
func ValidateScopePath(scopePath string) error {
	norm := strings.ReplaceAll(scopePath, "\\", "/")
	switch {
	case strings.TrimSpace(norm) == "":
		return oops.With("scope", scopePath).Errorf("scope path is required")
	case strings.HasPrefix(norm, "/") || filepath.IsAbs(scopePath) || filepath.VolumeName(scopePath) != "":
		return oops.With("scope", scopePath).Hint("use a path relative to the project root").
			Errorf("scope path %q must be relative", scopePath)
	case strings.ContainsAny(norm, "*?[]{},!"):
		return oops.With("scope", scopePath).
			Errorf("scope path %q must not contain glob characters (* ? [ ] { } , !)", scopePath)
	}
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return oops.With("scope", scopePath).Errorf("scope path %q must stay inside the project (no \"..\")", scopePath)
		}
	}
	if clean := path.Clean(norm); clean == "." {
		return oops.With("scope", scopePath).Errorf("scope path %q does not name a subdirectory", scopePath)
	}
	return nil
}
