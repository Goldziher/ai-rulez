package review

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func builtin(t *testing.T) *Rubric {
	t.Helper()
	rb, err := LoadBuiltin("skill-quality")
	require.NoError(t, err)
	return rb
}

func skill(name, desc string) Item {
	return Item{
		ID: "skill:" + name, Kind: KindSkill, Name: name, Path: ".ai-rulez/skills/" + name + "/SKILL.md",
		Owned: true, Description: desc, Keys: []string{"description", "name"}, Body: "body of " + name + "\n",
	}
}

func finding(code string, sev lint.Severity, item Item, line int, msg string) lint.Finding {
	return lint.Finding{Code: code, Name: strings.ToLower(code), Severity: sev, File: item.Path, Line: line, Message: msg}
}

func TestRunScoresFromLintEvidence(t *testing.T) {
	deploy := skill("deploy", "Helps with deployments")
	tests := []struct {
		name     string
		findings []lint.Finding
		wantNil  bool
		want     int
		verdicts map[string]string
	}{
		{"clean item scores 100 over the scored dimensions", nil, false, 100, map[string]string{"trigger-quality": VerdictPass}},
		{"a warning twin halves its dimension", []lint.Finding{finding("AR802", lint.SeverityWarning, deploy, 3, "short")}, false, 84, map[string]string{"trigger-quality": VerdictWarn}},
		{"an error twin fails its dimension", []lint.Finding{finding("AR201", lint.SeverityError, deploy, 9, "broken link")}, false, 84, map[string]string{"body-accuracy": VerdictFail}},
		{"info findings do not lower a verdict", []lint.Finding{finding("AR805", lint.SeverityInfo, deploy, 1, "n")}, false, 100, map[string]string{"body-structure": VerdictPass}},
		{"unrelated codes are ignored", []lint.Finding{finding("AR903", lint.SeverityWarning, deploy, 1, "n")}, false, 100, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			in := Input{Rubric: builtin(t), Items: []Item{deploy}, Findings: tt.findings}

			// Act
			res := Run(in)

			// Assert
			require.Len(t, res.Items, 1)
			r := res.Items[0]
			require.Equal(t, StatusScored, r.Status)
			require.NotNil(t, r.Score)
			assert.Equal(t, tt.want, *r.Score)
			for _, d := range r.Dimensions {
				if want, ok := tt.verdicts[d.ID]; ok {
					assert.Equal(t, want, d.Verdict, d.ID)
				}
			}
		})
	}
}

func TestScoreIgnoresDimensionsWithoutATwin(t *testing.T) {
	// Arrange
	res := Run(Input{Rubric: builtin(t), Items: []Item{skill("a", "Use when asked to do a")}})

	// Act
	var notScored []string
	for _, d := range res.Items[0].Dimensions {
		if d.Status == DimNotScored {
			notScored = append(notScored, d.ID)
		}
	}

	// Assert
	assert.Equal(t, []string{"instruction-conflict"}, notScored)
}

func TestPrefilters(t *testing.T) {
	leak := skill("leak", "Use when asked to rotate keys")
	hidden := skill("hidden", "Use when asked to do hidden things")
	imported := skill("imported", "Use when asked to import")
	imported.Owned = false
	internal := skill("internal-x", "Use when asked to run internals")
	agent := Item{ID: "rule:r", Kind: KindRule, Name: "r", Path: ".ai-rulez/rules/r.md", Owned: true}
	unreadable := skill("broken", "x")
	unreadable.ReadError = "permission denied"
	items := []Item{leak, hidden, imported, internal, agent, unreadable, skill("ok", "Use when asked to be fine")}
	findings := []lint.Finding{
		finding("AR001", lint.SeverityError, leak, 5, "key"),
		finding("AR002", lint.SeverityError, hidden, 2, "zero width"),
	}
	cfg := &config.ReviewConfig{Exclude: []string{"internal-*"}}

	tests := []struct {
		name    string
		imports bool
		want    map[string]string
	}{
		{"defaults", false, map[string]string{
			"skill:leak": StatusWithheld, "skill:hidden": StatusWithheld, "skill:imported": StatusSkipped,
			"skill:internal-x": StatusExcluded, "rule:r": StatusSkipped, "skill:broken": StatusSkipped, "skill:ok": StatusScored,
		}},
		{"include imports", true, map[string]string{"skill:imported": StatusScored}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res := Run(Input{Rubric: builtin(t), Items: items, Findings: findings, Config: cfg, IncludeImports: tt.imports})

			// Assert
			got := map[string]ItemResult{}
			for _, r := range res.Items {
				got[r.ID] = r
			}
			for id, status := range tt.want {
				assert.Equal(t, status, got[id].Status, id)
			}
			assert.Nil(t, got["skill:leak"].Score)
			assert.Contains(t, got["skill:leak"].Reason, "AR001")
		})
	}
}

func TestEvidenceIncludesSkillResources(t *testing.T) {
	// Arrange
	s := skill("deploy", "Use when asked to deploy")
	res := lint.Finding{Code: "AR001", Severity: lint.SeverityError, File: ".ai-rulez/skills/deploy/references/notes.md", Line: 4, Message: "key"}

	// Act
	out := Run(Input{Rubric: builtin(t), Items: []Item{s}, Findings: []lint.Finding{res}})

	// Assert
	assert.Equal(t, StatusWithheld, out.Items[0].Status)
}

func TestSelectedAndUnmatched(t *testing.T) {
	items := []Item{skill("a", "x"), skill("b", "y")}
	tests := []struct {
		name      string
		selectors []string
		want      []string
		missing   []string
	}{
		{"none selects all", nil, []string{"skill:a", "skill:b"}, nil},
		{"by id", []string{"skill:a"}, []string{"skill:a"}, nil},
		{"by name", []string{"b"}, []string{"skill:b"}, nil},
		{"by directory path", []string{".ai-rulez/skills/a"}, []string{"skill:a"}, nil},
		{"by file path", []string{".ai-rulez/skills/b/SKILL.md"}, []string{"skill:b"}, nil},
		{"typo", []string{"skill:zz"}, nil, []string{"skill:zz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, it := range Selected(items, tt.selectors) {
				got = append(got, it.ID)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.missing, Unmatched(items, tt.selectors))
		})
	}
}

func TestFindingsAreAdvisoryAndStable(t *testing.T) {
	// Arrange
	deploy := skill("deploy", "Helps")
	in := Input{Rubric: builtin(t), Items: []Item{deploy}, Findings: []lint.Finding{
		finding("AR004", lint.SeverityWarning, deploy, 8, "override"),
	}}
	first := Run(in).Findings(in.Rubric)
	in.Findings[0].Line = 40
	second := Run(in).Findings(in.Rubric)

	// Assert
	require.Len(t, first, 1)
	assert.Equal(t, lint.CodeReviewInjectionIntent, first[0].Code)
	assert.Equal(t, "warning", first[0].Severity)
	assert.Equal(t, first[0].Fingerprint, second[0].Fingerprint, "a line move keeps the fingerprint")
	assert.Equal(t, 40, second[0].Line)
}

func TestPlanEstimate(t *testing.T) {
	rb := builtin(t)
	a := skill("a", "Deploy the service to staging")
	b := skill("b", "Deploy the service to production")
	leak := skill("leak", "Use when rotating keys")
	findings := []lint.Finding{finding("AR001", lint.SeverityError, leak, 3, "key")}
	res := Run(Input{Rubric: rb, Items: []Item{a, b, leak}, Findings: findings})
	prices := func(in, out int) (float64, bool) { return float64(in)*1/1e6 + float64(out)*5/1e6, true }
	base := EstimateInput{Rubric: rb, Results: res, Content: config.ReviewContentDescriptions, Model: "m", Host: "h", Prices: prices, MaxCostUSD: 1, MaxCalls: 100}

	t.Run("manifest lists sizes and hashes but never content", func(t *testing.T) {
		// Act
		est := Plan(base)

		// Assert
		assert.False(t, est.Sent)
		require.Len(t, est.Items, 2)
		assert.Equal(t, []string{"skill:leak"}, est.Withheld)
		for _, it := range est.Items {
			assert.Len(t, it.SHA256, 64)
			assert.Len(t, it.Calls, 2)
			assert.Positive(t, it.Bytes)
		}
		raw, err := json.Marshal(est)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "Deploy the service")
		assert.True(t, est.CostKnown)
		assert.Less(t, *est.CostMinUSD, *est.CostMaxUSD)
		assert.Empty(t, est.Refused)
		assert.Equal(t, 4, est.Totals.CallsMin)
		assert.Equal(t, 12, est.Totals.CallsMax)
	})

	t.Run("a withheld item contributes nothing", func(t *testing.T) {
		est := Plan(base)
		for _, it := range est.Items {
			assert.NotEqual(t, "skill:leak", it.ID)
		}
	})

	t.Run("plan is deterministic", func(t *testing.T) {
		first, err1 := json.Marshal(Plan(base))
		second, err2 := json.Marshal(Plan(base))
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, string(first), string(second))
	})

	t.Run("a cost over the cap is refused", func(t *testing.T) {
		in := base
		in.MaxCostUSD = 0.0001
		est := Plan(in)
		require.NotEmpty(t, est.Refused)
		assert.Contains(t, est.Refused[0], "exceeds the cap")
	})

	t.Run("calls over the cap are refused", func(t *testing.T) {
		in := base
		in.MaxCalls = 3
		est := Plan(in)
		require.NotEmpty(t, est.Refused)
		assert.Contains(t, est.Refused[0], "4 calls exceed the cap of 3")
	})

	t.Run("an unknown price with a cost cap is refused", func(t *testing.T) {
		in := base
		in.Prices = func(int, int) (float64, bool) { return 0, false }
		est := Plan(in)
		assert.False(t, est.CostKnown)
		require.NotEmpty(t, est.Refused)
		assert.Contains(t, est.Refused[0], "no price for model m")
	})

	t.Run("no model configured is not a refusal", func(t *testing.T) {
		in := base
		in.Prices, in.Model = nil, ""
		est := Plan(in)
		assert.False(t, est.CostKnown)
		assert.Empty(t, est.Refused)
	})

	t.Run("a twin error pre-empts its dimension", func(t *testing.T) {
		pre := Run(Input{Rubric: rb, Items: []Item{a, b}, Findings: []lint.Finding{finding("AR201", lint.SeverityError, a, 2, "x")}})
		in := base
		in.Results = pre
		est := Plan(in)
		var dims []string
		for _, it := range est.Items {
			if it.ID == "skill:a" {
				dims = it.Calls[0].Dimensions
			}
		}
		assert.NotContains(t, dims, "body-accuracy")
		assert.Contains(t, dims, "trigger-quality")
	})

	t.Run("prompts only with show-prompt, with a fenced nonce block", func(t *testing.T) {
		assert.Empty(t, Plan(base).Prompts)
		in := base
		in.ShowPrompt = true
		est := Plan(in)
		require.NotEmpty(t, est.Prompts)
		assert.Regexp(t, `<<<DATA-<nonce:0{24}> item=skill:a>>>`, est.Prompts[0].User)
		assert.Regexp(t, `<<<END-DATA-<nonce:0{24}>>>>`, est.Prompts[0].User)
	})
}

func TestFullContentTruncatesLongBodies(t *testing.T) {
	// Arrange
	rb := builtin(t)
	long := skill("long", "Use when asked for the long thing")
	long.Body = strings.Repeat("line of text\n", 5000)
	res := Run(Input{Rubric: rb, Items: []Item{long}})

	// Act
	full := Plan(EstimateInput{Rubric: rb, Results: res, Content: config.ReviewContentFull, ShowPrompt: true, MaxCalls: 10})
	desc := Plan(EstimateInput{Rubric: rb, Results: res, Content: config.ReviewContentDescriptions, MaxCalls: 10})

	// Assert
	require.Len(t, full.Items, 1)
	assert.True(t, full.Items[0].Truncated)
	assert.Contains(t, full.Prompts[0].User, "[... truncated ...]")
	assert.Greater(t, full.Items[0].Bytes, desc.Items[0].Bytes)
	assert.Less(t, full.Items[0].Bytes, len(long.Body), "the body is cut to the rubric's token limit")
}

func TestShortlistOrdersBySimilarityThenID(t *testing.T) {
	// Arrange
	it := skill("me", "deploy the service to production")
	pool := []Item{skill("z", "unrelated cooking recipe"), skill("close", "deploy the service to staging"), skill("a", "unrelated cooking recipe"), it}

	// Act
	got := shortlist(pool, it, 2)

	// Assert
	require.Len(t, got, 2)
	assert.Equal(t, "skill:close", got[0].ID)
	assert.Equal(t, "skill:a", got[1].ID)
}

func TestReportFormats(t *testing.T) {
	// Arrange
	rb := builtin(t)
	deploy := skill("deploy", "Helps")
	leak := skill("leak", "Use when rotating keys")
	res := Run(Input{Rubric: rb, Items: []Item{deploy, leak}, Findings: []lint.Finding{
		finding("AR004", lint.SeverityWarning, deploy, 8, "override"),
		finding("AR001", lint.SeverityError, leak, 3, "key"),
	}})
	report := NewReport(rb, res, nil)

	t.Run("json carries the rubric weights and formula and no model call", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, report.WriteJSON(&buf))
		var got map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
		assert.Equal(t, ReportSchema, got["schema"])
		rubric := got["rubric"].(map[string]any)
		assert.Equal(t, Formula, rubric["formula"])
		assert.Len(t, rubric["dimensions"], 7)
		run := got["run"].(map[string]any)
		assert.Equal(t, true, run["offline"])
		assert.EqualValues(t, 0, run["calls"])
		items := got["items"].([]any)
		assert.Nil(t, items[1].(map[string]any)["score"], "a withheld item has a null score")
	})

	t.Run("text names the status of every item", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, report.WriteText(&buf))
		out := buf.String()
		assert.Contains(t, out, "skill:deploy")
		assert.Contains(t, out, "AR9G4")
		assert.Contains(t, out, "withheld: AR001")
		assert.Contains(t, out, "2 items: 1 scored, 1 withheld")
	})

	t.Run("sarif has rules, fingerprints and advisory results", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, WriteSARIF(&buf, rb, res, "9.9.9"))
		var doc struct {
			Version string `json:"version"`
			Runs    []struct {
				Tool struct {
					Driver struct {
						Version string `json:"version"`
						Rules   []struct{ ID string }
					}
				}
				Results []struct {
					RuleID              string            `json:"ruleId"`
					Level               string            `json:"level"`
					PartialFingerprints map[string]string `json:"partialFingerprints"`
					Properties          map[string]any    `json:"properties"`
				}
			}
		}
		require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
		assert.Equal(t, "2.1.0", doc.Version)
		run := doc.Runs[0]
		assert.Equal(t, "9.9.9", run.Tool.Driver.Version)
		require.Len(t, run.Results, 2)
		for _, r := range run.Results {
			assert.NotEmpty(t, r.PartialFingerprints["aiRulezReviewFingerprint/v1"])
			assert.Equal(t, true, r.Properties["advisory"])
		}
		assert.Len(t, run.Tool.Driver.Rules, 2)
	})

	t.Run("an empty run still has arrays", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, WriteSARIF(&buf, rb, &Results{}, ""))
		assert.Contains(t, buf.String(), `"results": []`)
		assert.Contains(t, buf.String(), `"rules": []`)
	})
}

func TestCollectReadsOwnedItemsOnly(t *testing.T) {
	// Arrange
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	skillDir := filepath.Join(cfgDir, "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	path := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nname: deploy\ndescription: Use when deploying\nallowed-tools: Read\n---\nbody\n"), 0o644))
	cfg := &config.Config{ConfigDir: cfgDir, Content: &config.ContentTree{
		Skills: []config.ContentFile{
			{Name: "deploy", Path: path},
			{Name: "ext", Path: "/elsewhere/ext/SKILL.md"},
		},
	}}

	// Act
	items := Collect(cfg, func(abs string) string {
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		return filepath.ToSlash(rel)
	})

	// Assert
	require.Len(t, items, 2)
	byID := map[string]Item{items[0].ID: items[0], items[1].ID: items[1]}
	d := byID["skill:deploy"]
	assert.True(t, d.Owned)
	assert.Equal(t, ".ai-rulez/skills/deploy/SKILL.md", d.Path)
	assert.Equal(t, "Use when deploying", d.Description)
	assert.Equal(t, []string{"allowed-tools", "description", "name"}, d.Keys)
	assert.Equal(t, "body\n", d.Body)
	assert.True(t, strings.HasPrefix(d.Digest, "sha256:"))
	ext := byID["skill:ext"]
	assert.False(t, ext.Owned)
	assert.Empty(t, ext.Description, "imported content is never read")
}

func TestRunWithholdsWhatHoldsASecretOrHiddenTextWithoutLintFindings(t *testing.T) {
	// Arrange: no lint finding at all, as when [lint] ignore, severity or an inline ignore removed them
	secret := skill("secret", "Use when rotating keys")
	secret.Body = "token = ghp_" + strings.Repeat("a1B2c3D4e5", 4) + "\n"
	hiddenDesc := skill("hidden-desc", "Use when asked\u200bto do things")
	hiddenRaw := skill("hidden-raw", "Use when asked to do things")
	hiddenRaw.Raw = "---\nname: x\nnote: a\u202eb\n---\nbody\n"
	clean := skill("clean", "Use when asked to be fine")
	clean.Raw = "---\nname: clean\n---\nbody of clean\n"

	// Act
	res := Run(Input{Rubric: builtin(t), Items: []Item{secret, hiddenDesc, hiddenRaw, clean}})

	// Assert
	got := map[string]ItemResult{}
	for _, r := range res.Items {
		got[r.ID] = r
	}
	for _, id := range []string{"skill:secret", "skill:hidden-desc", "skill:hidden-raw"} {
		assert.Equal(t, StatusWithheld, got[id].Status, id)
		assert.Nil(t, got[id].Score, id)
	}
	assert.Equal(t, StatusScored, got["skill:clean"].Status)
	assert.Contains(t, got["skill:secret"].Reason, "secret")
}

func TestPlanHashesTheDataOfEveryCall(t *testing.T) {
	// Arrange: two pools that differ only in a sibling description, which only the contextual call sends
	rb := builtin(t)
	it := skill("me", "Deploy the service to production")
	planFor := func(sibling string) *Estimate {
		res := Run(Input{Rubric: rb, Items: []Item{it, skill("sib", sibling)}})
		return Plan(EstimateInput{Rubric: rb, Results: res, Content: config.ReviewContentDescriptions, MaxCalls: 10})
	}

	// Act
	a, b := planFor("Deploy the service to staging"), planFor("Roll back the service in staging")

	// Assert
	require.NotEmpty(t, a.Items)
	require.Len(t, a.Items[0].Calls, 2)
	assert.NotEqual(t, a.Items[0].SHA256, b.Items[0].SHA256)
}

func TestPlanRefusesWhenThePolicyForbidsModelCalls(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging"), skill("b", "Deploy it to production")}})

	// Act
	est := Plan(EstimateInput{Rubric: rb, Results: res, Content: config.ReviewContentDescriptions, MaxCalls: 100, PolicyForbidsLLM: true})

	// Assert
	require.NotEmpty(t, est.Refused)
	assert.Contains(t, strings.Join(est.Refused, " "), "policy")
}

func TestTruncateBodyIsLinearOnInvalidUTF8(t *testing.T) {
	// Arrange
	const maxTokens = 1 << 19
	body := strings.Repeat("\xff", 8<<20)
	start := time.Now()

	// Act
	got, cut := truncateBody(body, maxTokens)

	// Assert
	assert.True(t, cut)
	assert.True(t, utf8.ValidString(got))
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestTruncateBodyKeepsWholeRunesAtTheCut(t *testing.T) {
	// Arrange
	body := strings.Repeat("\u00e9", 40000)

	// Act
	got, cut := truncateBody(body, 100)

	// Assert
	assert.True(t, cut)
	assert.True(t, utf8.ValidString(got))
	assert.NotContains(t, got, "\ufffd")
}
