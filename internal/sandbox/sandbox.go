// Package sandbox confines a child process: no network, and no writes outside
// the directories the caller names. It wraps an argv with the platform's
// unprivileged confinement tool and leaves starting the process to the caller
// (internal/runner), so it composes with timeouts, output caps and process
// group kills.
//
// Backends: macOS sandbox-exec (deprecated by Apple but present and
// functional), Linux bubblewrap (network and filesystem), Linux unshare
// (network only). On any other platform, or when no tool is installed, Wrap
// refuses with ErrUnavailable: a caller that wants to run anyway must say so
// explicitly with ModeNone, never by ignoring the error.
//
// Confinement is best effort, not a security boundary against a determined
// attacker. The macOS profile starts from "allow default" and denies the
// network, file writes and the Mach services that start applications
// (LaunchServices, Apple Events), but it does not hide the rest of the user
// session. bubblewrap adds a new session and PID namespace and is the stronger
// of the two; unshare confines the network only.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Mode is how a caller wants confinement applied.
type Mode string

// Modes. Auto confines when a backend is usable and runs unconfined otherwise;
// Require refuses to run when no backend is usable; None never confines.
const (
	ModeAuto    Mode = "auto"
	ModeNone    Mode = "none"
	ModeRequire Mode = "require"
)

// ParseMode validates a mode name; the empty string is Auto.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return ModeAuto, nil
	case ModeAuto, ModeNone, ModeRequire:
		return m, nil
	}
	return "", fmt.Errorf("unknown isolation %q (use auto, none or require)", s)
}

// Backend names a confinement tool.
type Backend string

// Backends.
const (
	BackendNone        Backend = ""
	BackendSandboxExec Backend = "sandbox-exec"
	BackendBwrap       Backend = "bwrap"
	BackendUnshare     Backend = "unshare"
)

// ErrUnavailable is returned when no confinement backend can be used.
var ErrUnavailable = errors.New("no process isolation is available on this system")

// Spec says what the confined process may do.
type Spec struct {
	// WriteDirs are the only directories it may write into (with everything
	// below them). Each must exist. Everything else is read-only.
	WriteDirs []string
	// AllowNetwork lets the process use the network. The default (false) denies
	// every socket.
	AllowNetwork bool
}

// Wrapped is an argv ready to start.
type Wrapped struct {
	Argv    []string
	Backend Backend
	// NoNetwork and NoWrites report what the backend actually enforces (unshare
	// confines the network only).
	NoNetwork bool
	NoWrites  bool
}

// Sandbox picks and applies a backend. The zero value is not usable; use
// Default or New.
type Sandbox struct {
	goos     string
	lookPath func(string) (string, error)
	run      runner.Runner

	once     sync.Once
	probeErr error
}

// Default is the sandbox of the running system.
func Default() *Sandbox { return New(runtime.GOOS, runner.LookPath) }

// New builds a sandbox for goos with lookPath finding the tools. Tests use it
// to build the argv of another platform.
func New(goos string, lookPath func(string) (string, error)) *Sandbox {
	return &Sandbox{goos: goos, lookPath: lookPath, run: runner.Exec{}}
}

// WithRunner makes the availability probe start its process through r (a test,
// or a host that records every command). Call it before the first Check.
func (s *Sandbox) WithRunner(r runner.Runner) *Sandbox {
	s.run = runner.Or(r)
	return s
}

// Backend returns the backend that would be used (preferring one that confines
// writes as well as the network), or BackendNone. It does not run anything.
func (s *Sandbox) Backend() Backend {
	b, _ := s.find()
	return b
}

func (s *Sandbox) find() (Backend, string) {
	candidates := []Backend{}
	switch s.goos {
	case "darwin":
		candidates = []Backend{BackendSandboxExec}
	case "linux":
		candidates = []Backend{BackendBwrap, BackendUnshare}
	}
	for _, b := range candidates {
		if p, err := s.lookPath(string(b)); err == nil && filepath.IsAbs(p) {
			return b, p
		}
	}
	return BackendNone, ""
}

// probeTimeout bounds the availability probe.
const probeTimeout = 10 * time.Second

// Check runs a trivial command under the backend once and caches the answer, so
// a tool that is installed but cannot work (user namespaces disabled, already
// inside a sandbox) is reported as unavailable instead of failing every scan.
// The answer outlives the call, so the probe does not inherit ctx's
// cancellation: a caller that gives up must not leave every later caller
// (isolation = "auto") believing there is no backend.
func (s *Sandbox) Check(ctx context.Context) error {
	s.once.Do(func() {
		b, _ := s.find()
		if b == BackendNone {
			s.probeErr = ErrUnavailable
			return
		}
		dir, err := os.MkdirTemp("", "ai-rulez-sandbox-")
		if err != nil {
			s.probeErr = fmt.Errorf("probe isolation: %w", err)
			return
		}
		defer os.RemoveAll(dir) //nolint:errcheck // a temp directory
		true_, lerr := s.lookPath("true")
		if lerr != nil {
			true_ = "/usr/bin/true"
		}
		w, err := s.Wrap(Spec{WriteDirs: []string{dir}}, []string{true_})
		if err != nil {
			s.probeErr = err
			return
		}
		res := s.run.Run(context.WithoutCancel(ctx), runner.Spec{Argv: w.Argv, Dir: dir, Timeout: probeTimeout, MaxOutput: 64 << 10})
		if res.Status != runner.StatusOK {
			detail := strings.TrimSpace(string(res.Stdout) + " " + string(res.Stderr))
			s.probeErr = fmt.Errorf("%s is installed but cannot confine a process: %v (%s)", b, res.Err, detail)
		}
	})
	return s.probeErr
}

// Resolve applies a mode: it reports whether to confine. ModeNone never does;
// ModeAuto does when Check passes; ModeRequire fails when it does not.
func (s *Sandbox) Resolve(ctx context.Context, mode Mode) (bool, error) {
	switch mode {
	case ModeNone:
		return false, nil
	case ModeRequire:
		if err := s.Check(ctx); err != nil {
			return false, err
		}
		return true, nil
	default:
		return s.Check(ctx) == nil, nil
	}
}

// Wrap returns argv prefixed with the backend's confinement command. It refuses
// with ErrUnavailable when there is no backend. Paths in spec are resolved
// through symlinks (macOS keeps temp directories behind /var -> /private/var).
func (s *Sandbox) Wrap(spec Spec, argv []string) (Wrapped, error) {
	if len(argv) == 0 {
		return Wrapped{}, errors.New("sandbox: empty command")
	}
	b, tool := s.find()
	if b == BackendNone {
		return Wrapped{}, ErrUnavailable
	}
	dirs := make([]string, 0, len(spec.WriteDirs))
	for _, d := range spec.WriteDirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return Wrapped{}, fmt.Errorf("sandbox: %w", err)
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		dirs = append(dirs, abs)
	}
	switch b {
	case BackendSandboxExec:
		return wrapSandboxExec(tool, spec, dirs, argv), nil
	case BackendBwrap:
		return wrapBwrap(tool, spec, dirs, argv), nil
	case BackendUnshare:
		return wrapUnshare(tool, spec, argv), nil
	case BackendNone:
	}
	return Wrapped{}, ErrUnavailable
}

// launchServices are the Mach services through which a confined process can
// make a daemon outside the sandbox start or script an application.
var launchServices = []string{
	`(global-name "com.apple.coreservices.launchservicesd")`,
	`(global-name "com.apple.coreservices.appleevents")`,
	`(global-name-prefix "com.apple.lsd.")`,
	`(global-name "com.apple.pasteboard.1")`,
	`(global-name-prefix "com.apple.axserver")`,
	`(global-name "com.apple.windowserver.active")`,
	`(global-name "com.apple.dock.server")`,
}

// sandboxProfile denies the network and every write except the parameters
// W0..Wn (passed with -D, never spliced into the profile text) and a few device
// files every program opens.
func sandboxProfile(spec Spec, n int) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	if !spec.AllowNetwork {
		b.WriteString("(deny network*)\n")
	}
	// Without these, `open -b <bundle id>` asks launchservicesd (outside the
	// sandbox) to start an application, which then runs unconfined.
	b.WriteString("(deny mach-lookup\n")
	for _, svc := range launchServices {
		fmt.Fprintf(&b, "  %s\n", svc)
	}
	b.WriteString(")\n")
	b.WriteString("(deny file-write*)\n(allow file-write*\n")
	b.WriteString("  (literal \"/dev/null\") (literal \"/dev/dtracehelper\") (literal \"/dev/tty\")\n")
	for i := range n {
		fmt.Fprintf(&b, "  (subpath (param \"W%d\"))\n", i)
	}
	b.WriteString(")\n")
	return b.String()
}

func wrapSandboxExec(tool string, spec Spec, dirs, argv []string) Wrapped {
	out := []string{tool}
	for i, d := range dirs {
		out = append(out, "-D", fmt.Sprintf("W%d=%s", i, d))
	}
	out = append(out, "-p", sandboxProfile(spec, len(dirs)))
	out = append(out, argv...)
	return Wrapped{Argv: out, Backend: BackendSandboxExec, NoNetwork: !spec.AllowNetwork, NoWrites: true}
}

func wrapBwrap(tool string, spec Spec, dirs, argv []string) Wrapped {
	out := []string{tool, "--die-with-parent", "--new-session", "--unshare-pid", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc"}
	if !spec.AllowNetwork {
		out = append(out, "--unshare-net")
	}
	for _, d := range dirs {
		out = append(out, "--bind", d, d)
	}
	out = append(out, "--")
	out = append(out, argv...)
	return Wrapped{Argv: out, Backend: BackendBwrap, NoNetwork: !spec.AllowNetwork, NoWrites: true}
}

func wrapUnshare(tool string, spec Spec, argv []string) Wrapped {
	out := []string{tool}
	if !spec.AllowNetwork {
		out = append(out, "--net", "--map-root-user")
	}
	out = append(out, "--")
	out = append(out, argv...)
	return Wrapped{Argv: out, Backend: BackendUnshare, NoNetwork: !spec.AllowNetwork}
}
