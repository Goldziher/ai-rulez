package forge_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/forge/forgetest"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	repo = forge.Repo{Host: "github.com", Owner: "o", Name: "r"}
	t0   = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	sha1 = strings.Repeat("a", 40)
	sha2 = strings.Repeat("b", 40)
	tok  = "ghp_secretsecretsecret"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    forge.Repo
		wantErr bool
	}{
		{"https", "https://github.com/o/r", forge.Repo{Host: "github.com", Owner: "o", Name: "r"}, false},
		{"https .git and slash", "https://GitHub.com/o/r.git/", forge.Repo{Host: "github.com", Owner: "o", Name: "r"}, false},
		{"git+https", "git+https://github.com/o/r", forge.Repo{Host: "github.com", Owner: "o", Name: "r"}, false},
		{"userinfo is dropped", "https://user:pw@github.com/o/r", forge.Repo{Host: "github.com", Owner: "o", Name: "r"}, false},
		{"scp form", "git@github.com:o/r.git", forge.Repo{Host: "github.com", Owner: "o", Name: "r"}, false},
		{"ssh url", "ssh://git@ghe.example.com/o/r", forge.Repo{Host: "ghe.example.com", Owner: "o", Name: "r"}, false},
		{"plain http", "http://github.com/o/r", forge.Repo{}, true},
		{"git protocol", "git://github.com/o/r", forge.Repo{}, true},
		{"file", "file:///srv/repo.git", forge.Repo{}, true},
		{"local path", "../rules", forge.Repo{}, true},
		{"nested path", "https://gitlab.com/g/sub/r", forge.Repo{}, true},
		{"tree suffix", "https://github.com/o/r/tree/main", forge.Repo{}, true},
		{"dot dot owner", "https://github.com/../r", forge.Repo{}, true},
		{"missing name", "https://github.com/o", forge.Repo{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := forge.ParseRepo(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, forge.ErrUnsupportedSource)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRepoErrorsDoNotLeakCredentials(t *testing.T) {
	_, err := forge.ParseRepo("http://user:hunter2@github.com/o/r")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestReleases(t *testing.T) {
	// Arrange: a draft is never a release; pages are followed.
	srv := forgetest.New(t)
	srv.PageSize = 2
	srv.Releases = []forgetest.Release{
		{Tag: "v3", Published: t0.Add(3 * time.Hour)},
		{Tag: "v2-draft", Draft: true, Published: t0.Add(2 * time.Hour)},
		{Tag: "v2", Published: t0.Add(time.Hour), Prerelease: true},
		{Tag: "v1", Name: "first", Published: t0},
	}
	c := srv.Client(nil)

	// Act
	got, err := c.Releases(context.Background(), repo)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"v3", "v2", "v1"}, []string{got[0].Tag, got[1].Tag, got[2].Tag})
	assert.True(t, got[1].Prerelease)
	assert.True(t, got[2].Published.Equal(t0))
	assert.Len(t, srv.Requests(), 2, "one request per page")
}

func TestReleasesPageCap(t *testing.T) {
	srv := forgetest.New(t)
	srv.PageSize = 1
	for _, tag := range []string{"v5", "v4", "v3", "v2", "v1"} {
		srv.Releases = append(srv.Releases, forgetest.Release{Tag: tag, Published: t0})
	}
	c := srv.Client(nil, func(o *forge.Options) { o.MaxPages = 2 })

	got, err := c.Releases(context.Background(), repo)

	require.ErrorIs(t, err, forge.ErrTruncated)
	assert.Len(t, got, 2, "the items read before the cap are returned")
	assert.Len(t, srv.Requests(), 2)
}

func TestPaginationLinkToAnotherHostIsRefused(t *testing.T) {
	other := forgetest.New(t)
	srv := forgetest.New(t)
	srv.NextLink = other.URL + "/repos/o/r/releases?page=2"
	srv.Releases = []forgetest.Release{{Tag: "v1", Published: t0}}
	c := srv.Client(map[string]string{"GITHUB_TOKEN": tok})

	_, err := c.Releases(context.Background(), repo)

	require.Error(t, err)
	assert.Empty(t, other.Requests(), "the token must not follow a pagination link off the host")
}

func TestRelease(t *testing.T) {
	srv := forgetest.New(t)
	srv.Releases = []forgetest.Release{
		{Tag: "deploy/v2.1.3", Published: t0},
		{Tag: "draft", Draft: true, Published: t0},
	}
	c := srv.Client(nil)
	tests := []struct {
		name    string
		tag     string
		want    time.Time
		wantErr error
	}{
		{"slash tag", "deploy/v2.1.3", t0, nil},
		{"draft is not a release", "draft", time.Time{}, forge.ErrNotFound},
		{"unknown", "v9", time.Time{}, forge.ErrNotFound},
		{"dot dot", "../x", time.Time{}, forge.ErrUnsupportedSource},
		{"empty", "", time.Time{}, forge.ErrUnsupportedSource},
		{"control char", "v1\n", time.Time{}, forge.ErrUnsupportedSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Release(context.Background(), repo, tt.tag)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, got.Published.Equal(tt.want))
		})
	}
}

func TestTag(t *testing.T) {
	srv := forgetest.New(t)
	srv.Tags = map[string]forgetest.Tag{
		"light": {Commit: sha1},
		"annot": {Commit: sha2, Object: strings.Repeat("c", 40), Tagged: t0},
	}
	c := srv.Client(nil)

	light, err := c.Tag(context.Background(), repo, "light")
	require.NoError(t, err)
	assert.Equal(t, forge.TagInfo{Name: "light", Commit: sha1}, light)

	annot, err := c.Tag(context.Background(), repo, "annot")
	require.NoError(t, err)
	assert.Equal(t, sha2, annot.Commit)
	assert.Equal(t, strings.Repeat("c", 40), annot.TagObject)
	assert.True(t, annot.Tagged.Equal(t0))

	_, err = c.Tag(context.Background(), repo, "missing")
	require.ErrorIs(t, err, forge.ErrNotFound)
}

func TestCommitDate(t *testing.T) {
	srv := forgetest.New(t)
	srv.Commits = map[string]time.Time{sha1: t0}
	c := srv.Client(nil)

	got, err := c.CommitDate(context.Background(), repo, sha1)
	require.NoError(t, err)
	assert.True(t, got.Equal(t0))

	_, err = c.CommitDate(context.Background(), repo, sha2)
	require.ErrorIs(t, err, forge.ErrNotFound)

	_, err = c.CommitDate(context.Background(), repo, "main")
	require.ErrorIs(t, err, forge.ErrUnsupportedSource, "a ref name is not a commit id")
	assert.Len(t, srv.Requests(), 2, "the invalid id made no request")
}

func TestPullRequestsAndReviews(t *testing.T) {
	srv := forgetest.New(t)
	srv.PRs = map[string][]forge.PullRequest{sha1: {{Number: 7, State: "closed", Merged: true, Author: "alice", BaseRef: "main", HeadSHA: sha2, MergeCommit: sha1}}}
	srv.Reviews = map[int][]forge.Review{7: {
		{ID: 1, Login: "bob", State: forge.ReviewChangesRequested, CommitID: sha2, Submitted: t0},
		{ID: 2, Login: "bob", State: forge.ReviewApproved, CommitID: sha2, Submitted: t0.Add(time.Hour), AuthorAssociation: forge.AssociationCollaborator},
	}}
	c := srv.Client(nil)

	prs, err := c.PullRequestsForCommit(context.Background(), repo, sha1)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, forge.PullRequest{Number: 7, State: "closed", Merged: true, Author: "alice", BaseRef: "main", HeadSHA: sha2, MergeCommit: sha1}, prs[0])

	pr, err := c.PullRequest(context.Background(), repo, 7)
	require.NoError(t, err)
	assert.Equal(t, forge.PullRequest{Number: 7, State: "closed", Merged: true, Author: "alice", BaseRef: "main", HeadSHA: sha2, MergeCommit: sha1}, pr)
	_, err = c.PullRequest(context.Background(), repo, 8)
	require.ErrorIs(t, err, forge.ErrNotFound)
	_, err = c.PullRequest(context.Background(), repo, 0)
	require.ErrorIs(t, err, forge.ErrUnsupportedSource)

	reviews, err := c.Reviews(context.Background(), repo, 7)
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	assert.Equal(t, forge.ReviewApproved, reviews[1].State)
	assert.Equal(t, sha2, reviews[1].CommitID)
	assert.True(t, reviews[1].Maintainer(), "a collaborator is a maintainer")
	assert.Equal(t, forge.AssociationCollaborator, reviews[1].AuthorAssociation)
	assert.False(t, reviews[0].Maintainer(), "no association reported is an outsider")
	assert.True(t, reviews[1].Submitted.Equal(t0.Add(time.Hour)))

	_, err = c.Reviews(context.Background(), repo, 0)
	require.ErrorIs(t, err, forge.ErrUnsupportedSource)
}

func TestCodeownersFallbackOrder(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantPath string
		wantErr  error
	}{
		{"github dir wins", map[string]string{".github/CODEOWNERS": "* @a\n", "CODEOWNERS": "* @b\n"}, ".github/CODEOWNERS", nil},
		{"root", map[string]string{"CODEOWNERS": "* @b\n", "docs/CODEOWNERS": "* @c\n"}, "CODEOWNERS", nil},
		{"docs", map[string]string{"docs/CODEOWNERS": "* @c\n"}, "docs/CODEOWNERS", nil},
		{"none", map[string]string{}, "", forge.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := forgetest.New(t)
			srv.Owners = tt.files
			c := srv.Client(nil)

			got, err := c.Codeowners(context.Background(), repo, "abc123")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got.Path)
			assert.Equal(t, tt.files[tt.wantPath], string(got.Content))
			assert.Contains(t, srv.Requests()[0].RawQuery, "ref=abc123")
			assert.Contains(t, srv.Requests()[0].Accept, "raw")
		})
	}
}

func TestCodeownersRejectsOptionLikeRef(t *testing.T) {
	srv := forgetest.New(t)
	_, err := srv.Client(nil).Codeowners(context.Background(), repo, "--upload-pack=x")
	require.ErrorIs(t, err, forge.ErrUnsupportedSource)
	assert.Empty(t, srv.Requests())
}

func TestTeams(t *testing.T) {
	srv := forgetest.New(t)
	srv.Teams = map[string][]string{"acme/reviewers": {"alice", "bob"}}
	c := srv.Client(map[string]string{"GITHUB_TOKEN": tok})
	team := forge.Team{Host: "github.com", Org: "acme", Slug: "reviewers"}

	members, err := c.TeamMembers(context.Background(), team)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob"}, members)

	in, err := c.IsTeamMember(context.Background(), team, "alice")
	require.NoError(t, err)
	assert.True(t, in)
	out, err := c.IsTeamMember(context.Background(), team, "mallory")
	require.NoError(t, err)
	assert.False(t, out, "a non-member is false, not an error")

	_, err = c.IsTeamMember(context.Background(), team, "../x")
	require.ErrorIs(t, err, forge.ErrUnsupportedSource)
	_, err = c.TeamMembers(context.Background(), forge.Team{Host: "github.com", Org: "acme", Slug: "no/slash"})
	require.ErrorIs(t, err, forge.ErrUnsupportedSource)
	_, err = c.TeamMembers(context.Background(), forge.Team{Host: "github.com", Org: "acme", Slug: "gone"})
	require.ErrorIs(t, err, forge.ErrNotFound)
}

func TestTokenSources(t *testing.T) {
	ghRuns := 0
	fakeGH := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		ghRuns++
		assert.Equal(t, []string{"gh", "auth", "token", "--hostname", "github.com"}, spec.Argv, "gh runs with a fixed argv")
		for _, kv := range spec.Env {
			assert.False(t, strings.HasPrefix(kv, "GITHUB_TOKEN="), "no credential variable is handed to gh")
		}
		return runner.Result{Status: runner.StatusOK, Stdout: []byte("gh-token\n")}
	}}
	failGH := &runner.Fake{Handle: func(runner.Spec) runner.Result { return runner.Result{Status: runner.StatusExit, ExitCode: 1} }}
	tests := []struct {
		name     string
		env      map[string]string
		run      runner.Runner
		wantAuth string
		wantGH   int
	}{
		{"GITHUB_TOKEN", map[string]string{"GITHUB_TOKEN": "env-a", "GH_TOKEN": "env-b"}, fakeGH, "Bearer env-a", 0},
		{"GH_TOKEN", map[string]string{"GH_TOKEN": "env-b"}, fakeGH, "Bearer env-b", 0},
		{"gh auth token", map[string]string{"PATH": "/usr/bin"}, fakeGH, "Bearer gh-token", 1},
		{"gh fails", nil, failGH, "", 0},
		{"no runner permission", nil, runner.Deny{}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghRuns = 0
			srv := forgetest.New(t)
			srv.Commits = map[string]time.Time{sha1: t0}
			c := srv.Client(tt.env, func(o *forge.Options) { o.Host.Runner = tt.run })

			for range 2 { // the token is resolved once
				_, err := c.CommitDate(context.Background(), repo, sha1)
				require.NoError(t, err)
			}

			for _, r := range srv.Requests() {
				assert.Equal(t, tt.wantAuth, r.Authorization)
			}
			assert.Equal(t, tt.wantGH, ghRuns)
		})
	}
}

func TestHostAllowlist(t *testing.T) {
	tests := []struct {
		name     string
		repo     forge.Repo
		env      map[string]string
		wantErr  bool
		wantReqs int
	}{
		{"github by default", repo, nil, false, 1},
		{"other host refused by default", forge.Repo{Host: "gitlab.com", Owner: "o", Name: "r"}, map[string]string{"GITHUB_TOKEN": tok}, true, 0},
		{"enterprise host allowed by env", forge.Repo{Host: "ghe.example.com", Owner: "o", Name: "r"}, map[string]string{"AI_RULEZ_FORGE_HOSTS": "ghe.example.com"}, false, 1},
		{"default host no longer allowed once the env narrows it", repo, map[string]string{"AI_RULEZ_FORGE_HOSTS": "ghe.example.com"}, true, 0},
		{"a clone-token host is not a forge host (it would receive the GitHub token)", forge.Repo{Host: "gitlab.example.com", Owner: "o", Name: "r"},
			map[string]string{"AI_RULEZ_GIT_TOKEN_HOSTS": "github.com,gitlab.example.com", "GITHUB_TOKEN": tok}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := forgetest.New(t)
			srv.Commits = map[string]time.Time{sha1: t0}
			c := srv.Client(tt.env)

			_, err := c.CommitDate(context.Background(), tt.repo, sha1)

			if tt.wantErr {
				require.ErrorIs(t, err, forge.ErrHostNotAllowed)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, srv.Requests(), tt.wantReqs, "a refused host is not contacted, with or without a token")
		})
	}
}

func TestRedirects(t *testing.T) {
	t.Run("another host is refused and never sees the token", func(t *testing.T) {
		other := forgetest.New(t)
		srv := forgetest.New(t)
		srv.Redirect = map[string]string{"/repos/o/r/commits/": other.URL + "/repos/o/r/commits/" + sha1}
		c := srv.Client(map[string]string{"GITHUB_TOKEN": tok})

		_, err := c.CommitDate(context.Background(), repo, sha1)

		require.Error(t, err)
		assert.Empty(t, other.Requests())
		assert.NotContains(t, err.Error(), tok)
	})
	t.Run("same host over https is followed", func(t *testing.T) {
		srv := forgetest.New(t)
		srv.Commits = map[string]time.Time{sha1: t0}
		srv.Redirect = map[string]string{"/repos/o/r/commits/" + sha2: srv.URL + "/repos/o/r/commits/" + sha1}
		c := srv.Client(map[string]string{"GITHUB_TOKEN": tok})

		got, err := c.CommitDate(context.Background(), repo, sha2)

		require.NoError(t, err)
		assert.True(t, got.Equal(t0))
	})
	t.Run("downgrade to http is refused", func(t *testing.T) {
		srv := forgetest.New(t)
		srv.Redirect = map[string]string{"/repos/o/r/commits/": "http://" + strings.TrimPrefix(srv.URL, "https://") + "/x"}
		c := srv.Client(nil)

		_, err := c.CommitDate(context.Background(), repo, sha1)

		require.Error(t, err)
	})
}

func TestNonHTTPSAPIBaseIsRefused(t *testing.T) {
	srv := forgetest.New(t)
	c := srv.Client(nil, func(o *forge.Options) { o.APIBase = "http://" + strings.TrimPrefix(srv.URL, "https://") })

	_, err := c.CommitDate(context.Background(), repo, sha1)

	require.Error(t, err)
	assert.Empty(t, srv.Requests())
}

func TestResponseSizeCap(t *testing.T) {
	srv := forgetest.New(t)
	srv.Raw = map[string][]byte{"/repos/o/r/commits/": []byte(`{"commit":{"committer":{"date":"` + strings.Repeat("x", 2048) + `"}}}`)}
	c := srv.Client(nil, func(o *forge.Options) { o.MaxBody = 1024 })

	_, err := c.CommitDate(context.Background(), repo, sha1)

	require.ErrorIs(t, err, forge.ErrTooLarge)
}

func TestCodeownersSizeCap(t *testing.T) {
	srv := forgetest.New(t)
	srv.Raw = map[string][]byte{"/repos/o/r/contents/": []byte(strings.Repeat("* @a\n", forge.MaxCodeownersBody/5+10))}

	_, err := srv.Client(nil).Codeowners(context.Background(), repo, "")

	require.ErrorIs(t, err, forge.ErrTooLarge)
}

func TestMalformedJSON(t *testing.T) {
	srv := forgetest.New(t)
	srv.Raw = map[string][]byte{"/repos/": []byte(`<html>`)}

	_, err := srv.Client(nil).CommitDate(context.Background(), repo, sha1)

	require.Error(t, err)
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		env    map[string]string
		want   error
	}{
		{"not found", http.StatusNotFound, nil, forge.ErrNotFound},
		{"unauthorized", http.StatusUnauthorized, nil, forge.ErrUnauthorized},
		{"forbidden without a token asks for one", http.StatusForbidden, nil, forge.ErrUnauthorized},
		{"rate limited by 429", http.StatusTooManyRequests, nil, forge.ErrRateLimited},
		{"server error", http.StatusInternalServerError, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := forgetest.New(t)
			srv.Status = map[string]int{"/repos/": tt.status}

			_, err := srv.Client(tt.env).CommitDate(context.Background(), repo, sha1)

			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
		})
	}
	t.Run("forbidden with a token and no spent limit lacks permission", func(t *testing.T) {
		srv := forgetest.New(t)
		srv.Status = map[string]int{"/repos/": http.StatusForbidden}
		_, err := srv.Client(map[string]string{"GITHUB_TOKEN": tok}).CommitDate(context.Background(), repo, sha1)
		require.ErrorIs(t, err, forge.ErrForbidden)
	})
	t.Run("forbidden with a token and a spent limit is a rate limit with a reset time", func(t *testing.T) {
		srv := forgetest.New(t)
		srv.Status = map[string]int{"/repos/": http.StatusForbidden}
		srv.RateLimited = true
		_, err := srv.Client(map[string]string{"GITHUB_TOKEN": tok}).CommitDate(context.Background(), repo, sha1)
		var rl *forge.RateLimitError
		require.ErrorAs(t, err, &rl)
		assert.False(t, rl.Reset.IsZero())
		require.ErrorIs(t, err, forge.ErrRateLimited)
	})
}

func TestOfflineMakesNoRequest(t *testing.T) {
	srv := forgetest.New(t)
	c := srv.Client(map[string]string{"GITHUB_TOKEN": tok}, func(o *forge.Options) { o.Offline = true })
	ctx := context.Background()
	team := forge.Team{Host: "github.com", Org: "acme", Slug: "x"}

	calls := map[string]error{}
	_, calls["Releases"] = c.Releases(ctx, repo)
	_, calls["Release"] = c.Release(ctx, repo, "v1")
	_, calls["Tag"] = c.Tag(ctx, repo, "v1")
	_, calls["CommitDate"] = c.CommitDate(ctx, repo, sha1)
	_, calls["PullRequestsForCommit"] = c.PullRequestsForCommit(ctx, repo, sha1)
	_, calls["PullRequest"] = c.PullRequest(ctx, repo, 1)
	_, calls["Reviews"] = c.Reviews(ctx, repo, 1)
	_, calls["Codeowners"] = c.Codeowners(ctx, repo, "")
	_, calls["TeamMembers"] = c.TeamMembers(ctx, team)
	_, calls["IsTeamMember"] = c.IsTeamMember(ctx, team, "alice")

	for name, err := range calls {
		assert.ErrorIs(t, err, forge.ErrOffline, name)
	}
	assert.Empty(t, srv.Requests())
}

func TestTokenIsNeverInErrors(t *testing.T) {
	srv := forgetest.New(t)
	srv.Token = "another-token"
	c := srv.Client(map[string]string{"GITHUB_TOKEN": tok})

	_, err := c.CommitDate(context.Background(), repo, sha1)

	require.ErrorIs(t, err, forge.ErrUnauthorized)
	assert.NotContains(t, err.Error(), tok)
}

func TestFakeImplementsClient(t *testing.T) {
	f := &forge.Fake{
		ReleasesBy: map[string][]forge.Release{repo.String(): {{Tag: "v1", Published: t0}}},
		Teams:      map[string][]string{"github.com/acme/x": {"bob", "alice"}},
	}
	var c forge.Client = f

	rel, err := c.Release(context.Background(), repo, "v1")
	require.NoError(t, err)
	assert.True(t, rel.Published.Equal(t0))
	_, err = c.Release(context.Background(), repo, "v2")
	require.ErrorIs(t, err, forge.ErrNotFound)
	members, err := c.TeamMembers(context.Background(), forge.Team{Host: "github.com", Org: "acme", Slug: "x"})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob"}, members)

	f.Err = forge.ErrOffline
	_, err = c.Releases(context.Background(), repo)
	require.True(t, errors.Is(err, forge.ErrOffline))
	assert.Len(t, f.Calls(), 4)
}
