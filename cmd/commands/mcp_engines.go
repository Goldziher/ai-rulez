package commands

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

// engineOptions wire the engines of the command line into the authoring MCP
// server, so a tool and its command run the same code: the lint of `validate`
// behind validate_config and scan_content, and the policy discovery of
// `validate --show-policy` behind policy_show.
func engineOptions() []mcp.Option {
	return []mcp.Option{
		mcp.WithValidator((&mcpValidator{}).validate),
		mcp.WithJudger(judgeForTool),
		mcp.WithPolicyViewer(func(_ context.Context, _ string, cfg *config.Config) (policy.Report, error) {
			return policyReportFor(cfg)
		}),
	}
}

// NewAuthoringMCPServer builds the authoring MCP server `ai-rulez mcp` runs,
// with the command line's engines. opts follow the engines, so they can narrow
// the directories the tools may touch.
func NewAuthoringMCPServer(opts ...mcp.Option) *mcp.Server {
	return mcp.NewServer(Version, append(engineOptions(), opts...)...)
}

// judgeForTool runs `ai-rulez judge` for the judge tool. It builds the command's flags (so defaults
// and "set explicitly" mean what they mean on the command line), fills only what the tool may set,
// and refuses a real run unless the user has agreed in their own config as well.
func judgeForTool(ctx context.Context, p handlers.JudgeParams) (*judge.Report, error) {
	cmd := &cobra.Command{Use: "judge"}
	f := registerJudgeFlags(cmd)
	if err := setToolFlags(cmd, p); err != nil {
		return nil, err
	}
	f.explainDrops, f.allowPartial, f.estimate, f.redact, f.allowLLM = p.ExplainDrops, p.AllowPartial, p.Estimate, p.Redact, p.AllowLLM
	f.format = formatJSON
	f.forceReference = true
	in := judgeInput{flags: f, changed: cmd.Flags().Changed, args: p.Paths, fromTool: true}
	var err error
	if in.cwd, err = filepath.Abs(p.WorkingDirectory); err != nil {
		return nil, oops.Wrapf(err, "resolve the working directory")
	}
	if err := in.addToolSkills(f, p); err != nil {
		return nil, err
	}
	out, err := defaultJudgeDeps().execute(ctx, in)
	if err != nil {
		return nil, err
	}
	return out.report, nil
}

// setToolFlags applies the tool parameters the tool may set to the command's flags.
func setToolFlags(cmd *cobra.Command, p handlers.JudgeParams) error {
	set := func(name string, values ...string) error {
		for _, v := range values {
			if err := cmd.Flags().Set(name, v); err != nil {
				return oops.Wrapf(err, "judge %s", name)
			}
		}
		return nil
	}
	flags := []struct {
		name   string
		values []string
	}{
		{"categories", p.Categories}, {"fail-on", p.FailOn},
	}
	for _, fl := range flags {
		if err := set(fl.name, fl.values...); err != nil {
			return err
		}
	}
	scalars := map[string]string{"min-confidence": p.MinConfidence, "since": p.Since}
	for _, name := range []string{"min-confidence", "since"} {
		if scalars[name] != "" {
			if err := set(name, scalars[name]); err != nil {
				return err
			}
		}
	}
	if p.Changed != "" {
		if err := set("changed", p.Changed); err != nil {
			return err
		}
	}
	bools := map[string]*bool{"impact": p.Impact, "attach-rules": p.AttachRules, "attach-claude-md": p.AttachClaudeMD}
	for name, v := range bools {
		if v != nil {
			if err := set(name, strconv.FormatBool(*v)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (in *judgeInput) addToolSkills(f *judgeFlags, p handlers.JudgeParams) error {
	for _, sk := range p.Skills {
		in.addToolSkill(f, sk, judge.CategoryGovernance)
	}
	for _, cat := range sortedKeysOf(p.SkillFor) {
		c, ok := judge.ParseCategory(cat)
		if !ok {
			return oops.Errorf("skill_for: %q is not security, quality or governance", cat)
		}
		for _, sk := range p.SkillFor[cat] {
			in.addToolSkill(f, sk, c)
		}
	}
	return nil
}

// addToolSkill routes a skill of a tool call: inline Markdown becomes an inline rubric, a name or a
// directory becomes a --skill / --skill-for reference.
func (in *judgeInput) addToolSkill(f *judgeFlags, sk handlers.JudgeSkill, cat judge.Category) {
	switch {
	case sk.InlineMarkdown != "":
		in.inline = append(in.inline, judgeInlineSkill{name: sk.Name, markdown: sk.InlineMarkdown, cats: []judge.Category{cat}})
	default:
		ref := sk.Name
		if sk.Path != "" {
			ref = "path:" + sk.Path
		}
		f.skillFor = append(f.skillFor, string(cat)+"="+ref)
	}
}
