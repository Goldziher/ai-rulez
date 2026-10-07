// Package airulez is the public Go API of the ai-rulez engine: load a project,
// validate it, plan or apply a generate run and write or check its lock, from a
// directory, from memory or from a git commit, in a process that may serve many
// projects at once.
//
// Stability: everything in this package is Experimental for its first minor
// release. After that it follows the module's semantic versioning: additive
// changes in minor releases, breaking changes only with a new major version
// (and a new /vN module path). Packages under internal/ carry no promise. The
// Plan document has its own Schema number (see schema/plan.schema.json): a new
// field is additive, a removed or re-typed one raises it.
//
// What a plan is. A plan lists every file a generate run would write, merge into
// or remove, with a digest of the content ai-rulez renders for it. Planning
// reads the project through the Workspace, writes nothing and starts no process
// unless a Runner allows one. Everything the plan reads comes from the
// Workspace, the existing outputs included: the previous manifest, documents
// the engine merges into (.claude/settings.json) and hand-edited files, so a
// workspace in memory or in a commit plans its own removals as a directory
// does. The Plan digest therefore covers the sources and the outputs and
// manifests the workspace holds: two workspaces with the same sources but
// different existing outputs give different digests.
//
// Out of scope here: the agent subcommand, file watching, init prompts,
// scanners, usage recording, the eval runners and the MCP server. They stay in
// the command line.
package airulez

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/registry"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Workspace is a read-only view of a project tree. Names are slash-separated and
// relative to the root; ".." and absolute names are rejected.
//
// Experimental.
type Workspace = workspace.Workspace

// Snapshot is a Workspace over one git commit.
//
// Experimental.
type Snapshot = workspace.Snapshot

// MemWorkspace is a writable in-memory Workspace for tests and for callers that
// receive files over an API.
//
// Experimental.
type MemWorkspace = workspace.Mem

// Spec describes one external command a Runner is asked to start.
//
// Experimental.
type Spec struct {
	// Argv is the command and its arguments.
	Argv []string
	// Dir is the working directory; empty means the current directory.
	Dir string
	// Env is the complete environment (KEY=VALUE) of the child, unless InheritEnv.
	Env []string
	// InheritEnv passes the parent's environment to the child.
	InheritEnv bool
	// Stdin is piped to the child; nil connects it to the null device.
	Stdin []byte
	// Timeout bounds the run; zero asks for the default.
	Timeout time.Duration
	// MaxOutput caps each of stdout and stderr in bytes; zero asks for the default.
	MaxOutput int64
}

// Status is the coarse outcome of a command.
//
// Experimental.
type Status string

// Outcomes of a command.
const (
	// StatusOK means the command ran and exited with status 0.
	StatusOK Status = "ok"
	// StatusExit means the command ran and exited non-zero.
	StatusExit Status = "exit"
	// StatusTimeout means the command was killed at its timeout.
	StatusTimeout Status = "timeout"
	// StatusUnavailable means the command was not found, or was refused.
	StatusUnavailable Status = "unavailable"
	// StatusError means the command could not be started or waited on.
	StatusError Status = "error"
)

// Result is what a command produced. A failure is a Status, never a Go error.
//
// Experimental.
type Result struct {
	Status Status
	// ExitCode is the exit status, -1 when the command did not exit normally.
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// StdoutTruncated and StderrTruncated report output beyond Spec.MaxOutput.
	StdoutTruncated bool
	StderrTruncated bool
	// Timeout is the timeout that applied; Duration how long the run took.
	Timeout  time.Duration
	Duration time.Duration
	// Err is the underlying error for every Status but StatusOK.
	Err error
}

// Runner starts the external commands (git) an operation needs. An embedding
// service implements it to deny, record, sandbox or fake them; DenyAll() refuses
// every one.
//
// Experimental.
type Runner interface {
	Run(ctx context.Context, spec Spec) Result
}

type denyAll struct{}

func (denyAll) Run(_ context.Context, spec Spec) Result {
	return Result{Status: StatusUnavailable, ExitCode: -1, Err: &runner.DeniedError{Argv: append([]string(nil), spec.Argv...)}}
}

// runnerAdapter presents a public Runner to the engine.
type runnerAdapter struct{ r Runner }

func (a runnerAdapter) Run(ctx context.Context, spec runner.Spec) runner.Result {
	res := a.r.Run(ctx, Spec{Argv: spec.Argv, Dir: spec.Dir, Env: spec.Env, InheritEnv: spec.InheritEnv,
		Stdin: spec.Stdin, Timeout: spec.Timeout, MaxOutput: spec.MaxOutput})
	return runner.Result{Status: runner.Status(res.Status), ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr,
		StdoutTruncated: res.StdoutTruncated, StderrTruncated: res.StderrTruncated,
		Timeout: res.Timeout, Duration: res.Duration, Err: res.Err}
}

// engineRunner is r as the engine's runner; nil stays nil (the real git).
func engineRunner(r Runner) runner.Runner {
	if r == nil {
		return nil
	}
	return runnerAdapter{r: r}
}

// Env is the environment a load may read.
//
// Experimental.
type Env = ambient.Env

// MapEnv is a fixed Env.
//
// Experimental.
type MapEnv = ambient.MapEnv

// Clock reports the time stamped into generated files that carry one.
//
// Experimental.
type Clock = ambient.Clock

// Logger receives the engine's log output; a *slog.Logger satisfies it.
//
// Experimental.
type Logger = logger.Logger

// DenyAll returns the Runner that refuses every command. It is the default of
// Options. It is a function, not a variable, so no package can swap the default
// for one that grants process execution to every later Load.
//
// Experimental.
func DenyAll() Runner { return denyAll{} }

// DirWorkspace returns a Workspace over the directory dir of the real file
// system. Only such a workspace can be generated to disk.
//
// Experimental.
func DirWorkspace(dir string) (Workspace, error) {
	ws, err := workspace.OS(dir)
	return ws, err //nolint:wrapcheck // already contextual
}

// NewMemWorkspace returns an empty in-memory Workspace under a virtual root.
//
// Experimental.
func NewMemWorkspace() *MemWorkspace { return workspace.NewMem(virtualRoot()) }

// GitSnapshot returns a Workspace over the tree of rev in the repository at
// repoDir, without checking anything out. git runs through r; nil uses the real
// git, DenyAll() makes the call fail.
//
// Experimental.
func GitSnapshot(ctx context.Context, repoDir, rev string, r Runner) (Snapshot, error) {
	return workspace.GitSnapshot(ctx, repoDir, rev, engineRunner(r)) //nolint:wrapcheck // already contextual
}

// virtualRoot is a root that names no directory on the disk of this process.
func virtualRoot() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "/.ai-rulez-virtual/0"
	}
	return "/.ai-rulez-virtual/" + hex.EncodeToString(b[:])
}

// Options is what a Project is loaded with. Every field but Workspace is optional.
//
// Experimental.
type Options struct {
	// Workspace is the project tree (required).
	Workspace Workspace
	// Runner starts the commands a load needs (git, to resolve a remote include or
	// to bundle skill files); the default is DenyAll().
	Runner Runner
	// Env is the environment the load reads; the default is an empty one.
	Env Env
	// Clock stamps generated files that carry a time; the default is the wall clock.
	Clock Clock
	// Logger receives log output; the default drops it.
	Logger Logger
	// Remote lets the load fetch the remote includes and installed skills the
	// configuration declares, through Runner and with GitToken. The default
	// loads the local content only.
	Remote bool
	// GitToken authenticates remote fetches that need one; it goes only to hosts
	// the fetch code allowlists.
	GitToken string
	// WithLocal reads the machine-local overlay (config.local.toml) and content
	// (.ai-rulez/local) of the directory. It is off by default: an embedding
	// service loads untrusted trees, and machine-local files carry the trust of
	// the machine they were written on.
	WithLocal bool
}

// Project is a loaded configuration and its content. Its operations are
// serialized: a service that plans in parallel loads one Project per request (or
// per worker), which share nothing.
//
// Experimental.
type Project struct {
	mu   sync.Mutex
	cfg  *config.Config
	disk bool
	// git answers repository questions through the Options.Runner.
	git gitutil.Git
	// opts reload the project as it was loaded (workspace, host, remote access),
	// without its lock policy: Lock and LockCheck load under their own.
	opts []config.LoadOption
	// run is the Options.Runner; remote and token are Options.Remote and GitToken.
	run    runner.Runner
	remote bool
	token  string
}

// Error is returned for the failures a caller is expected to tell apart; the
// cause is available through errors.Is and errors.As.
//
// Experimental.
type Error struct {
	// Code is a short stable identifier.
	Code string
	// Err is the cause.
	Err error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}
func (e *Error) Unwrap() error { return e.Err }

// Error codes.
const (
	// CodeLoad is a project that could not be loaded.
	CodeLoad = "load"
	// CodePlan is a plan that could not be rendered.
	CodePlan = "plan"
	// CodeValidate is a validation that could not run (as opposed to findings).
	CodeValidate = "validate"
	// CodeApply is a generate run that failed while applying its plan.
	CodeApply = "apply"
	// CodeDiskRequired is a write requested on a workspace that is not a directory.
	CodeDiskRequired = "disk-required"
	// CodeRefused is a generate run that refused to overwrite a file ai-rulez
	// cannot prove it wrote (a hand-written file in the way, or a symlink), as
	// opposed to an I/O failure (CodeApply). Nothing was written.
	CodeRefused = "refused"
	// CodeLock is a Lock or LockCheck that could not run (as opposed to findings).
	CodeLock = "lock"
	// CodeFindings is a Lock the security scan refused: a remote tree about to
	// be pinned, or a served skill under LockOptions.Strict. The cause is a
	// *FindingsError listing them with their AR codes. Nothing was written.
	CodeFindings = "findings"
)

// ErrDiskRequired is the cause of a CodeDiskRequired error.
var ErrDiskRequired = errors.New("only a directory workspace can be written to")

// ErrRefused is reachable with errors.Is from a CodeRefused error.
var ErrRefused = config.ErrOutputRefused

// applyError classifies a failed apply: a refusal to overwrite a file is
// CodeRefused, anything else CodeApply.
func applyError(err error) *Error {
	if errors.Is(err, ErrRefused) {
		return &Error{Code: CodeRefused, Err: err}
	}
	return &Error{Code: CodeApply, Err: err}
}

// Load reads the project o.Workspace holds.
//
// Experimental.
func Load(ctx context.Context, o Options) (*Project, error) {
	if o.Workspace == nil {
		return nil, &Error{Code: CodeLoad, Err: errors.New("Options.Workspace is required")}
	}
	ws, disk := o.Workspace, workspace.IsDisk(o.Workspace)
	if !disk {
		// Nothing on the disk of this process belongs to the project: give it a
		// root that names no directory, so existing outputs are never read from there.
		ws = workspace.WithRoot(ws, virtualRoot())
	}
	log := logger.Logger(logger.Discard())
	if o.Logger != nil {
		log = o.Logger
	}
	run := DenyAll()
	if o.Runner != nil {
		run = o.Runner
	}
	var env ambient.Env = ambient.MapEnv{}
	if o.Env != nil {
		env = o.Env
	}
	opts := []config.LoadOption{
		config.WithWorkspace(ws),
		config.WithHost(ambient.Host{Env: env, Clock: o.Clock, Runner: runnerAdapter{r: run}, Log: log}),
		config.WithRegistry(registry.Default()),
	}
	if !o.WithLocal {
		opts = append(opts, config.WithoutLocal())
	}
	if o.Remote {
		opts = append(opts, config.WithResolvers(includes.Resolvers(o.GitToken, lint.OKFScanner(nil))))
	} else {
		opts = append(opts, config.WithoutRemote())
	}
	// A Project plans and writes outputs: an enforced ai-rulez.lock ([lock]
	// enforce, on whenever the lock exists) is required, as by `generate`.
	cfg, err := config.LoadConfig(ctx, ".", append(opts[:len(opts):len(opts)], config.WithLockPolicy(config.LockPolicy{RequireWhenEnforced: true}))...)
	if err != nil {
		return nil, &Error{Code: CodeLoad, Err: err}
	}
	return &Project{cfg: cfg, disk: disk, git: gitutil.New(runnerAdapter{r: run}), opts: opts,
		run: runnerAdapter{r: run}, remote: o.Remote, token: o.GitToken}, nil
}

// Name is the project name from its configuration.
//
// Experimental.
func (p *Project) Name() string { return p.cfg.Name }

// Presets lists the presets the configuration generates for, in configuration order.
//
// Experimental.
func (p *Project) Presets() []string {
	names := make([]string, 0, len(p.cfg.Presets))
	for i := range p.cfg.Presets {
		names = append(names, p.cfg.Presets[i].GetName())
	}
	return names
}

// ValidateOptions selects how much Validate checks.
//
// Experimental.
type ValidateOptions struct {
	// Strict also runs the content and security checks (stable AR codes). It
	// needs a directory workspace; git, to index tracked files, runs through
	// Options.Runner (with the default DenyAll() the directory is walked instead).
	Strict bool
}

// Finding is one problem Validate found.
//
// Experimental.
type Finding struct {
	// Code is the stable AR code of a strict check, "" for a configuration error.
	Code     string
	Severity string
	File     string
	Line     int
	Message  string
}

// Report is the result of Validate.
//
// Experimental.
type Report struct {
	Findings []Finding
}

// OK reports whether nothing was found.
func (r *Report) OK() bool { return len(r.Findings) == 0 }

// Validate checks the configuration and, with Strict, the content. A canceled
// ctx stops it between its steps with a CodeValidate error wrapping ctx.Err().
//
// Experimental.
func (p *Project) Validate(ctx context.Context, o ValidateOptions) (*Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, &Error{Code: CodeValidate, Err: err}
	}
	report := &Report{}
	if err := p.cfg.Validate(); err != nil {
		report.Findings = append(report.Findings, Finding{Severity: "error", Message: err.Error()})
		return report, nil //nolint:nilerr // a configuration error is a finding
	}
	if !o.Strict {
		return report, nil
	}
	if !p.disk {
		return nil, &Error{Code: CodeDiskRequired, Err: oops.Wrapf(ErrDiskRequired, "strict validation reads the repository tree")}
	}
	// git runs through Options.Runner: the default DenyAll() indexes by walking the directory.
	tree, err := lint.LoadTreeContext(ctx, p.git, p.cfg.BaseDir, "")
	if err != nil {
		return nil, &Error{Code: CodeValidate, Err: oops.Wrapf(err, "index repository files")}
	}
	if err := ctx.Err(); err != nil {
		return nil, &Error{Code: CodeValidate, Err: err}
	}
	lr, err := lint.Run(p.cfg, tree)
	if err != nil {
		return nil, &Error{Code: CodeValidate, Err: oops.Wrapf(err, "run the strict checks")}
	}
	for _, f := range lr.Findings {
		report.Findings = append(report.Findings, Finding{Code: f.Code, Severity: string(f.Severity), File: f.File, Line: f.Line, Message: f.Message})
	}
	return report, nil
}

// PlanOptions selects what to plan.
//
// Experimental.
type PlanOptions struct {
	// Profile is the profile to render; "" is the configured default.
	Profile string
	// Role renders a role's slice of the content instead of a profile.
	Role string
}

// PlanFile is one output of a plan.
//
// Experimental.
type PlanFile struct {
	// Path is slash-separated and relative to the project root.
	Path string
	// Action is "write", "merge" or "mkdir".
	Action string
	// Mode is the permission bits as an octal string; empty for a directory.
	Mode string
	// Size and SHA256 describe the rendered content, without the generation stamp;
	// they are zero for a sensitive output.
	Size   int
	SHA256 string
	// LocalOnly marks a machine-local output; Sensitive one that may carry a secret;
	// Raw a verbatim payload.
	LocalOnly, Sensitive, Raw bool
}

// PlanRemoval is something a run takes back.
//
// Experimental.
type PlanRemoval struct {
	Path string
	// Reason is "stale", "unmerge" or "delete".
	Reason string
}

// Plan is what a generate run would do, computed without touching anything.
//
// Experimental.
type Plan struct {
	// Schema is the version of the plan document.
	Schema int
	// Profile is the profile rendered (or the role, in Role).
	Profile, Role string
	// Files are the outputs, sorted by path; Removals what a run takes back.
	Files    []PlanFile
	Removals []PlanRemoval
	// Digest is the SHA-256 of the canonical plan document. It covers the
	// sources and the existing outputs and manifests the workspace holds, so it
	// is equal for equal workspaces, not for equal sources alone.
	Digest string

	doc *generator.Plan
}

// JSON is the plan document (schema/plan.schema.json), indented, deterministic.
func (p *Plan) JSON() ([]byte, error) { return generator.MarshalPlan(p.doc) } //nolint:wrapcheck // already contextual

// Plan renders what a generate run would write, merge or remove.
//
// Experimental.
func (p *Project) Plan(ctx context.Context, o PlanOptions) (*Plan, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	doc, err := generator.PlanOutputs(ctx, p.cfg, generator.PlanOptions{Profile: o.Profile, Role: o.Role})
	if err != nil {
		return nil, &Error{Code: CodePlan, Err: err}
	}
	data, err := generator.MarshalPlan(doc)
	if err != nil {
		return nil, &Error{Code: CodePlan, Err: err}
	}
	sum := sha256.Sum256(data)
	plan := &Plan{Schema: doc.Schema, Profile: doc.Profile, Role: doc.Role, Digest: hex.EncodeToString(sum[:]), doc: doc,
		Files: make([]PlanFile, len(doc.Files)), Removals: make([]PlanRemoval, len(doc.Removals))}
	for i, f := range doc.Files {
		plan.Files[i] = PlanFile{Path: f.Path, Action: f.Action, Mode: f.Mode, Size: f.Size, SHA256: f.SHA256,
			LocalOnly: f.LocalOnly, Sensitive: f.Sensitive, Raw: f.Raw}
	}
	for i, r := range doc.Removals {
		plan.Removals[i] = PlanRemoval{Path: r.Path, Reason: r.Reason}
	}
	return plan, nil
}

// Mode says what Generate does with the plan.
//
// Experimental.
type Mode int

// Modes of Generate.
const (
	// Write renders and writes the project's outputs (directory workspaces only).
	Write Mode = iota
	// DryRun lists what Write would do and writes nothing.
	DryRun
	// Check reports the files that differ from the plan and writes nothing.
	Check
)

// GenerateOptions selects a run.
//
// Experimental.
type GenerateOptions struct {
	Profile string
	Mode    Mode
}

// Drift is one file that differs from what the sources render.
//
// Experimental.
type Drift struct {
	Path string
	// Kind is "missing", "stale", "edited", "orphan" or "blocked".
	Kind string
}

// GenerateResult is the outcome of Generate.
//
// Experimental.
type GenerateResult struct {
	// Written is the number of files Write wrote.
	Written int
	// Lines are the actions DryRun lists.
	Lines []string
	// Drift are the files Check found to differ.
	Drift []Drift
}

// Generate applies a plan in the given mode. Write needs a directory workspace:
// anything else fails with CodeDiskRequired before reading or writing a thing.
//
// Experimental.
func (p *Project) Generate(ctx context.Context, o GenerateOptions) (*GenerateResult, error) {
	if o.Mode == Write && !p.disk {
		return nil, &Error{Code: CodeDiskRequired, Err: ErrDiskRequired}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	g := generator.NewGenerator(p.cfg)
	g.SetContext(ctx)
	plan, err := g.Plan(o.Profile)
	if err != nil {
		return nil, &Error{Code: CodePlan, Err: err}
	}
	applier := generator.DiskApplier
	switch o.Mode {
	case DryRun:
		applier = generator.DryRunApplier
	case Check:
		applier = generator.CheckApplier
	case Write:
	default:
		return nil, &Error{Code: CodePlan, Err: errors.New("unknown mode")}
	}
	res, err := g.Apply(plan, applier)
	if err != nil {
		return nil, applyError(err)
	}
	out := &GenerateResult{Written: res.Written, Lines: res.Lines}
	for _, d := range res.Drift {
		out.Drift = append(out.Drift, Drift{Path: d.Path, Kind: string(d.Kind)})
	}
	return out, nil
}

// Slog adapts a *slog.Logger to Logger; a *slog.Logger already satisfies it, this
// exists so the dependency is visible in the documentation.
//
// Experimental.
func Slog(l *slog.Logger) Logger { return l }

// FixedClock returns a Clock that always reports t.
//
// Experimental.
func FixedClock(t time.Time) Clock { return ambient.Fixed(t) }
