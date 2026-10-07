package improve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Defaults of the acceptance gate and the loop.
const (
	DefaultHoldoutTag      = "holdout"
	DefaultHoldoutFraction = 0.3
	DefaultMinGain         = 0.05
	DefaultRuns            = 3
	DefaultMaxRounds       = 3
	DefaultMaxHoldoutEvals = 3
	DefaultTimeout         = 20 * time.Minute
)

// ProtocolVersion is the optimizer protocol version.
const ProtocolVersion = 1

// LocalDir is where runs live, relative to the config directory.
var LocalDir = filepath.Join("local", "improve")

// Refusal is returned when a precondition fails: nothing ran and nothing was
// spent. The command exits 1.
type Refusal struct {
	// Code is the AR9J code when the refusal has one.
	Code   string
	Reason string
}

func (r *Refusal) Error() string {
	if r.Code != "" {
		return r.Code + ": " + r.Reason
	}
	return r.Reason
}

// refuse builds a Refusal. The reason often names optimizer- or repository-chosen text (file names,
// skill ids), so control characters are replaced before it can reach a terminal.
func refuse(code, format string, args ...any) error {
	return &Refusal{Code: code, Reason: SanitizeMultiline(fmt.Sprintf(format, args...))}
}

// Options configure Prepare. Library code reads no environment and no clock:
// the caller passes both.
type Options struct {
	// ConfigDir is the absolute config directory (.ai-rulez).
	ConfigDir string
	// RepoDir is the project root, for the git clean check and the patch paths.
	RepoDir string
	SkillID string

	// OptimizerArgv is the optimizer command, run without a shell.
	OptimizerArgv []string
	// Exec starts the optimizer; nil runs a real process.
	Exec runner.Runner
	// HostEnv is the parent environment the optimizer environment is scrubbed from.
	HostEnv []string
	EnvPass []string
	Egress  []string
	Timeout time.Duration
	Stderr  io.Writer

	// Eval scores skill directories.
	Eval    evals.Runner
	Harness string
	Model   string
	Runs    int
	Grade   evals.GradeOptions
	Price   evals.Price
	Counter tokens.Counter

	HoldoutTag      string
	HoldoutFraction float64
	MinGain         float64
	MaxRegressions  int
	MaxRounds       int
	MaxHoldoutEvals int
	// MinHoldoutCases is the fewest scored held-out cases a run needs ([improve] min_holdout_cases; 0 or
	// less than MinHoldoutCases: MinHoldoutCases).
	MinHoldoutCases int
	// MaxCostUSD is required (> 0): measured eval cost plus reported optimizer cost.
	MaxCostUSD        float64
	StopAtFirstAccept bool
	// Isolation is how the optimizer is confined: none (the default), auto or require.
	Isolation sandbox.Mode
	// Sandbox is the confinement backend; nil uses the system's.
	Sandbox *sandbox.Sandbox
	// Adapter names the bundled adapter in use (recorded in the report).
	Adapter string
	// MaxSkillGrowth bounds SKILL.md growth as a factor of the original tokens (0: 1.25).
	MaxSkillGrowth float64
	// RequireCIAboveZero makes the bootstrap interval of the gain part of the gate.
	RequireCIAboveZero bool
	AllowFrontmatter   bool
	AllowScripts       bool

	// SiblingNative, when set, also runs the sibling trigger guard on the native surface: the harness's
	// model, over repeated runs, decides which skill loads. It costs money and counts against MaxCostUSD,
	// so it is off unless asked for.
	SiblingNative *SiblingNative

	// Git answers git questions for the clean-tree check; nil skips it.
	Git evals.GitFunc
	// Date and ToolVersion are recorded in the report.
	Date        string
	ToolVersion string
}

// DefaultSiblingRuns is how often the native sibling guard repeats each trigger prompt.
const DefaultSiblingRuns = 3

// SiblingNative configures the native sibling guard.
type SiblingNative struct {
	// Runner drives the harness; it must declare the activation capability and the native surface.
	Runner evals.Runner
	// Runs repeats each trigger prompt (0: DefaultSiblingRuns).
	Runs int
}

func (n *SiblingNative) runs() int {
	if n == nil || n.Runs <= 0 {
		return DefaultSiblingRuns
	}
	return n.Runs
}

// Plan is a run that passed every precondition and has not started.
type Plan struct {
	Opts        Options
	Skill       evals.Skill
	RunID       string
	SkillRel    string
	Split       Split
	OrigDigest  string
	CasesDigest string
	OrigTokens  int
	Constraints Constraints
	Estimate    evals.Estimate
	Warnings    []string
	orig        *Tree
	heldCases   []evals.Case
	trainCases  []evals.Case
	runDir      string
	isolation   *IsolationReport
	sandbox     *sandbox.Sandbox
	// nativeBase is the original skill's native sibling measurement, taken once and reused by every round.
	nativeBase map[string]evals.ActivationSkill
}

// ParseArgv splits an optimizer command: a JSON array of strings, or words
// separated by whitespace (no shell, so no quoting).
func ParseArgv(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("the optimizer command is empty")
	}
	if strings.HasPrefix(s, "[") {
		var argv []string
		if err := json.Unmarshal([]byte(s), &argv); err != nil {
			return nil, fmt.Errorf("the optimizer command looks like a JSON array but is not one: %w", err)
		}
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			return nil, errors.New("the optimizer command is empty")
		}
		return argv, nil
	}
	return strings.Fields(s), nil
}

// ResolveArgv makes the relative paths of an optimizer command absolute from dir
// (the directory improve was started in): the optimizer runs in the run's
// throwaway workspace, where `./optimize.sh` or `python optimize.py` would name
// nothing. The program is resolved when it has a path separator (a bare name
// stays a PATH lookup); a later argument is resolved when it is not a flag and
// names an existing regular file in dir.
func ResolveArgv(argv []string, dir string) []string {
	out := append([]string(nil), argv...)
	for i, arg := range out {
		if arg == "" || filepath.IsAbs(arg) {
			continue
		}
		local := filepath.Join(dir, filepath.FromSlash(arg))
		if i == 0 {
			if strings.ContainsAny(arg, `/\`) {
				out[0] = local
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if info, err := os.Stat(local); err == nil && info.Mode().IsRegular() {
			out[i] = local
		}
	}
	return out
}

// Prepare checks every precondition and computes the plan. It writes nothing
// and calls no optimizer or eval runner, so it backs --dry-run too.
func Prepare(ctx context.Context, opts *Options) (*Plan, error) {
	o := *opts
	applyDefaults(&o)
	if err := checkSettings(&o); err != nil {
		return nil, err
	}
	if err := defaultCounter(&o); err != nil {
		return nil, err
	}
	skills, err := evals.FindSkills(o.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	idx := slices.IndexFunc(skills, func(s evals.Skill) bool { return s.ID == o.SkillID })
	if idx < 0 {
		return nil, refuse("", "%q is not an authored skill of this project: improve works on skills under .ai-rulez/skills or a domain, not on includes, installed or built-in skills", o.SkillID)
	}
	p := &Plan{Opts: o, Skill: skills[idx]}
	if err := p.resolveIsolation(ctx); err != nil {
		return nil, err
	}
	if err := p.loadCases(); err != nil {
		return nil, err
	}
	if err := p.loadSkill(); err != nil {
		return nil, err
	}
	if err := p.checkClean(skills); err != nil {
		return nil, err
	}
	if err := p.checkSecrets(); err != nil {
		return nil, err
	}
	if err := p.estimate(); err != nil {
		return nil, err
	}
	p.RunID = p.runID()
	return p, nil
}

func applyDefaults(o *Options) {
	if o.HoldoutTag == "" {
		o.HoldoutTag = DefaultHoldoutTag
	}
	if o.Runs <= 0 {
		o.Runs = DefaultRuns
	}
	if o.MaxRounds <= 0 {
		o.MaxRounds = DefaultMaxRounds
	}
	if o.MaxHoldoutEvals <= 0 {
		o.MaxHoldoutEvals = DefaultMaxHoldoutEvals
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	o.MinHoldoutCases = max(o.MinHoldoutCases, MinHoldoutCases)
	if o.Exec == nil {
		o.Exec = runner.Exec{}
	}
}

func defaultCounter(o *Options) error {
	if o.Counter != nil {
		return nil
	}
	c, err := tokens.New("")
	if err != nil {
		return fmt.Errorf("token counter: %w", err)
	}
	o.Counter = c
	return nil
}

func checkSettings(o *Options) error {
	switch {
	case len(o.OptimizerArgv) == 0:
		return refuse("", "no optimizer: pass --with <command>")
	case !(o.MaxCostUSD > 0) || math.IsInf(o.MaxCostUSD, 0):
		return refuse("", "--max-cost is required and must be a finite number > 0: improve spends money and refuses to run without a ceiling")
	case o.Eval == nil:
		return refuse("", "no eval runner configured")
	case o.HoldoutFraction < 0 || o.HoldoutFraction >= 1 || math.IsNaN(o.HoldoutFraction):
		return refuse("", "--holdout-fraction must be in [0, 1), got %v", o.HoldoutFraction)
	case o.MinGain < 0 || o.MinGain > 1 || math.IsNaN(o.MinGain):
		return refuse("", "--min-gain must be between 0 and 1, got %v", o.MinGain)
	case o.MaxRegressions < 0:
		return refuse("", "--max-regressions must be >= 0, got %d", o.MaxRegressions)
	}
	return checkEgressSettings(o)
}

// checkEgressSettings validates --env-pass and --egress.
func checkEgressSettings(o *Options) error {
	for _, name := range o.EnvPass {
		if !runner.ValidEnvName(name) {
			return refuse("", "--env-pass %q is not a valid environment variable name", name)
		}
		if runner.Sensitive(name) && len(o.Egress) == 0 {
			return refuse("", "--env-pass %s looks like a credential or proxy setting: declare where the optimizer sends data with --egress HOST so it appears in the consent summary and the report", name)
		}
	}
	for _, host := range o.Egress {
		if strings.TrimSpace(host) == "" || strings.ContainsAny(host, " \t\n/") {
			return refuse("", "--egress %q is not a host name", host)
		}
	}
	return nil
}

func (p *Plan) loadCases() error {
	authored, problems := evals.LoadCases(&p.Skill)
	switch {
	case len(problems) > 0:
		return refuse("AR996", "the eval cases of %s do not load (%d problem(s), first: %s): fix them first", p.Skill.ID, len(problems), problems[0].String())
	case len(authored) == 0:
		return refuse("AR962", "%s has no eval cases: improve needs an eval suite with a held-out split", p.Skill.ID)
	}
	p.Split = SplitCases(authored, p.Opts.HoldoutTag, p.Opts.HoldoutFraction)
	p.trainCases, p.heldCases = evals.Expand(p.Split.Train), evals.Expand(p.Split.Held)
	if len(p.heldCases) < p.Opts.MinHoldoutCases || Negatives(p.heldCases) == 0 {
		return refuse(CodeNoHoldout, "%s has %d held-out case(s) (%d negative) after the %s split; improve needs at least %d including one negative (expect_trigger: false or a near miss): tag cases %q or add cases",
			p.Skill.ID, len(p.heldCases), Negatives(p.heldCases), p.Split.Method, p.Opts.MinHoldoutCases, p.Opts.HoldoutTag)
	}
	if len(p.trainCases) == 0 {
		return refuse(CodeNoHoldout, "every case of %s is held out, so the optimizer would have nothing to learn from: keep some cases untagged", p.Skill.ID)
	}
	if dup := DuplicatePrompts(p.Split.Train, p.Split.Held); len(dup) > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("train case(s) %s repeat a held-out prompt, which weakens the held-out set", strings.Join(dup, ", ")))
	}
	return nil
}

func (p *Plan) loadSkill() error {
	var err error
	if p.orig, err = ReadTree(p.Skill.Dir); err != nil {
		return fmt.Errorf("read skill: %w", err)
	}
	if len(p.orig.Odd) > 0 {
		return refuse("", "%s contains symlinks, hard links or oversized files (%s): improve will not copy them", p.Skill.ID, strings.Join(p.orig.Odd, ", "))
	}
	skillMD, ok := p.orig.Files[skillFile]
	if !ok {
		return refuse("", "%s has no SKILL.md", p.Skill.ID)
	}
	if p.OrigDigest, err = evals.SkillDigest(p.Skill.Dir); err != nil {
		return fmt.Errorf("digest skill: %w", err)
	}
	if p.CasesDigest, err = evals.CasesDigest(&p.Skill); err != nil {
		return fmt.Errorf("digest cases: %w", err)
	}
	p.OrigTokens = p.Opts.Counter.Count(string(skillMD.Data))
	p.Constraints = ConstraintsFor(p.OrigTokens, p.Opts.MaxSkillGrowth, p.Opts.AllowFrontmatter, p.Opts.AllowScripts)
	rel, err := filepath.Rel(p.Opts.RepoDir, p.Skill.Dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel, err = filepath.Rel(filepath.Dir(p.Opts.ConfigDir), p.Skill.Dir)
		if err != nil {
			return fmt.Errorf("locate skill: %w", err)
		}
	}
	p.SkillRel = filepath.ToSlash(rel)
	return nil
}

// checkClean refuses a skill with uncommitted changes, so the baseline is
// reproducible. Outside a git repository it only warns.
func (p *Plan) checkClean(all []evals.Skill) error {
	git := p.Opts.Git
	if git == nil {
		return nil
	}
	if _, err := git(p.Opts.RepoDir, "rev-parse", "--show-toplevel"); err != nil {
		p.Warnings = append(p.Warnings, "not a git repository: could not verify that the skill has no uncommitted changes")
		return nil //nolint:nilerr // outside a git repository the check only warns
	}
	changed, err := evals.ChangedSkills(git, p.Opts.RepoDir, "HEAD", all)
	if err != nil {
		p.Warnings = append(p.Warnings, "could not verify that the skill has no uncommitted changes: "+err.Error())
		return nil //nolint:nilerr // an unverifiable state is reported as a warning, as documented above
	}
	if changed[p.Skill.ID] {
		return refuse("", "%s has uncommitted changes (including its eval cases): commit or stash them so the baseline is reproducible", p.Skill.ID)
	}
	return nil
}

// checkSecrets runs the secret scan over everything the optimizer will receive.
func (p *Plan) checkSecrets() error {
	for _, rel := range p.orig.Paths() {
		for _, f := range lint.ScanText(rel, string(p.orig.Files[rel].Data)) {
			if f.Code == lint.CodeSecretDetected {
				return refuse(f.Code, "%s/%s contains a secret (%s): the optimizer would receive it; remove it first", p.Skill.ID, rel, f.Message)
			}
		}
	}
	for i := range p.trainCases {
		for _, text := range caseTexts(&p.trainCases[i]) {
			if name, found := lint.DetectSecret(text); found {
				return refuse(lint.CodeSecretDetected, "train case %s contains a secret (%s): the optimizer would receive it; remove it first", p.trainCases[i].ID, name)
			}
		}
	}
	return nil
}

func caseTexts(c *evals.Case) []string {
	// RubricText covers both rubric and rubric_items: the optimizer receives either.
	texts := []string{c.Prompt, c.RubricText()}
	for _, a := range c.Assertions {
		texts = append(texts, a.Value, a.Command)
	}
	for _, f := range c.Files {
		texts = append(texts, f.Content)
	}
	return texts
}

// estimate projects the eval cost of the whole run and refuses a run whose
// estimate already exceeds --max-cost.
func (p *Plan) estimate() error {
	o := &p.Opts
	req := func(cases []evals.Case) *evals.Request { return &evals.Request{Cases: cases} }
	held, train := req(p.heldCases), req(p.trainCases)
	est := func(r *evals.Request) evals.Estimate {
		return evals.EstimateRun(r, p.OrigTokens, o.Runs, o.Price, o.Counter)
	}
	total := est(held).Add(est(train)) // baseline on both sets
	for round := 0; round < o.MaxRounds; round++ {
		total = total.Add(est(train))
		if round < o.MaxHoldoutEvals {
			total = total.Add(est(held))
		}
	}
	p.Estimate = total
	if total.CostUSD > o.MaxCostUSD {
		return refuse("", "the estimated eval cost $%.2f exceeds --max-cost $%.2f (%d agent runs): raise --max-cost, lower --runs or --max-rounds, or use fewer cases", total.CostUSD, o.MaxCostUSD, total.AgentRuns)
	}
	return nil
}

func (p *Plan) runID() string {
	o := &p.Opts
	h := sha256.New()
	for _, part := range []string{
		p.OrigDigest, p.CasesDigest, strings.Join(o.OptimizerArgv, "\x00"), o.Harness, o.Model,
		fmt.Sprint(o.Runs, o.MinGain, o.MaxRegressions, o.MaxRounds, o.MaxHoldoutEvals, o.HoldoutTag, o.HoldoutFraction),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	base := "imp-" + hex.EncodeToString(h.Sum(nil))[:8]
	id := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(o.ConfigDir, LocalDir, id)); os.IsNotExist(err) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// Summary is the consent text shown before anything runs.
func (p *Plan) Summary() string {
	o := &p.Opts
	var b strings.Builder
	fmt.Fprintf(&b, "improve %s (experimental)\n", Sanitize(p.Skill.ID, 120))
	optimizer := Sanitize(strings.Join(o.OptimizerArgv, " "), 300)
	if o.Adapter != "" {
		optimizer = o.Adapter + " (bundled adapter, a child process of this binary)"
	}
	fmt.Fprintf(&b, "  optimizer:   %s (run without a shell, in a throwaway copy)\n", optimizer)
	fmt.Fprintf(&b, "  split:       %d train case(s), %d held-out case(s) (%s)\n", len(p.trainCases), len(p.heldCases), p.Split.Method)
	fmt.Fprintf(&b, "  gate:        gain >= %.0f points and one stable win, <= %d regression(s), %d round(s), %d held-out evaluation(s)\n", o.MinGain*100, o.MaxRegressions, o.MaxRounds, o.MaxHoldoutEvals)
	ci := "not required"
	if o.RequireCIAboveZero {
		ci = "required above zero"
	}
	fmt.Fprintf(&b, "               held-out >= %d case(s) (share %.0f%% when untagged), SKILL.md growth <= %.2fx, 95%% interval %s\n",
		o.MinHoldoutCases, o.HoldoutFraction*100, EffectiveGrowth(o.MaxSkillGrowth), ci)
	fmt.Fprintf(&b, "  estimate:    %d agent runs, about $%.2f of evals; --max-cost $%.2f\n", p.Estimate.AgentRuns, p.Estimate.CostUSD, o.MaxCostUSD)
	fmt.Fprintf(&b, "  sends out:   the skill text and the %d train case(s) go to whatever the optimizer calls; held-out cases never leave ai-rulez\n", len(p.trainCases))
	if len(o.EnvPass) > 0 {
		fmt.Fprintf(&b, "  environment: scrubbed, plus %s\n", strings.Join(o.EnvPass, ", "))
	} else {
		b.WriteString("  environment: scrubbed (no credentials are passed)\n")
	}
	if len(o.Egress) > 0 {
		fmt.Fprintf(&b, "  egress:      declared %s (informational: ai-rulez cannot block the optimizer's network access)\n", strings.Join(o.Egress, ", "))
	} else {
		b.WriteString("  egress:      none declared (ai-rulez cannot block the optimizer's network access; use a container or CI egress policy)\n")
	}
	if n := o.SiblingNative; n != nil {
		fmt.Fprintf(&b, "  siblings:    native guard on: %d run(s) per trigger prompt of every sibling, a model call each, counted against --max-cost\n", n.runs())
	}
	b.WriteString(p.isolationLine())
	fmt.Fprintf(&b, "  run:         .ai-rulez/%s/%s (nothing outside it changes until `improve apply`)\n", filepath.ToSlash(LocalDir), p.RunID)
	for _, w := range p.Warnings {
		fmt.Fprintf(&b, "  warning:     %s\n", Sanitize(w, 400))
	}
	return b.String()
}
