package generator

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateGolden rewrites the committed golden report instead of asserting against
// it. Run with: go test ./internal/generator -run Golden -update-tokens-golden
var updateGolden = flag.Bool("update-tokens-golden", false, "rewrite the tokens golden file")

const tokensGoldenPath = "testdata/tokens_report.json"

// tokensFixture copies the tokens fixture into a temp dir and loads it. The copy
// matters: TokenReport records the outputs it would write, and a fixture loaded in
// place would tempt a future change to compare against a checked-in tree.
func tokensFixture(t *testing.T) *config.Config {
	t.Helper()
	return tokensFixtureWith(t, nil)
}

// tokensFixtureWith copies the fixture, lets a test rewrite its config file, and
// loads the result.
func tokensFixtureWith(t *testing.T, edit func(config string) string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "config", "tokens"), dir)
	if edit != nil {
		path := filepath.Join(dir, ".ai-rulez", "config.yaml")
		original, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte(edit(string(original))), 0o600))
	}
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

// tokensReport builds a report for one profile with the deterministic offline
// counter.
func tokensReport(t *testing.T, profile string) *TokenReport {
	t.Helper()
	report, err := NewGenerator(tokensFixture(t)).TokenReport(TokenReportOptions{
		Profile: profile,
		Counter: tokens.CL100KBase(),
	})
	require.NoError(t, err)
	return report
}

// tokensReportInline is tokensReport with `[rules] mode = "inline"`, for tests
// about the rules_inline section of the root file (the default mode is split).
func tokensReportInline(t *testing.T, profile string) *TokenReport {
	t.Helper()
	cfg := tokensFixtureWith(t, func(original string) string {
		return original + "rules:\n  mode: inline\n"
	})
	report, err := NewGenerator(cfg).TokenReport(TokenReportOptions{
		Profile: profile,
		Counter: tokens.CL100KBase(),
	})
	require.NoError(t, err)
	return report
}

// findRuntime returns the runtime for a preset in the repository-root scope.
func findRuntime(t *testing.T, report *TokenReport, preset string) RuntimeTokens {
	t.Helper()
	for _, runtime := range report.Runtimes {
		if runtime.Preset == preset {
			return runtime
		}
	}
	t.Fatalf("report has no %q runtime", preset)
	return RuntimeTokens{}
}

// findEntry returns a top-level entry by label.
func findEntry(t *testing.T, runtime RuntimeTokens, label string) Entry {
	t.Helper()
	for _, entry := range runtime.Entries {
		if entry.Label == label {
			return entry
		}
	}
	t.Fatalf("runtime %q has no entry %q", runtime.Preset, label)
	return Entry{}
}

func findChild(t *testing.T, entry Entry, label string) Entry {
	t.Helper()
	for _, child := range entry.Children {
		if child.Label == label {
			return child
		}
	}
	t.Fatalf("entry %q has no child %q", entry.Label, label)
	return Entry{}
}

// TestTokenReport_FoldedScalarDescription is a white-box check on the recorded
// description text. A folded block scalar (`description: >`) is the case a regex
// gets wrong: it reads the value as the single character ">" and reports a real
// description as costing nothing.
func TestTokenReport_FoldedScalarDescription(t *testing.T) {
	cfg := tokensFixture(t)
	collector := config.NewAnalysisCollector()
	cfg.Analysis = collector

	_, _, err := NewGenerator(cfg).collectOutputs("backend")
	require.NoError(t, err)

	var description string
	for _, analysis := range collector.Analyses() {
		if analysis.Kind != config.OutputKindSkill || !strings.Contains(analysis.Path, "folded-description") {
			continue
		}
		for _, part := range analysis.Parts {
			if part.Kind == config.PartKindItemDescription {
				description = part.Content
			}
		}
	}

	require.NotEmpty(t, description, "folded-description skill recorded no description part")
	assert.Contains(t, description, "folded block scalar")
	assert.Contains(t, description, "reports its cost as zero",
		"the folded scalar must be read to its end, not truncated at the first line")
	assert.Greater(t, tokens.CL100KBase().Count(description), 30,
		"a folded description costs real tokens; 1 would mean the %q marker was counted instead", ">")
}

// TestTokenReport_SplitsDescriptionFromBody proves the split the whole report
// exists for: the fixture's big-body skill has a two-word description and a long
// body, and the two must not be reported as one per-file number.
func TestTokenReport_SplitsListingFromBody(t *testing.T) {
	runtime := findRuntime(t, tokensReport(t, "backend"), "claude")

	listing := findEntry(t, runtime, "skill listing")
	bodies := findEntry(t, runtime, "skill bodies")

	assert.Equal(t, BucketAlways, listing.Bucket,
		"the harness lists a skill's name and description on every request")
	assert.Equal(t, BucketOnDemand, bodies.Bucket)
	assert.Greater(t, bodies.Tokens, listing.Tokens,
		"fixture bodies are larger than their listing entries; a single per-file total would hide that")
	assert.Positive(t, findChild(t, listing, "names").Tokens)
	assert.Positive(t, findChild(t, listing, "descriptions").Tokens)

	for _, entry := range runtime.Entries {
		assert.NotEqual(t, "skill names", entry.Label, "names are inside the listing, not counted twice")
		assert.NotEqual(t, "skill descriptions", entry.Label, "descriptions are inside the listing, not counted twice")
	}
}

// TestTokenReport_RootSectionsSplitPerRuleAndContext checks that the root
// instructions file is broken down far enough to name the expensive rule, and that
// the children follow the order the provider DSL renders them in.
func TestTokenReport_RootSectionsSplitPerRuleAndContext(t *testing.T) {
	runtime := findRuntime(t, tokensReportInline(t, "backend"), "claude")
	require.True(t, runtime.Detailed, "claude is described by the provider DSL")
	require.Equal(t, 1, runtime.RootFiles)

	root := findEntry(t, runtime, "CLAUDE.md")
	rules := findChild(t, root, "rules_inline")
	context := findChild(t, root, "context_inline")

	assert.Equal(t, 2, rules.Artifacts, "backend profile has the root rule and the backend rule")
	assert.Positive(t, findChild(t, rules, "root-rule").Tokens)
	assert.Positive(t, findChild(t, rules, "backend-rule").Tokens)
	assert.Positive(t, findChild(t, context, "layout").Tokens)

	assert.Equal(t,
		[]string{"header", "title", "description", "rules_inline", "context_inline",
			"provenance hashes and section boundaries"},
		childLabels(root), "children follow the DSL section order so the report reads like the file")
}

// TestTokenReport_DefaultSplitMovesRulesOutOfRootFile checks that with the
// default split mode the rules are reported as provider rule files and the root
// file carries no rules section.
func TestTokenReport_DefaultSplitMovesRulesOutOfRootFile(t *testing.T) {
	runtime := findRuntime(t, tokensReport(t, "backend"), "claude")

	root := findEntry(t, runtime, "CLAUDE.md")
	assert.NotContains(t, childLabels(root), "rules_inline")
	assert.Equal(t, 2, findEntry(t, runtime, "provider rule files").Artifacts,
		"backend profile has the root rule and the backend rule")
}

// TestTokenReport_AgentsRosterIsItsOwnSection covers the root section that
// duplicates every agent file's name and description: it is reported apart from the
// rules and context so its cost can be seen and decided about.
func TestTokenReport_AgentsRosterIsItsOwnSection(t *testing.T) {
	cfg := tokensFixtureWith(t, func(original string) string {
		return original + "builtins:\n  - agent-delegation\n"
	})
	report, err := NewGenerator(cfg).TokenReport(TokenReportOptions{
		Profile: "backend",
		Counter: tokens.CL100KBase(),
	})
	require.NoError(t, err)

	root := findEntry(t, findRuntime(t, report, "claude"), "CLAUDE.md")
	roster := findChild(t, root, "agents_delegation")
	assert.Positive(t, roster.Tokens)
	assert.Equal(t, BucketAlways, roster.Bucket)

	labels := childLabels(root)
	assert.Equal(t, "agents_delegation", labels[len(labels)-2],
		"the roster renders after rules and context, ahead of the provenance residual")
}

// TestAgentsRosterExclusion proves that the "## Agents" roster can already be
// suppressed from configuration, with no new knob: it renders only when the
// agent-delegation builtin domain is loaded (internal/generator/presets/helpers.go,
// renderAgentsSection), so "!agent-delegation" drops it. The roster restates every
// agent's name and description in a file loaded on every request — measured at 1,094
// always-loaded tokens on a 32-agent tree — while the per-agent files that carry the
// same text on demand are still generated, so the duplication costs nothing to drop.
func TestAgentsRosterExclusion(t *testing.T) {
	tests := []struct {
		name       string
		builtins   string
		wantRoster bool
	}{
		{
			name:       "roster renders when the builtin is loaded",
			builtins:   "builtins:\n  - agent-delegation\n",
			wantRoster: true,
		},
		{
			// A bare "!name" suppresses an auto-included builtin, so this list loads
			// the other six auto-includes and no agent-delegation.
			name:       "roster is gone when the builtin is excluded",
			builtins:   "builtins:\n  - \"!agent-delegation\"\n",
			wantRoster: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tokensFixtureWith(t, func(original string) string {
				return original + tt.builtins
			})
			outputs, _, err := NewGenerator(cfg).collectOutputs("backend")
			require.NoError(t, err)

			root := ""
			agentFile := ""
			for _, output := range outputs {
				switch {
				case filepath.Base(output.Path) == "CLAUDE.md":
					root = output.Content
				case strings.HasSuffix(filepath.ToSlash(output.Path), ".claude/agents/reviewer.md"):
					agentFile = output.Content
				}
			}
			require.NotEmpty(t, root, "the claude preset always writes a root instructions file")

			if tt.wantRoster {
				assert.Contains(t, root, "## Agents")
				assert.Contains(t, root, "- **reviewer**")
			} else {
				assert.NotContains(t, root, "## Agents")
				assert.NotContains(t, root, "- **reviewer**")
			}

			assert.NotEmpty(t, agentFile,
				"excluding the roster must not stop the per-agent file from being generated")
			assert.Contains(t, agentFile, "reviewer",
				"the agent file still carries the name the roster duplicated")
		})
	}
}

func childLabels(entry Entry) []string {
	labels := make([]string, 0, len(entry.Children))
	for _, child := range entry.Children {
		labels = append(labels, child.Label)
	}
	return labels
}

// TestTokenReport_ProfilesDiffer checks that composing more domains costs more,
// and that each profile is reported under its own resolved name.
func TestTokenReport_ProfilesDiffer(t *testing.T) {
	tests := []struct {
		profile string
		rules   int
	}{
		{profile: "frontend", rules: 2},
		{profile: "backend", rules: 2},
		{profile: "full", rules: 3},
	}

	totals := make(map[string]int, len(tests))
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			report := tokensReportInline(t, tt.profile)
			assert.Equal(t, tt.profile, report.Profile)
			runtime := findRuntime(t, report, "claude")
			assert.Equal(t, tt.rules, findChild(t, findEntry(t, runtime, "CLAUDE.md"), "rules_inline").Artifacts)
			totals[tt.profile] = runtime.Always
		})
	}

	assert.Greater(t, totals["full"], totals["backend"],
		"the full profile carries both domains' rules, so it costs more")
	assert.Greater(t, totals["backend"], 0)
	assert.NotEqual(t, totals["backend"], totals["frontend"],
		"different domains render different rules")
}

// TestTokenReport_PerDomainTotalsAreSingleRuntime guards against the per-domain
// table being summed across every provider: each provider renders the same
// authored rules, so a cross-runtime sum reads well above the headline it explains.
func TestTokenReport_PerDomainTotalsAreSingleRuntime(t *testing.T) {
	report := tokensReport(t, "full")
	require.NotEmpty(t, report.Domains)

	total := 0
	names := make([]string, 0, len(report.Domains))
	for _, domain := range report.Domains {
		total += domain.Always
		names = append(names, domain.Name)
	}
	assert.Contains(t, names, "backend")
	assert.Contains(t, names, "frontend")
	assert.LessOrEqual(t, total, report.HeadlineAlways,
		"per-domain always-loaded cost cannot exceed the headline it decomposes")
}

// TestTokenReport_BudgetAndNotes covers the threshold mode and the scope
// statements the report is required to carry.
func TestTokenReport_BudgetAndNotes(t *testing.T) {
	tests := []struct {
		name     string
		budget   int
		exceeded bool
	}{
		{name: "under budget", budget: 1_000_000, exceeded: false},
		{name: "over budget", budget: 1, exceeded: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := NewGenerator(tokensFixture(t)).TokenReport(TokenReportOptions{
				Profile: "backend",
				Counter: tokens.CL100KBase(),
				Budget:  tt.budget,
			})
			require.NoError(t, err)
			require.NotNil(t, report.Budget)
			assert.Equal(t, tt.exceeded, report.Budget.Exceeded)
			assert.Equal(t, report.HeadlineAlways, report.Budget.Actual)
		})
	}

	report := tokensReport(t, "backend")
	assert.Nil(t, report.Budget, "no budget requested, no budget result")
	joined := strings.Join(report.Notes, "\n")
	assert.Contains(t, joined, "approximations")
	assert.Contains(t, joined, "never predicts a session total")
	assert.Contains(t, joined, "not additive")
	assert.True(t, report.Tokenizer.Approximate)
	assert.False(t, report.Tokenizer.Estimate)
}

// TestTokenReport_EstimateCounterIsLabelled checks the byte-ratio fallback is
// declared as an estimate and carries its own caveat, so nobody reads a ratio
// based number as a measurement.
func TestTokenReport_EstimateCounterIsLabelled(t *testing.T) {
	report, err := NewGenerator(tokensFixture(t)).TokenReport(TokenReportOptions{
		Profile: "backend",
		Counter: tokens.ByteRatio(tokens.EstimateBytesPerToken),
	})
	require.NoError(t, err)
	assert.True(t, report.Tokenizer.Estimate)
	assert.Contains(t, strings.Join(report.Notes, "\n"), "bytes-per-token estimate")
}

// TestTokenReport_RequiresCounter refuses to invent a tokenizer.
func TestTokenReport_RequiresCounter(t *testing.T) {
	_, err := NewGenerator(tokensFixture(t)).TokenReport(TokenReportOptions{Profile: "backend"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter")
}

// TestTokenReport_LeavesAnalysisDisabled checks the report does not leave the
// recording facility switched on: generation must keep paying nothing for it.
func TestTokenReport_LeavesAnalysisDisabled(t *testing.T) {
	cfg := tokensFixture(t)
	_, err := NewGenerator(cfg).TokenReport(TokenReportOptions{
		Profile: "backend",
		Counter: tokens.CL100KBase(),
	})
	require.NoError(t, err)
	assert.Nil(t, cfg.Analysis)
}

// TestGolden_TokenReportJSON pins the whole serialized report. Any change to how
// output is classified, split or counted shows up here as a diff.
func TestGolden_TokenReportJSON(t *testing.T) {
	report := tokensReport(t, "full")
	actual, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	actual = append(actual, '\n')

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(tokensGoldenPath), 0o755))
		require.NoError(t, os.WriteFile(tokensGoldenPath, actual, 0o600))
		return
	}

	expected, err := os.ReadFile(tokensGoldenPath)
	require.NoError(t, err, "run: go test ./internal/generator -run Golden_TokenReportJSON -update-tokens-golden")
	assert.JSONEq(t, string(expected), string(actual))
}
