package verifiers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// proposal is a complete model proposal; fields not given are "" (and -1 for min/max).
func proposal(over map[string]any) map[string]any {
	p := map[string]any{
		"id": "no-todo", "predicate": "forbid", "description": "no TODO left", "severity": "warning", "message": "TODO found", "fix": "remove it",
		"when_changed": []string{"src/**/*.go"}, "exclude": []string{}, "regex": "TODO", "in": "same-file", "files": "", "path": "", "exists": true,
		"for_each": "", "requires_changed": "", "requires_exists": "", "min": -1, "max": -1,
		"pass_path": "src/ok.go", "pass_content": "package a\n", "fail_path": "src/bad.go", "fail_content": "package a\n// TODO fix\n",
		"rationale": "enforces the no TODO sentence",
	}
	for k, v := range over {
		p[k] = v
	}
	return p
}

func suggestion(skipped string, proposals ...map[string]any) string {
	if proposals == nil {
		proposals = []map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{"proposals": proposals, "skipped_reason": skipped})
	return string(b)
}

func suggestFake(text string) *llm.Fake {
	return &llm.Fake{ChatFunc: func(llm.ChatRequest) (string, error) { return text, nil }}
}

func suggestProject(t *testing.T) *config.Config {
	t.Helper()
	cfg := specProject(t, map[string]string{"src/a.go": "package a\n", "src/old.go": "package a\n// TODO later\n// TODO again\n", "README.md": "# x\n"}, "")
	return cfg
}

func TestSuggest_ProposalIsValidatedExampleCheckedAndCounted(t *testing.T) {
	// Arrange
	cfg := suggestProject(t)
	fake := suggestFake(suggestion("", proposal(nil)))

	// Act
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake}})

	// Assert
	require.NoError(t, err)
	require.Len(t, res.Proposals, 1)
	p := res.Proposals[0]
	assert.Empty(t, p.Rejected)
	assert.Equal(t, "no-todo", p.ID)
	assert.Equal(t, "verified", p.Examples)
	assert.Equal(t, 2, p.Hits, "two TODOs exist in src/old.go today")
	assert.Equal(t, []string{"src/old.go"}, p.HitFiles)
	assert.Contains(t, p.TOML, "[[verifiers]]")
	assert.Contains(t, p.TOML, `rule = 'database'`)
	assert.Contains(t, p.TOML, "[verifiers.require.forbid]")
	require.Len(t, fake.ChatCalls(), 1)
	assert.Zero(t, fake.ChatCalls()[0].Temperature)
	assert.Equal(t, SuggestPromptVersion, fake.ChatCalls()[0].PromptVersion)
	raw, _ := json.Marshal(fake.ChatCalls()[0].ResponseFormat.Schema)
	assert.NotContains(t, string(raw), "additionalProperties")
	require.NotNil(t, res.LLM)
	assert.Equal(t, 1, res.LLM.Calls)
}

func TestSuggest_RenderedTOMLRoundTripsThroughTheLoader(t *testing.T) {
	cfg := suggestProject(t)
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(nil)))}})
	require.NoError(t, err)

	parsed, err := parseSpecs([]byte(res.Proposals[0].TOML))

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Empty(t, validateSpec(cfg, &parsed[0]))
	assert.Equal(t, res.Proposals[0].Spec.Require, parsed[0].Require)
	assert.Len(t, parsed[0].Examples, 2)
}

func TestSuggest_Rejections(t *testing.T) {
	tests := []struct {
		name string
		over map[string]any
		want string
	}{
		{"invalid regex", map[string]any{"regex": "("}, "invalid regex"},
		{"backreference is not RE2", map[string]any{"regex": `(a)\1`}, "invalid regex"},
		{"scope missing", map[string]any{"when_changed": []string{}}, "needs when_changed"},
		{"bad glob", map[string]any{"when_changed": []string{"{" + strings.Repeat("a,b}{", 30) + "x}"}}, "glob"},
		{"unknown predicate", map[string]any{"predicate": "command"}, "unknown predicate"},
		{"paired without a requirement", map[string]any{"predicate": "paired", "for_each": "src/{rel}.go"}, "exactly one of requires_changed"},
		{"example that does not fail", map[string]any{"fail_content": "package a\n"}, `example "violates the check" expected fail but got pass`},
		{"example that does not pass", map[string]any{"pass_content": "// TODO\n"}, `example "satisfies the check" expected pass but got fail`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := suggestProject(t)

			// Act
			res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(tt.over)))}})

			// Assert
			require.NoError(t, err)
			require.Len(t, res.Proposals, 1)
			assert.Contains(t, res.Proposals[0].Rejected, tt.want)
			assert.Empty(t, res.Proposals[0].TOML, "a rejected candidate is never offered")
			assert.Empty(t, res.Usable())
		})
	}
}

func TestSuggest_SeverityStartsAtWarningAndIDsStayValidAndUnique(t *testing.T) {
	cfg := suggestProject(t)
	cfg.Verifiers = []config.VerifierConfig{{Name: "no-todo", Type: "file_exists", Path: "README.md"}}
	fake := suggestFake(suggestion("", proposal(map[string]any{"severity": "error"}), proposal(map[string]any{"id": "No TODO!!", "severity": "error"})))

	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake}})

	require.NoError(t, err)
	require.Len(t, res.Proposals, 2)
	assert.Equal(t, "no-todo-2", res.Proposals[0].ID, "an id that exists is suffixed, never reused")
	assert.Equal(t, "no-todo-3", res.Proposals[1].ID)
	for _, p := range res.Proposals {
		assert.Equal(t, severityWarning, p.Spec.Severity, "a suggestion starts as a warning")
	}
}

func TestSuggest_NothingMechanicalIsAnAnswer(t *testing.T) {
	cfg := suggestProject(t)

	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("The rule is about design taste."))}})

	require.NoError(t, err)
	assert.Empty(t, res.Proposals)
	assert.Equal(t, "The rule is about design taste.", res.SkippedReason)
}

func TestSuggest_MaxProposals(t *testing.T) {
	cfg := suggestProject(t)
	fake := suggestFake(suggestion("", proposal(map[string]any{"id": "a"}), proposal(map[string]any{"id": "b"}), proposal(map[string]any{"id": "c"})))

	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", MaxProposals: 2, LLM: LLMOptions{Client: fake}})

	require.NoError(t, err)
	assert.Len(t, res.Proposals, 2)
	assert.Contains(t, strings.Join(res.Notes, " "), "1 proposal(s) beyond --max-proposals 2")
}

func TestSuggest_UnusableRepliesAreErrors(t *testing.T) {
	tests := []struct{ name, reply string }{
		{"prose", "Here are some verifiers"},
		{"extra top-level field", `{"proposals":[],"skipped_reason":"","x":1}`},
		{"missing skipped_reason", `{"proposals":[]}`},
		{"incomplete proposal", `{"proposals":[{"id":"a"}],"skipped_reason":""}`},
		{"extra proposal field", strings.Replace(suggestion("", proposal(nil)), `"id":"no-todo"`, `"id":"no-todo","extra":1`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Suggest(context.Background(), suggestProject(t), SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(tt.reply)}})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unusable")
		})
	}
}

func TestSuggest_RefusesWithoutAModelAndNeverSendsASecret(t *testing.T) {
	cfg := suggestProject(t)

	_, errOff := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Disabled: "pass --allow-llm"}})
	_, errMissing := Suggest(context.Background(), cfg, SuggestOptions{ID: "ghost", LLM: LLMOptions{Client: suggestFake("")}})
	cfg.Content.Rules[0].Content = "Use the key sk-abcdefghijklmnop12345678 for the API.\n"
	fake := suggestFake(suggestion(""))
	_, errSecret := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake}})

	require.Error(t, errOff)
	assert.Contains(t, errOff.Error(), "LLM use is off (pass --allow-llm)")
	require.Error(t, errMissing)
	assert.Contains(t, errMissing.Error(), "does not exist")
	require.Error(t, errSecret)
	assert.Contains(t, errSecret.Error(), "credential-looking")
	assert.Empty(t, fake.ChatCalls())
}

func TestSuggest_EstimateCallsNothingAndStatesTheEgress(t *testing.T) {
	cfg := suggestProject(t)
	fake := suggestFake(suggestion(""))
	price := func(_ string, u llm.Usage) (float64, bool) {
		return float64(u.PromptTokens+u.CompletionTokens) * 1e-6, true
	}

	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake, Estimate: true, Prices: price, Model: "m"}})

	require.NoError(t, err)
	assert.Contains(t, res.Estimate, "estimate: 1 call to m")
	assert.Contains(t, res.Estimate, "nothing was sent")
	assert.Contains(t, res.Estimate, "directory names")
	assert.Empty(t, fake.ChatCalls())
}

func TestSuggest_CostCapRefusesBeforeCalling(t *testing.T) {
	cfg := suggestProject(t)
	fake := suggestFake(suggestion(""))
	dear := func(_ string, u llm.Usage) (float64, bool) { return 100, true }

	_, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake, MaxCostUSD: 0.5, Prices: dear}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "would exceed --max-cost")
	assert.Empty(t, fake.ChatCalls())
}

func TestSuggest_PromptCarriesTheLayoutButNoFileContent(t *testing.T) {
	cfg := suggestProject(t)
	fake := suggestFake(suggestion(""))

	_, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: fake}})

	require.NoError(t, err)
	user := fake.ChatCalls()[0].Messages[1].Content
	assert.Contains(t, user, "directories: src")
	assert.Contains(t, user, "extensions: .go")
	assert.Contains(t, user, "<<<DATA ")
	assert.NotContains(t, user, "TODO later", "no file content is sent")
}

func TestSuggest_NeverWritesAndWriteSuggestionsCreatesANewFileOnly(t *testing.T) {
	// Arrange
	cfg := suggestProject(t)
	dir := filepath.Join(cfg.ConfigDir, VerifiersDirName)
	require.NoError(t, os.RemoveAll(dir))
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(nil)))}})
	require.NoError(t, err)
	_, statErr := os.Stat(dir)
	require.True(t, os.IsNotExist(statErr), "Suggest alone writes nothing")

	// Act
	path, err := WriteSuggestions(cfg, res)
	_, again := WriteSuggestions(cfg, res)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "suggested-database.toml"), path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Suggested by `ai-rulez verifiers suggest`")
	require.Error(t, again)
	assert.Contains(t, again.Error(), "never overwritten")
	specs, problems := LoadSpecs(cfg)
	assert.Empty(t, problems)
	require.Len(t, specs, 1, "the written file loads as a normal declaration")
	assert.Equal(t, "no-todo", specs[0].ID)
}

func TestWriteSuggestions_RefusesASymlinkedVerifiersDirectory(t *testing.T) {
	// Arrange
	cfg := suggestProject(t)
	dir := filepath.Join(cfg.ConfigDir, VerifiersDirName)
	require.NoError(t, os.RemoveAll(dir))
	outside := t.TempDir()
	testutil.SymlinkOrSkip(t, outside, dir)
	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(nil)))}})
	require.NoError(t, err)

	// Act
	_, err = WriteSuggestions(cfg, res)

	// Assert
	require.Error(t, err)
	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "nothing may be written through the planted link")
}

func TestWriteSuggestions_NothingUsableWritesNothing(t *testing.T) {
	cfg := suggestProject(t)
	require.NoError(t, os.RemoveAll(filepath.Join(cfg.ConfigDir, VerifiersDirName)))

	_, err := WriteSuggestions(cfg, &SuggestResult{Target: &Target{Kind: "rule", ID: "database"}, Proposals: []Proposal{{Rejected: "bad"}}})

	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(cfg.ConfigDir, VerifiersDirName))
	assert.True(t, os.IsNotExist(statErr))
}
