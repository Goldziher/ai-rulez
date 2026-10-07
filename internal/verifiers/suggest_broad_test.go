package verifiers

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverBroadReason(t *testing.T) {
	forbid := func(regex, in string) *Spec {
		return &Spec{Require: &Require{Forbid: &RegexPred{Regex: regex, In: in}}}
	}
	tests := []struct {
		name     string
		spec     *Spec
		hitFiles int
		want     string
	}{
		{"specific pattern, few hits", forbid(`TODO\(`, "same-file"), 3, ""},
		{"stdlib call open(", forbid(`open\(`, "same-file"), 0, "standard-library"},
		{"stdlib call json.load", forbid(`json\.load`, "any-file"), 0, "standard-library"},
		{"rust File::open", forbid(`File::open`, "same-file"), 0, "standard-library"},
		{"alternation of stdlib calls", forbid(`open\(|read\(|File::open`, "same-file"), 0, "standard-library"},
		{"a call with its arguments", forbid(`\bopen\([^)]*\)`, "same-file"), 0, "standard-library"},
		{"a call with a quoted argument", forbid(`open\(["'][^"']+["']`, "same-file"), 0, "standard-library"},
		{"an assignment from open", forbid(`=\s*open\(`, "same-file"), 0, "standard-library"},
		{"a Go call with arguments", forbid(`os\.Getenv\("[A-Z_]+"\)`, "same-file"), 0, "standard-library"},
		{"a whole import line", forbid(`^import os$`, "same-file"), 0, "standard-library"},
		{"a ratchet on new code is allowed", forbid(`open\(`, "diff-added"), 0, ""},
		{"fires on many existing files", forbid(`legacy_helper`, "same-file"), maxExistingHitFiles + 1, "existing files"},
		{"at the threshold", forbid(`legacy_helper`, "same-file"), maxExistingHitFiles, ""},
		{"not a forbid", &Spec{Require: &Require{Regex: &RegexPred{Regex: `open\(`, In: "same-file"}}}, 99, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := overBroadReason(tt.spec, tt.hitFiles)

			// Assert
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestSuggest_RejectsAnOverBroadProposal(t *testing.T) {
	// Arrange: a proposal that forbids a stdlib call everywhere
	cfg := suggestProject(t)
	broad := proposal(map[string]any{
		"regex": `open\(`, "when_changed": []string{"src/**/*.go"},
		"fail_content": "package a\nvar _ = open(1)\n",
	})

	// Act
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", broad))}})

	// Assert
	require.NoError(t, err)
	require.Len(t, res.Proposals, 1)
	assert.Contains(t, res.Proposals[0].Rejected, "standard-library")
	assert.Empty(t, res.Proposals[0].TOML)
}

func TestSuggest_RejectsAProposalThatFiresOnManyExistingFiles(t *testing.T) {
	// Arrange
	files := map[string]string{"README.md": "# x\n"}
	for i := range maxExistingHitFiles + 2 {
		files[fmt.Sprintf("src/f%d.go", i)] = "package a\n// TODO later\n"
	}
	cfg := specProject(t, files, "")

	// Act
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(nil)))}})

	// Assert
	require.NoError(t, err)
	require.Len(t, res.Proposals, 1)
	assert.Contains(t, res.Proposals[0].Rejected, "existing files")
}

func TestSuggestPromptVersionIsBumped(t *testing.T) {
	assert.Equal(t, "verifier-suggest/v2", SuggestPromptVersion)
}
