package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Default limits of the dynamic-loading tools.
const (
	// DefaultSessionBudgetBytes caps the bytes load_skill returns in one
	// session (about 64k tokens): enough for a dozen skills, not for the catalog.
	DefaultSessionBudgetBytes = 256 * 1024
	// UnlimitedBudget disables the session cap.
	UnlimitedBudget = -1

	// maxTrackedSessions bounds the per-session byte counters a long-running
	// server keeps; the oldest session is forgotten first.
	maxTrackedSessions = 1024

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
	// Revalidate admits the current build again at the current time (approval
	// expiry, signing key validity) without a file change; nil disables it.
	Revalidate func() *Catalog
	// RevalidateInterval is how often Revalidate runs; 0 selects one minute.
	RevalidateInterval time.Duration
	// Search ranks find_skill with the configured mode; nil ranks lexically.
	Search *SearchRuntime
	// Log receives the server's reports (live reload); nil is the CLI's logger.
	Log logger.Logger
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
	seen []string       // sessions in first-use order, for eviction

	// ids holds the generated id of each connection whose transport has none.
	ids map[*sdkmcp.ServerSession]string
}

func newServeState(opts ServeOptions) *serveState {
	return &serveState{opts: opts, used: map[string]int{}, ids: map[*sdkmcp.ServerSession]string{}}
}

// charge adds n bytes to the session's usage. It fails without charging when the
// cap would be exceeded. remaining is -1 when the cap is off.
func (st *serveState) charge(session string, n int) (remaining int, ok bool) {
	limit := st.opts.budget()
	st.mu.Lock()
	defer st.mu.Unlock()
	if limit == UnlimitedBudget {
		st.add(session, n)
		return UnlimitedBudget, true
	}
	left := limit - st.used[session]
	if n > left {
		return left, false
	}
	st.add(session, n)
	return left - n, true
}

// add records n bytes for a session, forgetting the oldest session when more
// than maxTrackedSessions are tracked. The caller holds st.mu.
func (st *serveState) add(session string, n int) {
	if _, known := st.used[session]; !known {
		st.seen = append(st.seen, session)
		if len(st.seen) > maxTrackedSessions {
			delete(st.used, st.seen[0])
			st.seen = st.seen[1:]
		}
	}
	st.used[session] += n
}

// sessionID names a connection. A transport that has its own session id (HTTP)
// keeps it. One without (stdio) gets a random id the first time the connection
// is seen, so a usage line carries a session and the byte budget is per
// connection: a new connection starts a new budget, the same pipe keeps its own.
func (st *serveState) sessionID(sess *sdkmcp.ServerSession) string {
	if sess == nil {
		return ""
	}
	if id := sess.ID(); id != "" {
		return id
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if id, ok := st.ids[sess]; ok {
		return id
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	id := "conn-" + hex.EncodeToString(raw)
	if len(st.ids) >= maxTrackedSessions {
		clear(st.ids) // a long-lived server forgets old connections, as it does their budgets
	}
	st.ids[sess] = id
	return id
}

// chargeRead charges n bytes of skill content read outside load_skill
// (get_skill, read_skill_file, resources/read) to the session, so no read path
// bypasses the budget.
func (s *Server) chargeRead(session string, n int) error {
	if _, ok := s.serve.charge(session, n); !ok {
		return fmt.Errorf("session budget exhausted: reading %d bytes would exceed the %d-byte cap; restart the session", n, s.serve.opts.budget())
	}
	return nil
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
