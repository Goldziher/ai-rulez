package commands

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

const (
	// judgeDefaultMaxCost is the default --max-cost, in USD.
	judgeDefaultMaxCost = 2.0
	// judgeTimeoutSeconds bounds one model call including retries. A review writes a long structured
	// reply, and a thinking model spends part of its budget first, so the 60-second default of other
	// features is too short.
	judgeTimeoutSeconds = 180
	// judgeCacheMaxAge is how long a cached model answer is served.
	judgeCacheMaxAge = 30 * 24 * time.Hour
)

// The output formats of judge.
const formatSARIF = "sarif"

// judgeClient is the model client a judge run uses: the middleware stack plus its accounting.
type judgeClient interface {
	llm.Client
	Spent() llm.Spent
}

// judgeDeps are the outside inputs of the command, replaced in tests: the model client, the
// environment, the clock and the organization policy.
type judgeDeps struct {
	newClient    func(llm.Config, llm.Options) (judgeClient, error)
	getenv       func(string) string
	policyLocked func(projectDir string) bool
	clock        ambient.Clock
	// maxUnits overrides the per-run unit limit; zero keeps judgeMaxUnits (a test lowers it).
	maxUnits int
}

// unitLimit is how many units one real run may judge.
func (d judgeDeps) unitLimit() int {
	if d.maxUnits > 0 {
		return d.maxUnits
	}
	return judgeMaxUnits
}

func (d judgeDeps) now() time.Time { return d.clock.Now() }

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
	model, provider, adjudicatorModel                                        string
	modelFor, skills, skillFor, categories, failOn                           []string
	minConfidence, since, format                                             string
	maxCost, priceInput, priceOutput                                         float64
	maxTokens, maxCalls, concurrency, rpm                                    int
	allowLLM, estimate, dryRun, redact, noCache, cacheReadOnly, allowPartial bool
	explainDrops, allowPromptOverride, attachRules, attachClaudeMD, impact   bool
	changed                                                                  string
	// forceReference makes every named rubric skill reference data (T2): set for a tool caller,
	// whose paths and names are not configured text.
	forceReference bool
}

func newJudgeCmd(deps judgeDeps) *cobra.Command {
	var f *judgeFlags
	cmd := &cobra.Command{
		Use:   "judge [paths...]",
		Short: "Review agent configuration with models: security, quality and governance, with verified evidence",
		Long: `Review agent configuration (skills, agents, commands, rules, CLAUDE.md and AGENTS.md files, MCP and
settings files) with language models. Three reviewers (security, quality, governance) each read a unit with the
static facts of validate and scan, and every finding has to quote the file: a verifier checks each quote against
the text that was sent, and findings that fail are corrected once, then deleted. An independent adjudicator keeps,
downgrades, merges or drops what survives. Scores and verdicts are computed in code from the findings.

With no paths it judges everything the agent harnesses read below the working directory, nested .claude
directories at any depth included. Name files or directories to judge those instead. --changed[=REV] (or --since REV) judges
only the units that changed since REV (HEAD for a bare --changed; the working tree is included), and with --impact (the default)
also the units a changed rule or CLAUDE.md applies to.

Governance is repo-aware: every unit is reviewed with the path-scoped rules (D/.claude/rules/**/*.md of every
ancestor directory D, globs relative to D) and the CLAUDE.md and AGENTS.md files that apply to it. Rubric skills
(--skill NAME or path:DIR) add checkable criteria.

File content is sent to the models, so it needs --allow-llm. The egress host and models are printed before
anything is sent. A unit with secret-looking text is not sent and counts as a finding; --redact masks the text
and sends it. Symlinks are never followed.

--estimate (or --dry-run) lists the units, what is attached, the egress host and models, and the token and cost
estimate, and sends nothing. Spend is capped by --max-cost (default $2.00), --max-tokens and --max-calls: calls
that do not fit are not made and their units are reported as partial. An estimate covers every unit, even past
the 200 a real run takes, and exits 1 with a warning when it exceeds --max-cost.

Exit codes: 0 every unit passed; 1 the run could not complete (refused, capped, model failure, nothing to
judge, or a unit only partly reviewed without --allow-partial); 2 a unit met --fail-on or was withheld for
secrets. Under an organization policy that forbids model use, only --estimate runs. See docs/judge.md.`,
		Example: `  ai-rulez judge --estimate
  ai-rulez judge --allow-llm
  ai-rulez judge .claude/skills/deploy --allow-llm --fail-on security:medium,governance:high
  ai-rulez judge --changed=origin/main --allow-llm --format sarif > judge.sarif
  ai-rulez judge --allow-llm --skill path:rubrics/acme --skill-for quality=style-rubric`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fail(deps.run(cmd, args, f))
		},
	}
	f = registerJudgeFlags(cmd)
	addPolicyFlags(cmd)
	return cmd
}

// registerJudgeFlags adds the flags of judge to cmd and returns the values they fill. The MCP tool
// builds a command only for this: its defaults and its notion of "set explicitly" are the flags'.
func registerJudgeFlags(cmd *cobra.Command) *judgeFlags {
	f := &judgeFlags{}
	fl := cmd.Flags()
	fl.StringVar(&f.model, "model", "", "Model of the reviewers as provider/model (or AI_RULEZ_LLM_MODEL); the adjudicator uses it too unless --adjudicator-model names another. Default bedrock/global.anthropic.claude-sonnet-5-5 for the reviewers and bedrock/global.anthropic.claude-opus-5-5 for the adjudicator")
	fl.StringVar(&f.adjudicatorModel, "adjudicator-model", "", "Model of the adjudicator, which should be the strongest you use")
	fl.StringArrayVar(&f.modelFor, "model-for", nil, "Model of one role as ROLE=MODEL, where ROLE is security, quality, governance or adjudicator (repeatable)")
	fl.StringVar(&f.provider, "provider", "", "Provider prefix for a --model without one (or AI_RULEZ_LLM_PROVIDER)")
	fl.StringSliceVar(&f.categories, "categories", nil, "Categories to run: security, quality, governance (default all)")
	fl.StringSliceVar(&f.failOn, "fail-on", nil, "When a unit fails: <category>:<severity>, <category>:none, score:<n> or <category>:score:<n>, comma separated. Default security:high,governance:high,quality:none (quality findings are advice)")
	fl.StringVar(&f.minConfidence, "min-confidence", "", "Drop findings below this confidence: low (default), medium, high")
	fl.BoolVar(&f.attachRules, "attach-rules", true, "Attach the path-scoped rules that govern each unit to its governance review")
	fl.BoolVar(&f.attachClaudeMD, "attach-claude-md", true, "Attach the CLAUDE.md and AGENTS.md files (with their @imports) that apply to each unit")
	fl.StringArrayVar(&f.skills, "skill", nil, "Rubric skill applied to the governance review: a skill name from the project, or path:DIR (repeatable)")
	fl.StringArrayVar(&f.skillFor, "skill-for", nil, "Rubric skill for one category as CATEGORY=NAME or CATEGORY=path:DIR (repeatable)")
	fl.StringVar(&f.changed, "changed", "", "Judge only the units that changed since a base: bare --changed is --since HEAD (uncommitted and untracked work), --changed=REV is --since REV")
	fl.Lookup("changed").NoOptDefVal = changedDefaultBase
	fl.StringVar(&f.since, "since", "", "Judge only the units that changed since this git revision: committed, staged, unstaged and untracked work (and, with --impact, the units a changed rule applies to)")
	fl.BoolVar(&f.impact, "impact", true, "With --changed, also judge the units a changed rule or instruction file applies to")
	fl.Float64Var(&f.maxCost, "max-cost", judgeDefaultMaxCost, "Spend cap in USD (0 removes it; every model must be priced when a cap is set)")
	fl.Float64Var(&f.priceInput, "price-input", 0, "Price of the model in USD per million input tokens, for a model the price table does not know (use with --price-output)")
	fl.Float64Var(&f.priceOutput, "price-output", 0, "Price of the model in USD per million output tokens (use with --price-input)")
	fl.IntVar(&f.maxTokens, "max-tokens", 0, "Cap on prompt plus completion tokens of the whole run (0 means no cap)")
	fl.IntVar(&f.maxCalls, "max-calls", 0, "Cap on model calls of the whole run (0 means no cap)")
	fl.IntVar(&f.concurrency, "concurrency", 0, "Model calls in flight at once (default 4)")
	fl.IntVar(&f.rpm, "rpm", 0, "Model calls started per minute (0 means no limit)")
	fl.BoolVar(&f.allowLLM, "allow-llm", false, "Agree to send the units' content to the model's provider (required to call a model)")
	fl.BoolVar(&f.estimate, "estimate", false, "Alias of --dry-run")
	specDryRun.Bool(fl, &f.dryRun, "List the units, what is attached, the egress host and models and the token and cost estimate; send nothing")
	fl.BoolVar(&f.redact, "redact", false, "Mask secret-looking text and send the unit, instead of withholding it")
	fl.BoolVar(&f.noCache, "no-cache", false, "Do not read or write the model response cache")
	fl.BoolVar(&f.cacheReadOnly, "cache-readonly", false, "Serve cached answers but store none")
	fl.BoolVar(&f.allowPartial, "allow-partial", false, "Exit 0 or 2 by the findings even when some unit was only partly reviewed")
	fl.BoolVar(&f.explainDrops, "explain-drops", false, "List in the report every candidate that was dropped, and why")
	fl.BoolVar(&f.allowPromptOverride, "allow-prompt-override", false, "Let a repository prompt file replace a reviewer's role prompt (an extension needs no flag)")
	addFormatFlag(fl, &f.format, "text", "text", formatText, formatJSON, formatSARIF)
	return f
}

// sinceRev is the revision --since or --changed names ("" when neither is set).
func (f *judgeFlags) sinceRev() string {
	switch {
	case strings.TrimSpace(f.since) != "":
		return strings.TrimSpace(f.since)
	case strings.TrimSpace(f.changed) != "":
		return strings.TrimSpace(f.changed)
	}
	return ""
}

func (f *judgeFlags) validate() error {
	switch {
	case !llm.Finite(f.maxCost) || f.maxCost < 0:
		return oops.Errorf("--max-cost must be a finite number >= 0, got %v", f.maxCost)
	case f.maxTokens < 0 || f.maxCalls < 0 || f.concurrency < 0 || f.rpm < 0:
		return oops.Errorf("--max-tokens, --max-calls, --concurrency and --rpm must be >= 0")
	case !llm.Finite(f.priceInput) || !llm.Finite(f.priceOutput) || f.priceInput < 0 || f.priceOutput < 0:
		return oops.Errorf("--price-input and --price-output must be finite numbers >= 0")
	case (f.priceInput > 0) != (f.priceOutput > 0):
		return oops.Errorf("--price-input and --price-output must be given together")
	}
	return checkFormat(f.format, []string{formatText, formatJSON, formatSARIF})
}

// llmConfig builds the model configuration from the user's [llm] table, the environment and the
// flags. The repository's [llm] table is never read: a repository must not choose where content
// goes. Network use is on only because --allow-llm (or the user's allow_llm) said so, and the plain-http
// opt-in does not apply: the endpoint is checked separately.
func (d judgeDeps) llmConfig(changed flagChanged, f *judgeFlags, userJudge *config.JudgeConfig) (llm.Config, error) {
	var none *config.Config
	res, err := none.ResolveLLM(d.getenv)
	if err != nil {
		return llm.Config{}, err //nolint:wrapcheck // typed config error
	}
	lc := res.Config
	if lc.MaxCostUSD == 0 {
		lc.MaxCostUSD = judgeDefaultMaxCost
	}
	if lc.TimeoutSeconds == 0 {
		lc.TimeoutSeconds = judgeTimeoutSeconds
	}
	applyUserJudgeLimits(&lc, userJudge)
	lc = applyLLMFlags(lc, changed, f)
	lc.AllowNetwork = f.allowLLM || (userJudge != nil && userJudge.AllowLLM)
	lc.AllowPlainHTTP, lc.PlainHTTPHosts = false, nil
	return lc, nil
}

func applyUserJudgeLimits(lc *llm.Config, userJudge *config.JudgeConfig) {
	if userJudge == nil {
		return
	}
	if userJudge.MaxCostUSD > 0 {
		lc.MaxCostUSD = userJudge.MaxCostUSD
	}
	if userJudge.MaxTokens > 0 {
		lc.MaxTokens = userJudge.MaxTokens
	}
	if userJudge.MaxCalls > 0 {
		lc.MaxCalls = userJudge.MaxCalls
	}
}

// applyLLMFlags lets the flags the user set explicitly override the configured values.
func applyLLMFlags(lc llm.Config, changed flagChanged, f *judgeFlags) llm.Config {
	if changed("model") {
		lc.Model = f.model
	}
	if changed("provider") {
		lc.Provider = f.provider
	}
	if changed("max-cost") {
		lc.MaxCostUSD = f.maxCost
	}
	if changed("max-tokens") {
		lc.MaxTokens = f.maxTokens
	}
	if changed("max-calls") {
		lc.MaxCalls = f.maxCalls
	}
	if changed("price-input") || changed("price-output") {
		lc = lc.WithPriceOverride(f.priceInput, f.priceOutput)
	}
	return lc
}

// judgeInput is one run of the engine: the command line and the MCP tool both build it.
type judgeInput struct {
	cwd   string
	args  []string
	flags *judgeFlags
	// changed reports whether a flag was set explicitly (so a default can yield to the configuration).
	changed flagChanged
	// inline are rubric skills given as text by a tool caller, with the categories they apply to.
	inline []judgeInlineSkill
	// stderr receives the line that announces the egress; nil prints nothing.
	stderr io.Writer
	// fromTool marks a run requested by an MCP client, which has no consent prompt: it needs the
	// user's standing consent (allow_llm in the user config) as well as the request's own.
	fromTool bool
}

// judgeInlineSkill is a rubric skill given as Markdown text.
type judgeInlineSkill struct {
	name, markdown string
	cats           []judge.Category
}

// judgeOutcome is the result of a run: the report and whether the plan sent anything.
type judgeOutcome struct {
	report *judge.Report
	// overCap is set when an estimate exceeds the spend cap: the run would stop short of the caps.
	overCap string
}

func (d judgeDeps) run(cmd *cobra.Command, args []string, f *judgeFlags) error {
	ctx := watchParentContext(cmd)
	cwd, err := os.Getwd()
	if err != nil {
		return oops.Wrapf(err, "read the working directory")
	}
	out, err := d.execute(ctx, judgeInput{cwd: cwd, args: args, flags: f, changed: cmd.Flags().Changed, stderr: cmd.ErrOrStderr()})
	if err != nil {
		return err
	}
	if err := d.write(cmd, f, out.report); err != nil {
		return err
	}
	if out.report.Mode == judge.ModeEstimate {
		if out.overCap != "" {
			cmd.PrintErrln("warning: " + out.overCap)
			return &ExitError{Code: exitFailure, Silent: true}
		}
		return nil
	}
	return d.exitFor(cmd, out.report, f)
}

// judgePrep is everything an execute needs before it selects units: the workspace, the project
// config, the model configuration and the settings.
type judgePrep struct {
	w       *judgeWorkspace
	cfg     *config.Config
	lc      llm.Config
	set     *judgeSettings
	secrets []string
}

// prepare opens the workspace and resolves the configuration, the models and the secrets to mask.
func (d judgeDeps) prepare(ctx context.Context, in judgeInput) (*judgePrep, error) {
	f := in.flags
	if err := f.validate(); err != nil {
		return nil, err
	}
	w, err := openJudgeWorkspace(ctx, in.cwd, len(in.args) > 0, lint.WalkLimits{MaxEntries: judgeWalkMaxEntries, MaxDepth: judgeWalkMaxDepth})
	if err != nil {
		return nil, err
	}
	cfg, err := loadJudgeProjectConfigAt(ctx, w.root)
	if err != nil {
		return nil, err
	}
	res, err := cfg.ResolveJudge(d.getenv)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if problems := res.Repo.Validate(d.now()); len(problems) > 0 {
		return nil, oops.Errorf("invalid [judge] table: %s", strings.Join(problems, "; "))
	}
	lc, err := d.llmConfig(in.changed, f, res.User)
	if err != nil {
		return nil, err
	}
	set, err := d.buildSettings(f, in.changed, w, res, lc)
	if err != nil {
		return nil, err
	}
	repoNotices, err := repoConfigNotices(cfg)
	if err != nil {
		return nil, err
	}
	set.notices = append(set.notices, repoNotices...)
	if err := d.addInlineSkills(in.inline, set); err != nil {
		return nil, err
	}
	if lc, err = d.routeModels(lc, &set.opts.Models); err != nil {
		return nil, err
	}
	secrets := d.secretValues(lc)
	set.opts.Attach.Secrets = secrets
	return &judgePrep{w: w, cfg: cfg, lc: lc, set: set, secrets: secrets}, nil
}

// execute plans and, unless the run is an estimate, performs a judge run.
func (d judgeDeps) execute(ctx context.Context, in judgeInput) (*judgeOutcome, error) {
	f := in.flags
	p, err := d.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	w, lc, set := p.w, p.lc, p.set
	refs, changed, err := d.selectUnits(ctx, w, in.args, f, set, p.secrets)
	if err != nil {
		return nil, err
	}
	units := w.buildUnits(refs, judge.UnitOptions{Redact: f.redact, Secrets: p.secrets})
	facts, factNote := w.staticFacts(ctx, units, p.cfg)
	if factNote != "" {
		set.notices = append(set.notices, factNote)
	}
	host, err := judgeEgressHost(ctx, lc, d.getenv)
	if err != nil {
		return nil, err
	}
	eng := judge.NewEngine(nil, llm.NewPricing(lc), set.opts)
	plan := eng.Plan(judge.Input{Units: units, Facts: facts, Repo: w.repo()})
	estimating := f.estimate || f.dryRun
	over := ""
	if estimating && estimateExceedsCap(plan, lc.MaxCostUSD) {
		over = overCapNotice(plan, lc.MaxCostUSD)
		set.notices = append(set.notices, over)
	}
	meta := judge.RunMeta{EgressHosts: nonEmptyList(host), Skills: set.skills, Notices: set.notices, Changed: changed}
	if estimating {
		return &judgeOutcome{report: eng.EstimateReport(plan, meta), overCap: over}, nil
	}
	if sendable(units) {
		client, cerr := d.connect(in, p, plan, host)
		if cerr != nil {
			return nil, cerr
		}
		defer client.Close() //nolint:errcheck // releasing a client
		eng.Client = client
	}
	return &judgeOutcome{report: eng.Run(ctx, plan, meta)}, nil
}

// connect checks the run may send content, starts the model client and announces the egress.
func (d judgeDeps) connect(in judgeInput, p *judgePrep, plan *judge.Plan, host string) (judgeClient, error) {
	if err := d.preflight(p.w.root, p.lc, in.flags, p.set, plan, host, in.fromTool); err != nil {
		return nil, err
	}
	client, err := d.startClient(p.w.root, p.lc, in.flags)
	if err != nil {
		return nil, err
	}
	if in.stderr != nil {
		//nolint:errcheck // an announcement that cannot be written must not stop a run the user already agreed to
		fmt.Fprintf(in.stderr, "judging %d units with %s (adjudicator %s) via %s: content is sent there; spend capped at %s\n",
			plan.Estimate.Units, p.set.opts.Models.Security, p.set.opts.Models.Adjudicator, judgeHostLabel(host), judgeCapLabel(p.lc.MaxCostUSD))
	}
	return client, nil
}

// addInlineSkills parses rubric skills a tool caller gave as text. They are reference data (T2): text
// nobody pinned never counts as configured instructions.
func (d judgeDeps) addInlineSkills(skills []judgeInlineSkill, set *judgeSettings) error {
	for _, sk := range skills {
		r, err := judge.ParseRubric(sk.name, "inline", sk.markdown, judge.TierReference)
		if err != nil {
			return oops.Wrapf(err, "inline rubric skill %q", sk.name)
		}
		if set.opts.Rubrics == nil {
			set.opts.Rubrics = map[judge.Category][]judge.Rubric{}
		}
		ls := judge.LoadedSkill{Name: r.Name, Path: "inline", SHA256: r.SHA256, Tier: r.Tier}
		for _, c := range sk.cats {
			if !hasRubric(set.opts.Rubrics[c], r) {
				set.opts.Rubrics[c] = append(set.opts.Rubrics[c], r)
			}
			ls.Categories = append(ls.Categories, string(c))
		}
		set.skills = append(set.skills, ls)
	}
	return nil
}

func nonEmptyList(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func sendable(units []judge.Unit) bool {
	for i := range units {
		if units[i].Status == "" {
			return true
		}
	}
	return false
}

// selectUnits picks the units: what is named or discovered, narrowed to the changed ones with
// --changed.
func (d judgeDeps) selectUnits(ctx context.Context, w *judgeWorkspace, args []string, f *judgeFlags, set *judgeSettings, secrets []string) ([]judge.Ref, *judge.ChangedInfo, error) {
	refs, err := w.resolveRefs(args)
	if err != nil {
		return nil, nil, err
	}
	var info *judge.ChangedInfo
	base := f.sinceRev()
	if base != "" {
		sel, ci, cerr := w.changedRefs(ctx, refs, base, f.impact, judge.UnitOptions{Redact: f.redact, Secrets: secrets}, set.opts.Attach)
		if cerr != nil {
			return nil, nil, cerr
		}
		refs, info = sel, &ci
		if len(refs) == 0 {
			set.notices = append(set.notices, fmt.Sprintf("no unit changed since %s", base))
		}
	}
	if len(refs) == 0 && base == "" {
		return nil, nil, oops.Hint("Name the files or directories to judge, for example: ai-rulez judge .claude/skills/deploy").
			Errorf("nothing to judge: found no skill, agent, command, rule, CLAUDE.md, AGENTS.md, MCP or settings file below %s", w.cwd)
	}
	limit := d.unitLimit()
	if len(refs) > limit && (f.estimate || f.dryRun) {
		set.notices = append(set.notices, fmt.Sprintf("%d units match; a real run is limited to %d units and would be refused: name paths or use --changed (this estimate covers all of them)", len(refs), limit))
	} else if len(refs) > limit {
		return nil, nil, oops.Hint("Name fewer paths, or use --changed; every unit is up to four paid model calls").
			Errorf("%d units match; the limit is %d per run", len(refs), limit)
	}
	return refs, info, nil
}

// routeModels checks that every role routes to one provider (one client, one credential) and
// gives the client configuration the reviewers' model, the provider and the price knowledge.
func (d judgeDeps) routeModels(lc llm.Config, m *judge.Models) (llm.Config, error) {
	providerOf := func(model string) string {
		if i := strings.Index(model, "/"); i > 0 {
			return model[:i]
		}
		return lc.Provider
	}
	prov := providerOf(m.Security)
	if prov == "" {
		return lc, oops.Hint("Pass --model provider/model or set AI_RULEZ_LLM_MODEL").Errorf("no provider for model %q", m.Security)
	}
	full := func(model string) string {
		if strings.Contains(model, "/") {
			return model
		}
		return prov + "/" + model
	}
	m.Security, m.Quality, m.Governance, m.Adjudicator = full(m.Security), full(m.Quality), full(m.Governance), full(m.Adjudicator)
	for _, model := range []string{m.Quality, m.Governance, m.Adjudicator} {
		if providerOf(model) != prov {
			return lc, oops.Hint("Use models of one provider for all roles: the run signs in once").
				Errorf("the models %q and %q route to different providers", m.Security, model)
		}
	}
	lc.Provider = prov
	lc.Model = strings.TrimPrefix(m.Security, prov+"/")
	return lc, lc.Err() //nolint:wrapcheck // typed llm error
}

// startClient builds the model client; a Bedrock model resolves the AWS credential chain itself.
func (d judgeDeps) startClient(root string, lc llm.Config, f *judgeFlags) (judgeClient, error) {
	client, err := d.newClient(lc, llm.Options{ConfigDir: root, NoCache: f.noCache, CacheReadOnly: f.cacheReadOnly, CacheMaxAge: judgeCacheMaxAge, Getenv: d.getenv})
	if err != nil {
		return nil, oops.Wrapf(err, "start the model client")
	}
	return client, nil
}

// secretValues are the values of the API key variables: the one AI_RULEZ_LLM_API_KEY_ENV names and
// the provider's own. No unit may carry one, and none is ever printed.
func (d judgeDeps) secretValues(lc llm.Config) []string {
	_, keyEnv := providerEndpoint(routedProvider(lc))
	names := []string{lc.APIKeyEnv, keyEnv}
	if routedProvider(lc) == llm.ProviderBedrock {
		names = append(names, "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN")
	}
	var out []string
	for _, name := range names {
		if name == "" {
			continue
		}
		if v := strings.TrimSpace(d.getenv(name)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// preflight refuses, before anything is sent, a run the user has not agreed to, the organization
// policy forbids, or the cap cannot cover.
func (d judgeDeps) preflight(root string, lc llm.Config, f *judgeFlags, set *judgeSettings, plan *judge.Plan, egressHost string, fromTool bool) error {
	switch {
	case fromTool && (!f.allowLLM || !set.consent):
		return oops.Hint("Set allow_llm = true in the [judge] table of your user config file and pass allow_llm: true; use estimate: true to see what would be sent").
			Errorf("the judge tool sends file content to your model provider and spends money; a tool client has no consent prompt, so it needs your standing consent in the user config as well as the request's own")
	case !f.allowLLM && !set.consent:
		return oops.Hint("Pass --allow-llm to agree, or --estimate to see what would be sent").
			Errorf("judge sends file content to the model's provider; --allow-llm is required")
	case d.policyLocked(root):
		return oops.Hint("The organization policy forbids model use; --estimate still works").
			Errorf("the organization policy forbids calling a model")
	case lc.FullModel() == "":
		return oops.Hint("Pass --model provider/model or set AI_RULEZ_LLM_MODEL").Errorf("no model: --model is required")
	case egressHost == "":
		return oops.Hint("Set AI_RULEZ_LLM_BASE_URL to the endpoint, or use a provider ai-rulez knows the host of (openai, anthropic, gemini, mistral, groq, deepseek, xai, openrouter, cohere, or bedrock with AWS_REGION set); --estimate still works").
			Errorf("cannot tell which host would receive the file content for model %q, and no base URL (AI_RULEZ_LLM_BASE_URL) is set", lc.FullModel())
	}
	if lc.MaxCostUSD > 0 {
		if !plan.Estimate.CostKnown {
			return oops.Hint("Pick priced models, pass --price-input and --price-output, or --max-cost 0 to drop the cap").
				Errorf("no price is known for a model of this run, so the --max-cost cap cannot be enforced")
		}
		if len(plan.Calls) > 0 && plan.Skipped() == len(plan.Calls) {
			return oops.Hint("Raise --max-cost, judge fewer units, or pick cheaper models").
				Errorf("not one call fits the caps: the expected cost of judging %d units is $%.4f, over --max-cost $%.4f", plan.Estimate.Units, plan.Estimate.ExpectedCostUSD, lc.MaxCostUSD)
		}
	}
	return nil
}

// write prints the report in the chosen format.
func (d judgeDeps) write(cmd *cobra.Command, f *judgeFlags, rep *judge.Report) error {
	out := cmd.OutOrStdout()
	switch f.format {
	case formatJSON:
		return jsondoc.Write(out, rep.Sanitized())
	case formatSARIF:
		return rep.WriteSARIF(out) //nolint:wrapcheck // writer error
	}
	rep.WriteText(out)
	return nil
}

// exitFor maps the run onto the exit code: 1 when it could not complete, 2 when a unit met the gate
// or was withheld for secrets.
func (d judgeDeps) exitFor(cmd *cobra.Command, rep *judge.Report, f *judgeFlags) error {
	incomplete, failed, withheld, reviewed := false, false, false, 0
	for i := range rep.Units {
		u := &rep.Units[i]
		switch u.Status {
		case judge.StatusWithheld:
			withheld = true
		case judge.StatusSkipped:
		case judge.StatusError:
			incomplete = true
		default:
			reviewed++
			if !u.Complete {
				incomplete = true
			}
			if u.Status == judge.StatusFail {
				failed = true
			}
		}
	}
	switch {
	case incomplete && !f.allowPartial:
		cmd.PrintErrln("the run is incomplete: some unit was only partly reviewed (see the coverage gaps above); pass --allow-partial to accept that")
		return &ExitError{Code: exitFailure, Silent: true}
	case failed || withheld:
		return exitStatus(exitFindings)
	case reviewed == 0 && !withheld:
		cmd.PrintErrln("nothing judged: every selected unit was skipped (see the reasons above)")
		return &ExitError{Code: exitFailure, Silent: true}
	}
	return nil
}

// judgeEgressHost is where file content would go, after checking that the endpoint is safe for a
// key: https, or http to a loopback host.
func judgeEgressHost(ctx context.Context, lc llm.Config, getenv func(string) string) (string, error) {
	if lc.BaseURL == "" {
		if routedProvider(lc) == llm.ProviderBedrock {
			return llm.BedrockEgressHost(ctx, getenv) //nolint:wrapcheck // typed llm error
		}
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

// providerEndpoint names the host a well-known provider uses when no base URL is set and the variable
// liter-llm reads its key from; both are "" for any other provider. The values are those of liter-llm's
// provider registry (schemas/providers.json): a provider is added here only when the registry lists it.
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
	case llm.ProviderBedrock:
		// The host depends on the region (see llm.BedrockEgressHost); the bearer-token key is optional.
		return "", "AWS_BEARER_TOKEN_BEDROCK"
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

// estimateExceedsCap reports whether an estimate is over the spend cap: the expected cost is above it,
// or some planned call does not fit. A cap of 0 is no cap, and an unpriced run has no cost to compare.
func estimateExceedsCap(plan *judge.Plan, maxCost float64) bool {
	if maxCost <= 0 || !plan.Estimate.CostKnown {
		return false
	}
	return plan.Estimate.ExpectedCostUSD > maxCost || plan.Skipped() > 0
}

func overCapNotice(plan *judge.Plan, maxCost float64) string {
	return fmt.Sprintf("the estimated cost ~$%.4f exceeds --max-cost $%.4f: %d of %d planned calls fit under the cap; raise it, judge fewer units, or pick cheaper models",
		plan.Estimate.ExpectedCostUSD, maxCost, plan.Estimate.CallsFit, plan.Estimate.Calls)
}
