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
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
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

	once     sync.Once
	probeErr error
}

// Default is the sandbox of the running system.
func Default() *Sandbox { return New(runtime.GOOS, exec.LookPath) }

// New builds a sandbox for goos with lookPath finding the tools. Tests use it
// to build the argv of another platform.
func New(goos string, lookPath func(string) (string, error)) *Sandbox {
	return &Sandbox{goos: goos, lookPath: lookPath}
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

// Check runs a trivial command under the backend once and caches the answer, so
// a tool that is installed but cannot work (user namespaces disabled, already
// inside a sandbox) is reported as unavailable instead of failing every scan.
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
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(pctx, w.Argv[0], w.Argv[1:]...).CombinedOutput(); err != nil { //nolint:gosec // the argv is built by Wrap from fixed tools
			s.probeErr = fmt.Errorf("%s is installed but cannot confine a process: %w (%s)", b, err, strings.TrimSpace(string(out)))
			return
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

// sandboxProfile denies the network and every write except the parameters
// W0..Wn (passed with -D, never spliced into the profile text) and a few device
// files every program opens.
func sandboxProfile(spec Spec, n int) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	if !spec.AllowNetwork {
		b.WriteString("(deny network*)\n")
	}
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
	out := []string{tool, "--die-with-parent", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc"}
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
