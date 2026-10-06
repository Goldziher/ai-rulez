// Package verifiers runs the deterministic repo checks declared as
// [[verifiers]] in the project configuration. Each verifier is one predicate
// (a file exists, a glob matches a bounded number of files, a regex is present
// or absent, a JSON/YAML/TOML key has a value, generated output is in sync)
// evaluated read-only against the project root. Run never writes, never uses
// the network and never starts a process.
//
// Extension point: a predicate is a function registered under its config
// type name with Register. Phase 2 adds a "command" predicate that way, on top
// of the hardened command runner; it is not implemented here and the config
// validator rejects the type until it is.
package verifiers

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
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
	// AR9H2 (the declaration is invalid) or AR9H5 (dead scope).
	Code string `json:"code,omitempty"`
	// Target is the rule, skill, agent or command the verifier enforces.
	Target *Target `json:"target,omitempty"`
	// Fix says how to make the verifier pass.
	Fix string `json:"fix,omitempty"`
	// Source is the declaration file of a verifier declared under verifiers/.
	Source   string    `json:"source,omitempty"`
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
}

// Run evaluates the configured verifiers: the [[verifiers]] of config.toml in
// declaration order (flat entries, then spec-form ones), then those of
// .ai-rulez/verifiers/*.toml.
func Run(ctx context.Context, cfg *config.Config, opts Options) *Report {
	rep := &Report{Root: cfg.BaseDir, Results: []Result{}}
	if opts.Since != "" && opts.Staged {
		rep.Err = oops.New("--since and --staged cannot be combined")
		return rep
	}
	specs, problems := LoadSpecs(cfg)
	entries := make([]entry, 0, len(cfg.Verifiers)+len(specs))
	for i := range cfg.Verifiers {
		if !cfg.Verifiers[i].IsSpec() {
			entries = append(entries, entry{id: cfg.Verifiers[i].Name, legacy: &cfg.Verifiers[i]})
		}
	}
	for i := range specs {
		entries = append(entries, entry{id: specs[i].ID, spec: &specs[i]})
	}
	selected, err := selectEntries(entries, opts, len(problems) > 0)
	if err != nil {
		rep.Err = err
		return rep
	}
	env := &Env{Cfg: cfg, Root: cfg.BaseDir, opts: opts}
	needScope := opts.Since != "" || opts.Staged
	for _, e := range selected {
		needScope = needScope || e.spec != nil
	}
	if needScope {
		if err := env.prepareScope(ctx); err != nil {
			rep.Err = err
			return rep
		}
		rep.Mode = env.scope.describe()
	}
	for _, e := range selected {
		if e.spec != nil {
			rep.Results = append(rep.Results, evaluateSpec(ctx, env, e.spec))
			continue
		}
		rep.Results = append(rep.Results, evaluate(ctx, env, *e.legacy))
	}
	if len(opts.Names) == 0 && opts.Rule == "" {
		rep.Results = append(rep.Results, problemResults(problems)...)
	}
	return rep
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
		res.Status, res.Message = StatusFail, out.Message
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
