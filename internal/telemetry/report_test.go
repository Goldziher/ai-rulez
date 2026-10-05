package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadEvent(kind, id, session, reason string) Event {
	return Event{Kind: kind, ID: id, Session: session, Outcome: OutcomeLoaded, LoadReason: reason, Time: "2026-10-05T09:00:00Z", Source: SourceHook}
}

func TestBuildItemsReport_NeverLoadedMedianAndReasons(t *testing.T) {
	events := []Event{
		// session A loads two rules, B one, C none (only CLAUDE.md), D three.
		loadEvent(KindRule, "a", "aaaaaaaaaaaaaaaa", ReasonSessionStart), loadEvent(KindRule, "b", "aaaaaaaaaaaaaaaa", ReasonPathGlob),
		loadEvent(KindRule, "a", "bbbbbbbbbbbbbbbb", ReasonSessionStart),
		loadEvent(KindContext, "CLAUDE.md", "cccccccccccccccc", ReasonSessionStart),
		loadEvent(KindRule, "a", "dddddddddddddddd", ReasonSessionStart), loadEvent(KindRule, "b", "dddddddddddddddd", ReasonPathGlob), loadEvent(KindRule, "a", "dddddddddddddddd", ReasonCompact),
		loadEvent(KindRule, "handwritten", "dddddddddddddddd", ReasonInclude),
		{Kind: KindAgent, ID: "code-reviewer", Outcome: OutcomeLoaded, Time: "2026-10-05T10:00:00Z", Session: "aaaaaaaaaaaaaaaa"},
		{Kind: KindAgent, ID: "code-reviewer", Outcome: OutcomeUsed, Time: "2026-10-05T10:01:00Z", Session: "aaaaaaaaaaaaaaaa", DurationMS: 60000},
		loadEvent(KindRule, ListID, "", ReasonList),
		loadEvent(KindRule, "nosession", "", ReasonSessionStart),
	}
	catalog := &Catalog{Rules: []string{"a", "b", "c-never"}, Agents: []string{"code-reviewer", "idle-agent"}, Contexts: []string{"CLAUDE.md", "AGENTS.md"}}
	scores := map[string]usage.EvalSummary{"c-never": {PassRate: 0.5}}
	r := BuildItemsReport(events, catalog, scores)

	require.Len(t, r.Rules.Never, 1)
	assert.Equal(t, "c-never", r.Rules.Never[0].ID)
	require.NotNil(t, r.Rules.Never[0].Eval, "eval results join by id when present")
	assert.InDelta(t, 0.5, r.Rules.Never[0].Eval.PassRate, 0)

	ids := func(rows []ItemRow) []string {
		var out []string
		for _, row := range rows {
			out = append(out, row.ID)
		}
		return out
	}
	assert.Equal(t, []string{"a", "b", "handwritten", "nosession"}, ids(r.Rules.Loaded), "ordered by loads, the listing id is never a row")
	assert.Equal(t, []string{"handwritten", "nosession"}, ids(r.Rules.Unknown))
	assert.Equal(t, []string{"idle-agent"}, ids(r.Agents.Never))
	assert.Equal(t, []string{"AGENTS.md"}, ids(r.Contexts.Never))

	// Per session distinct rules: A=2, B=1, C=0, D=3 -> median 1.5.
	assert.Equal(t, 4, r.RulesPerSession.Sessions)
	assert.InDelta(t, 1.5, r.RulesPerSession.Median, 0)
	assert.InDelta(t, 1.5, r.RulesPerSession.Mean, 0)
	assert.Equal(t, 3, r.RulesPerSession.Max)
	assert.Equal(t, 2, r.EventsWithoutSession, "the listing and the sessionless load")

	assert.Equal(t, 4, r.LoadReasons[KindRule][ReasonSessionStart])
	assert.Equal(t, 1, r.LoadReasons[KindRule][ReasonCompact])
	assert.Equal(t, 1, r.LoadReasons[KindRule][ReasonList])

	a := r.Rules.Loaded[0]
	assert.Equal(t, 4, a.Loads)
	assert.Equal(t, 3, a.Sessions)
	agent := r.Agents.Loaded[0]
	assert.Equal(t, 1, agent.Used)
	assert.Equal(t, "2026-10-05T10:01:00Z", agent.LastSeen)

	raw, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"never_loaded"`)
}

func TestBuildItemsReport_NoCatalogMeansNoNeverList(t *testing.T) {
	r := BuildItemsReport([]Event{loadEvent(KindRule, "a", "aaaaaaaaaaaaaaaa", ReasonSessionStart)}, nil, nil)
	assert.Empty(t, r.Rules.Never)
	assert.Empty(t, r.Rules.Unknown, "without a manifest nothing can be unknown")
	assert.Len(t, r.Rules.Loaded, 1)
}

func TestCatalogFromFiles(t *testing.T) {
	c := CatalogFromFiles([]string{".claude/rules/z.md", ".claude/rules/go/a.md", ".claude/agents/reviewer.md", "CLAUDE.md", "services/api/AGENTS.md", ".agents/skills/x/SKILL.md"})
	assert.Equal(t, []string{"go/a", "z"}, c.Rules)
	assert.Equal(t, []string{"reviewer"}, c.Agents)
	assert.Equal(t, []string{"CLAUDE.md", "services/api/AGENTS.md"}, c.Contexts)
}

func TestDiagnose_ShowsHostOnlyAndNoSecrets(t *testing.T) {
	s := Resolve(Layers{
		User: &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://collector.example.org:4318/secret-path", HeadersEnv: []string{"OTLP_HEADERS"}}, Getenv: env("OTLP_HEADERS", "authorization=Bearer topsecret"),
	})
	dir := t.TempDir()
	spool := &Spool{Dir: dir}
	require.NoError(t, spool.Append(newTestEvent(1)))
	report := Diagnose(&s, dir, env("OTLP_HEADERS", "authorization=Bearer topsecret"))
	assert.True(t, report.Export)
	assert.Equal(t, "collector.example.org:4318", report.EndpointHost)
	assert.Equal(t, []string{"OTLP_HEADERS"}, report.HeadersSet)
	assert.Equal(t, 1, report.Buffer.Events)

	var text bytes.Buffer
	report.Render(&text)
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	for _, out := range []string{text.String(), string(raw)} {
		assert.NotContains(t, out, "topsecret")
		assert.NotContains(t, out, "/v1/")
		assert.NotContains(t, out, "secret-path")
	}
	assert.Contains(t, text.String(), "otlp export:         on")
}

func TestDiagnose_ExplainsWhyExportIsOff(t *testing.T) {
	s := Resolve(Layers{Repo: &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://evil.example.com"}, Getenv: env()})
	report := Diagnose(&s, t.TempDir(), env())
	assert.False(t, report.Export)
	assert.True(t, report.Recording)
	joined := strings.Join(report.Blockers, "|")
	assert.Contains(t, joined, "allow_network")
	assert.Contains(t, joined, "otlp_endpoint")
	assert.Contains(t, report.IgnoredRepoKeys, "otlp_endpoint")
	var text bytes.Buffer
	report.Render(&text)
	assert.Contains(t, text.String(), "ignored in repository config")

	killed := Resolve(Layers{Getenv: env("DO_NOT_TRACK", "1")})
	assert.Contains(t, strings.Join(Diagnose(&killed, t.TempDir(), env()).Blockers, "|"), "DO_NOT_TRACK")
}

func TestFlushViaPipelineClose(t *testing.T) {
	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	s := Resolve(Layers{User: &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "http://127.0.0.1:1"}, Getenv: env()})
	s.Endpoint = srv.URL
	p, _, _ := pipelineFor(t, s)
	require.NoError(t, p.Record(context.Background(), Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}))
	require.NoError(t, p.Close(context.Background()))
	assert.Equal(t, []string{"/v1/logs", "/v1/metrics"}, c.paths())
}
