package telemetry

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lockDigest = "sha256:262d72131a4f8b0c9e5d7a31f6c2b84e0d9a1c7e5f3b6a8d2c4e6f8091a3b5c7"

func f(v float64) *float64 { return &v }

func sampleEvalResults() []EvalResult {
	return []EvalResult{
		{SkillID: "deploy-staging", LockDigest: lockDigest, Harness: "claude", Date: "2026-10-05", PassRate: 0.62, TriggerPrecision: f(0.9), TriggerRecall: f(0.75), AblationDelta: f(0.1)},
		{SkillID: "release-notes", LockDigest: lockDigest[:7] + strings.Repeat("a", 64), Harness: "claude", Date: "2026-10-04T08:00:00Z", PassRate: 0.95, AblationDelta: f(-0.2)},
	}
}

func TestEvalEvents_AreDeterministicAndNormalized(t *testing.T) {
	// Arrange
	results := sampleEvalResults()

	// Act
	first, rejected := EvalEvents(results)
	second, _ := EvalEvents(results)

	// Assert
	require.Zero(t, rejected)
	require.Len(t, first, 2)
	assert.Equal(t, first, second, "the same results give the same events, ids included")
	e := first[0]
	assert.Equal(t, EventEvalResult, e.Name)
	assert.Equal(t, KindSkill, e.Kind)
	assert.Equal(t, SourceCLI, e.Source)
	assert.Equal(t, "2026-10-05T00:00:00Z", e.Time)
	assert.Equal(t, usage.DigestSchemeSkill, e.DigestScheme)
	assert.Len(t, e.EventID, 16)
	assert.NotEqual(t, first[0].EventID, first[1].EventID)
	assert.Equal(t, "2026-10-04T08:00:00Z", first[1].Time)

	changed := results[0]
	changed.PassRate = 0.63
	other, _ := EvalEvents([]EvalResult{changed})
	assert.NotEqual(t, e.EventID, other[0].EventID, "a different score is a different event")
}

func TestEvalEvents_RejectsInvalidResultsAndDropsBadOptionals(t *testing.T) {
	tests := []struct {
		name         string
		result       EvalResult
		wantRejected bool
		check        func(*testing.T, Event)
	}{
		{name: "pass rate above one", result: EvalResult{SkillID: "a", PassRate: 1.5}, wantRejected: true},
		{name: "negative pass rate", result: EvalResult{SkillID: "a", PassRate: -0.1}, wantRejected: true},
		{name: "invalid skill id", result: EvalResult{SkillID: "has space", PassRate: 0.5}, wantRejected: true},
		{name: "out of range optionals are dropped", result: EvalResult{SkillID: "a", PassRate: 0.5, TriggerPrecision: f(2), TriggerRecall: f(-1), AblationDelta: f(3)}, check: func(t *testing.T, e Event) {
			assert.Nil(t, e.TriggerPrecision)
			assert.Nil(t, e.TriggerRecall)
			assert.Nil(t, e.AblationDelta)
		}},
		{name: "a malformed digest is dropped with its scheme", result: EvalResult{SkillID: "a", PassRate: 0.5, LockDigest: "sha256:short"}, check: func(t *testing.T, e Event) {
			assert.Empty(t, e.Digest)
			assert.Empty(t, e.DigestScheme)
		}},
		{name: "no digest joins by id only", result: EvalResult{SkillID: "a", PassRate: 0.5}, check: func(t *testing.T, e Event) {
			assert.Empty(t, e.Digest)
			assert.Empty(t, e.DigestScheme)
		}},
		{name: "an unparseable date leaves the time empty", result: EvalResult{SkillID: "a", PassRate: 0.5, Date: "last tuesday"}, check: func(t *testing.T, e Event) { assert.Empty(t, e.Time) }},
		{name: "a bad harness is dropped", result: EvalResult{SkillID: "a", PassRate: 0.5, Harness: "Bad Harness!"}, check: func(t *testing.T, e Event) { assert.Empty(t, e.Harness) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, rejected := EvalEvents([]EvalResult{tt.result})

			if tt.wantRejected {
				assert.Equal(t, 1, rejected)
				assert.Empty(t, events)
				return
			}
			require.Len(t, events, 1)
			tt.check(t, events[0])
		})
	}
}

func TestNormalize_AnEvalResultCarriesNothingAboutALoad(t *testing.T) {
	e := Event{Name: EventEvalResult, Kind: KindSkill, ID: "a", PassRate: f(0.5), Path: ".claude/x", Session: "5b1c0e9a7d3f2a64", Role: "ops", Outcome: OutcomeUsed, LoadReason: "read", Served: true, DurationMS: 9, MemoryType: "User", Source: SourceHook}

	require.NoError(t, e.Normalize())

	assert.Empty(t, e.Path)
	assert.Empty(t, e.Session)
	assert.Empty(t, e.Role)
	assert.Empty(t, e.Outcome)
	assert.Empty(t, e.LoadReason)
	assert.False(t, e.Served)
	assert.Zero(t, e.DurationMS)
	assert.Empty(t, e.MemoryType)
	assert.Equal(t, SourceCLI, e.Source)

	rule := Event{Name: EventEvalResult, Kind: KindRule, ID: "a", PassRate: f(0.5)}
	assert.Error(t, rule.Normalize(), "an eval result is about a skill")
}

func TestNormalize_AnItemEventCannotCarryScores(t *testing.T) {
	e := Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded, PassRate: f(0.5), AblationDelta: f(0.1)}

	require.NoError(t, e.Normalize())

	assert.Nil(t, e.PassRate)
	assert.Nil(t, e.AblationDelta)
	assert.Equal(t, EventItem, e.Name)
}

func TestEncodeEvalResult_LogsAndGaugesGolden(t *testing.T) {
	events, _ := EvalEvents(sampleEvalResults())
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "5.0.0"}

	logs, err := en.EncodeLogs(events, fixedNow)
	require.NoError(t, err)
	metrics, err := en.EncodeMetrics(events, fixedNow)
	require.NoError(t, err)

	golden(t, "eval_logs.golden.json", pretty(t, logs))
	golden(t, "eval_metrics.golden.json", pretty(t, metrics))
	assert.NotContains(t, string(logs), `"ai_rulez.served"`, "a result is not a load")
	assert.NotContains(t, string(metrics), "ai_rulez.item.loads", "a result is not a load")
	assert.NotContains(t, string(metrics), lockDigest, "the digest is a log attribute, never a metric label")
}

func TestEvalGauges_LatestResultPerSkillAndHarnessWins(t *testing.T) {
	older := EvalResult{SkillID: "a", Harness: "claude", Date: "2026-10-01", PassRate: 0.4}
	newer := EvalResult{SkillID: "a", Harness: "claude", Date: "2026-10-03", PassRate: 0.9}
	other := EvalResult{SkillID: "a", Harness: "codex", Date: "2026-10-02", PassRate: 0.7}
	events, _ := EvalEvents([]EvalResult{newer, older, other})

	body, err := (&Encoder{}).EncodeMetrics(events, fixedNow)

	require.NoError(t, err)
	assert.Contains(t, string(body), `"asDouble":0.9`)
	assert.Contains(t, string(body), `"asDouble":0.7`)
	assert.NotContains(t, string(body), `"asDouble":0.4`, "an older result in the same batch must not win")
}

func TestEncodeMixedBatch_ItemMetricsAreUnchangedByEvalEvents(t *testing.T) {
	items := sampleEvents()
	evalEvents, _ := EvalEvents(sampleEvalResults())
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "4.30.0", IncludeSession: true, IncludePaths: true}

	itemsOnly, err := en.EncodeMetrics(items, fixedNow)
	require.NoError(t, err)
	mixed, err := en.EncodeMetrics(append(append([]Event(nil), items...), evalEvents...), fixedNow)
	require.NoError(t, err)

	assert.Contains(t, string(mixed), MetricLoads)
	assert.Contains(t, string(mixed), MetricEvalPassRate)
	assert.NotContains(t, string(itemsOnly), "ai_rulez.skill.eval")
}

func TestSpool_UnsentDropsQueuedAndDeliveredEvals(t *testing.T) {
	spool := &Spool{Dir: t.TempDir()}
	events, _ := EvalEvents(sampleEvalResults())
	require.NoError(t, spool.Append(&events[0]))

	fresh, err := spool.Unsent(events)
	require.NoError(t, err)
	assert.Equal(t, events[1:], fresh)

	require.NoError(t, spool.MarkSent([]string{events[1].EventID}, ""))
	fresh, err = spool.Unsent(events)
	require.NoError(t, err)
	assert.Empty(t, fresh)
}
