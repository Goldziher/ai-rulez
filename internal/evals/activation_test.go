package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWilson(t *testing.T) {
	tests := []struct {
		name     string
		k, n     int
		low, hi  float64
		wantZero bool
	}{
		{name: "no trials has no interval", k: 0, n: 0, wantZero: true},
		{name: "one of ten", k: 1, n: 10, low: 0.0179, hi: 0.4042},
		{name: "all of five", k: 5, n: 5, low: 0.5655, hi: 1},
		{name: "none of five", k: 0, n: 5, low: 0, hi: 0.4345},
		{name: "three of five", k: 3, n: 5, low: 0.2307, hi: 0.8824},
		{name: "one of one", k: 1, n: 1, low: 0.2065, hi: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Wilson(tt.k, tt.n)

			// Assert
			if tt.wantZero {
				assert.Equal(t, Interval{}, got)
				return
			}
			assert.InDelta(t, tt.low, got.Low, 0.001)
			assert.InDelta(t, tt.hi, got.High, 0.001)
		})
	}
}

func TestRequireCapability(t *testing.T) {
	// Arrange
	plain := goodRunner()

	// Act and Assert
	err := RequireCapability(plain, CapabilityActivation)
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not support activation mode")
	assert.ErrorContains(t, err, `"fake"`)
	assert.Error(t, RequireCapability(nil, CapabilityActivation))
	assert.NoError(t, RequireCapability(&capableRunner{fakeRunner: plain}, CapabilityActivation))
}

type capableRunner struct{ *fakeRunner }

func (capableRunner) Capabilities() []string { return []string{CapabilityActivation} }

const activationCases = `cases:
  - id: fires
    prompt: Deploy the billing service to staging
    expect_trigger: true
    near_miss:
      - Write release notes for version two
    files:
      - {path: a.txt, content: x}
    assertions:
      - {type: contains, value: ok}
  - id: stolen
    prompt: Write the changelog for this release
    expect_trigger: true
  - id: unrelated
    prompt: bake a sourdough loaf
    expect_trigger: false
`

// activationProject has two root skills that compete and one in a domain.
func activationProject(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	writeSkill(t, cfg, "deploy-staging", "---\nname: deploy-staging\ndescription: Deploy a service to the staging environment\ntriggers: [deploy to staging]\nkeywords: [staging, deploy]\n---\nbody\n", activationCases)
	writeSkill(t, cfg, "release-notes", "---\nname: release-notes\ndescription: Write release notes and changelog entries for a release\nkeywords: [changelog, release]\n---\nbody\n", "")
	domainSkill := filepath.Join(cfg, "domains", "web", "skills", "css-tuning")
	require.NoError(t, os.MkdirAll(domainSkill, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(domainSkill, "SKILL.md"), []byte("---\nname: css-tuning\ndescription: Tune css for the web\n---\nbody\n"), 0o600))
	return cfg
}

func runActivation(t *testing.T, opts *ActivationOptions) *ActivationReport {
	t.Helper()
	report, err := RunActivationRetrieval(context.Background(), opts)
	require.NoError(t, err)
	return report
}

func TestRunActivationRetrieval_ScoresPromptsAndConfusion(t *testing.T) {
	// Arrange
	cfg := activationProject(t)

	// Act
	report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}, Date: "2026-10-06"})

	// Assert
	assert.Equal(t, ModeActivation, report.Mode)
	assert.Equal(t, SurfaceRetrieval, report.Surface)
	assert.Equal(t, ScopeDomain, report.Scope)
	assert.Equal(t, ActivationSchemaVersion, report.SchemaVersion)
	assert.True(t, report.Failed, "the stolen prompt fails the skill")
	assert.Zero(t, report.Cost.ActualUSD)
	require.Len(t, report.Skills, 1)
	skill := report.Skills[0]
	assert.Equal(t, RunRan, skill.Status)
	assert.False(t, skill.Passing)
	assert.Equal(t, []string{"deploy-staging", "release-notes"}, skill.Competing, "scope domain: the root skills, not the web domain's")
	assert.Equal(t, 1, skill.Ignored, "the case with fixtures and assertions is run on its prompt only")

	byCase := map[string]ActivationPrompt{}
	for _, p := range skill.Prompts {
		byCase[p.Case] = p
	}
	require.Len(t, byCase, 4)
	assert.Equal(t, PromptPassed, byCase["fires"].Status)
	assert.Equal(t, 1.0, byCase["fires"].Rate)
	require.NotNil(t, byCase["fires"].Rank)
	assert.Equal(t, 1, *byCase["fires"].Rank)
	assert.Equal(t, PromptFailed, byCase["stolen"].Status)
	assert.Equal(t, "release-notes", byCase["stolen"].Winner)
	assert.Equal(t, PromptPassed, byCase["fires.near-miss-1"].Status)
	assert.True(t, byCase["fires.near-miss-1"].NearMiss)
	assert.Equal(t, activationNone, byCase["unrelated"].Winner)
	assert.Equal(t, PromptPassed, byCase["unrelated"].Status)

	require.NotNil(t, skill.Recall)
	assert.InDelta(t, 0.5, skill.Recall.Value, 1e-9)
	assert.Equal(t, 2, skill.Recall.N)
	assert.Less(t, skill.Recall.Interval.Low, 0.5)
	assert.Greater(t, skill.Recall.Interval.High, 0.5)
	require.NotNil(t, skill.Precision)
	assert.InDelta(t, 1.0, skill.Precision.Value, 1e-9)
	require.NotNil(t, skill.FalseActivation)
	assert.Zero(t, skill.FalseActivation.Value)
	require.NotNil(t, skill.RecallAt1)
	assert.InDelta(t, 0.5, *skill.RecallAt1, 1e-9)
	assert.Equal(t, []StolenBy{{Skill: "release-notes", Prompts: 1, Share: 0.5}}, skill.StolenBy)
	assert.Equal(t, map[string]map[string]int{"deploy-staging": {"deploy-staging": 1, "release-notes": 1}}, report.Confusion)
}

func TestRunActivationRetrieval_ScopeAllAddsTheDomainSkills(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		want  []string
	}{
		{"domain is the default", "", []string{"deploy-staging", "release-notes"}},
		{"all", ScopeAll, []string{"css-tuning", "deploy-staging", "release-notes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := activationProject(t)

			// Act
			report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}, Scope: tt.scope})

			// Assert
			assert.Equal(t, tt.want, report.Skills[0].Competing)
		})
	}
}

func TestRunActivationRetrieval_SiblingChangeChangesTheSetDigest(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	before := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}}).Skills[0]

	// Act
	writeSkill(t, cfg, "release-notes", "---\nname: other-thing\ndescription: Something else entirely\n---\nbody\n", "")
	after := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}}).Skills[0]

	// Assert
	assert.Equal(t, before.Digest, after.Digest, "the skill itself did not change")
	assert.NotEqual(t, before.SetDigest, after.SetDigest, "its competition did")
	assert.Empty(t, after.StolenBy, "and the changelog prompt is no longer stolen")
}

func TestRunActivationRetrieval_IsDeterministicAndFree(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	var first, second bytes.Buffer

	// Act
	require.NoError(t, runActivation(t, &ActivationOptions{ConfigDir: cfg}).Write(&first, FormatJSON))
	require.NoError(t, runActivation(t, &ActivationOptions{ConfigDir: cfg}).Write(&second, FormatJSON))

	// Assert
	assert.Equal(t, first.String(), second.String())
	var doc map[string]any
	require.NoError(t, json.Unmarshal(first.Bytes(), &doc))
	assert.Equal(t, "activation", doc["mode"])
}

func TestRunActivationRetrieval_StatusesAndOptions(t *testing.T) {
	t.Run("a skill without cases and an unchanged skill are not measured", func(t *testing.T) {
		// Arrange
		cfg := activationProject(t)

		// Act
		report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Changed: map[string]bool{"release-notes": true}})

		// Assert
		statuses := map[string]string{}
		for _, s := range report.Skills {
			statuses[s.ID] = s.Status
		}
		assert.Equal(t, map[string]string{"css-tuning": RunNotChanged, "deploy-staging": RunNotChanged, "release-notes": RunNoCases}, statuses)
		assert.False(t, report.Failed)
	})
	t.Run("invalid cases fail the run", func(t *testing.T) {
		// Arrange
		cfg := activationProject(t)
		writeSkill(t, cfg, "broken", "---\nname: broken\ndescription: b\n---\n", "cases:\n  - {id: x, prompt: hi}\n")

		// Act
		report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"broken"}})

		// Assert
		assert.Equal(t, RunInvalid, report.Skills[0].Status)
		assert.True(t, report.Failed)
	})
	t.Run("unknown scope, skill and threshold are rejected", func(t *testing.T) {
		cfg := activationProject(t)
		bad := 2.0
		for _, opts := range []*ActivationOptions{
			{ConfigDir: cfg, Scope: "galaxy"}, {ConfigDir: cfg, Skills: []string{"nope"}}, {ConfigDir: cfg, PassThreshold: &bad},
		} {
			_, err := RunActivationRetrieval(context.Background(), opts)
			assert.Error(t, err)
		}
	})
	t.Run("a lone skill warns that confusion cannot be measured", func(t *testing.T) {
		// Arrange
		cfg := t.TempDir()
		writeSkill(t, cfg, "solo", "---\nname: solo\ndescription: Deploy to staging\n---\n", "cases:\n  - {id: a, prompt: deploy to staging, expect_trigger: true}\n")

		// Act
		report := runActivation(t, &ActivationOptions{ConfigDir: cfg})

		// Assert
		assert.True(t, report.Skills[0].Passing)
		require.NotEmpty(t, report.Skills[0].Warnings)
		assert.Contains(t, report.Skills[0].Warnings[0], "stolen trigger cannot be measured")
	})
	t.Run("a threshold below one lets some prompts fail", func(t *testing.T) {
		// Arrange
		cfg := activationProject(t)
		half := 0.5

		// Act
		report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}, PassThreshold: &half})

		// Assert: 3 of 4 prompts pass
		assert.True(t, report.Skills[0].Passing)
		assert.False(t, report.Failed)
	})
}

func TestRunActivationRetrieval_RecordsRatesWithoutClobberingARun(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	store := NewStore()
	store.Put(SkillRecord{ID: "deploy-staging", Runner: "fake", CacheKey: "k", Score: SkillScore{Scored: 3, PassRate: 1}, Passing: true})

	// Act
	runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}, Date: "2026-10-06", Store: store})
	rec, ok := store.Get("deploy-staging")
	require.True(t, ok)
	require.NotNil(t, rec.Activation)
	activation, ranBefore := *rec.Activation, *rec
	store.Put(SkillRecord{ID: "deploy-staging", Runner: "fake", CacheKey: "k2", Score: SkillScore{Scored: 3, PassRate: 1}, Passing: true})
	again, _ := store.Get("deploy-staging")

	// Assert
	assert.Equal(t, 3, ranBefore.Score.Scored, "the case-run result is untouched")
	assert.Equal(t, "k", ranBefore.CacheKey)
	assert.Equal(t, SurfaceRetrieval, activation.Surface)
	assert.Equal(t, 2, activation.Positives)
	assert.Equal(t, 2, activation.Negatives)
	require.NotNil(t, activation.Recall)
	assert.InDelta(t, 0.5, *activation.Recall, 1e-9)
	assert.NotEmpty(t, activation.SetDigest)
	require.NotNil(t, again.Activation, "a later full run keeps the activation block")
	assert.Equal(t, activation, *again.Activation)
}

func TestStore_ActivationOnlyRecordIsNotAnEvalResult(t *testing.T) {
	// Arrange
	store := NewStore()
	store.SetKey([]byte("test-key-0123456789abcdef"))

	// Act
	store.PutActivation("solo", "sha256:x", &ActivationRecord{Surface: SurfaceRetrieval})
	data, err := store.Marshal()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), StoreFileName)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	loaded, err := LoadStoreKeyed(path, []byte("test-key-0123456789abcdef"))
	require.NoError(t, err)

	// Assert
	rec, ok := loaded.Get("solo")
	require.True(t, ok)
	assert.True(t, rec.Verified(), "the activation block is signed with the user's key")
	assert.False(t, rec.HasRun())
	require.NotNil(t, rec.Activation)
}

func TestActivationReport_Markdown(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Skills: []string{"deploy-staging"}})
	var out bytes.Buffer

	// Act
	require.NoError(t, report.Write(&out, FormatMarkdown))

	// Assert
	text := out.String()
	assert.Contains(t, text, "# Skill activation results")
	assert.Contains(t, text, "| deploy-staging | ran, failing |")
	assert.Contains(t, text, "`release-notes` ranked first on 1 of the positive prompts (50%)")
	assert.Contains(t, text, "- deploy-staging: deploy-staging 1, release-notes 1")
	assert.Error(t, report.Write(&out, FormatJUnit))
}

func TestActivationReport_MatchesItsSchema(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "eval-activation.v1.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	cfg := activationProject(t)
	writeSkill(t, cfg, "broken", "---\nname: broken\ndescription: b\n---\n", "cases:\n  - {id: x, prompt: hi}\n")
	report := runActivation(t, &ActivationOptions{ConfigDir: cfg, Date: "2026-10-06"})
	var out bytes.Buffer

	// Act
	require.NoError(t, report.Write(&out, FormatJSON))

	// Assert
	result := compiled.Validate(out.Bytes())
	if !result.IsValid() {
		details, _ := json.Marshal(result.Errors) //nolint:errcheck // diagnostics only
		t.Fatalf("report does not match the schema: %s\n%s", details, out.String())
	}
	assert.False(t, compiled.Validate([]byte(`{"schema_version":2,"mode":"activation"}`)).IsValid(), "the schema rejects a malformed document")
	assert.Contains(t, out.String(), `"status": "invalid"`)
	assert.Contains(t, out.String(), `"status": "no-cases"`)
}
