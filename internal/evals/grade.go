package evals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// GradeOptions configures local grading of assertions.
type GradeOptions struct {
	// AllowExec permits command_exit assertions to run their command. Without it
	// such an assertion fails with a message saying so: cases are authored files,
	// and running their commands is an explicit choice.
	AllowExec bool
	// CommandTimeout bounds one command_exit command. Default 60s.
	CommandTimeout time.Duration
	// Runner starts the command; nil runs a real process (runner.Exec).
	Runner runner.Runner
}

// OutcomeGrade is the verdict on a case's outcome checks (assertions and rubric).
type OutcomeGrade struct {
	// Graded is false when the case has no assertions and no rubric.
	Graded   bool
	Passed   bool
	Failures []string
}

// GradeOutcome evaluates the assertions and rubric of c against a result. A
// runner-supplied verdict (Result.Passed) wins over local grading.
func GradeOutcome(c *Case, r *Result, opts GradeOptions) OutcomeGrade {
	grade := OutcomeGrade{Graded: len(c.Assertions) > 0 || c.Rubric != ""}
	if !grade.Graded {
		grade.Passed = true
		return grade
	}
	if r.Passed != nil {
		grade.Passed = *r.Passed
		if !grade.Passed {
			grade.Failures = append(grade.Failures, "runner reported the outcome checks as failed")
		}
		return grade
	}
	grade.Passed = true
	for i := range c.Assertions {
		if msg := checkAssertion(&c.Assertions[i], r, opts); msg != "" {
			grade.Passed = false
			grade.Failures = append(grade.Failures, fmt.Sprintf("assertions[%d] %s: %s", i, c.Assertions[i].Type, msg))
		}
	}
	if c.Rubric != "" {
		pass := DefaultRubricMinScore
		if c.RubricMinScore != nil {
			pass = *c.RubricMinScore
		}
		switch {
		case r.RubricScore == nil:
			grade.Passed = false
			grade.Failures = append(grade.Failures, "rubric was not graded by the runner")
		case *r.RubricScore < pass:
			grade.Passed = false
			grade.Failures = append(grade.Failures, fmt.Sprintf("rubric score %.2f is below %.2f", *r.RubricScore, pass))
		}
	}
	return grade
}

// checkAssertion returns "" when the assertion holds, else why not.
func checkAssertion(a *Assertion, r *Result, opts GradeOptions) string {
	switch a.Type {
	case AssertFileExists:
		return checkFileExists(a, r.WorkDir)
	case AssertCommandExit:
		if !opts.AllowExec {
			return "command assertions are not run without --allow-exec"
		}
		return runCommandAssertion(a, r.WorkDir, opts)
	case AssertContains, AssertNotContains, AssertRegex:
		return checkText(a, r)
	}
	return "unknown assertion type " + a.Type
}

func checkFileExists(a *Assertion, workDir string) string {
	full, ok := inWorkDir(workDir, a.Path)
	if !ok {
		return "no usable work_dir for " + a.Path
	}
	_, err := os.Stat(full)
	wantExists := a.Exists == nil || *a.Exists
	switch {
	case (err == nil) == wantExists:
		return ""
	case wantExists:
		return a.Path + " does not exist"
	}
	return a.Path + " exists"
}

// checkText evaluates contains, not_contains and regex against the answer or a file.
func checkText(a *Assertion, r *Result) string {
	subject := r.Output
	if a.Path != "" {
		full, ok := inWorkDir(r.WorkDir, a.Path)
		if !ok {
			return "no usable work_dir for " + a.Path
		}
		if info, err := os.Stat(full); err != nil || !info.Mode().IsRegular() {
			return "cannot read " + a.Path
		}
		data, err := readBounded(full)
		if err != nil {
			return "cannot read " + a.Path
		}
		subject = string(data)
	}
	switch a.Type {
	case AssertContains:
		if !strings.Contains(subject, a.Value) {
			return fmt.Sprintf("%s does not contain %q", subjectName(a), a.Value)
		}
	case AssertNotContains:
		if strings.Contains(subject, a.Value) {
			return fmt.Sprintf("%s contains %q", subjectName(a), a.Value)
		}
	default:
		re, err := regexp.Compile(a.Value)
		if err != nil {
			return "invalid regular expression"
		}
		if !re.MatchString(subject) {
			return fmt.Sprintf("%s does not match /%s/", subjectName(a), a.Value)
		}
	}
	return ""
}

func subjectName(a *Assertion) string {
	if a.Path != "" {
		return a.Path
	}
	return "output"
}

// inWorkDir joins rel onto workDir and reports whether the result stays inside it
// once symlinks are resolved: the path itself when it exists, else its nearest
// existing ancestor. A symlink planted by the agent under test cannot point an
// assertion at a host file.
func inWorkDir(workDir, rel string) (string, bool) {
	if workDir == "" || checkRelPath(rel) != "" {
		return "", false
	}
	full := filepath.Join(workDir, filepath.FromSlash(rel))
	if !within(workDir, full) {
		return "", false
	}
	probe := full
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", false
		}
		probe = parent
	}
	return full, resolvedInside(workDir, probe)
}

func runCommandAssertion(a *Assertion, workDir string, opts GradeOptions) string {
	timeout := opts.CommandTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	argv := []string{"sh", "-c", a.Command}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", a.Command}
	}
	// The assertion's command is authored content gated by --allow-exec; it runs
	// with the caller's full environment, as it always did, in its own process tree.
	res := runner.Or(opts.Runner).Run(context.Background(), runner.Spec{
		Argv: argv, Dir: workDir, InheritEnv: true, Timeout: timeout,
	})
	want := 0
	if a.ExitCode != nil {
		want = *a.ExitCode
	}
	switch res.Status {
	case runner.StatusOK, runner.StatusExit:
		if res.ExitCode != want {
			return fmt.Sprintf("exit status %d, want %d", res.ExitCode, want)
		}
		return ""
	default:
		return fmt.Sprintf("command did not run: %v", res.Err)
	}
}
