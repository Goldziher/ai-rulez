package improve

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// IsolationReport records how the optimizer was confined.
type IsolationReport struct {
	// Mode is the requested mode: none, auto or require.
	Mode string `json:"mode"`
	// Backend is the sandbox tool used; empty when the optimizer ran unconfined.
	Backend  string `json:"backend,omitempty"`
	Confined bool   `json:"confined"`
	// NoNetwork and NoWrites say what the backend enforces: writes only inside the run's workspace,
	// home and tmp directories, and no network unless --egress declared one.
	NoNetwork bool `json:"no_network,omitempty"`
	NoWrites  bool `json:"no_writes,omitempty"`
}

// resolveIsolation decides, before anything runs, whether the optimizer is confined. require refuses
// when no backend works (AR9J7); auto runs unconfined and says so in the plan; none never confines.
func (p *Plan) resolveIsolation(ctx context.Context) error {
	o := &p.Opts
	mode := o.Isolation
	if mode == "" {
		mode = sandbox.ModeNone
	}
	p.isolation = &IsolationReport{Mode: string(mode)}
	if mode == sandbox.ModeNone {
		return nil
	}
	sb := o.Sandbox
	if sb == nil {
		sb = sandbox.Default()
	}
	confine, err := sb.Resolve(ctx, mode)
	if err != nil {
		if errors.Is(err, sandbox.ErrUnavailable) || mode == sandbox.ModeRequire {
			return refuse(CodeIsolationUnavailable, "--isolation require, but the optimizer cannot be confined: %s; run improve in a container or CI job, or use --isolation auto", Sanitize(err.Error(), 300))
		}
		return refuse(CodeIsolationUnavailable, "%s", Sanitize(err.Error(), 300))
	}
	if !confine {
		p.Warnings = append(p.Warnings, CodeIsolationUnavailable+" isolation: no sandbox backend works on this system, so the optimizer runs unconfined (--isolation require refuses to run instead)")
		return nil
	}
	p.sandbox = sb
	p.isolation.Backend = string(sb.Backend())
	p.isolation.Confined = true
	// Ask the backend what it really enforces now, so the consent summary says so before anything
	// runs (unshare confines the network only). confineArgv records the same answer for the report.
	w, werr := sb.Wrap(sandbox.Spec{WriteDirs: []string{o.ConfigDir}, AllowNetwork: len(o.Egress) > 0}, []string{"true"})
	if werr != nil {
		return refuse(CodeIsolationUnavailable, "%s", Sanitize(werr.Error(), 300))
	}
	p.isolation.NoNetwork, p.isolation.NoWrites = w.NoNetwork, w.NoWrites
	return nil
}

// isolationLine is the consent-summary text about confinement. It states what the backend enforces and,
// always, what no backend does here: reads are open, and a confined optimizer is still not trusted.
func (p *Plan) isolationLine() string {
	switch {
	case p.isolation != nil && p.isolation.Confined:
		writes := "writes are NOT confined by this backend"
		if p.isolation.NoWrites {
			writes = "writes only inside the run's workspace, home and tmp directories"
		}
		net := "network is NOT blocked by this backend"
		switch {
		case p.isolation.NoNetwork:
			net = "no network"
		case len(p.Opts.Egress) > 0:
			net = "network allowed (egress declared)"
		}
		return fmt.Sprintf("  isolation:   %s sandbox: %s, %s; reads are not restricted, so the optimizer can still read your repository, the held-out cases and the report-signing key (see docs/improve.md)\n", p.isolation.Backend, writes, net)
	case p.isolation != nil && p.isolation.Mode != string(sandbox.ModeNone):
		return "  isolation:   requested but unavailable: the optimizer runs unconfined\n"
	}
	return "  isolation:   none (the optimizer runs as you, unconfined, and can read your repository, the held-out cases and the report-signing key; --isolation auto|require confines writes and network)\n"
}

// confineArgv wraps the optimizer argv in the sandbox: writes only below the workspace, home and tmp
// directories of the run, and no network unless the run declared egress (informational otherwise, but
// a declared host is the user saying the optimizer needs the network).
func (x *execution) confineArgv(argv []string) ([]string, error) {
	p := x.p
	if p.sandbox == nil {
		return argv, nil
	}
	// The sandbox tool resolves its command itself; hand it the absolute path found here so a
	// relative PATH entry cannot substitute another binary.
	confined := append([]string(nil), argv...)
	if bin, err := runner.LookPath(argv[0]); err == nil && filepath.IsAbs(bin) {
		confined[0] = bin
	}
	w, err := p.sandbox.Wrap(sandbox.Spec{
		WriteDirs:    []string{x.workspace, filepath.Join(x.dir, "home"), filepath.Join(x.dir, "tmp")},
		AllowNetwork: len(p.Opts.Egress) > 0,
	}, confined)
	if err != nil {
		return nil, fmt.Errorf("apply isolation: %w", err)
	}
	p.isolation.NoNetwork, p.isolation.NoWrites = w.NoNetwork, w.NoWrites
	return w.Argv, nil
}
