# Skill search

`ai-rulez search` ranks your project's skills against a query with the same ranker the MCP `find_skill` tool uses,
and `search --eval` measures that ranking against labeled queries. By default the ranking is lexical: nothing is sent
anywhere and the same input always gives the same order. Optionally it is hybrid, fusing the lexical ranking with
embedding similarity, so a paraphrase such as "customer wants money back" finds the skill described as "Issue a
refund".

Use it to check which skill an agent would find for a task, to tune skill descriptions, and to gate a pull request on
search quality.

## Example

```text
$ ai-rulez search refund a duplicate charge
RANK  SCORE   NAME           DOMAIN  DESCRIPTION
1     4.4971  refund-policy  root    Issue a refund for a duplicate or disputed charge.

$ ai-rulez search "customer wants money back"
no served skill matches
```

The paraphrase shares no words with the skill, so lexical search misses it; [hybrid ranking](#hybrid-ranking)
finds it.

```bash
ai-rulez search --format json --limit 10 deploy staging
ai-rulez search --role platform rotate credentials
ai-rulez search --mode hybrid --explain "customer wants money back"
```

## Searching

The skills searched are the ones `mcp --serve-skills` would serve, selected with the same flags: `--profile`,
`--role` (not both), `--targets`, `--domain`, `--allow`, `--deny`, `--source` (repeatable) and `--include-static`. A
project that sets no `delivery` anywhere searches every skill; otherwise only `served` and `both` skills, unless you
pass `--include-static`. Skills the server would refuse (lock or scan state) are not ranked. Remote skill sources are
fetched unless you pass `--offline` or `--frozen`.

All words after the flags form one query. A query that is only `index`, `status` or `mine` runs that subcommand; add
a word to search for it.

| Flag | Meaning |
| --- | --- |
| `--mode lexical\|hybrid\|vector` | Override `[search] mode` and `AI_RULEZ_SEARCH_MODE` for this run |
| `--limit N` | Maximum results (default 5, max 20) |
| `--explain` | Show each skill's rank in the lexical and vector lists, and how the query was embedded |
| `--format json` | Print [`search.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search.v1.schema.json) |

```json
{
  "schema_version": 1,
  "query": "deploy staging",
  "ranking": "lexical",
  "degraded": null,
  "count": 1,
  "results": [
    {
      "name": "deploy-staging",
      "description": "Deploy the web app to the staging cluster.",
      "score": 4.455101,
      "domain": "root",
      "digest": "sha256:897e58c3...",
      "lexical_rank": 1
    }
  ]
}
```

- `ranking` is what produced the order (`lexical`, `hybrid` or `vector`).
- `degraded` says why a hybrid or vector search fell back to lexical: `no_index`, `provider_unavailable`, `timeout`,
  `budget` or `network_disabled`. The reason is also printed on stderr.
- A hybrid result adds `vector_rank` and `vector_sim`; `stale_vector: true` marks a skill that changed since it was
  indexed and ranks lexically only.
- `abstained: true` means no skill was close enough (see [Abstaining](#abstaining)).

## The ranker

The lexical ranker is BM25F over four skill fields, weighted name 3, `triggers` 2.5, `keywords` 2 and description 1,
after lowercasing, dropping stopwords and light stemming (`migrations` matches `migration`). Ties are ordered by name.
An empty or stopword-only query returns nothing. It is the reference for every other mode and the fallback when they
cannot run.

A query containing a skill's exact id as whole words ("use the deploy-staging skill") puts that skill first. An id
that is one ordinary word (`test`, `build`) does so only when the query calls it a skill ("use the build skill").

To make a skill easier to find, give it a precise `description` and `triggers` or `keywords` frontmatter (see
[Skills](skills.md)).

## Hybrid ranking

Off by default. Configure a `[search]` table and an embedding provider, then [build the index](#building-the-index).

```toml
# .ai-rulez/config.toml (excerpt)
[search]
mode = "hybrid"
```

```toml
# ~/.config/ai-rulez/config.toml (network settings apply only from the user config)
[llm]
allow_network   = true
provider        = "gemini"            # or base_url = "http://localhost:11434/v1" for Ollama, vLLM, LM Studio
embedding_model = "gemini-embedding-001"
api_key_env     = "GEMINI_API_KEY"
max_calls       = 200
```

Embeddings go through the [`[llm]`](llm.md) provider, so its network gate (`allow_network`, off by default), budget
(`max_cost_usd`, `max_tokens`, `max_calls`), response cache and error redaction apply. A `[search]` table in the user
config overrides the repository's key by key.

| `[search]` key | Default | Meaning |
| --- | --- | --- |
| `mode` | `lexical` | `lexical`, `hybrid` or `vector` (also `AI_RULEZ_SEARCH_MODE`) |
| `fields` | `["name", "triggers", "keywords", "description"]` | Skill fields sent to the embedder |
| `index_body` | `false` | Also embed the first `body_chars` characters of `SKILL.md` |
| `body_chars` | `1200` | See `index_body` |
| `fusion` | `auto` | `auto`, `rrf` or `weighted` |
| `rrf_k` | `60` | RRF constant |
| `weights` | `{ lexical = 1.0, vector = 1.0 }` | List weights for `rrf` and `weighted` |
| `candidates` | `50` | Length of each list before fusion |
| `vector_min_sim` | `0` (off) | Cosine below which a skill is not a match; see [Abstaining](#abstaining) |
| `query_timeout_ms` | `800` | Timeout of the query embedding; on timeout the ranking is lexical |
| `batch_size` | `64` | Texts per embedding call of `search index` |
| `index_dir` | `local/search` | Index directory under the configuration directory |
| `dtype` | `float32` | `float32` or `float16` (half the size) |
| `log_queries` | `false` | User config only; see [Query mining](#query-mining) |
| `embeddings.model` | | Overrides `[llm] embedding_model` for search |
| `embeddings.command`, `embeddings.pass_env` | | A [command provider](#command-provider) |

### Fusion

| `fusion` | Behavior |
| --- | --- |
| `auto` | While every skill has a current vector, rank by cosine alone (`ranking` is `vector`; an exact id still pins first). When a skill is new or changed since indexing, fall back to `rrf` so it still ranks lexically |
| `rrf` | Reciprocal rank fusion: `score = sum(weight / (rrf_k + rank))` over the lists a skill is in |
| `weighted` | Each list's scores are scaled to [0, 1] and mixed by `weights.lexical / (weights.lexical + weights.vector)` |

Compare the modes on your catalog with `search --eval cases.yaml --mode lexical,vector,hybrid` before choosing.

`vector` mode ranks by cosine alone; skills without a current vector follow in lexical order, and when no skill has
one the search is `degraded: no_index`.

`find_skill` uses the same ranker. The MCP server never builds the index: it reads the files `search index` wrote,
reloads them when they change, and embeds only the query. See [MCP server](mcp-server.md#dynamic-skill-loading).

### Abstaining

A query unrelated to any skill ("what is the weather in Berlin") still has nearest skills. Set `vector_min_sim` to
treat a skill whose cosine is below it as no match:

- a vector ranking (including `auto` on a fresh index) with no skill above the threshold returns nothing;
  `search --format json` reports `"abstained": true` and `find_skill` returns no skill;
- `rrf` and `weighted` drop only the weak vector candidates, so a skill that matches by words still ranks.

Cosines differ per model, so there is no default. `search --eval` with positive and negative cases suggests one
(`calibration` in the JSON): the threshold that answers the most positives while abstaining on the most negatives.
Re-run the evaluation with the value set before committing it. A `vector_min_sim` in the user config wins over the
repository's; `0` there switches it off.

## Building the index

```bash
ai-rulez search index --dry-run       # provider, texts, bytes and estimated cost; nothing is sent
ai-rulez search index                 # embed what changed and write the index
ai-rulez search index --rebuild       # re-embed everything
ai-rulez search index --items refund-policy,deploy-staging   # also re-embed these
ai-rulez search status                # compare the index with the served skills, offline
```

```text
$ ai-rulez search index --dry-run
provider   gemini@default   (allow_network=true)
model      gemini-embedding-001
items      25   cached 0   to embed 25   withheld 0   est. tokens 1627   est. cost <= $0.000244
fields     name, triggers, keywords, description   body: no
egress     25 texts, 4.7 KiB -> default
```

- A skill's embedded text is built from the configured `fields` (and the body with `index_body`). A vector is reused
  while the provider, model, fields and exact text are unchanged, so editing a skill's body costs nothing (unless
  `index_body` is set) and an edited description costs one text. A new model or field set re-embeds everything.
- A batch the provider rejects is split until the refused skill stands alone; that skill is skipped and named. A
  budget, network, key or provider error stops the build. Either way the finished vectors are written, the missing
  skills are listed, and the command exits 2.
- `search status` reports `none`, `unreadable`, `incompatible` (other provider, model or fields), `stale` or `fresh`,
  and lists skills that are current, changed, not indexed, or no longer served.

```text
$ ai-rulez search status
index      .ai-rulez/local/search
state      none
mode       lexical
...
next       run 'ai-rulez search index'
```

The index is two files in `index_dir`: `manifest.json` (provider, model, fields and a digest per skill, sorted and
without timestamps) and `vectors.bin`. A damaged or tampered file is treated as no index.

The default `local/search` is machine-local and gitignored. Set `index_dir` to a directory outside `local/` (for
example `search-index`) to commit the index, so CI can run a hybrid evaluation without an API key. `validate` then
warns (`AR9D1`) when the committed index no longer matches the skills. A tampered index can only change the order of
results: `load_skill` still enforces the lock, approval and scan state.

### Command provider

A program can produce the vectors instead of `[llm]`:

```toml
# user config
[search.embeddings]
command  = ["/usr/local/bin/embed"]
model    = "my-local-model"      # recorded in the index
pass_env = ["EMBED_TOKEN"]
```

It runs without a shell in the project root, with only `PATH`, `HOME` and the `pass_env` variables, a 30 s timeout
and a 64 MiB output cap. It reads `{"input": ["text", ...]}` on stdin and prints `{"vectors": [[...], ...]}`, one
vector per input in order.

Because it runs a program, a repository config cannot set it: it is honoured from the user config, or from the
repository config with `--allow-exec`. The MCP server honours only the user config.

## Data egress

| Operation | Data sent | To |
| --- | --- | --- |
| `search index` | One text per changed skill: the configured fields (by default name, triggers, keywords and description, already visible to agents) and, with `index_body`, the start of the body | The embedding endpoint |
| `search` and `find_skill` in hybrid or vector mode | The query, up to 2 KiB (an agent's task description can contain user data) | The same endpoint |
| Lexical mode | Nothing | |

Each skill text is checked with the secret scanner of `AR001` before sending; a hit withholds that skill (`AR9D3`) and
it ranks lexically only. Queries are not scanned. `allow_network` stays off until you enable it in the user config,
for loopback hosts too, and `--dry-run` shows the host first. For data residency, use a local or in-region endpoint;
the manifest records which provider produced the vectors.

## Evaluating the ranking

```bash
ai-rulez search --eval search-cases.yaml
ai-rulez search --eval search-cases.yaml --min top1=0.6,mrr=0.7
ai-rulez search --eval search-cases.yaml --mode lexical,hybrid
ai-rulez search --eval search-cases.yaml --output result.json
ai-rulez search --eval search-cases.yaml --baseline result.json --max-flips 0
ai-rulez search --from-evals
```

```text
$ ai-rulez search --eval search-cases.yaml
cases: 3 (+1 negative), k=3, mode: lexical
top1      0.6667  (95% CI 0.0000-1.0000)
recall@3  0.6667  (95% CI 0.0000-1.0000)
hit@3     0.6667
mrr       0.6667  (95% CI 0.0000-1.0000)

by tag:
  paraphrase (1): top1 0.0000  recall@3 0.0000  hit@3 0.0000  mrr 0.0000

negatives (nothing should match; shown is what ranked first, with the ranker's own score):
  weather: nothing;

misses:
  refund-paraphrase: expected refund-policy, got [] (not retrieved)
```

- `--mode` takes a comma-separated list (`lexical`, `vector`, `hybrid` or `all`). The first is the primary mode the
  gates check; the default is the configured mode. Modes that need vectors need the index.
- `--k` overrides the file's cut-off (max 100).
- `--from-evals` derives cases from the skills' [eval cases](evals.md#layout), alone or
  added to `--eval`: a prompt with `expect_trigger: true` expects that skill; a near-miss or `expect_trigger: false`
  prompt must not rank it first. These cases are tagged `from-evals`.

### Cases file

```yaml
version: 1          # required
k: 5                # cut-off of recall@k, hit@k and nDCG@k (default 5, max 100)
cases:
  - id: refund-paraphrase
    query: "customer wants money back for a double charge"
    expect: [refund-policy]       # any of them counts
    tags: [paraphrase, billing]
  - id: graded
    query: "set up staging deploy pipeline"
    expect: [{id: deploy-staging, grade: 3}, {id: ci-pipeline, grade: 1}]   # grades enable nDCG
  - id: scoped
    query: "rotate credentials"
    role: platform                # rank only this role's skills
    expect: [rotate-keys]
  - id: near-miss
    query: "deploy to production"
    expect: []
    avoid: [deploy-staging]       # must not rank first
  - id: negative
    query: "what is the weather tomorrow"
    expect: []                    # nothing relevant; reported separately
```

Skills are named as in the catalog. The file is read strictly: an unknown field, a missing `version`, `id` or
`query`, a duplicate id, an unknown skill, a grade outside 1-9, a skill both expected and avoided, an unknown `role`,
or an expected skill the role does not serve is reported together as `AR9D2` (exit 1).

### Metrics and gates

| Metric | Meaning |
| --- | --- |
| `top1` | A relevant skill ranked first |
| `recall_at_k` | Share of relevant skills in the top k |
| `hit_at_k` | Any relevant skill in the top k |
| `mrr` | 1 / rank of the first relevant skill (0 if none) |
| `ndcg_at_k` | With graded cases |
| `avoid_top1` | With `avoid` lists: share where an avoided skill ranked first (lower is better) |

Metrics are reported overall and per tag, with 95% bootstrap intervals; negative cases are listed separately. With
several modes, `paired_vs_lexical` gives each mode's difference to lexical over the same cases: an interval that
excludes 0 is a real change. A vector mode that fell back to lexical for any case is an error, not a pass.

| Gate | Fails when |
| --- | --- |
| `--min top1=0.6,recall=0.8,hit=0.9,mrr=0.7,ndcg=0.7` | A metric of the primary mode is below its floor |
| `--baseline prev.json --max-flips N` | More than N cases went from hit to miss since the baseline (default 0) |

With few cases, one case moves `top1` by several points, so prefer the flip count to a floor. A baseline must use the
same primary mode and `--k`. `--output` and `--format json` write
[`search-eval.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search-eval.v1.schema.json);
`--output` is not written when embeddings were degraded.

| Exit code | Meaning |
| --- | --- |
| `0` | Every gate passed |
| `1` | The command could not run (bad flags or cases file, unknown skill, unreadable baseline, a mode without an index, degraded embeddings) |
| `2` | A gate failed (printed as `AR9D4`), or `search index` stopped early |

```text
$ ai-rulez search --eval search-cases.yaml --min top1=0.9
...
gate failed
AR9D4: top1 is 0.6667, below the minimum 0.9000
```

## Query mining

Real queries make the best test cases. Query logging is opt-in and only from the user config (or
`AI_RULEZ_SEARCH_LOG_QUERIES=1`; a repository config cannot turn it on):

```toml
# user config
[search]
log_queries = true
```

`find_skill` then appends each query (up to 512 bytes; secret-looking queries are skipped) and the skill
the session loaded next to `<config dir>/local/search-queries.jsonl`. The file is mode 0600, gitignored, capped at
8 MiB, stores hashed session ids, and is never sent anywhere. `search mine` turns it into cases:

```bash
ai-rulez search mine --output mined-cases.yaml   # a query followed by loading S expects S
ai-rulez search mine --min-count 2 --purge       # keep pairs seen twice; delete the log
```

A query is labeled with the first skill loaded after it in the same session; queries followed by nothing, or by
different skills with no clear winner, are dropped. The agent's choice is not necessarily right, so mined cases are
tagged `mined`: review them before they gate anything.

## Validation codes

| Code | Raised by | Meaning |
| --- | --- | --- |
| `AR9D0` | `validate` | Invalid `[search]` table |
| `AR9D1` | `validate` | A committed index no longer matches the skills (warning) |
| `AR9D2` | `search --eval` | Invalid cases file |
| `AR9D3` | `search index` | A skill text looked like it held a secret and was not sent |
| `AR9D4` | `search --eval` | A gate failed |

`search_skills` (the MCP listing tool) always ranks lexically.
