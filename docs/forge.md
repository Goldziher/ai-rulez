# Forge Client

`internal/forge` is a small, read-only GitHub REST client. Two features use it: [`min_release_age`](lockfile.md#minimum-release-age) reads the forge's publish time of a tag, and [approvals](approvals.md) read pull request reviews, CODEOWNERS and team membership. It is a library, not a command; nothing in it writes to a forge.

## What it reads

| Method | GitHub endpoint | Used for |
| --- | --- | --- |
| `Releases(repo)` | `GET /repos/{o}/{r}/releases` | publish dates of all releases (drafts dropped) |
| `Release(repo, tag)` | `GET /repos/{o}/{r}/releases/tags/{tag}` | publish date of one tag |
| `Tag(repo, tag)` | `GET /git/ref/tags/{tag}`, `GET /git/tags/{sha}` | peeled commit, annotated tag object, tagger date |
| `CommitDate(repo, sha)` | `GET /commits/{sha}` | committer date (forgeable) |
| `PullRequestsForCommit(repo, sha)` | `GET /commits/{sha}/pulls` | the PR a merged commit came from |
| `Reviews(repo, n)` | `GET /pulls/{n}/reviews` | who approved, and at which head commit |
| `Codeowners(repo, ref)` | `GET /contents/{path}?ref=` | `.github/CODEOWNERS`, then `CODEOWNERS`, then `docs/CODEOWNERS` |
| `TeamMembers(team)`, `IsTeamMember(team, login)` | `GET /orgs/{org}/teams/{slug}/members`, `.../memberships/{login}` | expanding `@org/team` owners |

`ParseRepo` turns a source (`https://`, `git+https://`, `ssh://`, `git@host:o/r`) into a `Repo`. Plain `http`, `git://`, `file://`, local paths and nested paths are `ErrUnsupportedSource`: a repository whose identity is uncertain is never looked up.

## Safety properties

- **Token.** Only from the environment: `GITHUB_TOKEN`, then `GH_TOKEN`, then `gh auth token --hostname <host>` (fixed argv, started through the injected runner, no credential variable passed). Never from a config file or a flag, never logged, never in an error. With no token the client is anonymous, which is enough for release and commit dates of public repositories.
- **Hosts.** A repository is contacted only when its host is on the git-token allowlist (`AI_RULEZ_GIT_TOKEN_HOSTS`, default `github.com`), over https. Any other host is `ErrHostNotAllowed` and gets no request, with or without a token. GitHub Enterprise Server is `https://<host>/api/v3` once its host is allowlisted.
- **Redirects.** Followed only to https on the same host (at most 3); a pagination link that leaves the API host is refused. The token cannot follow a redirect elsewhere.
- **Caps.** A response body is capped at 4 MiB (CODEOWNERS 3 MiB, GitHub's own limit): larger is `ErrTooLarge`. A listing reads at most 10 pages of 100; the items read so far are returned with `ErrTruncated`.
- **Input.** Owner, repository, team, login, tag, ref and commit id are validated before they become part of a URL.
- **Offline.** With `Options.Offline` every method returns `ErrOffline` without a request.

## Errors

`ErrNotFound`, `ErrUnauthorized` (bad token, or a lookup that needs one), `ErrForbidden` (token lacks a permission, e.g. `read:org` for teams), `ErrRateLimited` (`*RateLimitError` carries the reset time), `ErrTruncated`, `ErrTooLarge`, `ErrOffline`, `ErrHostNotAllowed`, `ErrUnsupportedSource`. Test with `errors.Is`.

## Notes for approvals

The interface is shaped for approval checks (`internal/approval`):

- A review counts for a change only when `Review.CommitID` is the commit the change landed at, or the pull request's final head: an approval of an earlier head approved different content. Read `PullRequestsForCommit` first, then `Reviews`.
- The latest review of each reviewer decides: a later `CHANGES_REQUESTED` or `DISMISSED` replaces an earlier `APPROVED`. `COMMENTED` and `PENDING` never approve.
- `Reviews`, `PullRequestsForCommit` and `TeamMembers` can return `ErrTruncated` with a partial list. Count approvals only from a complete list; treat a truncated one as no approval.
- `IsTeamMember` returns `false`, not an error, for a non-member. Team calls need a token with `read:org`; without it they fail with `ErrUnauthorized` or `ErrForbidden`, and an approval check must fail closed.
- `Codeowners` returns the raw file; matching owners to paths is the caller's job. An owner may be `@user`, `@org/team` or an email.

## Testing

`forge.Fake` is an in-memory `Client` (a missing entry is `ErrNotFound`, `Err` fails every call, `Calls()` records them). `forgetest.New(t)` starts a local TLS server that answers the endpoints above from plain Go data, with knobs for pages, forced statuses, rate limits, redirects, oversized bodies and bad pagination links; `srv.Client(env)` returns an `HTTPClient` wired to it that never starts a process. Tests never call a real forge. A live check, if one is added, belongs behind `AI_RULEZ_LIVE_FORGE=1`.
