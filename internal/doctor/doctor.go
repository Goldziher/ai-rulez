// Package doctor runs read-only diagnostics over an ai-rulez project and
// reports findings by severity. Each check is a small function returning
// findings; Run executes them in a fixed order and never writes to the project.
package doctor

import (
	"context"
	"os/exec"
	"sort"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Severity ranks a finding.
type Severity string

// The severities, most serious first.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// Check names, stable for JSON consumers.
const (
	CheckConfig    = "config"
	CheckPresets   = "presets"
	CheckDrift     = "drift"
	CheckGitignore = "gitignore"
	CheckDocuments = "documents"
	CheckMCPEnv    = "mcp-env"
	CheckHooks     = "hooks"
	CheckLock      = "lock"
	CheckTools     = "tools"
)

// Finding is one diagnostic.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// Report is the outcome of a doctor run.
type Report struct {
	Root     string    `json:"root,omitempty"`
	Findings []Finding `json:"findings"`
	// Unloadable is set when the configuration could not be loaded at all, so
	// most checks could not run. Callers map it to a "cannot run" exit code,
	// distinct from findings in a project that was diagnosed.
	Unloadable bool `json:"-"`
}

// Counts returns the number of findings per severity.
func (r *Report) Counts() map[Severity]int {
	out := map[Severity]int{}
	for _, f := range r.Findings {
		out[f.Severity]++
	}
	return out
}

// Failed reports whether the run should exit non-zero: any error, and with
// strict any warning too.
func (r *Report) Failed(strict bool) bool {
	c := r.Counts()
	return c[SeverityError] > 0 || (strict && c[SeverityWarning] > 0)
}

// Loader loads the project configuration with the command's flags applied plus
// the given extra options. Run always passes config.WithoutRemote(): doctor is
// read-only, so it never fetches an include or writes the include cache.
type Loader func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error)

// Options configures a run.
type Options struct {
	// Load loads the configuration. Required.
	Load Loader
	// Profile is the generate profile to render for the drift and gitignore
	// checks ("" selects the default).
	Profile string
	// LookPath resolves a binary on PATH; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

// state is what the checks share.
type state struct {
	opts    Options
	cfg     *config.Config
	loadErr error
	// renderFailed is set by the drift check when outputs could not be rendered,
	// so later checks that render too do not report the same failure again.
	renderFailed bool
}

type check func(ctx context.Context, s *state) []Finding

// Run executes every check and returns the findings, most serious first and in
// check order within a severity. Checks that need the configuration are skipped
// when it does not load; that failure is itself reported by the config check.
func Run(ctx context.Context, o Options) *Report {
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	s := &state{opts: o}
	s.cfg, s.loadErr = o.Load(ctx, config.WithoutRemote())

	report := &Report{Unloadable: s.loadErr != nil}
	if s.cfg != nil {
		report.Root = s.cfg.BaseDir
	}
	checks := []check{
		checkConfig,
		checkPresets,
		checkMCPEnv,
		checkDrift,
		checkGitignore,
		checkDocuments,
		checkHooks,
		checkLock,
		checkTools,
	}
	for _, c := range checks {
		report.Findings = append(report.Findings, c(ctx, s)...)
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		return report.Findings[i].Severity.rank() < report.Findings[j].Severity.rank()
	})
	if report.Findings == nil {
		report.Findings = []Finding{}
	}
	return report
}
