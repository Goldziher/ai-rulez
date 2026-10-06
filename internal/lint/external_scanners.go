package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
	procrunner "github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
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
func (s ScannerInfo) NeedsAllow() bool { return s.Egress == "true" }

// Healthy reports whether the scanner can run as configured: a valid entry,
// an installed binary, and no network flag on an egress = false entry.
func (s ScannerInfo) Healthy() bool {
	return len(s.Problems) == 0 && s.EgressFlag == "" && s.Found()
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
	for _, sc := range resolveScanners(cfg.Lint, cfg.Plugin != nil || cfg.Marketplace != nil) {
		info := ScannerInfo{
			Name: sc.Name, Command: sc.Command[0], Egress: "undeclared", Format: sc.Format, Inputs: sc.Inputs,
			EnvPass: sc.EnvPass, Problems: sc.allProblems(), Profile: sc.Profile, Presets: sc.Presets,
			FromPreset: sc.FromPreset, Required: sc.Required, Version: sc.Version, DataSent: sc.DataSent,
			Isolation: string(pol.isolation), Backend: string(scannerSandbox.Backend()),
		}
		if info.Format == "" {
			info.Format = "sarif"
		}
		if sc.Egress != nil {
			info.Egress = "false"
			if *sc.Egress {
				info.Egress = "true"
			} else {
				info.EgressFlag = sc.egressFlag()
			}
		}
		var timeout time.Duration
		if sc.Timeout != "" {
			timeout, _ = time.ParseDuration(sc.Timeout) //nolint:errcheck // an invalid value is in Problems
		}
		info.Timeout = cmdrun.EffectiveTimeout(timeout)
		info.Path = lookExecutable(sc.Command[0], dir)
		out = append(out, info)
	}
	return out
}

// lookExecutable resolves an executable the way the runner does: a bare name
// on PATH (a binary found only through a relative PATH entry does not count),
// or a path relative to dir. It returns "" when there is none.
func lookExecutable(name, dir string) string {
	if !strings.ContainsAny(name, `/\`) {
		p, err := procrunner.LookPath(name)
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
	if err != nil || info.IsDir() || (info.Mode()&0o111 == 0 && runtime.GOOS != "windows") {
		return ""
	}
	return p
}

// ProbeScannerVersion asks the scanner for its version (`<command> --version`)
// through the hardened runner: scrubbed environment, a scratch working
// directory, a short timeout. The probe runs the scanner, so it follows the
// scanner's isolation mode: confined (no network, writes only in the scratch
// directory) whenever a backend works, unconfined only for "none" or for "auto"
// with no backend, and refused with an error for "require" with no backend.
// The version is "" when the scanner printed nothing usable.
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
		return "", fmt.Errorf("isolation = \"require\" but %w", err)
	}
	dir, err := os.MkdirTemp("", "ai-rulez-probe-")
	if err != nil {
		return "", nil
	}
	defer os.RemoveAll(dir) //nolint:errcheck // a temp directory
	argv := []string{info.Path, "--version"}
	if confine {
		w, werr := scannerSandbox.Wrap(sandbox.Spec{WriteDirs: []string{dir}}, argv)
		if werr != nil {
			return "", fmt.Errorf("could not apply isolation: %w", werr)
		}
		argv = w.Argv
	}
	res := cmdrun.Run(ctx, cmdrun.Spec{
		Argv: argv, Dir: dir, Timeout: 10 * time.Second, MaxOutput: 64 << 10,
		Env: cmdrun.ScrubEnv(cmdrun.HostEnv(), nil, []string{"HOME=" + dir, "TMPDIR=" + dir}),
	})
	if res.Status != cmdrun.StatusOK && res.Status != cmdrun.StatusExit {
		return "", nil
	}
	for _, text := range []string{string(res.Stdout), string(res.Stderr)} {
		for _, line := range strings.Split(text, "\n") {
			if line = sanitizeScannerText(line); line != "" {
				if len(line) > 80 {
					line = line[:80]
				}
				return line, nil
			}
		}
	}
	return "", nil
}
