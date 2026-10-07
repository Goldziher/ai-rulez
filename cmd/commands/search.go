package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch/setup"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// Exit statuses of `search --eval`: 0 pass, 1 cannot run (the generic error
// exit), 2 a gate (--min, --max-flips) failed.
const exitSearchGateFailed = 2

const (
	searchDefaultLimit = 5
	searchMaxLimit     = 20
	searchDescWidth    = 72
)

var searchFlags struct {
	limit         int
	format        string
	profile       string
	targets       string
	domains       []string
	allow         []string
	deny          []string
	sources       []string
	role          string
	includeStatic bool
	offline       bool
	frozen        bool
	mode          string
	explain       bool
	allowExec     bool
	eval          string
	fromEvals     bool
	k             int
	baseline      string
	out           string
	min           string
	maxFlips      int
}

// SearchCmd ranks the skills `mcp --serve-skills` would serve against a query.
var SearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Rank the served skills against a query, build the embedding index, or evaluate the ranking",
	Long: `Rank the skills that 'ai-rulez mcp --serve-skills' would serve against a query,
with the same ranker as the find_skill tool: lexical BM25F over name, triggers,
keywords and description (the default; no network, deterministic), or hybrid
(BM25F fused with cosine similarity over a bring-your-own embedding index, by
reciprocal rank fusion) with --mode hybrid or [search] mode. A hybrid or vector
search that cannot embed the query ranks lexically and says why ('degraded').
The catalog is selected with the flags of 'mcp --serve-skills': --profile,
--targets, --domain, --allow, --deny, --source, --role, --include-static,
--offline and --frozen.

Subcommands (the only argument): 'search index' builds or refreshes the
embedding index (--dry-run shows the host, the number of texts and bytes and an
estimate first, --rebuild re-embeds everything, --items re-embeds some skills;
only skills whose embedded text changed are sent, and a skill whose text looks
like it holds a secret is withheld, AR9D3), 'search status' reports the index
against the served skills without any network call, and 'search mine' turns the
opt-in query log into candidate cases (--out, --min-count, --purge). To search
for one of those words, add another word.

The embedding provider is [llm] (embedding_model, base_url, api_key_env and
allow_network, user scope) or a command provider ([search.embeddings] command,
user config or --allow-exec). The index lives in <config dir>/local/search
unless [search] index_dir names a directory to commit.

With --eval <cases.yaml> and/or --from-evals the command instead measures the
ranking against labeled queries and prints top-1, recall@k, hit@k, MRR and, for
graded cases, nDCG@k. --mode takes a comma-separated list (lexical, vector,
hybrid, or all) to compare rankings on the same cases with paired intervals.
--min fails the run when a metric is below a floor; --baseline <result.json>
with --max-flips fails it when more cases went from found to missed than
allowed. Save a run with --out and pass it as the next --baseline.

Exit codes: 0 success or all gates passed, 1 the command could not run (bad
flags, invalid cases file AR9D2, no catalog), 2 a gate failed (AR9D4) or an
index build stopped early.`,
	Example: `  ai-rulez search "customer wants money back"
  ai-rulez search --mode hybrid --explain "customer wants money back"
  ai-rulez search --format json --limit 10 deploy staging
  ai-rulez search index --dry-run
  ai-rulez search status
  ai-rulez search --eval search-cases.yaml --min top1=0.6,mrr=0.7
  ai-rulez search --eval search-cases.yaml --mode lexical,hybrid
  ai-rulez search --from-evals --out result.json
  ai-rulez search --eval search-cases.yaml --baseline result.json --max-flips 0`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if code := runSearch(cmd, cmd.OutOrStdout(), cmd.ErrOrStderr(), args); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	pf := SearchCmd.PersistentFlags()
	addFormatFlag(pf, &searchFlags.format, formatText, formatText, formatText, formatJSON)
	pf.StringVar(&searchFlags.profile, "profile", "", "Profile whose skills to search (default: the configured default profile)")
	pf.StringVar(&searchFlags.targets, "targets", "", "Preset whose rendering of the skills to search")
	pf.StringSliceVar(&searchFlags.domains, flagServeDomain, nil, "Only search skills of these domains; 'root' selects skills in no domain")
	pf.StringSliceVar(&searchFlags.allow, "allow", nil, "Only search skills whose name matches one of these glob patterns")
	pf.StringSliceVar(&searchFlags.deny, "deny", nil, "Never search skills whose name matches one of these glob patterns; wins over --allow")
	pf.StringArrayVar(&searchFlags.sources, flagServeSource, nil, "Also search the skills of a source, repeatable: [git+]<url>[@<tag|commit>][#<subdir>] or a local directory")
	pf.StringVar(&searchFlags.role, flagServeRole, "", "Search only the skills of this role (see 'ai-rulez roles list'); mutually exclusive with --profile")
	pf.BoolVar(&searchFlags.includeStatic, flagServeIncludeStatic, false, "Also search skills whose delivery is static")
	pf.BoolVar(&searchFlags.offline, flagServeOffline, false, "Never use the network; use cached content")
	pf.BoolVar(&searchFlags.frozen, flagServeFrozen, false, "Never use the network and require ai-rulez.lock to cover every remote source")
	pf.BoolVar(&searchFlags.allowExec, "allow-exec", false, "Honour [search.embeddings] command from the repository config (it runs a program; the user config needs no flag)")

	f := SearchCmd.Flags()
	f.IntVar(&searchFlags.limit, "limit", searchDefaultLimit, fmt.Sprintf("Maximum results (max %d)", searchMaxLimit))
	f.StringVar(&searchFlags.mode, "mode", "", "Ranking: lexical, hybrid or vector (default: [search] mode, then lexical). With --eval a comma-separated list, or all")
	f.BoolVar(&searchFlags.explain, "explain", false, "Show each skill's rank in the lexical and vector lists and how the query was embedded")
	f.StringVar(&searchFlags.eval, "eval", "", "Evaluate the ranking against the labeled queries of this YAML file instead of running a query")
	f.BoolVar(&searchFlags.fromEvals, "from-evals", false, "Evaluate with cases derived from the skills' eval-runner cases (alone, or added to --eval)")
	f.IntVar(&searchFlags.k, "k", 0, "With --eval: cut-off of recall@k, hit@k and nDCG@k (default: the file's k, then 5)")
	f.StringVar(&searchFlags.baseline, "baseline", "", "With --eval: a result file of an earlier run (--out) to compare against")
	f.StringVar(&searchFlags.out, "out", "", "With --eval: also write the result as JSON to this file")
	f.StringVar(&searchFlags.min, "min", "", "With --eval: fail when a metric is below a floor, e.g. top1=0.6,mrr=0.7 (metrics: top1, recall, hit, mrr, ndcg)")
	f.IntVar(&searchFlags.maxFlips, "max-flips", 0, "With --eval --baseline: fail when more cases than this went from found to missed")
}

// searchEvalOnlyFlags are the flags that only apply with --eval or --from-evals.
var searchEvalOnlyFlags = []string{"k", "baseline", "out", "min", "max-flips"}

// searchEnv is the catalog and the resolved search setup a search command works on.
type searchEnv struct {
	srv      *mcp.Server
	cat      *mcp.Catalog
	items    []skillsearch.Item
	resolved *setup.Resolved
	cfg      *config.Config
}

func newSearchEnv(ctx context.Context, mode string) (*searchEnv, error) {
	setupSrv := &mcp.ServeSetup{
		Version: Version, Profile: searchFlags.profile, Preset: searchFlags.targets, Role: searchFlags.role,
		Sources: searchFlags.sources, IncludeStatic: searchFlags.includeStatic, Frozen: searchFlags.frozen,
		Offline: searchFlags.offline, NoWatch: true, WorkDir: workingDir(),
	}
	setupSrv.Filter.Domains, setupSrv.Filter.Allow, setupSrv.Filter.Deny = searchFlags.domains, searchFlags.allow, searchFlags.deny
	applyServeNetworkPolicy(setupSrv.Frozen, setupSrv.Offline)
	srv, err := setupSrv.NewServer(ctx)
	if err != nil {
		return nil, oops.Wrapf(err, "search: build the skill catalog")
	}
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutRemote())
	if err != nil {
		return nil, oops.Wrapf(err, "search: load the configuration")
	}
	resolved, err := setup.Resolve(cfg, setup.Options{Mode: mode, AllowExec: searchFlags.allowExec})
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	for _, note := range resolved.Notes {
		logger.Warn(note)
	}
	cat := srv.Catalog()
	return &searchEnv{srv: srv, cat: cat, items: cat.CatalogItems(), resolved: resolved, cfg: cfg}, nil
}

// ranker builds the ranker for a mode: with an index and an embedder when the
// mode needs vectors. The returned function releases the embedder.
func (e *searchEnv) ranker(wantVectors bool) (*skillsearch.Ranker, func(), error) {
	r := &skillsearch.Ranker{Items: e.items, Cfg: e.resolved.Search, Timeout: time.Duration(e.resolved.Search.QueryTimeoutMS) * time.Millisecond}
	if !wantVectors {
		return r, func() {}, nil
	}
	release := func() {}
	emb, rel, err := e.resolved.Embedder()
	if err != nil {
		logger.Warn(err.Error())
	} else {
		r.Embedder, release = emb, rel
	}
	dir, err := e.resolved.IndexPath()
	if err != nil {
		return nil, release, err //nolint:wrapcheck // already contextual
	}
	idx, err := skillsearch.LoadIndex(dir)
	switch {
	case err == nil:
		if emb != nil {
			if reason := idx.Manifest.Reason(emb.Fingerprint(), emb.Model(), e.resolved.Search); reason != "" {
				logger.Warn("The search index is not usable: " + reason + "; run 'ai-rulez search index --rebuild'")
				return r, release, nil
			}
		}
		r.Index = idx
	case !errors.Is(err, skillsearch.ErrNoIndex):
		return nil, release, err //nolint:wrapcheck // already contextual
	default:
	}
	return r, release, nil
}

func runSearch(cmd *cobra.Command, out, errOut io.Writer, args []string) int {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = cmdContext()
	}
	if isSearchSubcommand(args) && searchFlags.eval == "" && !searchFlags.fromEvals {
		if err := checkSubcommandFlags(cmd, args[0]); err != nil {
			fmtError(err)
			return 1
		}
		return runSearchSubcommand(ctx, out, errOut, args[0])
	}
	if err := checkSearchFlags(cmd, args); err != nil {
		fmtError(err)
		return 1
	}
	evalMode := searchFlags.eval != "" || searchFlags.fromEvals
	mode := searchFlags.mode
	if evalMode {
		mode = "" // a list, parsed against the resolved setup below
	}
	env, err := newSearchEnv(ctx, mode)
	if err != nil {
		fmtError(err)
		return 1
	}
	if evalMode {
		return runSearchEval(ctx, out, errOut, env)
	}
	return runSearchQuery(ctx, out, errOut, env, strings.Join(args, " "))
}

// checkSubcommandFlags rejects flags that belong to a query, an evaluation or
// another subcommand: they would be silently ignored.
func checkSubcommandFlags(cmd *cobra.Command, sub string) error {
	if err := checkFormatFlag(searchFlags.format); err != nil {
		return err
	}
	others := []string{"limit", "mode", "explain", "k", "baseline", "min", "max-flips"}
	for name, flags := range searchSubFlagNames {
		if name != sub {
			others = append(others, flags...)
		}
	}
	if sub != searchSubMine {
		others = append(others, "out")
	}
	for _, name := range others {
		if cmd.Flags().Changed(name) {
			return oops.Errorf("--%s does not apply to 'search %s'", name, sub)
		}
	}
	return nil
}

func checkSearchFlags(cmd *cobra.Command, args []string) error {
	if err := checkFormatFlag(searchFlags.format); err != nil {
		return err
	}
	for sub, names := range searchSubFlagNames {
		for _, name := range names {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s requires 'search %s'", name, sub)
			}
		}
	}
	if searchFlags.k < 0 {
		return oops.Errorf("--k must not be negative")
	}
	if searchFlags.limit < 1 || searchFlags.limit > searchMaxLimit {
		return oops.Errorf("--limit must be between 1 and %d", searchMaxLimit)
	}
	evalMode := searchFlags.eval != "" || searchFlags.fromEvals
	if !evalMode {
		if strings.TrimSpace(strings.Join(args, " ")) == "" {
			return oops.Hint("Pass a query, or --eval <cases.yaml> / --from-evals").Errorf("search needs a query")
		}
		for _, name := range searchEvalOnlyFlags {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s requires --eval or --from-evals", name)
			}
		}
		if searchFlags.mode != "" {
			return checkQueryMode(searchFlags.mode)
		}
		return nil
	}
	if len(args) > 0 {
		return oops.Errorf("--eval takes its queries from the cases file; remove the query argument")
	}
	if searchFlags.explain {
		return oops.Errorf("--explain applies to a query, not to --eval")
	}
	if searchFlags.baseline == "" && cmd.Flags().Changed("max-flips") {
		return oops.Errorf("--max-flips requires --baseline")
	}
	return nil
}

func checkQueryMode(mode string) error {
	switch mode {
	case skillsearch.ModeLexical, skillsearch.ModeHybrid, skillsearch.ModeVector:
		return nil
	}
	return oops.Errorf("--mode %q must be lexical, hybrid or vector", mode)
}

type searchResultJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Score       float64  `json:"score"`
	Domain      string   `json:"domain"`
	Digest      string   `json:"digest"`
	LexicalRank *int     `json:"lexical_rank,omitempty"`
	VectorRank  *int     `json:"vector_rank,omitempty"`
	VectorSim   *float64 `json:"vector_sim,omitempty"`
	StaleVector bool     `json:"stale_vector,omitempty"`
}

type searchDoc struct {
	SchemaVersion int                `json:"schema_version"`
	Query         string             `json:"query"`
	Ranking       string             `json:"ranking"`
	Degraded      *string            `json:"degraded"`
	Abstained     bool               `json:"abstained,omitempty"`
	Count         int                `json:"count"`
	Results       []searchResultJSON `json:"results"`
}

func runSearchQuery(ctx context.Context, out, errOut io.Writer, env *searchEnv, query string) int {
	mode := env.resolved.Search.Mode
	ranker, release, err := env.ranker(mode != skillsearch.ModeLexical)
	defer release()
	if err != nil {
		fmtError(err)
		return 1
	}
	res := ranker.Search(ctx, query)
	hits := res.Hits
	if len(hits) > searchFlags.limit {
		hits = hits[:searchFlags.limit]
	}
	doc := searchDoc{SchemaVersion: skillsearch.ResultSchemaVersion, Query: query, Ranking: res.Ranking, Count: len(hits), Results: []searchResultJSON{}}
	doc.Abstained = res.Abstained
	if res.Degraded != "" {
		doc.Degraded = &res.Degraded
		reportWriter{errOut}.printf("%s\n", skillsearch.DegradedMessage(res.Degraded))
	}
	for i := range hits {
		h := hits[i]
		skill := env.cat.SkillAt(h.Index)
		domain := skill.Domain
		if domain == "" {
			domain = "root"
		}
		r := searchResultJSON{Name: skill.Name, Description: skill.Description, Score: h.Score, Domain: domain, Digest: skill.Digest, StaleVector: h.StaleVec}
		if h.LexRank > 0 {
			r.LexicalRank = &h.LexRank
		}
		if h.VecRank > 0 {
			r.VectorRank, r.VectorSim = &h.VecRank, &h.VecSim
		}
		doc.Results = append(doc.Results, r)
	}
	if searchFlags.format == formatJSON {
		return emit(writeJSON(out, doc))
	}
	if len(doc.Results) == 0 {
		if res.Abstained {
			reportWriter{out}.printf("no served skill is a confident match (best cosine %.3f is below vector_min_sim %.3f)\n", res.TopSim, env.resolved.Search.VectorMinSim)
			return 0
		}
		reportWriter{out}.printf("%s\n", "no served skill matches")
		return 0
	}
	return emit(printSearchText(out, doc, res, searchFlags.explain))
}

func printSearchText(out io.Writer, doc searchDoc, res skillsearch.SearchResult, explain bool) error {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	p := reportWriter{tw}
	if explain {
		p.printf("RANK\tSCORE\tLEX#\tVEC#\tNAME\tDOMAIN\n")
		for i, r := range doc.Results {
			p.printf("%d\t%.4f\t%s\t%s\t%s\t%s\n", i+1, r.Score, rankCell(r.LexicalRank), rankCell(r.VectorRank), r.Name, r.Domain)
		}
	} else {
		p.printf("RANK\tSCORE\tNAME\tDOMAIN\tDESCRIPTION\n")
		for i, r := range doc.Results {
			p.printf("%d\t%.4f\t%s\t%s\t%s\n", i+1, r.Score, r.Name, r.Domain, truncateText(r.Description, searchDescWidth))
		}
	}
	if err := tw.Flush(); err != nil {
		return oops.Wrapf(err, "write results")
	}
	if explain {
		w := reportWriter{out}
		w.printf("ranking: %s", res.Ranking)
		if res.Embedded {
			w.printf("   embed: 1 call, %d tokens, %d ms", res.Tokens, res.Elapsed.Milliseconds())
		}
		w.printf("\n")
	}
	return nil
}

func rankCell(r *int) string {
	if r == nil {
		return "-"
	}
	return fmt.Sprint(*r)
}

func writeJSONFile(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // a result file the user asked for
	if err != nil {
		return oops.Wrapf(err, "write %s", path)
	}
	if err := writeJSON(f, v); err != nil {
		_ = f.Close() //nolint:errcheck // the encode error is the one to report
		return err
	}
	return oops.Wrapf(f.Close(), "write %s", path)
}

// emit turns a write error into the failing exit status.
func emit(err error) int {
	if err != nil {
		fmtError(err)
		return 1
	}
	return 0
}

func truncateText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
