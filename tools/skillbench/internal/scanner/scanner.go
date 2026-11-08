// Package scanner runs each tool as a black box over one sample and reduces its output to a verdict.
// Only rule ids and severities survive: raw output can quote the sample, so it is never kept.
package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Severity levels, lowest first. The empty string means no finding.
var severities = []string{"", "info", "low", "medium", "high", "critical"}

// SeverityRank orders a normalized severity.
func SeverityRank(s string) int { return slices.Index(severities, s) }

// Verdict is one tool's answer for one sample.
type Verdict struct {
	Flagged     bool     `json:"flagged"`
	MaxSeverity string   `json:"max_severity"`
	Rules       []string `json:"rules"`
	Findings    int      `json:"findings"`
	// Error is a short class, never tool output: an error counts as no detection and is reported.
	Error string `json:"error,omitempty"`
}

// Tool adapts one scanner.
type Tool struct {
	Name string
	// Bin is the executable.
	Bin string
	// Timeout bounds one sample.
	Timeout time.Duration
	// scan runs the tool on a skill directory and parses the result.
	scan func(ctx context.Context, t *Tool, dir, work string) (Verdict, error)
	// versionArgs prints the version.
	versionArgs []string
	// supported, when set, reports whether the installed release has what the adapter calls.
	supported func(ctx context.Context, t *Tool) bool
}

var versionRE = regexp.MustCompile(`\d+\.\d+\.\d+\S*`)

// Version asks the tool for its version.
func (t *Tool) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, t.Bin, t.versionArgs...).CombinedOutput() //nolint:gosec // pinned tool
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", t.Name, err)
	}
	v := versionRE.Find(out)
	if v == nil {
		return "", fmt.Errorf("%s --version: no version in its output", t.Name)
	}
	return string(v), nil
}

// Scan runs the tool on one sample directory. A failure becomes a Verdict with Error set, so the
// denominators never change.
func (t *Tool) Scan(ctx context.Context, dir, work string) Verdict {
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	v, err := t.scan(ctx, t, dir, work)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Verdict{Error: "timeout"}
	case err != nil:
		return Verdict{Error: errorClass(err)}
	}
	slices.Sort(v.Rules)
	v.Rules = slices.Compact(v.Rules)
	if v.Rules == nil {
		v.Rules = []string{}
	}
	return v
}

type classedError string

func (e classedError) Error() string { return string(e) }

func errorClass(err error) string {
	var c classedError
	if errors.As(err, &c) {
		return string(c)
	}
	return "failed"
}

// command runs the tool with a clean environment: no AI_RULEZ_* setting, no model credential.
func command(ctx context.Context, t *Tool, dir, work string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.Bin, args...) //nolint:gosec // pinned tool, arguments built here
	cmd.Dir = dir
	cmd.Env = []string{
		"HOME=" + work, "PATH=" + os.Getenv("PATH"), "TMPDIR=" + work, "LANG=C.UTF-8",
		"LITELLM_LOCAL_MODEL_COST_MAP=True", "NO_COLOR=1", "PYTHONDONTWRITEBYTECODE=1",
	}
	return cmd
}

// reportPath clears the report location first, so a crash cannot leave the previous sample's report
// for this one to read.
func reportPath(work, name string) (string, error) {
	path := filepath.Join(work, name)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", classedError("work-directory")
	}
	return path, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // written by the tool in our work directory
	if err != nil {
		return classedError("no-report")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return classedError("unparseable")
	}
	return nil
}

// exitCode separates a tool's exit status, which may mean "found something", from a failure to run.
func exitCode(err error) (int, error) {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		return exit.ExitCode(), nil
	}
	return -1, err
}

// normalize maps a tool's severity onto the shared scale.
func normalize(s string) string {
	switch s = strings.ToLower(s); s {
	case "warning":
		return "medium"
	case "error":
		return "high"
	case "none":
		return ""
	}
	if SeverityRank(s) < 0 {
		return "low"
	}
	return s
}

func maxSeverity(a, b string) string {
	if SeverityRank(b) > SeverityRank(a) {
		return b
	}
	return a
}
