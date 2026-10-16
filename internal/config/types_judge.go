package config

import (
	"fmt"
	"math"
	"path"
	"strings"
	"time"
)

// The prompt modes of [judge.prompts.<category>].
const (
	JudgePromptExtend   = "extend"
	JudgePromptOverride = "override"
)

// JudgeConfig is the [judge] table: what `ai-rulez judge` reviews and how it gates. The keys of the
// first group are honored from the repository config; the second group names where a run sends
// content and what it may spend, so only the user config file, the environment and flags can set
// it. A repository value there is ignored and reported (see ResolveJudge).
type JudgeConfig struct {
	// Categories limits the review to security, quality and governance (default all).
	Categories []string `yaml:"categories,omitempty" json:"categories,omitempty" toml:"categories,omitempty"`
	// FailOn lists "<category>:<severity>", "<category>:none", "score:<n>" or "<category>:score:<n>"; the
	// default is security:high, governance:high, quality:none.
	FailOn []string `yaml:"fail_on,omitempty" json:"fail_on,omitempty" toml:"fail_on,omitempty"` //nolint:tagliatelle
	// MinConfidence drops findings below it: low (default), medium or high.
	MinConfidence string `yaml:"min_confidence,omitempty" json:"min_confidence,omitempty" toml:"min_confidence,omitempty"` //nolint:tagliatelle
	// AttachRules attaches the path-scoped rules that govern a unit to its governance review (default true).
	AttachRules *bool `yaml:"attach_rules,omitempty" json:"attach_rules,omitempty" toml:"attach_rules,omitempty"` //nolint:tagliatelle
	// AttachClaudeMD attaches the nested CLAUDE.md and AGENTS.md files that apply to a unit (default true).
	AttachClaudeMD *bool `yaml:"attach_claude_md,omitempty" json:"attach_claude_md,omitempty" toml:"attach_claude_md,omitempty"` //nolint:tagliatelle
	// Skills names the rubric skills a run applies.
	Skills *JudgeSkills `yaml:"skills,omitempty" json:"skills,omitempty" toml:"skills,omitempty"`
	// Prompts extends or replaces a category's role prompt, by category name.
	Prompts map[string]JudgePrompt `yaml:"prompts,omitempty" json:"prompts,omitempty" toml:"prompts,omitempty"`
	// Suppress silences known findings, pinned to their quote and expiring.
	Suppress []JudgeSuppression `yaml:"suppress,omitempty" json:"suppress,omitempty" toml:"suppress,omitempty"`
	// TrapsFile is a YAML list of repository false-positive shapes for the adjudicator (default .ai-rulez/judge/traps.yaml).
	TrapsFile string `yaml:"traps_file,omitempty" json:"traps_file,omitempty" toml:"traps_file,omitempty"` //nolint:tagliatelle

	// User scope only from here on.

	// Models names the model of each role (security, quality, governance, adjudicator, or reviewers for the first three).
	Models map[string]string `yaml:"models,omitempty" json:"models,omitempty" toml:"models,omitempty"`
	// MaxCostUSD, MaxTokens and MaxCalls are the spend caps of a run (0 = the command default / none).
	MaxCostUSD float64 `yaml:"max_cost_usd,omitempty" json:"max_cost_usd,omitempty" toml:"max_cost_usd,omitempty"` //nolint:tagliatelle
	MaxTokens  int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty" toml:"max_tokens,omitempty"`       //nolint:tagliatelle
	MaxCalls   int     `yaml:"max_calls,omitempty" json:"max_calls,omitempty" toml:"max_calls,omitempty"`          //nolint:tagliatelle
	// Concurrency and RPM bound the calls in flight and started per minute.
	Concurrency int `yaml:"concurrency,omitempty" json:"concurrency,omitempty" toml:"concurrency,omitempty"`
	RPM         int `yaml:"rpm,omitempty" json:"rpm,omitempty" toml:"rpm,omitempty"`
	// AllowLLM agrees to send content to the model's provider without --allow-llm.
	AllowLLM bool `yaml:"allow_llm,omitempty" json:"allow_llm,omitempty" toml:"allow_llm,omitempty"` //nolint:tagliatelle
	// AllowPromptOverride lets an `override` prompt mode take effect from a repository file.
	AllowPromptOverride bool `yaml:"allow_prompt_override,omitempty" json:"allow_prompt_override,omitempty" toml:"allow_prompt_override,omitempty"` //nolint:tagliatelle
}

// JudgeSkills is the [judge.skills] table.
type JudgeSkills struct {
	// Load lists skills (by name, from the project's skill directories) applied to the governance review.
	Load []string `yaml:"load,omitempty" json:"load,omitempty" toml:"load,omitempty"`
	// For assigns skills to a category: for = { quality = ["my-rubric"] }.
	For map[string][]string `yaml:"for,omitempty" json:"for,omitempty" toml:"for,omitempty"`
	// Pins maps a skill name to the start of its SHA-256: a pinned skill that matches is trusted as
	// configured text (tier T1); an unpinned or changed one is reference data (T2).
	Pins map[string]string `yaml:"pins,omitempty" json:"pins,omitempty" toml:"pins,omitempty"`
}

// JudgePrompt is one [judge.prompts.<category>] entry.
type JudgePrompt struct {
	// Mode is extend (the file's text is added to the reviewer's instructions) or override (it replaces the role prompt).
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" toml:"mode,omitempty"`
	// File is the prompt text, relative to the project root.
	File string `yaml:"file,omitempty" json:"file,omitempty" toml:"file,omitempty"`
}

// JudgeSuppression is one [[judge.suppress]] entry.
type JudgeSuppression struct {
	Code    string `yaml:"code" json:"code" toml:"code"`
	Path    string `yaml:"path" json:"path" toml:"path"`
	Quote   string `yaml:"quote" json:"quote" toml:"quote"`
	Reason  string `yaml:"reason" json:"reason" toml:"reason"`
	Expires string `yaml:"expires" json:"expires" toml:"expires"`
}

// attach reports AttachRules with its default.
func (j *JudgeConfig) AttachRulesOn() bool { return j == nil || j.AttachRules == nil || *j.AttachRules }

// AttachClaudeMDOn reports AttachClaudeMD with its default.
func (j *JudgeConfig) AttachClaudeMDOn() bool {
	return j == nil || j.AttachClaudeMD == nil || *j.AttachClaudeMD
}

// userOnlyKeys names the keys of the table a repository config cannot set.
func (j *JudgeConfig) userOnlyKeys() []string {
	if j == nil {
		return nil
	}
	var out []string
	if len(j.Models) > 0 {
		out = append(out, "models")
	}
	if j.MaxCostUSD != 0 {
		out = append(out, "max_cost_usd")
	}
	if j.MaxTokens != 0 {
		out = append(out, "max_tokens")
	}
	if j.MaxCalls != 0 {
		out = append(out, "max_calls")
	}
	if j.Concurrency != 0 {
		out = append(out, "concurrency")
	}
	if j.RPM != 0 {
		out = append(out, "rpm")
	}
	if j.AllowLLM {
		out = append(out, "allow_llm")
	}
	if j.AllowPromptOverride {
		out = append(out, "allow_prompt_override")
	}
	return out
}

// Validate returns the problems of the table, one message each.
func (j *JudgeConfig) Validate(now time.Time) []string {
	if j == nil {
		return nil
	}
	var problems []string
	problems = append(problems, j.validateCategories()...)
	problems = append(problems, j.validateMinConfidence()...)
	problems = append(problems, j.validatePrompts()...)
	problems = append(problems, j.validateSkills()...)
	problems = append(problems, j.validateSuppress(now)...)
	if math.IsNaN(j.MaxCostUSD) || math.IsInf(j.MaxCostUSD, 0) || j.MaxCostUSD < 0 || j.MaxTokens < 0 || j.MaxCalls < 0 || j.Concurrency < 0 || j.RPM < 0 {
		problems = append(problems, "judge.max_cost_usd, max_tokens, max_calls, concurrency and rpm must be finite numbers >= 0")
	}
	return problems
}

const (
	judgeCategorySecurity   = "security"
	judgeCategoryQuality    = "quality"
	judgeCategoryGovernance = "governance"
)

func isJudgeCategory(c string) bool {
	switch c {
	case judgeCategorySecurity, judgeCategoryQuality, judgeCategoryGovernance:
		return true
	}
	return false
}

func (j *JudgeConfig) validateCategories() []string {
	var problems []string
	for _, c := range j.Categories {
		if !isJudgeCategory(strings.ToLower(strings.TrimSpace(c))) {
			problems = append(problems, fmt.Sprintf("judge.categories %q must be security, quality or governance", c))
		}
	}
	return problems
}

func (j *JudgeConfig) validateMinConfidence() []string {
	switch Priority(strings.ToLower(strings.TrimSpace(j.MinConfidence))) {
	case "", PriorityLow, PriorityMedium, PriorityHigh:
		return nil
	}
	return []string{fmt.Sprintf("judge.min_confidence %q must be low, medium or high", j.MinConfidence)}
}

func (j *JudgeConfig) validatePrompts() []string {
	var problems []string
	for cat, p := range j.Prompts {
		if !isJudgeCategory(cat) {
			problems = append(problems, fmt.Sprintf("judge.prompts.%s: the category must be security, quality or governance", cat))
		}
		if p.Mode != JudgePromptExtend && p.Mode != JudgePromptOverride {
			problems = append(problems, fmt.Sprintf("judge.prompts.%s.mode %q must be %q or %q", cat, p.Mode, JudgePromptExtend, JudgePromptOverride))
		}
		if strings.TrimSpace(p.File) == "" || path.IsAbs(p.File) || strings.HasPrefix(path.Clean(p.File), "..") {
			problems = append(problems, fmt.Sprintf("judge.prompts.%s.file must be a path inside the project", cat))
		}
	}
	return problems
}

func (j *JudgeConfig) validateSkills() []string {
	if j.Skills == nil {
		return nil
	}
	var problems []string
	for cat := range j.Skills.For {
		if !isJudgeCategory(cat) {
			problems = append(problems, fmt.Sprintf("judge.skills.for.%s: the category must be security, quality or governance", cat))
		}
	}
	return problems
}

func (j *JudgeConfig) validateSuppress(now time.Time) []string {
	var problems []string
	for i := range j.Suppress {
		s := &j.Suppress[i]
		if strings.TrimSpace(s.Code) == "" || strings.TrimSpace(s.Path) == "" || strings.TrimSpace(s.Quote) == "" || strings.TrimSpace(s.Reason) == "" {
			problems = append(problems, fmt.Sprintf("judge.suppress[%d] needs code, path, quote and reason", i))
		}
		exp, err := time.Parse("2006-01-02", s.Expires)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("judge.suppress[%d].expires %q must be a date as YYYY-MM-DD", i, s.Expires))
		case exp.After(now.AddDate(0, 0, 366)):
			problems = append(problems, fmt.Sprintf("judge.suppress[%d].expires %s is more than 366 days away", i, s.Expires))
		}
	}
	return problems
}
