package mcp

import (
	"path"
	"strings"
	"sync"
	"time"
)

// Default limits of the dynamic-loading tools.
const (
	// DefaultSessionBudgetBytes caps the bytes load_skill returns in one
	// session (about 64k tokens): enough for a dozen skills, not for the catalog.
	DefaultSessionBudgetBytes = 256 * 1024
	// UnlimitedBudget disables the session cap.
	UnlimitedBudget = -1

	defaultFindLimit = 5
	maxFindLimit     = 20
)

// RoleScope is what a role sees: the skills find_skill ranks first for it, and
// the per-skill delivery it overrides.
type RoleScope struct {
	// Domains keeps skills of these domains; root skills (no domain) always apply.
	Domains []string
	// Allow keeps skills whose name matches one of these path.Match patterns.
	Allow []string
	// Deny drops skills whose name matches one of these patterns; it wins over Allow.
	Deny []string
	// Keep, when set, decides for a skill (its domain, "" for root, and name)
	// whether the role keeps it; it is what RolesFromConfig provides.
	Keep func(domain, name string) bool
	// Delivery maps a skill ("<domain>/<name>", or the name alone for a root
	// skill) to static/served/both. It is the per-role override
	// config.Config.EffectiveDelivery accepts.
	Delivery map[string]string
}

// RoleResolver maps a role name to its scope. ok is false for an unknown role.
// RolesFromConfig resolves the project's [[roles]].
type RoleResolver func(role string) (scope RoleScope, ok bool)

// Includes reports whether a skill is in the role's scope.
func (r RoleScope) Includes(s *CatalogSkill) bool {
	if r.Keep != nil && !r.Keep(s.Domain, s.Name) {
		return false
	}
	if matchesAny(r.Deny, s.Name) {
		return false
	}
	if len(r.Allow) > 0 && !matchesAny(r.Allow, s.Name) {
		return false
	}
	if len(r.Domains) > 0 && s.Domain != "" && !containsString(r.Domains, s.Domain) {
		return false
	}
	return true
}

// SessionTelemetry describes one successful load_skill, identifiers only.
type SessionTelemetry struct {
	Skill   string
	Digest  string
	Session string
	Client  string
	// Resource is true when a supporting file, not SKILL.md, was loaded.
	Resource bool
	// Role is the role the server was started with ("" when none).
	Role string
}

// ServeOptions configures the dynamic-loading surface of a skills server. The
// zero value serves with the default session budget, no roles, no telemetry and
// no live reload.
type ServeOptions struct {
	// Role is the default role of find_skill; the tool's role argument overrides it.
	Role string
	// Roles resolves role names; nil means no role is known.
	Roles RoleResolver
	// BudgetBytes caps the bytes load_skill returns per session. 0 selects
	// DefaultSessionBudgetBytes; UnlimitedBudget removes the cap.
	BudgetBytes int
	// Telemetry receives every successful load_skill; nil disables it.
	Telemetry func(SessionTelemetry)
	// Rebuild recomputes the catalog for a live reload; nil disables reload.
	Rebuild func() (*Catalog, error)
	// Fingerprint summarizes the files the catalog is built from; a different
	// value means Rebuild should run. Required with Rebuild.
	Fingerprint func() (string, error)
	// Baseline is the fingerprint taken before the catalog was built; Watch
	// compares against it, so an edit made while the server starts is not missed.
	// Empty means Watch takes its own first fingerprint.
	Baseline string
	// PollInterval is how often Fingerprint is checked; 0 selects two seconds.
	PollInterval time.Duration
}

func (o ServeOptions) budget() int {
	switch {
	case o.BudgetBytes == 0:
		return DefaultSessionBudgetBytes
	case o.BudgetBytes < 0:
		return UnlimitedBudget
	}
	return o.BudgetBytes
}

type serveState struct {
	opts ServeOptions

	mu   sync.Mutex
	used map[string]int // bytes returned per session
}

func newServeState(opts ServeOptions) *serveState {
	return &serveState{opts: opts, used: map[string]int{}}
}

// charge adds n bytes to the session's usage. It fails without charging when the
// cap would be exceeded. remaining is -1 when the cap is off.
func (st *serveState) charge(session string, n int) (remaining int, ok bool) {
	limit := st.opts.budget()
	st.mu.Lock()
	defer st.mu.Unlock()
	if limit == UnlimitedBudget {
		st.used[session] += n
		return UnlimitedBudget, true
	}
	left := limit - st.used[session]
	if n > left {
		return left, false
	}
	st.used[session] += n
	return left - n, true
}

// Used returns the bytes load_skill has returned to a session so far.
func (s *Server) Used(session string) int {
	s.serve.mu.Lock()
	defer s.serve.mu.Unlock()
	return s.serve.used[session]
}

func (s *Server) cat() *Catalog {
	s.catMu.RLock()
	defer s.catMu.RUnlock()
	return s.catalog
}

// scope resolves a role name through the configured resolver.
func (st *serveState) scope(role string) (RoleScope, bool) {
	if role == "" || st.opts.Roles == nil {
		return RoleScope{}, false
	}
	return st.opts.Roles(role)
}

// cleanRelPath normalizes a skill-relative path argument and rejects anything
// that could leave the skill: absolute paths, "..", backslashes and NULs.
func cleanRelPath(p string) (string, bool) {
	if p == "" {
		return skillMarkdown, true
	}
	if strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "/") {
		return "", false
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}
