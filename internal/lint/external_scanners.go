package lint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// ScannerInfo describes one [[lint.external]] entry for `scanners list` and
// `scanners doctor`. InspectScanners fills it without running anything.
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

// InspectScanners describes every named [[lint.external]] entry in config order.
// It looks the binary up but never starts it. dir anchors a relative command.
func InspectScanners(lc *config.LintConfig, dir string) []ScannerInfo {
	if lc == nil {
		return nil
	}
	var out []ScannerInfo
	for _, ex := range lc.External {
		if strings.TrimSpace(ex.Name) == "" || len(ex.Command) == 0 {
			continue
		}
		info := ScannerInfo{
			Name: ex.Name, Command: ex.Command[0], Egress: "undeclared", Format: ex.Format, Inputs: ex.Inputs,
			EnvPass: ex.EnvPass, Problems: externalProblems(ex),
		}
		if info.Format == "" {
			info.Format = "sarif"
		}
		if ex.Egress != nil {
			info.Egress = "false"
			if *ex.Egress {
				info.Egress = "true"
			} else {
				info.EgressFlag = egressFlagViolation(ex.Command)
			}
		}
		var timeout time.Duration
		if ex.Timeout != "" {
			timeout, _ = time.ParseDuration(ex.Timeout) //nolint:errcheck // an invalid value is in Problems
		}
		info.Timeout = cmdrun.EffectiveTimeout(timeout)
		info.Path = lookExecutable(ex.Command[0], dir)
		out = append(out, info)
	}
	return out
}

// lookExecutable resolves an executable the way the runner does: a bare name
// on PATH (a binary found only through a relative PATH entry does not count),
// or a path relative to dir. It returns "" when there is none.
func lookExecutable(name, dir string) string {
	if !strings.ContainsAny(name, `/\`) {
		p, err := exec.LookPath(name)
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
// directory, a short timeout. It returns "" when the scanner printed nothing usable.
func ProbeScannerVersion(ctx context.Context, info ScannerInfo) string {
	if !info.Found() {
		return ""
	}
	dir, err := os.MkdirTemp("", "ai-rulez-probe-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir) //nolint:errcheck // a temp directory
	res := cmdrun.Run(ctx, cmdrun.Spec{
		Argv: []string{info.Path, "--version"}, Dir: dir, Timeout: 10 * time.Second, MaxOutput: 64 << 10,
		Env: cmdrun.ScrubEnv(cmdrun.HostEnv(), nil, []string{"HOME=" + dir, "TMPDIR=" + dir}),
	})
	if res.Status != cmdrun.StatusOK && res.Status != cmdrun.StatusExit {
		return ""
	}
	for _, text := range []string{string(res.Stdout), string(res.Stderr)} {
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
