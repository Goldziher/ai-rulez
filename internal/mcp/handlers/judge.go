package handlers

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/judge"
)

// JudgeSkill is a rubric skill a tool caller names: a skill of the project by name, a directory of
// the project, or Markdown given inline. All three are reference data (T2) to the reviewers: a tool
// caller's text is never configured instructions.
type JudgeSkill struct {
	Name           string
	Path           string
	InlineMarkdown string
}

// JudgeParams are the inputs of the judge tool; each is the flag of `ai-rulez judge` of the same name.
// The tool takes no model, provider, price, spend cap or consent switch of its own beyond AllowLLM:
// where content goes and what a run may spend are the user's, from the user config file.
type JudgeParams struct {
	WorkingDirectory string
	Paths            []string
	Categories       []string
	FailOn           []string
	MinConfidence    string
	Changed          string // the base revision of --changed; "HEAD" for uncommitted and untracked work
	Since            string
	Impact           *bool
	AttachRules      *bool
	AttachClaudeMD   *bool
	Skills           []JudgeSkill
	// SkillFor assigns skills to a category.
	SkillFor     map[string][]JudgeSkill
	ExplainDrops bool
	AllowPartial bool
	Estimate     bool
	Redact       bool
	AllowLLM     bool
}

// Judger runs `ai-rulez judge`. The CLI supplies the engine, so the tool and the command review with
// the same code. A run that cannot start (nothing to judge, no consent, a policy that forbids models)
// is an error.
type Judger func(ctx context.Context, params JudgeParams) (*judge.Report, error)

// JudgeWith returns the judge handler. A run that fails the gate, or that is incomplete, is an error
// result that still carries the report, as the command prints the report before it exits non-zero.
func JudgeWith(run Judger) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		if run == nil {
			return ToolError(errors.New("this server was built without the judge engine"))
		}
		params, err := judgeParams(request)
		if err != nil {
			return ToolError(err)
		}
		report, err := run(ctx, params)
		if err != nil {
			return ToolError(err)
		}
		failed := report.Mode == judge.ModeJudge && report.Summary.Decision != judge.DecisionPass
		return reportResult(report.Sanitized(), failed)
	}
}

// changedBase reads the changed argument: a revision, or true for the uncommitted work (HEAD).
func changedBase(r *ToolRequest) string {
	if on, isBool := r.GetArguments()["changed"].(bool); isBool {
		if on {
			return "HEAD"
		}
		return ""
	}
	return strings.TrimSpace(r.GetString("changed", ""))
}

func judgeParams(r *ToolRequest) (JudgeParams, error) {
	p := JudgeParams{
		WorkingDirectory: workingDir(r),
		Paths:            r.GetStringSlice("paths", nil),
		Categories:       r.GetStringSlice("categories", nil),
		FailOn:           r.GetStringSlice("fail_on", nil),
		MinConfidence:    r.GetString("min_confidence", ""),
		Changed:          changedBase(r),
		Since:            r.GetString("since", ""),
		ExplainDrops:     r.GetBool("explain_drops", false),
		AllowPartial:     r.GetBool("allow_partial", false),
		Estimate:         r.GetBool("estimate", false),
		Redact:           r.GetBool("redact", false),
		AllowLLM:         r.GetBool("allow_llm", false),
	}
	for key, dst := range map[string]**bool{"impact": &p.Impact, "attach_rules": &p.AttachRules, "attach_claude_md": &p.AttachClaudeMD} {
		if _, set := r.GetArguments()[key]; set {
			v := r.GetBool(key, false)
			*dst = &v
		}
	}
	var err error
	if p.Skills, err = judgeSkillList(r.GetArguments()["skills"]); err != nil {
		return p, err
	}
	if raw, ok := r.GetArguments()["skill_for"].(map[string]any); ok {
		p.SkillFor = map[string][]JudgeSkill{}
		for cat, v := range raw {
			skills, serr := judgeSkillList(v)
			if serr != nil {
				return p, serr
			}
			p.SkillFor[cat] = skills
		}
	}
	return p, nil
}

// stringField reads an optional string value; a missing or non-string value is empty.
func stringField(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

func judgeSkillList(v any) ([]JudgeSkill, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, nil
	}
	var out []JudgeSkill
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, errors.New("each skill is an object with name, path or inline_markdown")
		}
		s := JudgeSkill{Name: stringField(m, "name"), Path: stringField(m, "path"), InlineMarkdown: stringField(m, "inline_markdown")}
		switch {
		case s.InlineMarkdown != "" && s.Path != "":
			return nil, errors.New("a skill takes path or inline_markdown, not both")
		case s.Name == "" && s.Path == "" && s.InlineMarkdown == "":
			return nil, errors.New("a skill needs a name, a path or inline_markdown")
		case s.InlineMarkdown != "" && s.Name == "":
			return nil, errors.New("an inline skill needs a name")
		}
		out = append(out, s)
	}
	return out, nil
}
