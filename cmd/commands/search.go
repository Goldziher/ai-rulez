package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
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
	eval          string
	k             int
	baseline      string
	out           string
	min           string
	maxFlips      int
}

// SearchCmd ranks the skills `mcp --serve-skills` would serve against a query.
var SearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Rank the served skills against a query, or evaluate the ranking against labeled queries",
	Long: `Rank the skills that 'ai-rulez mcp --serve-skills' would serve against a query,
with the same lexical BM25F ranker as the find_skill tool (name, triggers,
keywords and description; no network, deterministic). The catalog is selected
with the flags of 'mcp --serve-skills': --profile, --targets, --domain, --allow,
--deny, --source, --role, --include-static, --offline and --frozen.

With --eval <cases.yaml> the command instead measures the ranking against
labeled queries and prints top-1, recall@k, hit@k and MRR. --min fails the run
when a metric is below a floor; --baseline <result.json> with --max-flips fails
it when more cases went from found to missed than allowed. Save a run with
--out and pass it as the next --baseline.

Exit codes: 0 success or all gates passed, 1 the command could not run (bad
flags, invalid cases file AR9D2, no catalog), 2 a gate failed (AR9D4).`,
	Example: `  ai-rulez search "customer wants money back"
  ai-rulez search --format json --limit 10 deploy staging
  ai-rulez search --eval search-cases.yaml --min top1=0.6,mrr=0.7
  ai-rulez search --eval search-cases.yaml --out result.json
  ai-rulez search --eval search-cases.yaml --baseline result.json --max-flips 0`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if code := runSearch(cmd, cmd.OutOrStdout(), cmd.ErrOrStderr(), args); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := SearchCmd.Flags()
	f.IntVar(&searchFlags.limit, "limit", searchDefaultLimit, fmt.Sprintf("Maximum results (max %d)", searchMaxLimit))
	f.StringVar(&searchFlags.format, "format", formatText, "Output format: text or json")
	f.StringVar(&searchFlags.profile, "profile", "", "Profile whose skills to search (default: the configured default profile)")
	f.StringVar(&searchFlags.targets, "targets", "", "Preset whose rendering of the skills to search")
	f.StringSliceVar(&searchFlags.domains, flagServeDomain, nil, "Only search skills of these domains; 'root' selects skills in no domain")
	f.StringSliceVar(&searchFlags.allow, "allow", nil, "Only search skills whose name matches one of these glob patterns")
	f.StringSliceVar(&searchFlags.deny, "deny", nil, "Never search skills whose name matches one of these glob patterns; wins over --allow")
	f.StringArrayVar(&searchFlags.sources, flagServeSource, nil, "Also search the skills of a source, repeatable: [git+]<url>[@<tag|commit>][#<subdir>] or a local directory")
	f.StringVar(&searchFlags.role, flagServeRole, "", "Search only the skills of this role (see 'ai-rulez roles list'); mutually exclusive with --profile")
	f.BoolVar(&searchFlags.includeStatic, flagServeIncludeStatic, false, "Also search skills whose delivery is static")
	f.BoolVar(&searchFlags.offline, flagServeOffline, false, "Never use the network; use cached content")
	f.BoolVar(&searchFlags.frozen, flagServeFrozen, false, "Never use the network and require ai-rulez.lock to cover every remote source")
	f.StringVar(&searchFlags.eval, "eval", "", "Evaluate the ranking against the labeled queries of this YAML file instead of running a query")
	f.IntVar(&searchFlags.k, "k", 0, "With --eval: cut-off of recall@k and hit@k (default: the file's k, then 5)")
	f.StringVar(&searchFlags.baseline, "baseline", "", "With --eval: a result file of an earlier run (--out) to compare against")
	f.StringVar(&searchFlags.out, "out", "", "With --eval: also write the result as JSON to this file")
	f.StringVar(&searchFlags.min, "min", "", "With --eval: fail when a metric is below a floor, e.g. top1=0.6,mrr=0.7 (metrics: top1, recall, hit, mrr)")
	f.IntVar(&searchFlags.maxFlips, "max-flips", 0, "With --eval --baseline: fail when more cases than this went from found to missed")
}

// searchEvalOnlyFlags are the flags that only apply with --eval.
var searchEvalOnlyFlags = []string{"k", "baseline", "out", "min", "max-flips"}

func runSearch(cmd *cobra.Command, out, errOut io.Writer, args []string) int {
	if err := checkSearchFlags(cmd, args); err != nil {
		fmtError(err)
		return 1
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	setup := &mcp.ServeSetup{
		Version: Version, Profile: searchFlags.profile, Preset: searchFlags.targets, Role: searchFlags.role,
		Sources: searchFlags.sources, IncludeStatic: searchFlags.includeStatic, Frozen: searchFlags.frozen,
		Offline: searchFlags.offline, NoWatch: true,
	}
	setup.Filter.Domains, setup.Filter.Allow, setup.Filter.Deny = searchFlags.domains, searchFlags.allow, searchFlags.deny
	applyServeNetworkPolicy(setup.Frozen, setup.Offline)
	srv, err := setup.NewServer(ctx)
	if err != nil {
		fmtError(oops.Wrapf(err, "search: build the skill catalog"))
		return 1
	}
	cat := srv.Catalog()
	if searchFlags.eval != "" {
		return runSearchEval(out, errOut, cat)
	}
	return runSearchQuery(out, cat, strings.Join(args, " "))
}

func checkSearchFlags(cmd *cobra.Command, args []string) error {
	if f := searchFlags.format; f != formatText && f != formatJSON {
		return oops.Errorf("unknown --format %q (use text or json)", f)
	}
	if searchFlags.limit < 1 || searchFlags.limit > searchMaxLimit {
		return oops.Errorf("--limit must be between 1 and %d", searchMaxLimit)
	}
	if searchFlags.eval == "" {
		if strings.TrimSpace(strings.Join(args, " ")) == "" {
			return oops.Hint("Pass a query, or --eval <cases.yaml>").Errorf("search needs a query")
		}
		for _, name := range searchEvalOnlyFlags {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s requires --eval", name)
			}
		}
		return nil
	}
	if len(args) > 0 {
		return oops.Errorf("--eval takes its queries from the cases file; remove the query argument")
	}
	if searchFlags.baseline == "" && cmd.Flags().Changed("max-flips") {
		return oops.Errorf("--max-flips requires --baseline")
	}
	return nil
}

type searchResultJSON struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Domain      string  `json:"domain"`
	Digest      string  `json:"digest"`
}

type searchDoc struct {
	SchemaVersion int                `json:"schema_version"`
	Query         string             `json:"query"`
	Count         int                `json:"count"`
	Results       []searchResultJSON `json:"results"`
}

func runSearchQuery(out io.Writer, cat *mcp.Catalog, query string) int {
	hits := cat.Rank(query)
	if len(hits) > searchFlags.limit {
		hits = hits[:searchFlags.limit]
	}
	doc := searchDoc{SchemaVersion: skillsearch.ResultSchemaVersion, Query: query, Count: len(hits), Results: []searchResultJSON{}}
	for _, h := range hits {
		domain := h.Skill.Domain
		if domain == "" {
			domain = "root"
		}
		doc.Results = append(doc.Results, searchResultJSON{
			Name: h.Skill.Name, Description: h.Skill.Description, Score: h.Score, Domain: domain, Digest: h.Skill.Digest,
		})
	}
	if searchFlags.format == formatJSON {
		return emit(writeJSON(out, doc))
	}
	if len(doc.Results) == 0 {
		reportWriter{out}.printf("%s\n", "no served skill matches")
		return 0
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	p := reportWriter{tw}
	p.printf("RANK\tSCORE\tNAME\tDOMAIN\tDESCRIPTION\n")
	for i, r := range doc.Results {
		p.printf("%d\t%.4f\t%s\t%s\t%s\n", i+1, r.Score, r.Name, r.Domain, truncateText(r.Description, searchDescWidth))
	}
	return emit(tw.Flush())
}

func runSearchEval(out, errOut io.Writer, cat *mcp.Catalog) int {
	fail := func(err error) int {
		fmtError(err)
		return 1
	}
	mins, err := skillsearch.ParseMinimums(searchFlags.min)
	if err != nil {
		return fail(err)
	}
	cases, err := skillsearch.LoadCases(searchFlags.eval)
	if err != nil {
		return fail(err)
	}
	docs, ids := cat.SearchDocs()
	if err := cases.CheckSkills(ids); err != nil {
		return fail(err)
	}
	res := skillsearch.Eval(docs, ids, cases, cases.EffectiveK(searchFlags.k))
	maxFlips := -1
	if searchFlags.baseline != "" {
		base, err := skillsearch.LoadBaseline(searchFlags.baseline)
		if err != nil {
			return fail(err)
		}
		res.CompareBaseline(base)
		maxFlips = searchFlags.maxFlips
	}
	res.GateFailures = res.Gate(mins, maxFlips)
	if searchFlags.out != "" {
		if err := writeJSONFile(searchFlags.out, res); err != nil {
			return fail(err)
		}
	}
	if searchFlags.format == formatJSON {
		if err := writeJSON(out, res); err != nil {
			return fail(err)
		}
	} else {
		printEvalText(out, res)
	}
	for _, msg := range res.GateFailures {
		reportWriter{errOut}.printf("%s\n", msg)
	}
	if len(res.GateFailures) > 0 {
		return exitSearchGateFailed
	}
	return 0
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

func printEvalText(out io.Writer, r *skillsearch.Result) {
	p := reportWriter{out}
	m := r.Modes[skillsearch.ModeLexical]
	p.printf("cases: %d (+%d negative), k=%d\n", r.N, r.NNegative, r.K)
	p.printf("top1      %.4f  (95%% CI %.4f-%.4f)\n", m.Top1, r.CI95["top1"].Low, r.CI95["top1"].High)
	p.printf("recall@%d  %.4f  (95%% CI %.4f-%.4f)\n", r.K, m.RecallAt, r.CI95["recall_at_k"].Low, r.CI95["recall_at_k"].High)
	p.printf("hit@%d     %.4f\n", r.K, m.HitAt)
	p.printf("mrr       %.4f  (95%% CI %.4f-%.4f)\n", m.MRR, r.CI95["mrr"].Low, r.CI95["mrr"].High)
	if len(r.Misses) > 0 {
		reportWriter{out}.printf("%s\n", "\nmisses:")
		for i := range r.Misses {
			c := &r.Misses[i]
			rank := "not retrieved"
			if c.Rank != nil {
				rank = fmt.Sprintf("rank %d", *c.Rank)
			}
			p.printf("  %s: expected %s, got [%s] (%s)\n", c.ID, strings.Join(c.Expected, ","), strings.Join(c.Got, ","), rank)
		}
	}
	if r.Flips != nil {
		p.printf("\nvs baseline: %d regressed, %d fixed\n", len(r.Flips.Regressed), len(r.Flips.Fixed))
		for _, id := range r.Flips.Regressed {
			p.printf("  regressed: %s\n", id)
		}
	}
	if len(r.GateFailures) > 0 {
		reportWriter{out}.printf("%s\n", "\ngate failed")
	}
}

// searchExit turns a write error into the failing exit status.
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
