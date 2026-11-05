# Forge client

Some governance features ask GitHub a question: when was this tag released, who approved this pull request, who is in
this team. ai-rulez answers them with a small, read-only GitHub REST client. It never writes to a forge, and it is
contacted only by the commands below.

## Example

```bash
export GITHUB_TOKEN=ghp_...                      # or GH_TOKEN, or a logged-in `gh`
ai-rulez lock --outdated                         # release dates for min_release_age
ai-rulez approve include:shared --from-github-review 42 --yes
ai-rulez verify --approvals --online
```

For GitHub Enterprise Server, allow its host first:

```bash
export AI_RULEZ_FORGE_HOSTS=github.com,github.acme.internal
```

The client then calls `https://github.acme.internal/api/v3`.

## What uses it

| Feature | Commands | What is read |
| --- | --- | --- |
| [Minimum release age](lockfile.md#minimum-release-age) | `lock`, `update`, `lock --outdated`, `generate --refresh-remote` and the MCP pin tools, only when a `min_release_age` is set and `min_release_age_source` is `auto` or `forge` (`first-seen` and `commit` make no forge call) | the publish time and target commit of a tag's release |
| [Review-linked approvals](approvals.md#review-linked-approvals) | `approve --from-github-review`, `verify --approvals --online` | the pull request, its reviews and their head commit, the reviewer's repository role |
| [Team expansion](approvals.md#who-may-approve) | `approve --resolve-teams`, `approve --list --resolve-teams` | members of an `@org/team` |

`generate` (without `--refresh-remote`), `validate`, `lock --check` and the skills server never call the forge.

## Safety properties

- **Token from the environment only**: `GITHUB_TOKEN`, then `GH_TOKEN`, then the output of
  `gh auth token --hostname <host>`. Never from a config file or a flag, never logged, never in an error message.
  Without a token the client is anonymous, which is enough for release dates of public repositories.
- **Allowlisted hosts**: a repository is contacted only when its host is in `AI_RULEZ_FORGE_HOSTS` (comma-separated,
  environment only, default `github.com`), and only over https. Any other host gets no request, with or without a
  token. The list is separate from `AI_RULEZ_GIT_TOKEN_HOSTS`, so a host trusted with a git clone token never receives
  the GitHub token.
- **Known repository identity**: only `https://`, `git+https://`, `ssh://` and `git@host:owner/repo` sources are looked
  up. Plain `http://`, `git://`, `file://` and local paths are not.
- **Redirects** are followed only to https on the same host, at most 3 times. A pagination link that leaves the API
  host is refused.
- **Size caps**: a response body is at most 4 MiB. A listing reads at most 10 pages of 100 entries; a longer one is
  treated as incomplete, and an incomplete list of reviews or team members never approves anything.

## Tokens and permissions

| Lookup | Token needed |
| --- | --- |
| Release dates of a public repository | none |
| Anything in a private repository | read access to the repository |
| Team members (`--resolve-teams`) | `read:org` |
| A reviewer's repository role (`write`, `maintain` or `admin`) for `--from-github-review` | push access; without it the review's `author_association` is used instead |

In GitHub Actions, `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` covers the repository's own reviews and releases.
Reading organization teams needs a token with `read:org`.

## Errors

| Message | Cause | Fix |
| --- | --- | --- |
| `host is not in the forge allowlist` | the source's host is not in `AI_RULEZ_FORGE_HOSTS` | add the host, or accept that the feature does not apply to it |
| `not a repository on a supported forge` | the source is a local path, `http://`, `git://` or `file://` URL | use an https or ssh URL |
| `the forge needs a valid token for this lookup` | no token, or an invalid one | set `GITHUB_TOKEN` or log in with `gh auth login` |
| `the forge refused the request: the token lacks permission` | for example `read:org` is missing for a team | use a token with the scope in the table above |
| `the forge rate limit is exhausted` | anonymous or busy token | set a token, or wait for the reset time in the message |
| `the forge listing was cut at the page cap` | more than 1000 reviews or members | the result is treated as incomplete; nothing is approved from it |
| `forge lookups need the network and offline mode is on` | the command runs offline | drop `--offline` for this command |

A check that could not ask the forge fails: `verify --approvals --online` exits `1`, and a release time that cannot be
found holds the tag back.
