package commands

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

const (
	// judgeDefaultMaxCost is the default --max-cost, in USD.
	judgeDefaultMaxCost = 0.25
	// judgeDefaultMinScore is the default --min-score.
	judgeDefaultMinScore = 0.7
)

// judgeClient is the model client a judge run uses: the middleware stack plus its accounting.
type judgeClient interface {
	llm.Client
	Spent() llm.Spent
}

// judgeDeps are the outside inputs of the command, replaced in tests: the model
// client, the environment and the organization policy.
type judgeDeps struct {
	newClient    func(llm.Config, llm.Options) (judgeClient, error)
	getenv       func(string) string
	policyLocked func(projectDir string) bool
}

func defaultJudgeDeps() judgeDeps {
	return judgeDeps{
		newClient: func(lc llm.Config, opts llm.Options) (judgeClient, error) {
			managed, err := llm.New(lc, opts)
			if err != nil {
				return nil, err //nolint:wrapcheck // typed llm error
			}
			return managed, nil
		},
		getenv:       ambient.GetenvFunc(nil),
		policyLocked: func(dir string) bool { return config.PolicyLocksIn(activePolicy, "llm", dir) },
	}
}

type judgeFlags struct {
	model, provider, rubric                             string
	maxCost, minScore                                   float64
	maxTokens                                           int
	allowLLM, estimate, dryRun, redact, noCache, asJSON bool
}

func newJudgeCmd(deps judgeDeps) *cobra.Command {
	var f judgeFlags
	cmd := &cobra.Command{
		Use:   "judge [paths...]",
		Short: "Grade instruction files with a model (AI as a judge), without any configuration",
		Long: `Grade agent instruction files against a rubric with a language model. It needs no config file:
the model comes from flags and AI_RULEZ_LLM_* variables, the key from the provider's own variable
(OPENAI_API_KEY, ANTHROPIC_API_KEY, ...), and no ai-rulez or user config is read.

With no paths it judges AGENTS.md (at any depth) and the .agents/ and .ai-rulez/ content: rules, context,
skills, agents, commands and checks. Name files or directories to judge those instead. A file that carries
the ai-rulez generated header is skipped: judge its sources.

File content is sent to the model, so it needs --allow-llm. The egress host and model are printed before
anything is sent. A file with secret-looking text is not sent and counts as a finding; --redact masks the
text and sends it. Long files keep their head and tail. Symlinks are never followed.

--estimate (or --dry-run) lists the files, the egress host and model, and the token and cost estimate, and
sends nothing. Spend is capped by --max-cost (default $0.25); the run refuses to start when the worst case
exceeds it.

Exit codes: 0 every file passed, 1 the run could not complete or every selected file was skipped, 2 a file scored below --min-score or was
skipped for secrets. Under an organization policy that forbids model use, only --estimate runs. See docs/judge.md.`,
		Example: `  ai-rulez judge --estimate --model openai/gpt-4o-mini
  ai-rulez judge --allow-llm --model openai/gpt-4o-mini
  ai-rulez judge docs/guide.md --allow-llm --model anthropic/claude-haiku-4-5 --min-score 0.8
  ai-rulez judge --allow-llm --rubric rubrics/mine.md --format json`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fail(deps.run(cmd, args, &f))
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.model, "model", "", "Model as provider/model (or AI_RULEZ_LLM_MODEL), for example openai/gpt-4o-mini")
	fl.StringVar(&f.provider, "provider", "", "Provider prefix for a --model without one (or AI_RULEZ_LLM_PROVIDER)")
	fl.Float64Var(&f.maxCost, "max-cost", judgeDefaultMaxCost, "Spend cap in USD (0 removes it; the model must be priced when a cap is set)")
	fl.IntVar(&f.maxTokens, "max-tokens", 0, "Cap on prompt plus completion tokens of the whole run (0 means no cap)")
	fl.BoolVar(&f.allowLLM, "allow-llm", false, "Agree to send the files' content to the model's provider (required to call a model)")
	fl.BoolVar(&f.estimate, "estimate", false, "Alias of --dry-run")
	specDryRun.Bool(fl, &f.dryRun, "List the files, the egress host and model and the token and cost estimate; send nothing")
	fl.BoolVar(&f.redact, "redact", false, "Mask secret-looking text and send the file, instead of skipping it")
	fl.StringVar(&f.rubric, "rubric", "", "Rubric: builtin:instruction-quality (default) or a Markdown file")
	fl.Float64Var(&f.minScore, "min-score", judgeDefaultMinScore, "Lowest passing score, from 0 to 1; a file below it is a finding (exit 2)")
	fl.BoolVar(&f.noCache, "no-cache", false, "Do not read or write the model response cache")
	addJSONFormat(fl, &f.asJSON, "")
	addPolicyFlags(cmd)
	return cmd
}

func (f *judgeFlags) validate() error {
	switch {
	case !llm.Finite(f.minScore) || f.minScore < 0 || f.minScore > 1:
		return oops.Errorf("--min-score must be a number from 0 to 1, got %v", f.minScore)
	case !llm.Finite(f.maxCost) || f.maxCost < 0:
		return oops.Errorf("--max-cost must be a finite number >= 0, got %v", f.maxCost)
	case f.maxTokens < 0:
		return oops.Errorf("--max-tokens must be >= 0, got %d", f.maxTokens)
	}
	return nil
}

// config builds the model configuration from the environment and the flags alone;
// no repository or user config file is read. A flag beats its AI_RULEZ_LLM_*
// variable. Network use is on only because --allow-llm said so, and the plain-http
// opt-in of the environment does not apply: the endpoint is checked separately.
func (d judgeDeps) config(cmd *cobra.Command, f *judgeFlags) (llm.Config, error) {
	lc, err := (llm.Config{MaxCostUSD: judgeDefaultMaxCost}).WithEnv(d.getenv)
	if err != nil {
		return llm.Config{}, err //nolint:wrapcheck // typed llm error
	}
	fl := cmd.Flags()
	if fl.Changed("model") {
		lc.Model = f.model
	}
	if fl.Changed("provider") {
		lc.Provider = f.provider
	}
	if fl.Changed("max-cost") {
		lc.MaxCostUSD = f.maxCost
	}
	if fl.Changed("max-tokens") {
		lc.MaxTokens = f.maxTokens
	}
	lc.AllowNetwork = f.allowLLM
	lc.AllowPlainHTTP, lc.PlainHTTPHosts = false, nil
	if i := strings.Index(lc.Model, "/"); i > 0 && lc.Provider != "" && lc.Model[:i] != lc.Provider {
		return llm.Config{}, oops.Errorf("--model %q names another provider than --provider %q", lc.Model, lc.Provider)
	}
	return lc, lc.Err() //nolint:wrapcheck // typed llm error
}

func (d judgeDeps) run(cmd *cobra.Command, args []string, f *judgeFlags) error {
	ctx := watchParentContext(cmd)
	p, err := d.plan(ctx, cmd, args, f)
	if err != nil {
		return err
	}
	report := p.report
	if f.estimate || f.dryRun {
		report.Mode = judgeModeEstimate
		report.Files = planJudgeFiles(p.files)
		report.tally()
		return report.write(cmd.OutOrStdout(), f.asJSON)
	}
	report.Mode = judgeModeJudge
	var client judgeClient
	if p.est.Files > 0 {
		if err := d.preflight(p.root, p.lc, f, p.est, report.EgressHost); err != nil {
			return err
		}
		var cerr error
		if client, cerr = d.newClient(p.lc, llm.Options{ConfigDir: p.root, NoCache: f.noCache, Getenv: d.getenv}); cerr != nil {
			return oops.Wrapf(cerr, "start the model client")
		}
		defer client.Close() //nolint:errcheck // releasing a client
		cmd.PrintErrf("judging %d files with %s via %s (file content is sent there; spend capped at %s)\n",
			p.est.Files, p.lc.FullModel(), judgeHostLabel(report.EgressHost), judgeCapLabel(p.lc.MaxCostUSD))
	}
	var used judgeUsage
	report.Files, used = gradeJudgeFiles(ctx, client, p.lc, p.rubric, p.files, f.minScore)
	report.Summary.Calls, report.Summary.Tokens, report.Summary.CostUSD = used.calls, used.tokens, used.costUSD
	if client != nil {
		// With a cap the budget middleware counts every settled call; without one there is none.
		if spent := client.Spent(); spent.Calls > 0 {
			report.Summary.Calls, report.Summary.Tokens, report.Summary.CostUSD = spent.Calls, spent.Tokens, spent.CostUSD
		}
	}
	report.tally()
	if err := report.write(cmd.OutOrStdout(), f.asJSON); err != nil {
		return err
	}
	switch {
	case report.Summary.Errors > 0:
		return &ExitError{Code: exitFailure, Silent: true}
	case report.Summary.Failed > 0 || report.skippedForSecrets() > 0:
		return exitStatus(exitFindings)
	case report.Summary.Judged == 0 && report.Summary.Skipped > 0:
		cmd.PrintErrln("nothing judged: every selected file was skipped (see the reasons above)")
		return &ExitError{Code: exitFailure, Silent: true}
	}
	return nil
}

// judgePlan is everything a run decides before it sends anything.
type judgePlan struct {
	rubric judge.Rubric
	root   string
	files  []judge.Prepared
	lc     llm.Config
	est    judge.EstimateResult
	report *judgeReport
}

// plan validates the flags and resolves the rubric, the files, the model
// configuration and the estimate.
func (d judgeDeps) plan(ctx context.Context, cmd *cobra.Command, args []string, f *judgeFlags) (*judgePlan, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}
	lc, err := d.config(cmd, f)
	if err != nil {
		return nil, err
	}
	secrets := d.secretValues(lc)
	rubric, err := loadJudgeRubric(f.rubric, secrets)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, oops.Wrapf(err, "read the working directory")
	}
	root, sources, err := resolveJudgeSources(ctx, cwd, args, lint.WalkLimits{MaxEntries: judgeWalkMaxEntries, MaxDepth: judgeWalkMaxDepth})
	if err != nil {
		return nil, err
	}
	files := prepareJudgeFiles(sources, f.redact, secrets)
	host, err := judgeEgressHost(lc)
	if err != nil {
		return nil, err
	}
	est := judge.Estimate(lc, rubric, files)
	return &judgePlan{rubric: rubric, root: root, files: files, lc: lc, est: est, report: &judgeReport{
		Rubric: rubric.ID, Model: lc.FullModel(), EgressHost: host, MinScore: f.minScore,
		Estimate: judgeEstimate{Files: est.Files, PromptTokens: est.PromptTokens, MaxCompletionTokens: est.MaxCompletionTokens, CostUSD: est.CostUSD, CostKnown: est.CostKnown},
	}}, nil
}

// secretValues are the values of the API key variables: the one AI_RULEZ_LLM_API_KEY_ENV
// names and the provider's own. No file or rubric may carry one, and none is ever printed.
func (d judgeDeps) secretValues(lc llm.Config) []string {
	_, keyEnv := providerEndpoint(routedProvider(lc))
	var out []string
	for _, name := range []string{lc.APIKeyEnv, keyEnv} {
		if name == "" {
			continue
		}
		if v := strings.TrimSpace(d.getenv(name)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// preflight refuses, before anything is sent, a run the user has not agreed to,
// the organization policy forbids, or the cap cannot cover.
func (d judgeDeps) preflight(root string, lc llm.Config, f *judgeFlags, est judge.EstimateResult, egressHost string) error {
	switch {
	case !f.allowLLM:
		return oops.Hint("Pass --allow-llm to agree, or --estimate to see what would be sent").
			Errorf("judge sends file content to the model's provider; --allow-llm is required")
	case d.policyLocked(root):
		return oops.Hint("The organization policy forbids model use; --estimate still works").
			Errorf("the organization policy forbids calling a model")
	case lc.FullModel() == "":
		return oops.Hint("Pass --model provider/model or set AI_RULEZ_LLM_MODEL").Errorf("no model: --model is required")
	case egressHost == "":
		return oops.Hint("Set AI_RULEZ_LLM_BASE_URL to the endpoint, or use a provider ai-rulez knows the host of (openai, anthropic, gemini, mistral, groq, deepseek, xai, openrouter, cohere); --estimate still works").
			Errorf("cannot tell which host would receive the file content for model %q, and no base URL (AI_RULEZ_LLM_BASE_URL) is set", lc.FullModel())
	}
	if lc.MaxCostUSD > 0 {
		if !est.CostKnown {
			return oops.Hint("Pick a priced model, or pass --max-cost 0 to drop the cap").
				Errorf("no price is known for model %q, so the --max-cost cap cannot be enforced", lc.FullModel())
		}
		if est.CostUSD > lc.MaxCostUSD {
			return oops.Hint("Raise --max-cost, judge fewer files, or pick a cheaper model").
				Errorf("the worst-case cost $%.4f of judging %d files exceeds --max-cost $%.4f", est.CostUSD, est.Files, lc.MaxCostUSD)
		}
	}
	return nil
}

// planJudgeFiles lists the files an estimate would send or skip.
func planJudgeFiles(files []judge.Prepared) []judgeFileReport {
	out := make([]judgeFileReport, len(files))
	for i, p := range files {
		out[i] = judgeFileReport{Path: p.Path, Status: judgeStatusWouldSend, Bytes: p.Bytes, Truncated: p.Truncated, Redacted: p.Redacted}
		if p.Skip != "" {
			out[i].Status, out[i].Reason, out[i].skipKind = judgeStatusSkipped, p.SkipReason, p.Skip
		}
	}
	return out
}

// gradeJudgeFiles judges the files in order. A failure that every later call
// would share (key, budget, consent) stops the run and marks the rest; any other
// failure is that file's alone.
func gradeJudgeFiles(ctx context.Context, c llm.Client, lc llm.Config, rubric judge.Rubric, files []judge.Prepared, minScore float64) ([]judgeFileReport, judgeUsage) {
	out := planJudgeFiles(files)
	var used judgeUsage
	pricing := llm.NewPricing(lc)
	stopped := false
	for i, p := range files {
		if p.Skip != "" {
			continue
		}
		if stopped {
			out[i].Status, out[i].Reason = judgeStatusError, "not judged: the run stopped after an earlier failure"
			continue
		}
		res, err := judge.Judge(ctx, c, rubric, p)
		if err != nil {
			out[i].Status, out[i].Reason = judgeStatusError, llm.RedactSecrets(err.Error())
			stopped = stopsRun(err)
			continue
		}
		used.add(pricing, lc.FullModel(), res.Usage)
		score := res.Verdict.Score
		passed := score >= minScore && !math.IsNaN(score)
		out[i].Status, out[i].Score, out[i].Passed, out[i].Rationale = judgeStatusJudged, &score, &passed, res.Verdict.Rationale
	}
	return out, used
}

// judgeUsage is what the answered calls of a run used, counted here because the
// budget middleware that would count it only exists when a cap is set.
type judgeUsage struct {
	calls, tokens int
	costUSD       float64
}

func (u *judgeUsage) add(pricing llm.Pricing, model string, usage llm.Usage) {
	u.calls++
	u.tokens += usage.PromptTokens + usage.CompletionTokens
	cost, _ := pricing.Cost(model, usage)
	u.costUSD += cost
}

// stopsRun reports whether err would repeat for every later file.
func stopsRun(err error) bool {
	var le *llm.Error
	if errors.Is(err, context.Canceled) {
		return true
	}
	if !errors.As(err, &le) {
		return false
	}
	switch le.Kind {
	case llm.KindAuth, llm.KindBudget, llm.KindNetworkDisabled, llm.KindConfig:
		return true
	}
	return false
}

// judgeEgressHost is where file content would go, after checking that the
// endpoint is safe for a key: https, or http to a loopback host.
func judgeEgressHost(lc llm.Config) (string, error) {
	if lc.BaseURL == "" {
		host, _ := providerEndpoint(routedProvider(lc))
		return host, nil
	}
	u, err := url.Parse(lc.BaseURL)
	if err != nil || u.Host == "" {
		return "", oops.Errorf("the base URL (AI_RULEZ_LLM_BASE_URL) must be an https URL with a host")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost") || (net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return "", oops.Hint("Use an https endpoint; plain http is accepted only for a loopback host such as localhost").
			Errorf("the base URL (AI_RULEZ_LLM_BASE_URL) must use https, got %s", u.Scheme)
	}
	return u.Host, nil
}

// routedProvider is the provider the model routes to: --provider, else the prefix of --model.
func routedProvider(lc llm.Config) string {
	if i := strings.Index(lc.Model, "/"); lc.Provider == "" && i > 0 {
		return lc.Model[:i]
	}
	return lc.Provider
}

// providerEndpoint names the host a well-known provider uses when no base URL is
// set and the variable liter-llm reads its key from; both are "" for any other
// provider. The values are those of liter-llm's provider registry
// (schemas/providers.json): a provider is added here only when the registry lists it.
func providerEndpoint(provider string) (host, keyEnv string) {
	switch provider {
	case "openai":
		return "api.openai.com", "OPENAI_API_KEY"
	case "anthropic":
		return "api.anthropic.com", "ANTHROPIC_API_KEY"
	case "gemini":
		return "generativelanguage.googleapis.com", "GEMINI_API_KEY"
	case "mistral":
		return "api.mistral.ai", "MISTRAL_API_KEY"
	case "groq":
		return "api.groq.com", "GROQ_API_KEY"
	case "deepseek":
		return "api.deepseek.com", "DEEPSEEK_API_KEY"
	case "xai":
		return "api.x.ai", "XAI_API_KEY"
	case "openrouter":
		return "openrouter.ai", "OPENROUTER_API_KEY"
	case "cohere":
		return "api.cohere.com", "COHERE_API_KEY"
	}
	return "", ""
}

func judgeHostLabel(host string) string {
	if host == "" {
		return "the provider's default endpoint"
	}
	return host
}

func judgeCapLabel(maxCost float64) string {
	if maxCost <= 0 {
		return "no cap"
	}
	return fmt.Sprintf("$%.4f", maxCost)
}
