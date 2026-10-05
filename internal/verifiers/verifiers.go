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

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
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
}

// Report is the outcome of a run.
type Report struct {
	Root    string   `json:"root,omitempty"`
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
	for _, res := range r.Results {
		if res.Status != StatusFail {
			continue
		}
		if res.Severity == severityError || (strict && res.Severity == severityWarning) {
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
	// rootReal is the project root with symlinks resolved, computed once.
	rootReal string
	rootErr  error
	rootDone bool
}

// Run evaluates the configured verifiers in declaration order.
func Run(ctx context.Context, cfg *config.Config, opts Options) *Report {
	rep := &Report{Root: cfg.BaseDir, Results: []Result{}}
	selected, err := selectVerifiers(cfg.Verifiers, opts.Names)
	if err != nil {
		rep.Err = err
		return rep
	}
	env := &Env{Cfg: cfg, Root: cfg.BaseDir, opts: opts}
	for _, v := range selected {
		rep.Results = append(rep.Results, evaluate(ctx, env, v))
	}
	return rep
}

func selectVerifiers(all []config.VerifierConfig, names []string) ([]config.VerifierConfig, error) {
	if len(names) == 0 {
		return all, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []config.VerifierConfig
	for _, v := range all {
		if want[v.Name] {
			out = append(out, v)
			delete(want, v.Name)
		}
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for _, n := range names {
			if want[n] {
				missing = append(missing, n)
			}
		}
		return nil, oops.Hint("Run `ai-rulez verifiers list` to see the declared names.").
			Errorf("unknown verifier(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
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
