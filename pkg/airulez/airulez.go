// Package airulez is the public Go API of the ai-rulez engine: load a project,
// validate it and plan or apply a generate run, from a directory, from memory or
// from a git commit, in a process that may serve many projects at once.
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
// unless a Runner allows one. A Workspace that is not backed by a directory
// (memory, a git snapshot) is planned as a project that has no generated files
// yet: files the engine merges into (.claude/settings.json) are rendered from
// scratch, because the engine reads existing outputs from the directory a
// workspace names and a snapshot of sources has none. Two workspaces holding
// the same sources therefore give the same Plan digest.
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

// Runner starts the external commands (git) an operation needs; DenyAll refuses
// every one.
//
// Experimental.
type Runner = runner.Runner

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

// DenyAll is the Runner that refuses every command. It is the default of Options.
//
// Experimental.
var DenyAll Runner = runner.Deny{}

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
// git, DenyAll makes the call fail.
//
// Experimental.
func GitSnapshot(ctx context.Context, repoDir, rev string, r Runner) (Snapshot, error) {
	return workspace.GitSnapshot(ctx, repoDir, rev, r) //nolint:wrapcheck // already contextual
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
	// to bundle skill files); the default is DenyAll.
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
	// WithoutLocal ignores the machine-local overlay and content.
	WithoutLocal bool
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

func (e *Error) Error() string { return e.Code + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Error codes.
const (
	// CodeLoad is a project that could not be loaded.
	CodeLoad = "load"
	// CodePlan is a plan that could not be rendered.
	CodePlan = "plan"
	// CodeDiskRequired is a write requested on a workspace that is not a directory.
	CodeDiskRequired = "disk-required"
)

// ErrDiskRequired is the cause of a CodeDiskRequired error.
var ErrDiskRequired = errors.New("only a directory workspace can be written to")

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
	var run runner.Runner = runner.Deny{}
	if o.Runner != nil {
		run = o.Runner
	}
	var env ambient.Env = ambient.MapEnv{}
	if o.Env != nil {
		env = o.Env
	}
	opts := []config.LoadOption{
		config.WithWorkspace(ws),
		config.WithHost(ambient.Host{Env: env, Clock: o.Clock, Runner: run, Log: log}),
		config.WithRegistry(registry.Default()),
	}
	if o.WithoutLocal {
		opts = append(opts, config.WithoutLocal())
	}
	if o.Remote {
		opts = append(opts, config.WithResolvers(includes.Resolvers(o.GitToken)))
	} else {
		opts = append(opts, config.WithoutRemote())
	}
	cfg, err := config.LoadConfig(ctx, ".", opts...)
	if err != nil {
		return nil, &Error{Code: CodeLoad, Err: err}
	}
	return &Project{cfg: cfg, disk: disk}, nil
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
	// needs a directory workspace inside a git work tree.
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

// Validate checks the configuration and, with Strict, the content.
//
// Experimental.
func (p *Project) Validate(_ context.Context, o ValidateOptions) (*Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	tree, err := lint.LoadTree(p.cfg.BaseDir) //nolint:contextcheck // git runs through the Host runner, with no context to pass
	if err != nil {
		return nil, oops.Wrapf(err, "index repository files")
	}
	lr, err := lint.Run(p.cfg, tree)
	if err != nil {
		return nil, oops.Wrapf(err, "run the strict checks")
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
	// Digest is the SHA-256 of the canonical plan document: equal for equal
	// sources, whichever workspace they came from.
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
	// Kind is "missing", "stale", "edited" or "orphan".
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
	plan, err := g.Plan(o.Profile) //nolint:contextcheck // the context reaches the run through SetContext
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
		return nil, err //nolint:wrapcheck // already contextual
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
