# Skill Search

`ai-rulez search` ranks the skills your project serves against a query, with the same ranker as the `find_skill`
tool of [`mcp --serve-skills`](mcp-server.md), and measures that ranking against labeled queries. By default it is
lexical and deterministic: no query or skill text is sent anywhere and the same input always gives the same order.
Optionally it is hybrid: the lexical list is fused with cosine similarity over embeddings you bring (an
OpenAI-compatible endpoint, Gemini through `literllm`, a local server, or a command), so a paraphrase such as
"customer wants money back" finds the skill described as "issue a refund".

- [Searching](#searching)
- [The ranker](#the-ranker)
- [Hybrid ranking](#hybrid-ranking)
- [Building the index](#building-the-index)
- [Data egress](#data-egress)
- [Evaluating the ranking](#evaluating-the-ranking)
- [Cases file](#cases-file)
- [Result and gates](#result-and-gates)
- [Query mining](#query-mining)
- [Design decisions](#design-decisions)

## Searching

```bash
ai-rulez search "customer wants money back"
ai-rulez search --format json --limit 10 deploy staging
ai-rulez search --role platform rotate credentials
ai-rulez search --mode hybrid --explain "customer wants money back"
```

The catalog is what the server would serve, selected with the same flags: `--profile`, `--targets`, `--domain`,
`--allow`, `--deny`, `--source` (repeatable), `--role`, `--include-static`, `--offline` and `--frozen`. `--role` and
`--profile` are mutually exclusive. Like the server, `search` (and `search status`) fetches the remote
skill sources the catalog is built from (`--source`, configured sources and includes) unless you pass `--offline` or
`--frozen`; that fetch is separate from the embedding egress described under [Data egress](#data-egress). A skill the server refuses (lock or scan state) is not in the catalog and is
not ranked. Several words are joined into one query. `--mode lexical|hybrid|vector` overrides `[search] mode`
(and `AI_RULEZ_SEARCH_MODE`) for one run. A query that is the word `index`, `status` or `mine` alone is the
subcommand; add another word to search for it.

Text output lists rank, score, name, domain and description; `--explain` shows each skill's rank in the lexical
and the vector candidate list instead, and how the query was embedded. `--format json` prints
[`search.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search.v1.schema.json):

```json
{"schema_version": 1, "query": "money back", "ranking": "hybrid", "degraded": null, "count": 1,
 "results": [{"name": "refund-policy", "description": "...", "score": 0.0325, "domain": "root", "digest": "sha256:...",
              "lexical_rank": 3, "vector_rank": 1, "vector_sim": 0.71}]}
```

`ranking` is what produced the order. When a hybrid or vector search could not embed the query it ranks lexically
and `degraded` says why: `no_index`, `provider_unavailable`, `timeout`, `budget` or `network_disabled`; the reason
is also printed once on stderr. `stale_vector: true` marks a skill that changed since it was indexed: it ranks
lexically only. An empty or stopword-only query, or one nothing matches, returns no results in lexical mode. Unlike
`find_skill`, there is no in-role-first reordering: with `--role` the catalog already holds only that role's skills.

## The ranker

Lexical: BM25F over four fields of a skill with weights name 3, triggers 2.5, keywords 2 and description 1, after
lowercasing, dropping stopwords and a light suffix stemmer (`migrations` matches `migration`). Hits are ordered by
score descending, then name. The code is `internal/skillsearch`; `find_skill` calls the same function, and a golden
test pins the order and scores of the lexical ranking so it cannot change by accident. It stays the reference and
the fallback of every other mode.

## Hybrid ranking

Off by default. Turn it on with a `[search]` table and an embedding provider, then build the index once:

```toml
# .ai-rulez/config.toml
[search]
mode        = "hybrid"      # lexical (default) | hybrid | vector
fields      = ["name", "triggers", "keywords", "description"]   # the text sent to the embedder
index_body  = false         # true adds the first body_chars of SKILL.md
body_chars  = 1200
fusion      = "auto"        # auto | rrf | weighted
rrf_k       = 60
weights     = { lexical = 1.0, vector = 1.0 }
vector_min_sim = 0.0        # abstain below this cosine (0 = off); see Abstaining
candidates  = 50            # per list, before fusion
query_timeout_ms = 800      # the query embedding; on timeout the ranking is lexical
batch_size  = 64            # texts per embedding call of `search index` (Gemini through literllm is sent one by one automatically)
index_dir   = "local/search"  # under the config dir; any directory outside local/ is meant to be committed
dtype       = "float32"     # float32 | float16 (half the size)
log_queries = false         # user scope only (user config file or AI_RULEZ_SEARCH_LOG_QUERIES=1); see Query mining
```

```toml
# user config (~/.config/ai-rulez/config.toml): network settings only take effect here
[llm]
allow_network   = true
provider        = "gemini"                 # or base_url = "http://localhost:11434/v1" for Ollama, vLLM, LM Studio
embedding_model = "gemini-embedding-001"
api_key_env     = "GEMINI_API_KEY"
max_calls       = 200
```

The provider is the one `[llm]` table (see [LLM access](llm.md)), so the network gate (`allow_network`, off by
default and user scope only), the budget (`max_cost_usd`, `max_tokens`, `max_calls`, fail closed), the response
cache and the redaction of provider errors all apply to every embedding call. `[search.embeddings] model` overrides
`embedding_model` for search only. `AI_RULEZ_SEARCH_MODE` overrides `mode`; a `[search]` table in the user config
overrides the repository's, key by key.

Fusion. Each list is cut to `candidates` and the two are combined:

- `auto` (default): while every skill in scope has a current vector, rank by cosine alone (the `ranking` field then
  says `vector`; an exact skill id in the query still pins first), because that beat every fusion on this
  repository's evaluation (below). As soon as one skill is new or changed since indexing, fall back to `rrf`
  so that skill still ranks lexically. Set `fusion = "rrf"` or `"weighted"` to always fuse.
- `rrf`: reciprocal rank fusion, `score = sum(weight / (rrf_k + rank))` over the lists a skill is in.
  It needs no tuning of score ranges, because BM25 scores are unbounded and cosines are not comparable to them.
- `weighted`: each list's scores are min-max normalised to [0, 1] over its candidates and mixed with
  `a = weights.lexical / (weights.lexical + weights.vector)`. More sensitive to the score distribution.

Why `auto`. Evaluated with Gemini `gemini-embedding-001` on 36 labeled queries (plus 6 negatives) over this repository's
25 served skills, cut-off k = 3:

| Ranking | top-1 | hit@3 | MRR | nDCG@3 |
| --- | --- | --- | --- | --- |
| lexical | 0.750 | 0.806 | 0.783 | 0.743 |
| hybrid, rrf (1:1, k = 60) | 0.778 | 0.861 | 0.833 | 0.792 |
| hybrid, rrf (1:2, k = 10) | 0.750 | 0.889 | 0.834 | 0.802 |
| hybrid, weighted (1:2) | 0.833 | 0.917 | 0.896 | 0.847 |
| hybrid, weighted (1:4) | 0.861 | 0.944 | 0.907 | 0.872 |
| vector (also `auto` on a fresh index) | 0.861 | 0.972 | 0.910 | 0.894 |

No weight, `rrf_k` or fusion setting tried beat cosine alone, and only the vector ranking's paired gain over lexical
excluded 0 (MRR +0.126, 95% interval 0.007 to 0.261; the default RRF's was -0.004 to 0.110). Thirty-six queries is
small and the lexical-versus-paraphrase mix is this catalog's, so run `search --eval --mode lexical,vector,hybrid`
on your own cases before relying on it.

Ties order by name. A query that contains a skill's exact id as whole words ("use the deploy-staging skill") pins
that skill first. An id that is one ordinary word (`test`, `build`, `fix`) pins only when the query calls it a
skill ("use the build skill"): without that rule, "write the failing test first" pinned `test` over `tdd-workflow`
in an evaluation. Only exact ids pin, so a keyword-stuffed description gains nothing from it. `vector` mode ranks by
cosine alone (no id pin); a skill with no usable vector (new, or changed since indexing) follows the vector hits in
lexical order, and when no skill in scope has one the search is `degraded: no_index`. Unless you set `vector_min_sim`, a vector or hybrid result lists the nearest skills even when none is a good match,
because RRF scores have no absolute meaning (see [Abstaining](#abstaining)).

`find_skill` calls the same ranker. The MCP server never builds the index and never embeds a skill: it loads the
files `search index` wrote, reloads them when they change, and embeds only the query (bounded by
`query_timeout_ms`, with an in-memory cache of 256 queries on top of the `[llm]` cache). Its result gains `ranking`
and, when it fell back, `degraded`; see [the MCP server](mcp-server.md#dynamic-skill-loading).

### Abstaining

A query nothing resembles ("what is the weather in Berlin") still has nearest skills. With `vector_min_sim` set, a
skill whose cosine to the query is below it is not a match: a vector ranking (including the `auto` default on a fresh
index) with none above it returns nothing, `search --format json` says `"abstained": true` and `find_skill` returns
no skill; `rrf` and `weighted` drop only the weak vector candidates, so a skill that matches by word still ranks. Skills without a current vector do not hide an abstention in `vector` mode. The user config's `vector_min_sim` wins over the repository's, and an explicit `0` there switches the threshold off.
Cosines differ per model, so no default ships: `search --eval` calibrates one from your cases. When the file has
positive and negative cases and a vector or hybrid mode ran, it prints the threshold that answers the most positives
while abstaining on the most negatives (`calibration` in the JSON) and each case's best cosine (`top_sim`). On this
repository's cases, `vector_min_sim = 0.596` answered 36 of 36 positives and abstained on 5 of 6 negatives; the
near-miss "deploy the application to a kubernetes cluster" (0.619) still matched a skill. `abstain_rate` is the share
of negative cases answered with nothing. The advice comes from a small sample: re-run the evaluation with the value
set before committing it.

## Building the index

```bash
ai-rulez search index --dry-run   # the host, the number of texts and bytes, an estimate: nothing is sent
ai-rulez search index             # embed what changed, write the index
ai-rulez search index --rebuild   # re-embed everything
ai-rulez search index --items refund-policy,deploy-staging   # force these, besides what changed
ai-rulez search status            # state of the index against the served skills
```

```text
$ ai-rulez search index --dry-run
provider   gemini@default   (allow_network=true)
model      gemini-embedding-001
items      25   cached 0   to embed 25   withheld 0   est. tokens 1627   est. cost <= $0.000244
fields     name, triggers, keywords, description   body: no
egress     25 texts, 4.7 KiB -> default
```

The embedded text of a skill is a fixed template over the configured fields (`name:`, `description:`, `triggers:`,
`keywords:` lines, then `body:` when `index_body` is set). A vector is reused when the provider, the model, the
fields and the SHA-256 of the exact text match, so editing a skill's body (with `index_body = false`) costs nothing, one edited description costs one
text (so does a rename, since the name is part of the text unless `fields` leaves it out), and a changed model or field set re-embeds everything:
vectors of different models are never mixed. A batch the provider rejects (a 4xx, an over-long input) is split in halves
until the refused skill stands alone, which is then skipped and named on stderr (`rejected` in the JSON summary);
the run goes on and exits 2. Five rejections in a row stop the build, since that is a broken provider rather than
bad skills. A budget, network, key or transient provider error stops the build too. Either way the vectors that
finished are written (atomically: `vectors.bin`, then `manifest.json`), the skills still missing are printed and the
run exits 2; those rank lexically. A second concurrent run is refused by `index.lock`.

`search status` reads files only and reports `none`, `unreadable`, `incompatible` (other provider, model, fields or
template), `stale` or `fresh`, with the skills that are current, changed since indexing (the embedded text
differs), not indexed, or indexed but no longer served.

The index is two files in `index_dir`:

- `manifest.json`: schema and text-template versions, provider fingerprint, model, dimensions, dtype, fields, the
  digest of `vectors.bin`, and per skill its id, domain, lock digest and text digest. It is deterministic (sorted,
  no timestamps), so a committed index diffs cleanly and rebuilding from unchanged inputs rewrites nothing.
- `vectors.bin`: little-endian, row-major, L2-normalised `float32` (or `float16`: 25 skills of 3072 dimensions are
  300 KiB, or 150 KiB in half precision). Loading checks the length against the manifest, the SHA-256, and rejects
  NaN, infinities and rows that are not unit length (a scaled-up row in a committed index would otherwise win every
  query); a file that does not validate is "no index", never a crash. Search is a brute-force dot product, clamped to
  [-1, 1], fine to tens of thousands of skills.

The default `local/search` is machine-local and gitignored. Set `index_dir` to a directory outside `local/` (for
example `search-index`) to commit the index so CI can run a hybrid evaluation without a key; `validate` then
checks it (`AR9D1`, a warning) when a description changed, the model changed or the files are missing or damaged,
and `search status` gives the full comparison against the served catalog. A poisoned index can only change the
order of results: `load_skill` still enforces the lock, approval and scan state of what it returns.

### Command provider

A program can produce the vectors instead of `[llm]`:

```toml
# user config only (or --allow-exec with a repository config)
[search.embeddings]
command  = ["/usr/local/bin/embed"]
model    = "my-local-model"      # recorded in the index
pass_env = ["EMBED_TOKEN"]
```

It runs as an argv (no shell) in the project root with only `PATH`, `HOME` and the `pass_env` variables, a 30 s
timeout and a 64 MiB stdout cap. It reads `{"input": ["text", ...]}` on stdin and prints
`{"vectors": [[...], ...]}`, one vector per input in order. Because it runs a program, a repository config cannot
set it: it is honoured from the user config file, or from the repository config when you pass `--allow-exec`
(otherwise it is ignored with a warning). The MCP server only honours the user config.

## Data egress

| Operation | Data sent | To |
| --- | --- | --- |
| `search index` | One text per changed skill: the configured fields (default name, triggers, keywords, description, already visible to agents) and, only with `index_body`, the first `body_chars` of the body | the embedding endpoint |
| `search`, `find_skill` in hybrid or vector mode | The query text (capped at 2 KiB; an agent's task description can hold user data) | the same endpoint |
| lexical mode | nothing | nowhere |

Before sending, each skill text goes through the secret scanner of `AR001`; a hit withholds that skill
(`AR9D3`, reported on stderr) instead of sending a masked string, and the skill ranks lexically only. Queries are
not scanned. `allow_network` stays off until you turn it on in the user config, loopback hosts included, and
`--dry-run` shows the host first. Choose a local or in-region endpoint for residency: the provider fingerprint is
recorded in the manifest, so a committed index shows where its vectors came from.

## Evaluating the ranking

```bash
ai-rulez search --eval search-cases.yaml
ai-rulez search --eval search-cases.yaml --min top1=0.6,mrr=0.7
ai-rulez search --eval search-cases.yaml --mode lexical,hybrid     # side by side, with paired intervals
ai-rulez search --eval search-cases.yaml --output result.json
ai-rulez search --eval search-cases.yaml --baseline result.json --max-flips 0
ai-rulez search --from-evals                                        # cases derived from the skills' eval cases
```

Use it to see whether a change to a skill description, to the ranker or to the embedding model helped, and to gate
a pull request on it. `--eval` takes the queries from the file; it does not accept a query argument. `--k`
overrides the file's cut-off; a value above 100 is an error, not clamped. `--mode` takes a comma-separated list
(`lexical`, `vector`, `hybrid`, or `all`); the first is the primary mode the gates check, and the default is the
configured mode. A mode that needs vectors needs the index.

`--from-evals` derives cases from the eval-runner cases under `skills/<name>/evals/*.eval.yaml` (see
[Evals](evals.md)), alone or added to `--eval`: a prompt with `expect_trigger: true` for skill S becomes a case
that expects S; a near-miss prompt, or any prompt with `expect_trigger: false`, becomes a case with `avoid: [S]`
(S must not rank first). Their ids are `<skill>/<case>` and they are tagged `from-evals`. A case whose skill is not
in the served catalog is skipped and counted on stderr.

## Cases file

```yaml
version: 1          # required
k: 5                # cut-off of recall@k, hit@k and nDCG@k (default 5, max 100)
cases:
  - id: refund-paraphrase
    query: "customer wants money back for a double charge"
    expect: [refund-policy]       # skill names; any of them found counts
    tags: [paraphrase, billing]
  - id: graded
    query: "set up staging deploy pipeline"
    expect: [{id: deploy-staging, grade: 3}, {id: ci-pipeline, grade: 1}]   # grades enable nDCG
  - id: scoped
    query: "rotate credentials"
    role: platform                # rank only the skills this role serves
    expect: [rotate-keys]
  - id: near-miss
    query: "deploy to production"
    expect: []
    avoid: [deploy-staging]       # must not rank first
  - id: negative
    query: "what is the weather tomorrow"
    expect: []                    # nothing relevant; reported separately
```

Skills are keyed by their catalog name. Problems in the file are reported together as `AR9D2` (exit 1): an unknown
field, a missing `version`, `id` or `query`, a duplicate case id, a duplicate or unknown skill in `expect` or
`avoid`, a grade outside 1-9, a skill both expected and avoided, an unknown `role`, or an expected skill the role
does not serve. A bare name has grade 1.

## Result and gates

Per mode: `top1` (a relevant skill is first), `recall_at_k` (relevant skills in the top k over relevant skills),
`hit_at_k` (any relevant skill in the top k), `mrr` (1 over the rank of the first relevant skill, 0 if none),
`ndcg_at_k` when a case is graded (gain is the grade, discount `log2(rank + 1)`, normalised by the ideal order of
that case's grades) and `avoid_top1` when cases carry an avoid list (the share where an avoided skill ranked
first; lower is better), overall and per tag. Negative cases are left out of the metrics and listed with the skill
that ranked first and its score, which is the ranker's own (BM25F, RRF or cosine) and only informational. A 95%
bootstrap interval (1000 resamples, fixed seed) is printed for `top1`, `recall_at_k`, `mrr` and `ndcg_at_k`; it is
never gated on. With several modes, `paired_vs_lexical` gives each mode's mean difference to lexical over the same
cases with its own interval: an interval that excludes 0 is a real change, which two overlapping per-mode intervals
cannot tell you. A vector mode that fell back to lexical for any case (no network, budget, timeout) is an error
(exit 1), never a pass, because the numbers would not measure the ranker asked for.

`--format json` prints [`search-eval.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search-eval.v1.schema.json),
which is also what `--output` writes and a later `--baseline` reads. The top-level cases, tags, intervals, misses and
negatives are those of the primary `mode`; `by_mode` holds every mode. Each case records `rank` (null when not
retrieved) and `hit`.

| Gate | Fails when |
| --- | --- |
| `--min top1=0.6,recall=0.8,hit=0.9,mrr=0.7,ndcg=0.7` | a metric of the primary mode is below its floor |
| `--baseline prev.json --max-flips N` | more than N cases went from `hit` to a miss since the baseline (default 0) |

With a small set one case moves `top1` by several points, so prefer the flip count to a raw floor. Cases present in
only one of the two runs are not compared. A baseline must come from the same primary
mode and the same `--k`; otherwise the run is refused (exit 1). `--output` is not written when the embeddings were
degraded, so a fallback run cannot become a baseline.

Exit codes: `0` every gate passed, `1` the command could not run (bad flags, invalid cases file, unknown skill,
unreadable baseline, a mode without an index, degraded embeddings), `2` a gate failed (or `search index` stopped
early). Failed gates are printed to stderr as `AR9D4`.

## Query mining

Real queries are the best test cases, but the [usage log](usage-telemetry.md) holds identifiers only. Mining is a
separate, opt-in step: with `[search] log_queries = true` in the user config file (or `AI_RULEZ_SEARCH_LOG_QUERIES=1`; a
repository config cannot turn it on; `search` and the MCP server warn when it tries), `find_skill` appends each query's text (secret-looking
queries are skipped, text is capped at 512 bytes) and, when the session then calls `load_skill`, the skill it
loaded, to `<config dir>/local/search-queries.jsonl` (mode 0600, gitignored, never sent anywhere, capped at 8 MiB,
session ids hashed). `search mine` turns it into cases:

```bash
ai-rulez search mine --output mined-cases.yaml    # a query followed in its session by loading S expects S
ai-rulez search mine --min-count 2 --purge     # keep queries seen twice with the same skill; delete the log
```

A query's label is the first skill loaded in its session before the next query; queries followed by different
skills with no clear winner, or by nothing, are dropped. The label is weak, since the agent chose the skill, which
does not make it right, so the cases are tagged `mined` and meant to be reviewed before they gate anything.

## Design decisions

Follows [issue #222](https://github.com/Goldziher/ai-rulez/issues/222); where it left a choice open the proposed
default is taken.

- Default stays lexical, offline and deterministic. A hybrid or vector ranking always falls back to lexical
  with a `degraded` reason; it never fails a search or a `find_skill` call.
- Embeddings go through `internal/llm` (one egress configuration, budget and gate), including the `literllm`
  backend for Gemini and 170 other providers. The command provider is the escape hatch for anything else.
- The index is keyed by the SHA-256 of the exact embedded text, not by the skill digest, so an edit that does not
  change the embedded text is free. `search status` and `stale_vector` therefore call a skill stale when its
  embedded text changed (the design said "item digest"); a changed digest with the same text is reported as
  drift and still ranks with its vector.
- Staleness in `validate` (`AR9D1`) compares the project's own skills present in both the index and the content
  tree; which skills an index holds depends on the serve flags it was built with, so missing and orphaned skills
  are reported by `search status` only.
- Loopback endpoints get no exemption from `allow_network`, and network settings are honoured from the user
  config only (see the trust rule in [LLM access](llm.md)).
- `dtype = "float16"` and `[search] log_queries` are additions to the design's config; calibration is part of
  `search --eval` rather than a `search status --calibrate` flag, and there is no LLM-suggested paraphrase step.
- The cases file is YAML (as proposed in the issue) and read strictly: unknown fields are errors, so a file never
  means less than it says. Flip comparison uses `hit_at_k`, the issue's "hit to miss".
- `AR9D2` and `AR9D4` are emitted by `search --eval`, `AR9D3` by `search index`, `AR9D0` and `AR9D1` by `validate`.
- `search_skills` (the MCP listing tool) stays lexical.
