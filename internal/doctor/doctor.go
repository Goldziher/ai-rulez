// Package doctor runs read-only diagnostics over an ai-rulez project and
// reports findings by severity. Each check is a small function returning
// findings; Run executes them in a fixed order and never writes to the project.
package doctor

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// sharedWarnings makes every load of one run share a warning collector that says
// each warning once. Doctor loads the project twice (the lock check reads it
// without the local overlay) and renders it twice (drift, gitignore); each load
// and each render would otherwise repeat the same warnings, since a render
// starts a fresh de-duplication round.
func sharedWarnings(load Loader) Loader {
	var mu sync.Mutex
	said := map[string]bool{}
	out := diag.New(nil)
	c := diag.New(func(msg string, args ...any) {
		key := msg + "\x00" + fmt.Sprint(args...)
		mu.Lock()
		first := !said[key]
		said[key] = true
		mu.Unlock()
		if first {
			out.Raise(msg, args...)
		}
	})
	return func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
		return load(ctx, append(opts, config.WithCollector(c))...)
	}
}

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
	CheckPolicy    = "policy"
	CheckPresets   = "presets"
	CheckDrift     = "drift"
	CheckGitignore = "gitignore"
	CheckDocuments = "documents"
	CheckMCPEnv    = "mcp-env"
	CheckMCPScope  = "mcp-scope"
	CheckHooks     = "hooks"
	CheckLock      = "lock"
	CheckIncludes  = "includes"
	CheckTools     = "tools"
	CheckSource    = "source"
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
	// ReadUserMCP runs the mcp-scope check, which reads the configured
	// harnesses' user and local MCP files (for example ~/.claude.json) outside
	// the project. It is off by default so a run never reads outside the project.
	ReadUserMCP bool
	// Runner answers the repository question of the session-start check (is the settings file ignored);
	// nil means runner.Exec.
	Runner runner.Runner
	// Probe answers an HTTPS GET of the session-start check (is the npm registry reachable from a remote
	// session); nil skips the probe. It returns the HTTP status, or an error when there was no response.
	Probe func(ctx context.Context, url string) (status int, err error)
	// IndexCache, when set, places the index cache of a loaded project for the
	// index-cache check (it knows the secret that authenticates the cache). Nil
	// skips the check.
	IndexCache func(cfg *config.Config) indexcache.Options
	// OnCheck, when set, is called after each check with its name and how long it
	// took, in run order. It is how a slow run says where the time went.
	OnCheck func(name string, elapsed time.Duration)
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

// runsWithoutConfiguration reports whether the check says something about a
// project that has no configuration. The others judge presets, generated
// outputs, includes or a lock, which such a project does not have.
func runsWithoutConfiguration(name string) bool {
	return name == "config" || name == "policy"
}

// Run executes every check and returns the findings, most serious first and in
// check order within a severity. Checks that need the configuration are skipped
// when it does not load; that failure is itself reported by the config check.
func Run(ctx context.Context, o Options) *Report {
	if o.LookPath == nil {
		o.LookPath = runner.LookPath
	}
	o.Load = sharedWarnings(o.Load)
	s := &state{opts: o}
	s.cfg, s.loadErr = o.Load(ctx, config.WithoutRemote())

	report := &Report{Unloadable: s.loadErr != nil}
	if s.cfg != nil {
		report.Root = s.cfg.BaseDir
	}
	checks := []struct {
		name string
		run  check
	}{
		{"config", checkConfig},
		{"policy", checkPolicy},
		{"presets", checkPresets},
		{"source", checkSource},
		{"mcp-env", checkMCPEnv},
		{"mcp-scope", checkMCPScope},
		{"drift", checkDrift},
		{"gitignore", checkGitignore},
		{"documents", checkDocuments},
		{"okf", checkOKF},
		{"hooks", checkHooks},
		{"includes", checkIncludes},
		{"lock", checkLock},
		{"llm", checkLLM},
		{"tools", checkTools},
		{CheckSessionStart, checkSessionStart},
		{"role-modes", checkRoleModes},
		{CheckIndexCache, checkIndexCache},
	}
	for _, c := range checks {
		if s.cfg != nil && s.cfg.Implicit && !runsWithoutConfiguration(c.name) {
			continue
		}
		start := ambient.Clock(nil).Now()
		report.Findings = append(report.Findings, c.run(ctx, s)...)
		if o.OnCheck != nil {
			o.OnCheck(c.name, ambient.Clock(nil).Now().Sub(start))
		}
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		return report.Findings[i].Severity.rank() < report.Findings[j].Severity.rank()
	})
	if report.Findings == nil {
		report.Findings = []Finding{}
	}
	return report
}
