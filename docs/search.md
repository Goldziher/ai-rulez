# Skill Search

`ai-rulez search` ranks the skills your project serves against a query, with the same ranker as the `find_skill`
tool of [`mcp --serve-skills`](mcp-server.md), and measures that ranking against labelled queries. It is lexical,
offline and deterministic: nothing is sent anywhere and the same input always gives the same order.

- [Searching](#searching)
- [The ranker](#the-ranker)
- [Evaluating the ranking](#evaluating-the-ranking)
- [Cases file](#cases-file)
- [Result and gates](#result-and-gates)
- [Design decisions](#design-decisions)

## Searching

```bash
ai-rulez search "customer wants money back"
ai-rulez search --format json --limit 10 deploy staging
ai-rulez search --role platform rotate credentials
```

The catalog is what the server would serve, selected with the same flags: `--profile`, `--targets`, `--domain`,
`--allow`, `--deny`, `--source` (repeatable), `--role`, `--include-static`, `--offline` and `--frozen`. `--role` and
`--profile` are mutually exclusive. A skill the server refuses (lock or scan state) is not in the catalog and is
not ranked. Several words are joined into one query.

Text output lists rank, score, name, domain and description. `--format json` prints the schema
[`search.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search.v1.schema.json):

```json
{"schema_version": 1, "query": "money back", "count": 1,
 "results": [{"name": "refund-policy", "description": "...", "score": 4.21, "domain": "root", "digest": "sha256:..."}]}
```

An empty or stopword-only query, or one nothing matches, returns no results. Unlike `find_skill`, there is no
in-role-first reordering: with `--role` the catalog already holds only that role's skills.

## The ranker

BM25F over four fields of a skill with weights name 3, triggers 2.5, keywords 2 and description 1, after
lowercasing, dropping stopwords and a light suffix stemmer (`migrations` matches `migration`). Hits are ordered by
score descending, then name. The code is `internal/skillsearch`; `find_skill` calls the same function, and a
golden test pins the order and scores of `find_skill` so the ranker cannot change by accident.

## Evaluating the ranking

```bash
ai-rulez search --eval search-cases.yaml
ai-rulez search --eval search-cases.yaml --min top1=0.6,mrr=0.7
ai-rulez search --eval search-cases.yaml --out result.json
ai-rulez search --eval search-cases.yaml --baseline result.json --max-flips 0
```

Use it to see whether a change to a skill description or to the ranker helped, and to gate a pull request on it.
`--eval` takes the queries from the file; it does not accept a query argument. `--k` overrides the file's cut-off.

## Cases file

```yaml
version: 1          # required
k: 5                # cut-off of recall@k and hit@k (default 5, max 100)
cases:
  - id: refund-paraphrase
    query: "customer wants money back for a double charge"
    expect: [refund-policy]       # skill names; any of them found counts
    tags: [paraphrase, billing]
  - id: negative
    query: "what is the weather tomorrow"
    expect: []                    # nothing relevant; reported separately
```

Skills are keyed by their catalog name. Problems in the file are reported together as `AR9D2` (exit 1): an unknown
field, a missing `version`, `id` or `query`, a duplicate case id, a duplicate or unknown skill in `expect`.
`role` and graded `expect` entries (`{id, grade}`) are not supported yet and are rejected rather than ignored.

## Result and gates

Per run: `top1` (a relevant skill is first), `recall_at_k` (relevant skills in the top k over relevant skills),
`hit_at_k` (any relevant skill in the top k) and `mrr` (1 over the rank of the first relevant skill, 0 if
none), overall and per tag. Negative cases are left out of the metrics and listed with the skill that ranked first
and its score. A 95% bootstrap interval (1000 resamples, fixed seed) is printed for `top1`, `recall_at_k` and `mrr`;
it is never gated on.

`--format json` prints [`search-eval.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/search-eval.v1.schema.json),
which is also what `--out` writes and a later `--baseline` reads. Each case records `rank` (null when not
retrieved) and `hit`.

| Gate | Fails when |
| --- | --- |
| `--min top1=0.6,recall=0.8,hit=0.9,mrr=0.7` | a metric is below its floor |
| `--baseline prev.json --max-flips N` | more than N cases went from `hit` to a miss since the baseline (default 0) |

With a small set one case moves `top1` by several points, so prefer the flip count to a raw floor. Cases present in
only one of the two runs are not compared.

Exit codes: `0` every gate passed, `1` the command could not run (bad flags, invalid cases file, unknown skill,
unreadable baseline), `2` a gate failed. Failed gates are printed to stderr as `AR9D4`.

## Design decisions

Follows [issue #222](https://github.com/Goldziher/ai-rulez/issues/222), phase 0 (lexical only).

- Embeddings, the vector index, hybrid fusion and `--from-evals` are later phases and are not implemented; `modes`
  in the result holds `lexical` only, so adding modes later does not change the format.
- The cases file is YAML (as proposed in the issue) and read strictly: unknown fields are errors, so a file never
  means less than it says.
- Flip comparison uses `hit_at_k`, the issue's "hit to miss".
- `AR9D2` and `AR9D4` are emitted by `search --eval` only; they are not yet findings of `validate`.
