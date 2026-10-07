package forge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/samber/oops"
)

// Limits. They bound what a hostile or broken server can make the client read.
const (
	// DefaultMaxBody caps one JSON response body.
	DefaultMaxBody = 4 << 20
	// MaxCodeownersBody caps a CODEOWNERS file (GitHub itself allows 3 MB; a
	// larger one is not read).
	MaxCodeownersBody = 3 << 20
	// DefaultMaxPages caps the pages of one listing (100 items each).
	DefaultMaxPages = 10
	perPage         = 100
	maxRedirects    = 3
	defaultTimeout  = 30 * time.Second
	ghTokenTimeout  = 10 * time.Second
)

// Token environment variables, in order of precedence.
var tokenEnvs = []string{"GITHUB_TOKEN", "GH_TOKEN"}

// HostsEnv lists, comma separated, the hosts the forge client may contact (and
// send the GitHub token to). It is separate from the clone token allowlist: a
// GitLab or Gitea host trusted with a clone token must not receive the GitHub
// token, and does not speak this API.
const HostsEnv = "AI_RULEZ_FORGE_HOSTS"

// Options configure an HTTPClient. The zero value is a working online client
// for the real process environment.
type Options struct {
	// Host supplies the environment (token, allowlist), clock and runner (`gh`).
	Host ambient.Host
	// Offline makes every call return ErrOffline without a request.
	Offline bool
	// HTTP is the transport to use; nil builds one with a timeout. Tests pass
	// the httptest TLS server's client. Whatever it is, redirects are
	// restricted to the request's own host.
	HTTP *http.Client
	// APIBase overrides the API endpoint (default https://api.github.com for
	// github.com, https://<host>/api/v3 otherwise). It must be https. It exists
	// for tests and is never read from a config file.
	APIBase string
	// MaxBody and MaxPages override DefaultMaxBody and DefaultMaxPages.
	MaxBody  int64
	MaxPages int
}

// HTTPClient is the GitHub REST implementation of Client. It is safe for
// concurrent use.
type HTTPClient struct {
	opt  Options
	http *http.Client

	mu     sync.Mutex
	tokens map[string]string // host -> token, "" when none; resolved once per host
}

var _ Client = (*HTTPClient)(nil)

// NewClient builds a client from opt.
func NewClient(opt Options) *HTTPClient {
	if opt.MaxBody <= 0 {
		opt.MaxBody = DefaultMaxBody
	}
	if opt.MaxPages <= 0 {
		opt.MaxPages = DefaultMaxPages
	}
	hc := &http.Client{Timeout: defaultTimeout}
	if opt.HTTP != nil {
		copied := *opt.HTTP
		hc = &copied
	}
	hc.CheckRedirect = sameHostRedirect
	return &HTTPClient{opt: opt, http: hc, tokens: map[string]string{}}
}

// sameHostRedirect follows a redirect only to https on the host that was asked:
// a redirect elsewhere is where a token would leak.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	first := via[0].URL
	if !strings.EqualFold(req.URL.Scheme, "https") || !strings.EqualFold(req.URL.Host, first.Host) {
		return fmt.Errorf("refusing a redirect from %s to %s://%s", first.Host, req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// apiBase is the API endpoint of host.
func (c *HTTPClient) apiBase(host string) string {
	switch {
	case c.opt.APIBase != "":
		return strings.TrimRight(c.opt.APIBase, "/")
	case host == "github.com":
		return "https://api.github.com"
	default:
		return "https://" + host + "/api/v3"
	}
}

// Hosts returns the hosts the forge client may contact: HostsEnv, else github.com.
func Hosts(h ambient.Host) []string {
	var hosts []string
	for _, v := range strings.Split(h.GetEnv(HostsEnv), ",") {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			hosts = append(hosts, v)
		}
	}
	if len(hosts) == 0 {
		return []string{"github.com"}
	}
	return hosts
}

// allowed reports whether host may be contacted: the forge host allowlist.
func (c *HTTPClient) allowed(host string) bool {
	for _, h := range Hosts(c.opt.Host) {
		if h == host {
			return true
		}
	}
	return false
}

// token returns the credential for host: GITHUB_TOKEN, GH_TOKEN, else the
// output of `gh auth token --hostname <host>`; "" means anonymous.
func (c *HTTPClient) token(ctx context.Context, host string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tokens[host]; ok {
		return t
	}
	t := c.lookupToken(ctx, host)
	c.tokens[host] = t
	return t
}

func (c *HTTPClient) lookupToken(ctx context.Context, host string) string {
	for _, name := range tokenEnvs {
		if v := strings.TrimSpace(c.opt.Host.GetEnv(name)); v != "" {
			return v
		}
	}
	h := c.opt.Host
	// gh needs its config, not the process's credentials (those were checked above).
	var env []string
	for _, name := range []string{"PATH", "HOME", "USERPROFILE", "APPDATA", "XDG_CONFIG_HOME", "GH_CONFIG_DIR", "LANG"} {
		if v := h.GetEnv(name); v != "" {
			env = append(env, name+"="+v)
		}
	}
	res := h.Run().Run(ctx, runner.Spec{
		Argv:      []string{"gh", "auth", "token", "--hostname", host},
		Env:       env,
		Timeout:   ghTokenTimeout,
		MaxOutput: 4096,
	})
	if res.Status != runner.StatusOK {
		return ""
	}
	return strings.TrimSpace(string(res.Stdout))
}

// request is one GET, with the rate-limit and status mapping applied.
type request struct {
	host   string
	path   string // below the API base, starts with "/"
	query  url.Values
	accept string
	max    int64
}

// do performs req and returns the body and the response headers.
func (c *HTTPClient) do(ctx context.Context, req request, rawURL string) ([]byte, http.Header, error) {
	if c.opt.Offline {
		return nil, nil, ErrOffline
	}
	if !c.allowed(req.host) {
		return nil, nil, oops.With("host", req.host).Hint("Add the host to " + HostsEnv).Wrap(ErrHostNotAllowed)
	}
	target := rawURL
	if target == "" {
		target = c.apiBase(req.host) + req.path
		if len(req.query) > 0 {
			target += "?" + req.query.Encode()
		}
	}
	u, err := url.Parse(target)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return nil, nil, oops.Errorf("forge: refusing a non-https request")
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "build forge request")
	}
	accept := req.accept
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	hreq.Header.Set("Accept", accept)
	hreq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	hreq.Header.Set("User-Agent", "ai-rulez")
	if tok := c.token(ctx, req.host); tok != "" {
		hreq.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(hreq)
	if err != nil {
		// net/http errors embed the URL, never the headers: the token cannot appear.
		return nil, nil, oops.With("host", req.host).Wrapf(err, "forge request")
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	limit := req.max
	if limit <= 0 {
		limit = c.opt.MaxBody
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, oops.Wrapf(err, "read forge response")
	}
	if int64(len(body)) > limit {
		return nil, nil, oops.With("limit", limit).Wrap(ErrTooLarge)
	}
	if err := statusError(resp, c.token(ctx, req.host) != ""); err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}

func statusError(resp *http.Response, hadToken bool) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		rl := &RateLimitError{}
		if n, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && n > 0 {
			rl.Reset = time.Unix(n, 0)
		}
		return rl
	case resp.StatusCode == http.StatusForbidden:
		if !hadToken {
			return ErrUnauthorized
		}
		return ErrForbidden
	}
	return oops.Errorf("forge: unexpected status %d", resp.StatusCode)
}

func getJSON[T any](ctx context.Context, c *HTTPClient, req request) (T, error) {
	var out T
	body, _, err := c.do(ctx, req, "")
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, oops.Wrapf(err, "decode forge response")
	}
	return out, nil
}

// listJSON reads a paginated listing, following Link rel="next" within the page
// cap. The next link must stay on the same host over https.
func listJSON[T any](ctx context.Context, c *HTTPClient, req request) ([]T, error) {
	req.query = withPerPage(req.query)
	var all []T
	next := ""
	for page := 1; ; page++ {
		body, hdr, err := c.do(ctx, req, next)
		if err != nil {
			return all, err
		}
		var items []T
		if err := json.Unmarshal(body, &items); err != nil {
			return all, oops.Wrapf(err, "decode forge listing")
		}
		all = append(all, items...)
		link := nextLink(hdr.Get("Link"))
		if link == "" {
			return all, nil
		}
		if page >= c.opt.MaxPages {
			return all, ErrTruncated
		}
		if err := c.sameAPIHost(req.host, link); err != nil {
			return all, err
		}
		next = link
	}
}

func withPerPage(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = v
	}
	out.Set("per_page", strconv.Itoa(perPage))
	return out
}

// sameAPIHost refuses a pagination link that leaves the API endpoint.
func (c *HTTPClient) sameAPIHost(host, link string) error {
	base, err := url.Parse(c.apiBase(host))
	if err != nil {
		return oops.Wrapf(err, "parse API base")
	}
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Host, base.Host) {
		return oops.Errorf("forge: refusing a pagination link to another host")
	}
	return nil
}

// nextLink extracts the rel="next" URL of a Link header.
func nextLink(h string) string {
	for _, part := range strings.Split(h, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(target), "<>")
	}
	return ""
}

func (c *HTTPClient) repoPath(repo Repo, rest string) (request, error) {
	if err := repo.Valid(); err != nil {
		return request{}, err
	}
	return request{host: repo.Host, path: "/repos/" + repo.Owner + "/" + repo.Name + rest}, nil
}

// escapeTag escapes a tag for a URL path; slashes stay (monorepo tags such as deploy/v2.1.3).
func escapeTag(tag string) (string, error) {
	if tag == "" || len(tag) > 255 || strings.Contains(tag, "..") || strings.HasPrefix(tag, "/") || strings.HasSuffix(tag, "/") {
		return "", fmt.Errorf("%w: invalid tag %q", ErrUnsupportedSource, tag)
	}
	for _, r := range tag {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: invalid tag", ErrUnsupportedSource)
		}
	}
	parts := strings.Split(tag, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/"), nil
}

func checkSHA(sha string) error {
	if !shaRe.MatchString(sha) {
		return fmt.Errorf("%w: invalid commit id", ErrUnsupportedSource)
	}
	return nil
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Target      string    `json:"target_commitish"`
}

func (r ghRelease) release() Release {
	rel := Release{Tag: r.TagName, Name: r.Name, Prerelease: r.Prerelease, Published: r.PublishedAt}
	if shaRe.MatchString(r.Target) {
		rel.Commit = r.Target
	}
	return rel
}

// Releases implements Client.
func (c *HTTPClient) Releases(ctx context.Context, repo Repo) ([]Release, error) {
	req, err := c.repoPath(repo, "/releases")
	if err != nil {
		return nil, err
	}
	items, err := listJSON[ghRelease](ctx, c, req)
	out := make([]Release, 0, len(items))
	for _, it := range items {
		if !it.Draft && !it.PublishedAt.IsZero() {
			out = append(out, it.release())
		}
	}
	return out, err
}

// Release implements Client.
func (c *HTTPClient) Release(ctx context.Context, repo Repo, tag string) (Release, error) {
	esc, err := escapeTag(tag)
	if err != nil {
		return Release{}, err
	}
	req, err := c.repoPath(repo, "/releases/tags/"+esc)
	if err != nil {
		return Release{}, err
	}
	r, err := getJSON[ghRelease](ctx, c, req)
	if err != nil {
		return Release{}, err
	}
	if r.Draft || r.PublishedAt.IsZero() {
		return Release{}, ErrNotFound
	}
	return r.release(), nil
}

type ghObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type ghSig struct {
	Date time.Time `json:"date"`
}

// Tag implements Client.
func (c *HTTPClient) Tag(ctx context.Context, repo Repo, tag string) (TagInfo, error) {
	esc, err := escapeTag(tag)
	if err != nil {
		return TagInfo{}, err
	}
	req, err := c.repoPath(repo, "/git/ref/tags/"+esc)
	if err != nil {
		return TagInfo{}, err
	}
	ref, err := getJSON[struct {
		Object ghObject `json:"object"`
	}](ctx, c, req)
	if err != nil {
		return TagInfo{}, err
	}
	info := TagInfo{Name: tag}
	obj := ref.Object
	// An annotated tag points to a tag object; peel it (a tag of a tag is followed, bounded).
	for depth := 0; obj.Type == "tag" && depth < 3; depth++ {
		if err := checkSHA(obj.SHA); err != nil {
			return TagInfo{}, err
		}
		if info.TagObject == "" {
			info.TagObject = obj.SHA
		}
		treq, err := c.repoPath(repo, "/git/tags/"+obj.SHA)
		if err != nil {
			return TagInfo{}, err
		}
		t, err := getJSON[struct {
			Tagger ghSig    `json:"tagger"`
			Object ghObject `json:"object"`
		}](ctx, c, treq)
		if err != nil {
			return TagInfo{}, err
		}
		if info.Tagged.IsZero() {
			info.Tagged = t.Tagger.Date
		}
		obj = t.Object
	}
	if obj.Type != "commit" {
		return TagInfo{}, fmt.Errorf("%w: tag %q does not point to a commit", ErrNotFound, tag)
	}
	if err := checkSHA(obj.SHA); err != nil {
		return TagInfo{}, err
	}
	info.Commit = obj.SHA
	return info, nil
}

// CommitDate implements Client.
func (c *HTTPClient) CommitDate(ctx context.Context, repo Repo, sha string) (time.Time, error) {
	if err := checkSHA(sha); err != nil {
		return time.Time{}, err
	}
	req, err := c.repoPath(repo, "/commits/"+sha)
	if err != nil {
		return time.Time{}, err
	}
	cm, err := getJSON[struct {
		Commit struct {
			Committer ghSig `json:"committer"`
			Author    ghSig `json:"author"`
		} `json:"commit"`
	}](ctx, c, req)
	if err != nil {
		return time.Time{}, err
	}
	if d := cm.Commit.Committer.Date; !d.IsZero() {
		return d, nil
	}
	if d := cm.Commit.Author.Date; !d.IsZero() {
		return d, nil
	}
	return time.Time{}, fmt.Errorf("%w: commit has no date", ErrNotFound)
}

type ghUser struct {
	Login string `json:"login"`
}

// PullRequestsForCommit implements Client.
func (c *HTTPClient) PullRequestsForCommit(ctx context.Context, repo Repo, sha string) ([]PullRequest, error) {
	if err := checkSHA(sha); err != nil {
		return nil, err
	}
	req, err := c.repoPath(repo, "/commits/"+sha+"/pulls")
	if err != nil {
		return nil, err
	}
	items, err := listJSON[struct {
		Number     int        `json:"number"`
		State      string     `json:"state"`
		MergedAt   *time.Time `json:"merged_at"`
		User       ghUser     `json:"user"`
		MergeSHA   string     `json:"merge_commit_sha"`
		Base, Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}
	}](ctx, c, req)
	out := make([]PullRequest, 0, len(items))
	for i := range items {
		it := &items[i]
		pr := PullRequest{Number: it.Number, State: it.State, Merged: it.MergedAt != nil, Author: it.User.Login,
			BaseRef: it.Base.Ref, HeadSHA: it.Head.SHA}
		if pr.Merged {
			pr.MergeCommit = it.MergeSHA
		}
		out = append(out, pr)
	}
	return out, err
}

// PullRequest implements Client.
func (c *HTTPClient) PullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error) {
	if number <= 0 {
		return PullRequest{}, fmt.Errorf("%w: invalid pull request number", ErrUnsupportedSource)
	}
	req, err := c.repoPath(repo, "/pulls/"+strconv.Itoa(number))
	if err != nil {
		return PullRequest{}, err
	}
	it, err := getJSON[struct {
		Number     int        `json:"number"`
		State      string     `json:"state"`
		MergedAt   *time.Time `json:"merged_at"`
		User       ghUser     `json:"user"`
		MergeSHA   string     `json:"merge_commit_sha"`
		Base, Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}
	}](ctx, c, req)
	if err != nil {
		return PullRequest{}, err
	}
	pr := PullRequest{Number: it.Number, State: it.State, Merged: it.MergedAt != nil, Author: it.User.Login,
		BaseRef: it.Base.Ref, HeadSHA: it.Head.SHA}
	if pr.Merged {
		pr.MergeCommit = it.MergeSHA
	}
	return pr, nil
}

// Reviews implements Client.
func (c *HTTPClient) Reviews(ctx context.Context, repo Repo, pr int) ([]Review, error) {
	if pr <= 0 {
		return nil, fmt.Errorf("%w: invalid pull request number", ErrUnsupportedSource)
	}
	req, err := c.repoPath(repo, "/pulls/"+strconv.Itoa(pr)+"/reviews")
	if err != nil {
		return nil, err
	}
	items, err := listJSON[struct {
		ID          int64     `json:"id"`
		User        ghUser    `json:"user"`
		State       string    `json:"state"`
		CommitID    string    `json:"commit_id"`
		SubmittedAt time.Time `json:"submitted_at"`
		Association string    `json:"author_association"`
	}](ctx, c, req)
	out := make([]Review, 0, len(items))
	for _, it := range items {
		out = append(out, Review{ID: it.ID, Login: it.User.Login, State: strings.ToUpper(it.State), CommitID: it.CommitID, Submitted: it.SubmittedAt, AuthorAssociation: strings.ToUpper(it.Association)})
	}
	return out, err
}

// codeownersPaths is where GitHub looks for CODEOWNERS, in order.
var codeownersPaths = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// Codeowners implements Client.
func (c *HTTPClient) Codeowners(ctx context.Context, repo Repo, ref string) (Codeowners, error) {
	if ref != "" && (strings.ContainsAny(ref, " \t\n\r~^:?*[\\") || strings.Contains(ref, "..") || strings.HasPrefix(ref, "-")) {
		return Codeowners{}, fmt.Errorf("%w: invalid ref %q", ErrUnsupportedSource, ref)
	}
	for _, p := range codeownersPaths {
		req, err := c.repoPath(repo, "/contents/"+p)
		if err != nil {
			return Codeowners{}, err
		}
		req.accept, req.max = "application/vnd.github.raw+json", MaxCodeownersBody
		if ref != "" {
			req.query = url.Values{"ref": {ref}}
		}
		body, _, err := c.do(ctx, req, "")
		switch {
		case err == nil:
			return Codeowners{Path: p, Content: body}, nil
		case errors.Is(err, ErrNotFound):
			continue
		default:
			return Codeowners{}, err
		}
	}
	return Codeowners{}, fmt.Errorf("%w: no CODEOWNERS file in %s", ErrNotFound, repo)
}

func (c *HTTPClient) teamPath(t Team, rest string) (request, error) {
	if err := t.Valid(); err != nil {
		return request{}, err
	}
	return request{host: t.Host, path: "/orgs/" + t.Org + "/teams/" + t.Slug + rest}, nil
}

// TeamMembers implements Client.
func (c *HTTPClient) TeamMembers(ctx context.Context, team Team) ([]string, error) {
	req, err := c.teamPath(team, "/members")
	if err != nil {
		return nil, err
	}
	items, err := listJSON[ghUser](ctx, c, req)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Login)
	}
	return out, err
}

// CollaboratorPermission implements Client.
func (c *HTTPClient) CollaboratorPermission(ctx context.Context, repo Repo, login string) (string, error) {
	if !userRe.MatchString(login) {
		return "", fmt.Errorf("%w: invalid login", ErrUnsupportedSource)
	}
	req, err := c.repoPath(repo, "/collaborators/"+login+"/permission")
	if err != nil {
		return "", err
	}
	p, err := getJSON[struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}](ctx, c, req)
	if errors.Is(err, ErrNotFound) {
		return PermissionNone, nil
	}
	if err != nil {
		return "", err
	}
	// "permission" folds maintain into write and triage into read; role_name is exact.
	if role := strings.ToLower(p.RoleName); role == PermissionMaintain || role == PermissionTriage {
		return role, nil
	}
	return strings.ToLower(p.Permission), nil
}

// IsTeamMember implements Client.
func (c *HTTPClient) IsTeamMember(ctx context.Context, team Team, login string) (bool, error) {
	if !userRe.MatchString(login) {
		return false, fmt.Errorf("%w: invalid login", ErrUnsupportedSource)
	}
	req, err := c.teamPath(team, "/memberships/"+login)
	if err != nil {
		return false, err
	}
	m, err := getJSON[struct {
		State string `json:"state"`
	}](ctx, c, req)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.State == "active", nil
}
