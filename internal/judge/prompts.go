package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The role prompts and message templates are adapted from the research behind this command. Their
// taxonomy and calibration ideas were informed by three open-source scanners, whose licenses ask for
// attribution: Cisco AI Defense skill-scanner (Apache-2.0, Copyright 2026 Cisco Systems, Inc. and its
// affiliates), SkillSpector (Apache-2.0, Copyright 2026 NVIDIA CORPORATION & AFFILIATES) and AgentShield (MIT, Copyright (c) 2026 Affaan M). See
// prompts/NOTICE.

func promptText(name string) string {
	data, err := assets.ReadFile("prompts/" + name)
	if err != nil {
		panic("judge: missing embedded prompt " + name) // a build-time invariant
	}
	return strings.TrimRight(string(data), "\n")
}

// Trap is a known false-positive shape the adjudicator is told about.
type Trap struct {
	ID        string   `yaml:"id" json:"id"`
	Name      string   `yaml:"name" json:"name"`
	Text      string   `yaml:"text" json:"text"`
	AppliesTo []string `yaml:"applies_to" json:"applies_to"` //nolint:tagliatelle // snake_case file format
}

// ParseRepoTraps reads a repo-level traps file (extend-only; see the traps example in the docs).
func ParseRepoTraps(data []byte) ([]Trap, error) {
	var traps []Trap
	if err := yaml.Unmarshal(data, &traps); err != nil {
		return nil, fmt.Errorf("parse traps: %w", err)
	}
	seen := map[string]bool{}
	for i, t := range traps {
		switch {
		case strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Text) == "":
			return nil, fmt.Errorf("trap %d needs an id and a text", i+1)
		case seen[t.ID]:
			return nil, fmt.Errorf("trap id %q is used twice", t.ID)
		}
		seen[t.ID] = true
		for _, code := range t.AppliesTo {
			if _, ok := LookupCode(code); !ok {
				return nil, fmt.Errorf("trap %s applies to %q, which is not a finding code", t.ID, code)
			}
		}
	}
	return traps, nil
}

// PromptSet is the prompt configuration of a run: the repo-configured extensions (T1) and overrides.
type PromptSet struct {
	// Extend is rubric-like text added to a category's REVIEWER_INSTRUCTIONS (tier T1).
	Extend map[Category]string
	// Override replaces the role prompt of a category. The shared core (trust tiers, evidence
	// contract, calibration, self-check, coverage) and the output schema are never replaceable.
	Override map[Category]string
	// RepoTraps extend the adjudicator's trap list.
	RepoTraps []Trap
}

// Overridden lists the categories whose built-in role prompt is replaced, sorted.
func (p PromptSet) Overridden() []string {
	var out []string
	for c, text := range p.Override {
		if strings.TrimSpace(text) != "" {
			out = append(out, string(c))
		}
	}
	sort.Strings(out)
	return out
}

func rolePromptFile(c Category) string {
	switch c {
	case CategorySecurity:
		return "10_security_reviewer.txt"
	case CategoryQuality:
		return "20_quality_reviewer.txt"
	default:
		return "30_governance_reviewer.txt"
	}
}

// ReviewerSystem renders the system prompt of a category reviewer.
func (p PromptSet) ReviewerSystem(c Category, nonce string, maxFindings int) string {
	core := promptText("00_shared_core.txt")
	role := promptText(rolePromptFile(c))
	if o := strings.TrimSpace(p.Override[c]); o != "" {
		role = o
	}
	r := strings.NewReplacer("{{NONCE}}", nonce, "{{MAX_FINDINGS}}", itoa(maxFindings))
	return r.Replace(core) + "\n\n" + r.Replace(role)
}

// AdjudicatorSystem renders the system prompt of the adjudicator.
func (p PromptSet) AdjudicatorSystem(nonce string) string {
	core := promptText("00_shared_core.txt")
	role := promptText("40_adjudicator.txt")
	var repo strings.Builder
	for _, t := range p.RepoTraps {
		fmt.Fprintf(&repo, "%s %s  %s", t.ID, t.Name, strings.TrimSpace(t.Text))
		if len(t.AppliesTo) > 0 {
			fmt.Fprintf(&repo, " (applies to %s)", strings.Join(t.AppliesTo, ", "))
		}
		repo.WriteString("\n")
	}
	r := strings.NewReplacer(
		"{{NONCE}}", nonce,
		"{{MAX_FINDINGS}}", "3",
		"{{BUILTIN_TRAPS}}", promptText("45_builtin_traps.txt"),
		"{{REPO_TRAPS}}", strings.TrimRight(repo.String(), "\n"),
	)
	return r.Replace(core) + "\n\n" + r.Replace(role)
}

// PromptFingerprint hashes everything that shapes a prompt, for the run provenance.
func (p PromptSet) PromptFingerprint() string {
	h := sha256.New()
	for _, name := range []string{"00_shared_core.txt", "10_security_reviewer.txt", "20_quality_reviewer.txt", "30_governance_reviewer.txt", "40_adjudicator.txt", "45_builtin_traps.txt", "50_user_message_template.txt", "55_repair_message.txt", "56_adjudicator_user_template.txt", "57_adjudicator_feedback.txt"} {
		h.Write([]byte(promptText(name)))
	}
	h.Write(jsonText(p))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
