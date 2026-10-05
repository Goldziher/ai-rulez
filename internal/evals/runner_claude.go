package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// claudeTriggerGrader is the name of the generated grader that observes whether
// the skill fired.
const claudeTriggerGrader = "trigger"

// keyType is the frontmatter key that selects a grader type.
const keyType = "type"

// ClaudePluginEval adapts `claude plugin eval` (Claude Code 2.1.x): it writes the
// skill into a throwaway plugin, translates each case into a prompt.md with
// graders/*.md, runs the tool with its with/without-plugin ablation, and reads the
// JSON it prints.
//
// The adapter speaks the directory case format and the `--json` result shape that
// Claude Code 2.1.289 documents in its help and interview prompt. The result shape
// was not checked against a live run: an unrecognized document is an error, never a
// silent pass.
type ClaudePluginEval struct {
	// Bin is the claude executable. Default "claude".
	Bin string
	// Runs overrides the per-case run count (claude's default is 3). Zero keeps it.
	Runs int
	// JudgeModel overrides claude's grader model.
	JudgeModel string
	// ExtraArgs are appended to the claude command line (for example --trust-plugin,
	// which ai-rulez never adds on its own).
	ExtraArgs []string
	// KeepDir, when set, is where the throwaway plugin is built and kept.
	KeepDir string
	Stderr  io.Writer
	// Exec runs the tool; tests replace it. Nil uses os/exec.
	Exec func(ctx context.Context, bin string, args []string, stdout, stderr io.Writer) error
}

// Name implements Runner.
func (*ClaudePluginEval) Name() string { return RunnerClaudePluginEval }

// Fingerprint implements Fingerprinter: the settings that change what claude runs.
func (r *ClaudePluginEval) Fingerprint() string {
	return fmt.Sprintf("bin=%s runs=%d judge=%s args=%q", r.Bin, r.Runs, r.JudgeModel, r.ExtraArgs)
}

// Run implements Runner.
func (r *ClaudePluginEval) Run(ctx context.Context, req *Request) (*Response, error) {
	if req.Harness != "" && req.Harness != "claude" {
		return nil, fmt.Errorf("the %s runner only drives the claude harness, not %q", RunnerClaudePluginEval, req.Harness)
	}
	dir := r.KeepDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "ai-rulez-eval-*")
		if err != nil {
			return nil, fmt.Errorf("create temp plugin: %w", err)
		}
		defer os.RemoveAll(tmp) //nolint:errcheck // throwaway directory
		dir = tmp
	}
	translated, err := BuildClaudePlugin(dir, req)
	if err != nil {
		return nil, err
	}
	resultFile := filepath.Join(dir, "aggregate.json")
	args := r.args(dir, resultFile, req)

	bin := r.Bin
	if bin == "" {
		bin = "claude"
	}
	run := r.Exec
	if run == nil {
		run = execCommand
	}
	var stdout bytes.Buffer
	runErr := run(ctx, bin, args, &stdout, r.Stderr)
	data, readErr := os.ReadFile(resultFile) //nolint:gosec // the file this run was told to write
	if readErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("claude plugin eval failed: %w", runErr)
		}
		return nil, fmt.Errorf("claude plugin eval wrote no result file: %w", readErr)
	}
	resp, err := ParseClaudeResult(data, req, translated)
	if err != nil {
		return nil, err
	}
	// A non-zero exit with a parsed result is a partial run (cost ceiling, abort):
	// the cases missing from the result score as errors.
	return resp, nil
}

// args builds the `claude` command line for a throwaway plugin.
func (r *ClaudePluginEval) args(dir, resultFile string, req *Request) []string {
	args := []string{"plugin", "eval", dir, "--json", resultFile, "--no-publish", "--threshold", "0",
		"--output-dir", filepath.Join(dir, "results")}
	if req.Ablation {
		args = append(args, "--ablation", "with-without")
	} else {
		args = append(args, "--ablation", "none")
	}
	if r.Runs > 0 {
		args = append(args, "--runs", strconv.Itoa(r.Runs))
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if r.JudgeModel != "" {
		args = append(args, "--judge-model", r.JudgeModel)
	}
	if req.MaxCostUSD > 0 {
		args = append(args, "--max-cost-usd", strconv.FormatFloat(req.MaxCostUSD, 'f', 4, 64))
	}
	return append(args, r.ExtraArgs...)
}

func execCommand(ctx context.Context, bin string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // the user chose the runner binary
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

// ClaudeTranslation records how the cases were translated.
type ClaudeTranslation struct {
	// Dirs maps a case id to its directory name in the throwaway plugin.
	Dirs map[string]string
	// Skipped maps a case id to why it cannot run through this adapter.
	Skipped map[string]string
	// Inverted maps a case id to whether its trigger grader is inverted (the case
	// expects no trigger, so the grader passes when the skill stays quiet).
	Inverted map[string]bool
}

// BuildClaudePlugin writes the plugin dir: manifest, the skill, and one eval case
// directory per translatable case.
func BuildClaudePlugin(dir string, req *Request) (*ClaudeTranslation, error) {
	tr := &ClaudeTranslation{Dirs: map[string]string{}, Skipped: map[string]string{}, Inverted: map[string]bool{}}
	manifest := fmt.Sprintf("{\n  \"name\": \"ai-rulez-eval-%s\",\n  \"version\": \"0.0.0\",\n  \"description\": \"Throwaway plugin built by ai-rulez eval run\"\n}\n", req.Skill.ID)
	if err := writeFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return nil, err
	}
	if err := copyTree(req.Skill.Dir, filepath.Join(dir, "skills", req.Skill.ID), "evals"); err != nil {
		return nil, err
	}
	for i := range req.Cases {
		c := &req.Cases[i]
		if reason := claudeUnsupported(c); reason != "" {
			tr.Skipped[c.ID] = reason
			continue
		}
		caseDir := filepath.Join(dir, "evals", c.ID)
		tr.Dirs[c.ID] = c.ID
		tr.Inverted[c.ID] = !c.Expects()
		front := map[string]any{
			"max_turns":     10,
			"allowed_tools": []string{"Skill", "Read", "Glob", "Grep"},
		}
		if c.Model != "" {
			front["model"] = c.Model
		}
		if len(c.Tags) > 0 {
			front["tags"] = c.Tags
		}
		if err := writeMarkdown(filepath.Join(caseDir, "prompt.md"), front, c.Prompt); err != nil {
			return nil, err
		}
		trigger := map[string]any{keyType: "tool_used", "tool": "Skill", "input_match": triggerMatch(req.Skill.ID)}
		if !c.Expects() {
			trigger["min"], trigger["max"], trigger["arm"] = 0, 0, "both"
		}
		if err := writeMarkdown(filepath.Join(caseDir, "graders", claudeTriggerGrader+".md"), trigger, ""); err != nil {
			return nil, err
		}
		for n := range c.Assertions {
			front, body := claudeGrader(&c.Assertions[n])
			if err := writeMarkdown(filepath.Join(caseDir, "graders", fmt.Sprintf("assert-%02d.md", n+1)), front, body); err != nil {
				return nil, err
			}
		}
		if c.Rubric != "" {
			if err := writeMarkdown(filepath.Join(caseDir, "graders", "rubric.md"), map[string]any{keyType: "llm"}, c.Rubric); err != nil {
				return nil, err
			}
		}
	}
	return tr, nil
}

// triggerMatch anchors the skill id so a call to test-driven-development or
// plugin:contest does not count as a call to the skill "test". A plugin prefix
// ("plugin:id") still matches, since ":" is a boundary.
func triggerMatch(id string) string {
	const boundary = `[^A-Za-z0-9._-]`
	return "(^|" + boundary + ")" + regexp.QuoteMeta(id) + "($|" + boundary + ")"
}

// claudeUnsupported says why a case cannot be translated, or "".
func claudeUnsupported(c *Case) string {
	if len(c.Files) > 0 {
		return "file fixtures are not supported by the claude-plugin-eval adapter"
	}
	for i := range c.Assertions {
		if c.Assertions[i].Type == AssertCommandExit {
			return "command_exit assertions are not supported by the claude-plugin-eval adapter"
		}
	}
	return ""
}

// claudeGrader maps an assertion to a grader file (frontmatter and body).
func claudeGrader(a *Assertion) (front map[string]any, body string) {
	if a.Type == AssertFileExists {
		return map[string]any{keyType: "file_exists", "path": a.Path, "exists": a.Exists == nil || *a.Exists}, ""
	}
	front = map[string]any{keyType: "regex", "match": "contains"}
	if a.Path != "" {
		front["target"] = map[string]any{"source": "file", "path": a.Path}
	}
	body = a.Value
	switch a.Type {
	case AssertContains:
		body = strings.ReplaceAll(regexp.QuoteMeta(a.Value), "\n", `\n`)
	case AssertNotContains:
		body = strings.ReplaceAll(regexp.QuoteMeta(a.Value), "\n", `\n`)
		front["match"] = "not_contains"
	}
	return front, body
}

func writeMarkdown(file string, front map[string]any, body string) error {
	head, err := yaml.Marshal(front)
	if err != nil {
		return fmt.Errorf("encode frontmatter: %w", err)
	}
	return writeFile(file, "---\n"+string(head)+"---\n\n"+body+"\n")
}

func writeFile(file, content string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(file), err)
	}
	return os.WriteFile(file, []byte(content), 0o600)
}

// copyTree copies regular files from src to dst, skipping a top-level directory.
func copyTree(src, dst, skip string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel == skip {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // walking the skill's own tree
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm()|0o600)
	})
}

// claudeAggregate is the subset of `claude plugin eval --json` the adapter reads.
type claudeAggregate struct {
	CostUSD float64 `json:"costUsd"`
	Cases   []struct {
		Name string `json:"name"`
		Dir  string `json:"dir"`
		Arms struct {
			With    []claudeRun `json:"with"`
			Without []claudeRun `json:"without"`
		} `json:"arms"`
	} `json:"cases"`
}

type claudeRun struct {
	Score        float64 `json:"score"`
	CostUSD      float64 `json:"costUsd"`
	JudgeCostUSD float64 `json:"judgeCostUsd"`
	Error        *string `json:"error"`
	Graders      []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
	} `json:"graders"`
}

// ParseClaudeResult converts the tool's JSON into a Response.
func ParseClaudeResult(data []byte, req *Request, tr *ClaudeTranslation) (*Response, error) {
	var agg claudeAggregate
	if err := json.Unmarshal(data, &agg); err != nil {
		return nil, fmt.Errorf("parse claude plugin eval result: %w", err)
	}
	if agg.Cases == nil {
		return nil, fmt.Errorf("claude plugin eval result has no \"cases\" (unrecognized format)")
	}
	byName := map[string]int{}
	for i, c := range agg.Cases {
		byName[c.Name] = i
		if c.Dir != "" {
			byName[filepath.Base(c.Dir)] = i
		}
	}
	resp := &Response{Version: ProtocolVersion, CostUSD: agg.CostUSD}
	for i := range req.Cases {
		c := &req.Cases[i]
		if reason, skipped := tr.Skipped[c.ID]; skipped {
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Skipped: true, Reason: reason})
			continue
		}
		idx, ok := byName[tr.Dirs[c.ID]]
		if !ok {
			continue // scored as "no result"
		}
		entry := agg.Cases[idx]
		resp.Results = append(resp.Results, claudeArm(c, ArmWith, entry.Arms.With, tr.Inverted[c.ID]))
		if req.Ablation && len(entry.Arms.Without) > 0 {
			resp.Results = append(resp.Results, claudeArm(c, ArmWithout, entry.Arms.Without, false))
		}
	}
	if err := resp.Validate(req); err != nil {
		return nil, fmt.Errorf("claude plugin eval result: %w", err)
	}
	return resp, nil
}

// claudeArm collapses the runs of one arm by majority vote.
func claudeArm(c *Case, arm string, runs []claudeRun, inverted bool) Result {
	res := Result{Case: c.ID, Arm: arm}
	var usable, triggered, outcomePass int
	var cost float64
	var lastErr string
	graded := false
	for i := range runs {
		run := &runs[i]
		cost += run.CostUSD + run.JudgeCostUSD
		if run.Error != nil && *run.Error != "" {
			lastErr = *run.Error
			continue
		}
		usable++
		pass := true
		fired := false
		for _, g := range run.Graders {
			if g.Name == claudeTriggerGrader {
				fired = g.Passed != inverted
				continue
			}
			graded = true
			pass = pass && g.Passed
		}
		if fired {
			triggered++
		}
		if pass {
			outcomePass++
		}
	}
	res.CostUSD = cost
	if usable == 0 {
		res.Error = lastErr
		if res.Error == "" {
			res.Error = "claude plugin eval returned no runs for this case"
		}
		return res
	}
	fired := triggered*2 > usable
	res.Triggered = &fired
	if graded {
		passed := outcomePass*2 > usable
		res.Passed = &passed
	}
	return res
}
