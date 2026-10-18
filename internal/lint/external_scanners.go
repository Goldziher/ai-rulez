package lint

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
	"github.com/samber/oops"
)

// ScannerInfo describes one scanner (a [[lint.external]] entry or a member of
// the [lint.scanner_policy] preset) for `scanners list` and `scanners doctor`.
// InspectScanners fills it without running anything.
type ScannerInfo struct {
	Name    string
	Command string
	// Path is the resolved executable, "" when it is not installed.
	Path string
	// Egress is "true", "false" or "undeclared".
	Egress string
	Format string
	Inputs []string
	// Timeout is the effective timeout.
	Timeout time.Duration
	EnvPass []string
	// Problems are configuration errors (AR9E0); the scanner is not run.
	Problems []string
	// EgressFlag is the argument that makes an egress = false scanner reach the network (AR9E4).
	EgressFlag string
	// Profile is the embedded profile the entry uses ("" for none).
	Profile string
	// Presets lists the presets that contain the profile; FromPreset is set for a
	// scanner that exists only because of [lint.scanner_policy] preset.
	Presets    []string
	FromPreset bool
	// Required is set when a failure of this scanner is an error.
	Required bool
	// Version is the version range the entry requires ("" for none).
	Version string
	// VersionCommand is the entry's version_command (nil for `<command> --version`) and
	// Dir the project directory it and a relative command are anchored to.
	VersionCommand []string
	Dir            string
	// VersionTimeout bounds the version probe (0: DefaultVersionTimeout).
	VersionTimeout time.Duration
	// VersionLine is the version the probe printed and VersionProblem why the
	// version range check failed; doctor sets both after probing (--external).
	VersionLine    string
	VersionProblem string
	// VersionNote says why the probe printed no version when it timed out.
	VersionNote string
	// DataSent is what an egress scanner's vendor documents receiving.
	DataSent []string
	// Isolation is the [lint.scanner_policy] isolation mode and Backend the
	// confinement tool this system would use ("" when none).
	Isolation string
	Backend   string
}

// Found reports whether the executable was resolved.
func (s ScannerInfo) Found() bool { return s.Path != "" }

// NeedsAllow reports whether a run needs --allow-egress=<name>.
func (s ScannerInfo) NeedsAllow() bool { return s.Egress == boolTrue }

// Healthy reports whether the scanner can run as configured: a valid entry,
// an installed binary, and no network flag on an egress = false entry.
func (s ScannerInfo) Healthy() bool {
	return len(s.Problems) == 0 && s.EgressFlag == "" && s.Found() && s.VersionProblem == ""
}

// InspectScanners describes every named scanner of cfg: the preset's members,
// then the [[lint.external]] entries in config order. It looks the binary up but
// never starts it. dir anchors a relative command.
func InspectScanners(cfg *config.Config, dir string) []ScannerInfo {
	if cfg == nil || cfg.Lint == nil {
		return nil
	}
	pol := policyOf(cfg.Lint)
	var out []ScannerInfo
	resolved := resolveScanners(cfg.Lint, cfg.Plugin != nil || cfg.Marketplace != nil || cfg.HasPluginSet())
	for i := range resolved {
		sc := &resolved[i]
		info := ScannerInfo{
			Name: sc.Name, Command: sc.Command[0], Egress: "undeclared", Format: sc.Format, Inputs: sc.Inputs,
			EnvPass: sc.EnvPass, Problems: sc.allProblems(), Profile: sc.Profile, Presets: sc.Presets,
			FromPreset: sc.FromPreset, Required: sc.Required, Version: sc.Version, VersionCommand: sc.VersionCommand, Dir: dir, DataSent: sc.DataSent,
			Isolation: string(pol.isolation), Backend: string(scannerSandbox.Backend()),
		}
		if info.Format == "" {
			info.Format = FormatSARIF
		}
		if sc.Egress != nil {
			info.Egress = boolFalse
			if *sc.Egress {
				info.Egress = boolTrue
			} else {
				info.EgressFlag = sc.egressFlag()
			}
		}
		var timeout time.Duration
		if sc.Timeout != "" {
			timeout, _ = time.ParseDuration(sc.Timeout) //nolint:errcheck // an invalid value is in Problems
		}
		info.Timeout = cmdrun.EffectiveTimeoutUpTo(timeout, sc.timeoutCeiling)
		info.VersionTimeout = sc.versionTimeout()
		info.Path = lookExecutable(sc.Command[0], dir)
		out = append(out, info)
	}
	return out
}

// versionArgv is what the version probe runs and where: `<binary> --version` in
// the scratch directory, or the entry's version_command from the project directory.
func versionArgv(info ScannerInfo, scratch string) (argv []string, dir string, err error) {
	if len(info.VersionCommand) == 0 {
		return []string{info.Path, "--version"}, scratch, nil
	}
	exe := lookExecutable(info.VersionCommand[0], info.Dir)
	if exe == "" {
		return nil, "", oops.Errorf("version_command %q was not found", info.VersionCommand[0])
	}
	dir = scratch
	if info.Dir != "" {
		dir = info.Dir
	}
	return append([]string{exe}, info.VersionCommand[1:]...), dir, nil
}

// lookExecutable resolves an executable the way the runner does: a bare name
// on PATH (a binary found only through a relative PATH entry does not count),
// or a path relative to dir. It returns "" when there is none.
func lookExecutable(name, dir string) string {
	if !strings.ContainsAny(name, `/\`) {
		p, err := cmdrun.LookPath(name)
		if err != nil { // includes exec.ErrDot: found only through a relative PATH entry
			return ""
		}
		return p
	}
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() || (info.Mode()&0o111 == 0 && runtime.GOOS != osWindows) {
		return ""
	}
	return p
}

// DefaultVersionTimeout bounds a version probe when version_timeout is not set.
const DefaultVersionTimeout = 10 * time.Second

// ProbeTimeoutError is returned by ProbeScannerVersion when the version command
// was still running at its timeout, so the caller can tell a slow probe from one
// that printed nothing.
type ProbeTimeoutError struct{ After time.Duration }

// Error implements error.
func (e *ProbeTimeoutError) Error() string {
	return "version probe timed out after " + e.After.String()
}

// ProbeScannerVersion asks the scanner for its version (`<command> --version`)
// through the hardened runner: scrubbed environment, a scratch working
// directory, a short timeout. The probe runs the scanner, so it follows the
// scanner's isolation mode: confined (no network, writes only in the scratch
// directory) whenever a backend works, unconfined only for "none" or for "auto"
// with no backend, and refused with an error for "require" with no backend.
// The version is "" when the scanner printed nothing usable; a probe that outlives
// info.VersionTimeout (default DefaultVersionTimeout) returns a *ProbeTimeoutError.
func ProbeScannerVersion(ctx context.Context, info ScannerInfo) (string, error) {
	if !info.Found() {
		return "", nil
	}
	mode, err := sandbox.ParseMode(info.Isolation)
	if err != nil {
		mode = sandbox.ModeAuto
	}
	confine, err := scannerSandbox.Resolve(ctx, mode)
	if err != nil {
		return "", oops.Errorf("isolation = \"require\" but %w", err)
	}
	dir, err := os.MkdirTemp("", "ai-rulez-probe-")
	if err != nil {
		return "", nil
	}
	defer os.RemoveAll(dir) //nolint:errcheck // a temp directory
	argv, probeDir, err := versionArgv(info, dir)
	if err != nil {
		return "", err
	}
	if confine {
		w, werr := scannerSandbox.Wrap(sandbox.Spec{WriteDirs: []string{dir}}, argv)
		if werr != nil {
			return "", oops.Wrapf(werr, "could not apply isolation")
		}
		argv = w.Argv
	}
	limit := info.VersionTimeout
	if limit <= 0 {
		limit = DefaultVersionTimeout
	}
	res := cmdrun.Run(ctx, cmdrun.Spec{
		Argv: argv, Dir: probeDir, Timeout: limit, MaxOutput: 64 << 10,
		Env: cmdrun.ScrubEnv(cmdrun.HostEnv(), nil, []string{"HOME=" + dir, "TMPDIR=" + dir}),
	})
	if res.Status == cmdrun.StatusTimeout {
		return "", &ProbeTimeoutError{After: limit}
	}
	if res.Status != cmdrun.StatusOK && res.Status != cmdrun.StatusExit {
		return "", nil
	}
	return firstVersionLine(string(res.Stdout), string(res.Stderr)), nil
}

// firstVersionLine is the first non-empty sanitized line of the probe output,
// stdout first, cut to 80 bytes; "" when there is none.
func firstVersionLine(texts ...string) string {
	for _, text := range texts {
		for _, line := range strings.Split(text, "\n") {
			if line = sanitizeScannerText(line); line != "" {
				if len(line) > 80 {
					line = line[:80]
				}
				return line
			}
		}
	}
	return ""
}
