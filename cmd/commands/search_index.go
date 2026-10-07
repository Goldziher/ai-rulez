package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch/setup"
	"github.com/samber/oops"
)

const (
	searchSubcommandSchema = 1
	searchBytesPerKiB      = 1024
)

var searchSubFlags struct {
	dryRun   bool
	rebuild  bool
	items    []string
	minCount int
	purge    bool
}

// Subcommands of search. They are words of the search command rather than
// cobra children because a query is free text: `ai-rulez search deploy` must
// stay a query, which a command group cannot express.
const (
	searchSubIndex  = "index"
	searchSubStatus = "status"
	searchSubMine   = "mine"
)

// searchSubFlagNames are the flags that belong to one subcommand.
var searchSubFlagNames = map[string][]string{
	searchSubIndex: {"dry-run", "rebuild", "items"},
	searchSubMine:  {"min-count", "purge"},
}

func init() {
	f := SearchCmd.Flags()
	f.BoolVar(&searchSubFlags.dryRun, "dry-run", false, "With 'search index': show the provider, the number of texts and bytes and an estimate; send nothing")
	f.BoolVar(&searchSubFlags.rebuild, "rebuild", false, "With 'search index': re-embed every skill, ignoring the existing index")
	f.StringSliceVar(&searchSubFlags.items, "items", nil, "With 'search index': also re-embed these skills (by name) although their text did not change; skills that changed or have no vector are embedded anyway")
	f.IntVar(&searchSubFlags.minCount, "min-count", 1, "With 'search mine': keep a query only when it was followed by the same skill at least this many times")
	f.BoolVar(&searchSubFlags.purge, "purge", false, "With 'search mine': delete the query log after reading it")
}

// runSearchSubcommand runs 'search index|status|mine' when the only argument names one.
func runSearchSubcommand(ctx context.Context, out, errOut io.Writer, name string) int {
	switch name {
	case searchSubIndex:
		return runSearchIndex(ctx, out, errOut)
	case searchSubStatus:
		return runSearchStatus(ctx, out)
	default:
		return runSearchMine(ctx, out, errOut)
	}
}

func isSearchSubcommand(args []string) bool {
	return len(args) == 1 && (args[0] == searchSubIndex || args[0] == searchSubStatus || args[0] == searchSubMine)
}

type indexSummaryJSON struct {
	SchemaVersion int      `json:"schema_version"`
	DryRun        bool     `json:"dry_run"`
	Provider      string   `json:"provider"`
	Host          string   `json:"host"`
	Model         string   `json:"model"`
	Total         int      `json:"total"`
	Reused        int      `json:"reused"`
	Embedded      int      `json:"embedded"`
	ToEmbed       int      `json:"to_embed"`
	Withheld      []string `json:"withheld"`
	Missing       []string `json:"missing"`
	Bytes         int      `json:"bytes"`
	Tokens        int      `json:"tokens"`
	CostUSD       *float64 `json:"cost_usd"`
	Calls         int      `json:"calls"`
	Reason        string   `json:"rebuild_reason,omitempty"`
	Stopped       string   `json:"stopped,omitempty"`
	Rejected      []string `json:"rejected"`
	Written       bool     `json:"written"`
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return cmdContext()
	}
	return ctx
}

func runSearchIndex(ctx context.Context, out, errOut io.Writer) int {
	ctx = ctxOrBackground(ctx)
	if err := checkFormatFlag(searchFlags.format); err != nil {
		fmtError(err)
		return 1
	}
	env, err := newSearchEnv(ctx, "")
	if err != nil {
		fmtError(err)
		return 1
	}
	emb, release, err := env.resolved.Embedder()
	defer release()
	if err != nil {
		if !searchSubFlags.dryRun {
			fmtError(err)
			return 1
		}
		// A dry run still reports the plan when no model is configured yet.
		prov := env.resolved.Describe()
		emb = planOnlyEmbedder{fingerprint: prov.Fingerprint, model: prov.Model}
	}
	dir, err := env.resolved.IndexPath()
	if err != nil {
		fmtError(err)
		return 1
	}
	var old *skillsearch.Index
	if idx, loadErr := skillsearch.LoadIndex(dir); loadErr == nil {
		old = idx
	}
	only := map[string]bool{}
	known := skillsearch.IDs(env.items)
	for _, id := range searchSubFlags.items {
		if !slices.Contains(known, id) {
			fmtError(oops.Errorf("--items %q: no such served skill", id))
			return 1
		}
		only[id] = true
	}
	opts := &skillsearch.BuildOptions{
		Config: env.resolved.Search, Embedder: emb, Old: old, Rebuild: searchSubFlags.rebuild, Only: only, Scanner: lint.DetectSecret,
	}
	plan := skillsearch.PlanBuild(env.items, opts)
	prov := env.resolved.Describe()
	sum := indexSummaryJSON{
		SchemaVersion: searchSubcommandSchema, DryRun: searchSubFlags.dryRun, Provider: prov.Fingerprint, Host: prov.Host, Model: prov.Model,
		Total: plan.Total, Reused: plan.Reused, ToEmbed: plan.ToEmbed, Bytes: plan.Bytes, Tokens: plan.EstTokens, Reason: plan.Reason,
		Withheld: withheldIDs(plan), Missing: []string{}, Rejected: []string{},
	}
	for _, w := range plan.Withheld {
		reportWriter{errOut}.printf("%s: skill %q was not embedded: its text looks like it holds a secret (%s); it ranks lexically only\n", skillsearch.CodeTextWithheld, w.ID, w.Kind)
	}
	pricing := llm.NewPricing(env.resolved.LLM)
	cost, known2 := pricing.Cost(prov.Model, llm.Usage{PromptTokens: plan.EstTokens})
	if known2 {
		sum.CostUSD = &cost
	}
	if searchSubFlags.dryRun {
		if searchFlags.format == formatJSON {
			return emit(writeJSON(out, sum))
		}
		printIndexPlan(out, env, plan, prov, sum.CostUSD)
		return 0
	}
	if plan.ToEmbed > 0 && prov.Network && !env.resolved.LLM.AllowNetwork {
		fmtError(oops.Hint("Set allow_network = true in the user config file ("+userConfigHint()+") or AI_RULEZ_LLM_ALLOW_NETWORK=1; 'search index --dry-run' shows what would be sent").
			Errorf("search index would send %d texts (%d bytes) to %s, but the network is disabled", plan.ToEmbed, plan.Bytes, prov.Host))
		return 1
	}
	release2, err := skillsearch.Lock(dir, nil)
	if err != nil {
		fmtError(err)
		return 1
	}
	defer release2()
	res, err := skillsearch.Build(ctx, env.items, opts)
	if err != nil {
		fmtError(err)
		return 1
	}
	sum.Embedded, sum.Reused, sum.Calls, sum.Tokens, sum.Missing = res.Embedded, res.Reused, res.Calls, res.Tokens, res.Missing
	sum.Rejected = reportRejected(errOut, res.Rejected)
	if res.CostKnown {
		sum.CostUSD = &res.CostUSD
	} else {
		sum.CostUSD = nil
	}
	if res.Err != nil {
		sum.Stopped = llm.RedactSecrets(res.Err.Error())
	}
	if res.Index == nil {
		fmtError(oops.Errorf("nothing was embedded: %s", sum.Stopped))
		return 1
	}
	changed, err := writeIfChanged(dir, res.Index)
	if err != nil {
		fmtError(err)
		return 1
	}
	sum.Written = changed
	if searchFlags.format == formatJSON {
		if code := emit(writeJSON(out, sum)); code != 0 {
			return code
		}
	} else {
		printIndexResult(out, sum, dir)
	}
	return indexExitCode(errOut, res, &sum)
}

// indexExitCode reports why a build did not index every skill and returns the exit status: 2 for a skill the
// provider rejected or a stop (budget, provider), 0 for a complete build.
func indexExitCode(errOut io.Writer, res *skillsearch.BuildResult, sum *indexSummaryJSON) int {
	switch {
	case res.Err == nil && len(res.Rejected) > 0:
		reportWriter{errOut}.printf("search index skipped %d skills the provider rejected: %s\n", len(res.Rejected), strings.Join(sum.Rejected, ", "))
	case res.Err != nil:
		reportWriter{errOut}.printf("search index stopped early: %s\n  %d skills have no vector and rank lexically: %s\n", sum.Stopped, len(res.Missing), strings.Join(res.Missing, ", "))
	default:
		return 0
	}
	return exitSearchGateFailed
}

// reportRejected prints the skills the provider refused even alone and returns their ids.
func reportRejected(errOut io.Writer, rejected []skillsearch.Rejection) []string {
	ids := []string{}
	for _, r := range rejected {
		ids = append(ids, r.ID)
		reportWriter{errOut}.printf("search index: the provider rejected skill %q even alone (%s); it ranks lexically only\n", r.ID, r.Reason)
	}
	return ids
}

func withheldIDs(p *skillsearch.Plan) []string {
	out := []string{}
	for _, w := range p.Withheld {
		out = append(out, w.ID)
	}
	return out
}

func userConfigHint() string {
	return "~/.config/ai-rulez/config.toml"
}

func printIndexPlan(out io.Writer, env *searchEnv, plan *skillsearch.Plan, prov setup.Provider, cost *float64) {
	p := reportWriter{out}
	net := "no network"
	if prov.Network {
		net = fmt.Sprintf("allow_network=%t", env.resolved.LLM.AllowNetwork)
	}
	p.printf("provider   %s   (%s)\n", prov.Fingerprint, net)
	p.printf("model      %s\n", orUnset(prov.Model))
	costText := "unknown (no price for this model; set price_input_per_mtok in the user config)"
	if cost != nil {
		costText = fmt.Sprintf("<= $%.6f", *cost)
	}
	p.printf("items      %d   cached %d   to embed %d   withheld %d   est. tokens %d   est. cost %s\n", plan.Total, plan.Reused, plan.ToEmbed, len(plan.Withheld), plan.EstTokens, costText)
	body := "no"
	if env.resolved.Search.IndexBody {
		body = fmt.Sprintf("first %d characters", env.resolved.Search.BodyChars)
	}
	p.printf("fields     %s   body: %s\n", strings.Join(env.resolved.Search.Fields, ", "), body)
	p.printf("egress     %d texts, %.1f KiB -> %s\n", plan.ToEmbed, float64(plan.Bytes)/searchBytesPerKiB, prov.Host)
	if plan.Reason != "" {
		p.printf("rebuild    the existing index cannot be reused: %s\n", plan.Reason)
	}
}

func orUnset(s string) string {
	if s == "" {
		return "(none configured)"
	}
	return s
}

func printIndexResult(out io.Writer, s indexSummaryJSON, dir string) {
	p := reportWriter{out}
	cost := "unknown"
	if s.CostUSD != nil {
		cost = fmt.Sprintf("$%.6f", *s.CostUSD)
	}
	state := "index is current, nothing written"
	if s.Written {
		state = "wrote " + dir
	}
	p.printf("embedded %d, reused %d, withheld %d of %d skills   (%d calls, %d tokens, cost %s)\n%s\n", s.Embedded, s.Reused, len(s.Withheld), s.Total, s.Calls, s.Tokens, cost, state)
}

// writeIfChanged writes the index unless the bytes on disk are already these.
func writeIfChanged(dir string, idx *skillsearch.Index) (bool, error) {
	manifest, vectors, err := idx.Bytes()
	if err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	oldM, errM := os.ReadFile(filepath.Join(dir, skillsearch.ManifestFile)) //nolint:gosec // project-controlled path
	oldV, errV := os.ReadFile(filepath.Join(dir, skillsearch.VectorsFile))  //nolint:gosec // project-controlled path
	if errM == nil && errV == nil && bytes.Equal(oldM, manifest) && bytes.Equal(oldV, vectors) {
		return false, nil
	}
	return true, skillsearch.WriteIndex(dir, idx) //nolint:wrapcheck // already contextual
}

type statusJSON struct {
	SchemaVersion int      `json:"schema_version"`
	Mode          string   `json:"mode"`
	IndexDir      string   `json:"index_dir"`
	Committed     bool     `json:"committed"`
	State         string   `json:"state"`
	Reason        string   `json:"reason,omitempty"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	IndexProvider string   `json:"index_provider,omitempty"`
	IndexModel    string   `json:"index_model,omitempty"`
	Dims          int      `json:"dims,omitempty"`
	DType         string   `json:"dtype,omitempty"`
	Rows          int      `json:"rows"`
	Bytes         int64    `json:"bytes"`
	Skills        int      `json:"skills"`
	Current       int      `json:"current"`
	Drifted       int      `json:"drifted"`
	Stale         []string `json:"stale"`
	Missing       []string `json:"missing"`
	Orphaned      []string `json:"orphaned"`
	Hint          string   `json:"hint,omitempty"`
}

// Index states of `search status`.
const (
	stateNone         = "none"
	stateUnreadable   = "unreadable"
	stateIncompatible = "incompatible"
	stateStale        = "stale"
	stateFresh        = "fresh"
)

func runSearchStatus(ctx context.Context, out io.Writer) int {
	ctx = ctxOrBackground(ctx)
	if err := checkFormatFlag(searchFlags.format); err != nil {
		fmtError(err)
		return 1
	}
	env, err := newSearchEnv(ctx, "")
	if err != nil {
		fmtError(err)
		return 1
	}
	dir, err := env.resolved.IndexPath()
	if err != nil {
		fmtError(err)
		return 1
	}
	prov := env.resolved.Describe()
	doc := statusJSON{
		SchemaVersion: searchSubcommandSchema, Mode: env.resolved.Search.Mode, IndexDir: dir, Committed: skillsearch.CommittedIndexDir(env.resolved.Search.IndexDir),
		Provider: prov.Fingerprint, Model: prov.Model, Skills: len(env.items), Stale: []string{}, Missing: []string{}, Orphaned: []string{},
	}
	idx, err := skillsearch.LoadIndex(dir)
	switch {
	case errors.Is(err, skillsearch.ErrNoIndex):
		doc.State, doc.Hint = stateNone, "run 'ai-rulez search index'"
		if _, statErr := os.Stat(filepath.Join(dir, skillsearch.ManifestFile)); statErr == nil {
			doc.State, doc.Reason, doc.Hint = stateUnreadable, err.Error(), "rebuild with 'ai-rulez search index --rebuild'"
		}
	case err != nil:
		fmtError(err)
		return 1
	default:
		fillIndexStatus(&doc, idx, env, prov)
	}
	if info, statErr := os.Stat(filepath.Join(dir, skillsearch.VectorsFile)); statErr == nil {
		doc.Bytes = info.Size()
	}
	if searchFlags.format == formatJSON {
		return emit(writeJSON(out, doc))
	}
	printStatus(out, doc)
	return 0
}

func fillIndexStatus(doc *statusJSON, idx *skillsearch.Index, env *searchEnv, prov setup.Provider) {
	m := &idx.Manifest
	doc.IndexProvider, doc.IndexModel, doc.Dims, doc.DType, doc.Rows = m.Provider, m.Model, m.Dims, m.DType, idx.Len()
	if reason := m.Reason(prov.Fingerprint, prov.Model, env.resolved.Search); reason != "" {
		doc.State, doc.Reason, doc.Hint = stateIncompatible, reason, "rebuild with 'ai-rulez search index --rebuild'"
	}
	st := idx.Check(env.items, env.resolved.Search)
	doc.Current, doc.Drifted = st.Current, st.Drifted
	doc.Stale, doc.Missing, doc.Orphaned = nonNilStrings(st.Stale), nonNilStrings(st.Missing), nonNilStrings(st.Orphaned)
	if doc.State != "" {
		return
	}
	if st.Fresh() {
		doc.State = stateFresh
		return
	}
	doc.State, doc.Hint = stateStale, "run 'ai-rulez search index' to embed the changed and missing skills"
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func printStatus(out io.Writer, d statusJSON) {
	p := reportWriter{out}
	where := d.IndexDir
	if d.Committed {
		where += "  (committed location)"
	}
	p.printf("index      %s\n", where)
	p.printf("state      %s\n", d.State)
	if d.Reason != "" {
		p.printf("reason     %s\n", d.Reason)
	}
	p.printf("mode       %s\n", d.Mode)
	p.printf("provider   %s   model %s   (config)\n", d.Provider, orUnset(d.Model))
	if d.Rows > 0 {
		p.printf("built with %s   model %s   %d dims   %s   %d rows   %.1f KiB\n", d.IndexProvider, d.IndexModel, d.Dims, d.DType, d.Rows, float64(d.Bytes)/searchBytesPerKiB)
		p.printf("skills     %d served   %d current   %d changed   %d not indexed   %d no longer served\n", d.Skills, d.Current, len(d.Stale), len(d.Missing), len(d.Orphaned))
		if d.Drifted > 0 {
			p.printf("           %d skills changed outside their embedded text; their vectors are still valid\n", d.Drifted)
		}
		for _, g := range []struct {
			label string
			ids   []string
		}{{"changed", d.Stale}, {"not indexed", d.Missing}, {"no longer served", d.Orphaned}} {
			if len(g.ids) > 0 {
				p.printf("  %s: %s\n", g.label, strings.Join(g.ids, ", "))
			}
		}
	}
	if d.Hint != "" {
		p.printf("next       %s\n", d.Hint)
	}
}

func runSearchMine(ctx context.Context, out, errOut io.Writer) int {
	ctx = ctxOrBackground(ctx)
	if searchSubFlags.minCount < 1 {
		fmtError(oops.Errorf("--min-count must be at least 1"))
		return 1
	}
	env, err := newSearchEnv(ctx, "")
	if err != nil {
		fmtError(err)
		return 1
	}
	path := skillsearch.LogPath(env.cfg.ConfigDir)
	entries, err := skillsearch.ReadLog(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	if len(entries) == 0 {
		reportWriter{errOut}.printf("the query log %s is empty; set [search] log_queries = true in the user config (or AI_RULEZ_SEARCH_LOG_QUERIES=1) and use find_skill first\n", path)
	}
	mined := skillsearch.Mine(entries, skillsearch.MineOptions{Known: skillsearch.IDs(env.items), MinCount: searchSubFlags.minCount})
	raw, err := skillsearch.CasesYAML(0, mined.Cases)
	if err != nil {
		fmtError(err)
		return 1
	}
	if len(mined.Cases) == 0 {
		raw = []byte("# no query could be labelled\n")
	}
	if searchFlags.out != "" {
		if err := os.WriteFile(searchFlags.out, raw, 0o644); err != nil { //nolint:gosec // a cases file the user asked for
			fmtError(oops.Wrapf(err, "write %s", searchFlags.out))
			return 1
		}
	} else if _, err := out.Write(raw); err != nil {
		fmtError(oops.Wrapf(err, "write cases"))
		return 1
	}
	reportWriter{errOut}.printf("mined %d cases from %d distinct queries (%d never followed by a load, %d ambiguous, %d below --min-count); labels are weak: review before gating\n",
		len(mined.Cases), mined.Queries, mined.Unlabelled, mined.Ambiguous, mined.Below)
	if searchSubFlags.purge {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmtError(oops.Wrapf(err, "delete the query log"))
			return 1
		}
	}
	return 0
}

// planOnlyEmbedder stands in for the embedder of a dry run that has none: it
// names the provider and model and never embeds.
type planOnlyEmbedder struct{ fingerprint, model string }

func (planOnlyEmbedder) Embed(context.Context, []string) (skillsearch.Embedding, error) {
	return skillsearch.Embedding{}, oops.Errorf("no embedding provider is configured")
}
func (p planOnlyEmbedder) Fingerprint() string { return p.fingerprint }
func (p planOnlyEmbedder) Model() string       { return p.model }
