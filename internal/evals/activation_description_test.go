package evals

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunActivation_DescriptionOverrideRetrieval(t *testing.T) {
	// Arrange: with its authored description, "alpha" loses the zebra prompt to "beta".
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: Tidy spreadsheets and tables\n---\nbody\n",
		"cases:\n  - id: zebra\n    prompt: explain zebra stripes\n    expect_trigger: true\n")
	writeSkill(t, cfg, "beta", "---\nname: beta\ndescription: Explain zebra stripes and camouflage\n---\nbody\n", "")
	skillFile := filepath.Join(cfg, "skills", "alpha", "SKILL.md")
	before, err := os.ReadFile(skillFile)
	require.NoError(t, err)
	run := func(opts *ActivationOptions) ActivationPrompt {
		report, err := RunActivation(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, report.Skills, 1)
		require.Len(t, report.Skills[0].Prompts, 1)
		return report.Skills[0].Prompts[0]
	}
	base := &ActivationOptions{ConfigDir: cfg, Skills: []string{"alpha"}, Date: "2026-10-06"}
	require.Equal(t, PromptFailed, run(base).Status, "the authored description loses the prompt")
	store := &Store{}

	// Act
	opts := *base
	opts.Store, opts.Description, opts.DescriptionSkill = store, "Explain zebra stripes", "alpha"
	report, err := RunActivation(context.Background(), &opts)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, PromptPassed, report.Skills[0].Prompts[0].Status, "the candidate description takes the prompt")
	assert.Contains(t, strings.Join(report.Skills[0].Warnings, "\n"), "description replaced")
	after, err := os.ReadFile(skillFile)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the source is never edited")
	_, stored := store.Get("alpha")
	assert.False(t, stored, "a candidate description is never recorded")
}

func TestRunActivation_DescriptionOverrideNativeSendsTheCandidate(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	var gotDesc, gotFile string
	runner := nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
		for _, s := range req.Skills {
			if s.ID == "deploy-staging" {
				gotDesc = s.Description
				data, err := os.ReadFile(filepath.Join(s.Dir, "SKILL.md"))
				require.NoError(t, err)
				gotFile = string(data)
			}
		}
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			resp.Results = append(resp.Results, Result{Case: req.Cases[i].ID, Arm: ArmWith, Runs: req.Runs})
		}
		return resp, nil
	}}}
	opts := nativeOptions(cfg, runner)
	opts.Description, opts.DescriptionSkill = "A candidate description: ship it to staging", "deploy-staging"

	// Act
	_, err := RunActivation(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "A candidate description: ship it to staging", gotDesc)
	assert.Contains(t, gotFile, "A candidate description")
	assert.Contains(t, gotFile, "triggers:", "the rest of the frontmatter survives")
	assert.Contains(t, gotFile, "body", "and the body")
}

func TestRunActivation_DescriptionOverrideErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(o *ActivationOptions)
		wantErr string
	}{
		{"unknown skill", func(o *ActivationOptions) { o.DescriptionSkill, o.Skills = "nope", []string{"nope"} }, "unknown skill"},
		{"a skill outside the run", func(o *ActivationOptions) { o.Skills = []string{"release-notes"} }, "not among the skills of this run"},
		{"an empty description", func(o *ActivationOptions) { o.Description = "  \n" }, "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := activationProject(t)
			opts := &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}, Description: "x", DescriptionSkill: "deploy-staging"}
			tt.mutate(opts)

			_, err := RunActivation(context.Background(), opts)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWithDescription_RewritesOnlyTheDescription(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"replaces", "---\nname: a\ndescription: old\nkeywords: [x]\n---\nbody\n", []string{"name: a", "description: new: desc", "keywords: [x]", "\nbody\n"}},
		{"adds when absent", "---\nname: a\n---\nbody\n", []string{"name: a", "description: new: desc"}},
		{"no frontmatter", "body only\n", []string{"description: new: desc", "body only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := withDescription([]byte(tt.in), "new: desc")

			require.NoError(t, err)
			front, err := frontmatterOf(got)
			require.NoError(t, err)
			assert.Equal(t, "new: desc", front["description"], string(got))
			for _, w := range tt.want[1:] {
				assert.Contains(t, string(got), strings.TrimPrefix(w, "description: "))
			}
		})
	}
}
