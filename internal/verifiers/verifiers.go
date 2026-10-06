// Package verifiers runs the deterministic repo checks declared as
// [[verifiers]] in the project configuration. Each verifier is one predicate
// (a file exists, a glob matches a bounded number of files, a regex is present
// or absent, a JSON/YAML/TOML key has a value, generated output is in sync)
// evaluated read-only against the project root. Run never writes and never uses
// the network (the llm predicate calls a model only with Options.LLM).
//
// Only the spec-form "command" predicate starts a process, and only with
// Options.AllowExec; it runs through internal/runner. A predicate of the flat
// form is a function registered under its config type name with Register.
package verifiers

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/samber/oops"
)

// Status is the outcome of one verifier.
type Status string

// The statuses. StatusError means the verifier could not be evaluated (an
// unreadable file, a render failure); it is distinct from a failed check.
const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusError Status = "error"
	// StatusNotApplicable means when_changed selected no changed file, so the
	// verifier had nothing to check. It never fails a run.
	StatusNotApplicable Status = "not_applicable"
	// StatusSkipped means a verifier that needs a model (the llm predicate) was
	// not evaluated: LLM use is off, the budget would be exceeded, or --estimate
	// was given. It is shown, never counted as a pass, and never fails a run (AR9H4).
	StatusSkipped Status = "skipped"
	// StatusInactive means the rule or skill the verifier enforces is not part
	// of the active profile or role, so the verifier does not apply.
	StatusInactive Status = "inactive"
)

const (
	severityError   = "error"
	severityWarning = "warning"
)

// Result is the outcome of one verifier.
type Result struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Status      Status `json:"status"`
	Description string `json:"description,omitempty"`
	Message     string `json:"message,omitempty"`
	// Code is the rule code of a failure: AR9H1 (a predicate did not hold),
	// AR9H2 (the declaration is invalid), AR9H3 (a command was refused or did
	// not run), AR9H4 (an LLM verifier was skipped), AR9H5 (dead scope) or
	// AR9H6 (no self-test examples).
	Code string `json:"code,omitempty"`
	// Target is the rule, skill, agent or command the verifier enforces.
	Target *Target `json:"target,omitempty"`
	// Fix says how to make the verifier pass.
	Fix string `json:"fix,omitempty"`
	// Source is the declaration file of a verifier declared under verifiers/.
	Source string `json:"source,omitempty"`
	// Advisory marks a verdict a model produced: its severity is at most warning.
	Advisory bool      `json:"advisory,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	// Notes list files skipped (binary, oversized) and similar.
	Notes []string `json:"notes,omitempty"`
}

// Target is the item a verifier enforces, with the file (and heading line)
// a reviewer should read.
type Target struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Report is the outcome of a run.
type Report struct {
	Root string `json:"root,omitempty"`
	// Mode says which files counted as changed: "all", "since <rev>" or "staged".
	Mode    string   `json:"mode,omitempty"`
	Results []Result `json:"results"`
	// LLM totals the model use of the run; nil when no model was called.
	LLM *LLMUsage `json:"llm,omitempty"`
	// Err is set when the run could not start (for example an unknown name was
	// requested); no verifier ran.
	Err error `json:"-"`
}

// Counts returns the number of results per status.
func (r *Report) Counts() map[Status]int {
	out := map[Status]int{}
	for _, res := range r.Results {
		out[res.Status]++
	}
	return out
}

// Failed reports whether a verifier failed at a failing severity: error, and
// with strict warning too. An info-level failure never fails the run.
func (r *Report) Failed(strict bool) bool {
	if strict {
		return r.FailedAt(severityWarning)
	}
	return r.FailedAt(severityError)
}

// FailedAt reports whether a verifier failed at or above the severity: "error",
// "warning" or "info". "none" never fails.
func (r *Report) FailedAt(level string) bool {
	rank := map[string]int{"info": 1, severityWarning: 2, severityError: 3}
	threshold, ok := rank[level]
	if !ok {
		return false
	}
	for _, res := range r.Results {
		if res.Status == StatusFail && rank[res.Severity] >= threshold {
			return true
		}
	}
	return false
}

// CannotRun reports whether the run could not be completed: it did not start,
// or a verifier could not be evaluated.
func (r *Report) CannotRun() bool {
	return r.Err != nil || r.Counts()[StatusError] > 0
}

// Outcome is what a predicate returns when it evaluated.
type Outcome struct {
	Pass    bool
	Message string
	// Findings are the places a failed predicate did not hold; the SARIF and
	// JUnit reports locate a flat verifier through them. Their messages carry no
	// line numbers or counts, so a fingerprint built from them is stable.
	Findings []Finding
}

// Predicate evaluates one verifier. A returned error means it could not be
// evaluated; a failed check is Outcome{Pass: false}.
type Predicate func(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error)

var predicates = map[string]Predicate{
	config.VerifierFileExists:      predFileExists,
	config.VerifierFileAbsent:      predFileAbsent,
	config.VerifierGlobCount:       predGlobCount,
	config.VerifierRegex:           predRegex,
	config.VerifierForbid:          predForbid,
	config.VerifierKeyEquals:       predKeyEquals,
	config.VerifierGeneratedInSync: predGeneratedInSync,
}

// Register adds or replaces the predicate for a verifier type. It must be
// called during program initialization, before any Run.
func Register(typ string, p Predicate) { predicates[typ] = p }

// Options configures a run.
type Options struct {
	// Names restricts the run to these verifiers; empty runs all.
	Names []string
	// Drift lists generated files that differ from a fresh render of the
	// profile, as "kind: path". nil renders with the generator.
	Drift func(cfg *config.Config, profile string) ([]string, error)
	// Since evaluates only what changed since the merge base with this
	// revision (plus uncommitted and untracked files); Staged only what is
	// staged. At most one may be set. A revision that does not resolve is an
	// error, never an empty scope.
	Since  string
	Staged bool
	// Rule keeps only the verifiers that enforce this rule, skill, agent or
	// command (id or domain/id).
	Rule string
	// StrictApplicability reports a verifier whose when_changed matches no
	// file of the repository (AR9H5).
	StrictApplicability bool
	// Profile and Role select which rules and skills are active: a verifier whose
	// target lies outside them is reported inactive and never fails the run.
	// Empty means the configured default profile, as for `generate`.
	Profile string
	Role    string
	// AllowExec lets command predicates run (--allow-exec). Without it each
	// one is status error with AR9H3; nothing is ever started.
	AllowExec bool
	// Runner starts command predicates; nil runs real processes.
	Runner runner.Runner
	// LLM enables llm predicates (see LLMOptions); nil skips them.
	LLM *LLMOptions
	// Environ is the parent environment of commands (KEY=VALUE); nil is the
	// process environment. Only the allowlist survives into the child.
	Environ []string
}

// Env is what predicates share for one run.
type Env struct {
	Cfg  *config.Config
	Root string
	opts Options
	// files is the lazily built, sorted list of repo-relative files, and
	// unreadable the directories the walk could not read.
	files      []string
	unreadable []string
	built      bool
	scope      *scopeData
	// rootReal is the project root with symlinks resolved, computed once.
	rootReal string
	rootErr  error
	rootDone bool
	// llm is the model accounting of the run, created on first use.
	llm *llmRun
}

// Run evaluates the configured verifiers: the [[verifiers]] of config.toml in
// declaration order (flat entries, then spec-form ones), then those of
// .ai-rulez/verifiers/*.toml and of includes.
func Run(ctx context.Context, cfg *config.Config, opts Options) *Report {
	rep := &Report{Root: cfg.BaseDir, Results: []Result{}}
	if opts.Since != "" && opts.Staged {
		rep.Err = oops.New("--since and --staged cannot be combined")
		return rep
	}
	if cfg.VerifiersSettings != nil && cfg.VerifiersSettings.WarnDead {
		opts.StrictApplicability = true
	}
	specs, problems := LoadSpecs(cfg)
	selected, err := selectEntries(declaredEntries(cfg, specs), opts, len(problems) > 0)
	if err != nil {
		rep.Err = err
		return rep
	}
	active, err := activeContent(cfg, opts.Profile, opts.Role)
	if err != nil {
		rep.Err = err
		return rep
	}
	env := &Env{Cfg: cfg, Root: cfg.BaseDir, opts: opts}
	if needsScope(opts, selected) {
		if err := env.prepareScope(ctx); err != nil {
			rep.Err = err
			return rep
		}
		rep.Mode = env.scope.describe()
	}
	for _, e := range selected {
		rep.Results = append(rep.Results, evaluateEntry(ctx, env, active, e)...)
	}
	if len(opts.Names) == 0 && opts.Rule == "" {
		rep.Results = append(rep.Results, problemResults(problems)...)
	}
	if env.llm != nil && (env.llm.usage.Calls > 0 || env.llm.opts.Estimate) {
		u := env.llm.usage
		rep.LLM = &u
	}
	return rep
}

// declaredEntries lists the flat verifiers of config.toml, then the specs.
func declaredEntries(cfg *config.Config, specs []Spec) []entry {
	entries := make([]entry, 0, len(cfg.Verifiers)+len(specs))
	for i := range cfg.Verifiers {
		if !cfg.Verifiers[i].IsSpec() {
			entries = append(entries, entry{id: cfg.Verifiers[i].Name, legacy: &cfg.Verifiers[i]})
		}
	}
	for i := range specs {
		entries = append(entries, entry{id: specs[i].ID, spec: &specs[i]})
	}
	return entries
}

// needsScope reports whether the run must resolve the file set and changes.
func needsScope(opts Options, selected []entry) bool {
	if opts.Since != "" || opts.Staged {
		return true
	}
	for _, e := range selected {
		if e.spec != nil {
			return true
		}
	}
	return false
}

// evaluateEntry runs one declared verifier: a flat one, or a spec that is
// inactive (outside the active profile or role), evaluated, and optionally
// followed by its missing-examples finding.
func evaluateEntry(ctx context.Context, env *Env, active activeSet, e entry) []Result {
	if e.spec == nil {
		return []Result{evaluate(ctx, env, *e.legacy)}
	}
	if why, off := active.inactive(env.Cfg, e.spec); off {
		return []Result{inactiveResult(env, e.spec, why)}
	}
	out := []Result{evaluateSpec(ctx, env, e.spec)}
	if res, missing := missingExamples(env, e.spec); missing {
		out = append(out, res)
	}
	return out
}

// entry is one declared verifier: a flat config.toml one or a spec.
type entry struct {
	id     string
	legacy *config.VerifierConfig
	spec   *Spec
}

func selectEntries(all []entry, opts Options, hasProblems bool) ([]entry, error) {
	out := all
	if opts.Rule != "" {
		out = nil
		for _, e := range all {
			if e.spec == nil {
				continue
			}
			if _, id := e.spec.TargetKind(); id == opts.Rule || strings.HasSuffix(id, "/"+opts.Rule) {
				out = append(out, e)
			}
		}
	}
	if len(opts.Names) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, n := range opts.Names {
		want[n] = true
	}
	var picked []entry
	for _, e := range out {
		if want[e.id] {
			picked = append(picked, e)
			delete(want, e.id)
		}
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for _, n := range opts.Names {
			if want[n] {
				missing = append(missing, n)
			}
		}
		hint := "Run `ai-rulez verifiers list` to see the declared names."
		if hasProblems {
			hint += " A declaration with a problem is not selectable; run without --name to see it."
		}
		return nil, oops.Hint(hint).Errorf("unknown verifier(s): %s", strings.Join(missing, ", "))
	}
	return picked, nil
}

// problemResults turns invalid declarations into AR9H2 error results.
func problemResults(problems []Problem) []Result {
	out := make([]Result, 0, len(problems))
	for _, p := range problems {
		name := p.ID
		if name == "" {
			name = p.File
		}
		out = append(out, Result{
			Name: sanitize(name), Type: "invalid", Severity: severityError, Status: StatusError, Code: CodeVerifierInvalid,
			Message: sanitize(p.Message), Source: p.File,
		})
	}
	return out
}

func evaluate(ctx context.Context, env *Env, v config.VerifierConfig) (res Result) {
	res = Result{Name: v.Name, Type: v.Type, Severity: v.Severity, Description: v.Description}
	if res.Severity == "" {
		res.Severity = severityError
	}
	defer func() { res.Message = sanitize(res.Message) }()
	if err := ctx.Err(); err != nil {
		res.Status, res.Message = StatusError, "not run: "+err.Error()
		return res
	}
	pred, ok := predicates[v.Type]
	if !ok {
		res.Status, res.Message = StatusError, fmt.Sprintf("unknown verifier type %q", v.Type)
		return res
	}
	out, err := pred(ctx, env, v)
	switch {
	case err != nil:
		res.Status, res.Message = StatusError, err.Error()
	case out.Pass:
		res.Status, res.Message = StatusPass, out.Message
	default:
		res.Status, res.Message, res.Findings = StatusFail, out.Message, out.Findings
	}
	return res
}

func defaultDrift(cfg *config.Config, profile string) ([]string, error) {
	drift, err := generator.NewGenerator(cfg).CheckDrift(profile)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	out := make([]string, 0, len(drift))
	for _, d := range drift {
		out = append(out, fmt.Sprintf("%s: %s", d.Kind, d.Path))
	}
	return out, nil
}

// sanitize makes text safe to print to a terminal: control characters (ESC, so
// ANSI sequences, newlines, carriage returns) and other non-printable runes,
// which a hostile file name can carry, are shown as \xNN or \uNNNN escapes.
func sanitize(s string) string {
	clean := true
	for _, r := range s {
		if !unicode.IsPrint(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var sb strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsPrint(r):
			sb.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&sb, `\x%02x`, r)
		default:
			fmt.Fprintf(&sb, `\u%04x`, r)
		}
	}
	return sb.String()
}
