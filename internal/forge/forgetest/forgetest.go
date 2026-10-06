// Package forgetest serves the GitHub REST endpoints internal/forge reads from
// a local TLS server, so the real HTTP client is tested without a network.
package forgetest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Request is one request the server saw.
type Request struct {
	Path, RawQuery, Authorization, Accept string
}

// Server is a fake GitHub API for the repository github.com/o/r (any owner and
// name are accepted; data is keyed by path below /repos/<o>/<r> or /orgs/...).
// Set the fields before the first request.
type Server struct {
	*httptest.Server

	// Token, when set, is the bearer token the server requires (401 otherwise).
	Token string
	// PageSize splits listings into pages of this many items (0: one page).
	PageSize int
	// Releases are served newest first. Draft releases are served with draft = true.
	Releases []Release
	// Tags by tag name.
	Tags map[string]Tag
	// Commits by sha: the committer date.
	Commits map[string]time.Time
	// PRs by commit sha.
	PRs map[string][]forge.PullRequest
	// Reviews by pull request number.
	Reviews map[int][]forge.Review
	// Owners by file path ("CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS").
	Owners map[string]string
	// Teams by "org/slug": the active members.
	Teams map[string][]string
	// Status forces a response status for paths with this prefix (rate limiting, 500).
	Status map[string]int
	// RateLimited adds spent-rate-limit headers to the forced statuses (a 403 then is a rate limit).
	RateLimited bool
	// Raw replaces the body of a path with this prefix (oversized or malformed bodies).
	Raw map[string][]byte
	// NextLink forces this Link rel="next" target on every listing page.
	NextLink string
	// Redirect maps a path prefix to a Location to answer with a 302.
	Redirect map[string]string

	mu       sync.Mutex
	requests []Request
}

// Release is a served release.
type Release struct {
	Tag, Name         string
	Draft, Prerelease bool
	Published         time.Time
}

// Tag is a served tag; a non-empty Object makes it annotated.
type Tag struct {
	Commit, Object string
	Tagged         time.Time
}

// New starts the server; it is closed when t ends.
func New(t interface {
	Cleanup(func())
	Helper()
}) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Requests returns what the server saw, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Client returns a forge client wired to the server: github.com repositories
// are served here, no process is started (so a developer's `gh` login is never used), the token comes from GITHUB_TOKEN of env, and the allowlist
// is the default (github.com).
func (s *Server) Client(env map[string]string, mutate ...func(*forge.Options)) *forge.HTTPClient {
	opt := forge.Options{Host: ambient.Host{Env: ambient.MapEnv{Vars: env, Home: "/nonexistent"}, Runner: runner.Deny{}}, HTTP: s.Server.Client(), APIBase: s.URL}
	for _, m := range mutate {
		m(&opt)
	}
	return forge.NewClient(opt)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, Request{r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Accept")})
	s.mu.Unlock()
	if s.Token != "" && r.Header.Get("Authorization") != "Bearer "+s.Token {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	for prefix, loc := range s.Redirect {
		if strings.HasPrefix(r.URL.Path, prefix) {
			http.Redirect(w, r, loc, http.StatusFound)
			return
		}
	}
	for prefix, code := range s.Status {
		if strings.HasPrefix(r.URL.Path, prefix) {
			if s.RateLimited || code == http.StatusTooManyRequests {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1900000000")
			}
			http.Error(w, `{"message":"forced"}`, code)
			return
		}
	}
	for prefix, body := range s.Raw {
		if strings.HasPrefix(r.URL.Path, prefix) {
			_, _ = w.Write(body)
			return
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) >= 4 && parts[0] == "repos":
		s.repo(w, r, parts[3:])
	case len(parts) >= 4 && parts[0] == "orgs" && parts[2] == "teams":
		s.team(w, r, parts[1]+"/"+parts[3], parts[4:])
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) repo(w http.ResponseWriter, r *http.Request, rest []string) {
	path := strings.Join(rest, "/")
	switch {
	case path == "releases":
		var out []map[string]any
		for _, rel := range s.Releases {
			out = append(out, releaseJSON(rel))
		}
		s.list(w, r, out)
	case strings.HasPrefix(path, "releases/tags/"):
		tag := strings.TrimPrefix(path, "releases/tags/")
		for _, rel := range s.Releases {
			if rel.Tag == tag {
				writeJSON(w, releaseJSON(rel))
				return
			}
		}
		http.NotFound(w, r)
	case strings.HasPrefix(path, "git/ref/tags/"):
		name := strings.TrimPrefix(path, "git/ref/tags/")
		t, ok := s.Tags[name]
		switch {
		case !ok:
			http.NotFound(w, r)
		case t.Object != "":
			writeJSON(w, map[string]any{"object": map[string]any{"type": "tag", "sha": t.Object}})
		default:
			writeJSON(w, map[string]any{"object": map[string]any{"type": "commit", "sha": t.Commit}})
		}
	case strings.HasPrefix(path, "git/tags/"):
		obj := strings.TrimPrefix(path, "git/tags/")
		for _, t := range s.Tags {
			if t.Object == obj {
				writeJSON(w, map[string]any{"tagger": map[string]any{"date": t.Tagged}, "object": map[string]any{"type": "commit", "sha": t.Commit}})
				return
			}
		}
		http.NotFound(w, r)
	case strings.HasPrefix(path, "commits/") && strings.HasSuffix(path, "/pulls"):
		sha := strings.TrimSuffix(strings.TrimPrefix(path, "commits/"), "/pulls")
		var out []map[string]any
		for _, pr := range s.PRs[sha] {
			m := map[string]any{"number": pr.Number, "state": pr.State, "user": map[string]any{"login": pr.Author},
				"base": map[string]any{"ref": pr.BaseRef}, "head": map[string]any{"sha": pr.HeadSHA}, "merge_commit_sha": pr.MergeCommit}
			if pr.Merged {
				m["merged_at"] = time.Unix(1, 0).UTC()
			}
			out = append(out, m)
		}
		s.list(w, r, out)
	case strings.HasPrefix(path, "commits/"):
		sha := strings.TrimPrefix(path, "commits/")
		d, ok := s.Commits[sha]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{"commit": map[string]any{"committer": map[string]any{"date": d}, "author": map[string]any{"date": d}}})
	case strings.HasPrefix(path, "pulls/") && !strings.Contains(strings.TrimPrefix(path, "pulls/"), "/"):
		n, err := strconv.Atoi(strings.TrimPrefix(path, "pulls/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		for _, prs := range s.PRs {
			for _, pr := range prs {
				if pr.Number != n {
					continue
				}
				m := map[string]any{"number": pr.Number, "state": pr.State, "user": map[string]any{"login": pr.Author},
					"base": map[string]any{"ref": pr.BaseRef}, "head": map[string]any{"sha": pr.HeadSHA}, "merge_commit_sha": pr.MergeCommit}
				if pr.Merged {
					m["merged_at"] = time.Unix(1, 0).UTC()
				}
				writeJSON(w, m)
				return
			}
		}
		http.NotFound(w, r)
	case strings.HasPrefix(path, "pulls/") && strings.HasSuffix(path, "/reviews"):
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path, "pulls/"), "/reviews"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var out []map[string]any
		for _, rv := range s.Reviews[n] {
			out = append(out, map[string]any{"id": rv.ID, "user": map[string]any{"login": rv.Login}, "state": rv.State, "commit_id": rv.CommitID, "submitted_at": rv.Submitted})
		}
		s.list(w, r, out)
	case strings.HasPrefix(path, "contents/"):
		body, ok := s.Owners[strings.TrimPrefix(path, "contents/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) team(w http.ResponseWriter, r *http.Request, key string, rest []string) {
	members, ok := s.Teams[key]
	switch {
	case !ok:
		http.NotFound(w, r)
	case len(rest) == 1 && rest[0] == "members":
		var out []map[string]any
		for _, m := range members {
			out = append(out, map[string]any{"login": m})
		}
		s.list(w, r, out)
	case len(rest) == 2 && rest[0] == "memberships":
		for _, m := range members {
			if m == rest[1] {
				writeJSON(w, map[string]any{"state": "active"})
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// list writes one page of items, with a Link header when more follow.
func (s *Server) list(w http.ResponseWriter, r *http.Request, items []map[string]any) {
	if items == nil {
		items = []map[string]any{}
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	if s.NextLink != "" {
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, s.NextLink))
	}
	if s.PageSize > 0 {
		lo := (page - 1) * s.PageSize
		hi := lo + s.PageSize
		if lo > len(items) {
			lo = len(items)
		}
		if hi < len(items) {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?per_page=100&page=%d>; rel="next"`, s.URL, r.URL.Path, page+1))
		} else {
			hi = len(items)
		}
		items = items[lo:hi]
	}
	writeJSON(w, items)
}

func releaseJSON(r Release) map[string]any {
	return map[string]any{"tag_name": r.Tag, "name": r.Name, "draft": r.Draft, "prerelease": r.Prerelease, "published_at": r.Published}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
