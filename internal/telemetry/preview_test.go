package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const previewLog = `{"ts":"2026-10-01T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","invocation":"tool","harness":"claude","session":"raw-session-id-from-v1"}
{"v":2,"ts":"2026-10-02T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","hash":"blake3:aa11","session":"5b1c0e9a7d3f2a64","invocation":"tool","harness":"claude","outcome":"loaded"}
{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","hash":"blake3:aa11","digest":"sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05","digest_scheme":"ai-rulez/skill/v1","event_id":"0123456789abcdef","session":"5b1c0e9a7d3f2a64","invocation":"slash","harness":"claude","outcome":"loaded","role":"backend"}
{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","event_id":"0123456789abcdef","invocation":"slash","harness":"claude"}
{"v":3,"ts":"2026-10-03T09:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","resource":true,"invocation":"mcp","harness":"claude","served":true}
{"v":1,"event":"item_event","ts":"2026-10-04T08:00:00Z","event_id":"aaaaaaaaaaaaaaa1","kind":"rule","id":"atomic-commits","source":"hook","outcome":"loaded","harness":"claude","path":".claude/rules/atomic-commits.md"}
{"v":1,"event":"item_event","ts":"2026-10-04T08:00:01Z","event_id":"aaaaaaaaaaaaaaa2","kind":"bogus","id":"x","source":"hook","outcome":"loaded"}
{"v":1,"event":"feedback","ts":"2026-10-04T08:00:02Z","id":"deploy","kind":"misled"}
not json
`

func writeLog(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestReadLogEvents(t *testing.T) {
	// Arrange
	path := writeLog(t, previewLog)

	// Act
	read, err := ReadLogEvents(path)

	// Assert
	require.NoError(t, err)
	require.Len(t, read.Events, 4, "v1, v2, v3 skill lines and one item event; duplicates, resources, feedback and junk are left out")
	assert.Equal(t, 1, read.Rejected, "an item event with an unknown kind is counted")
	assert.Empty(t, read.Events[0].Session, "a raw v1 session id is never exported")
	assert.Equal(t, "5b1c0e9a7d3f2a64", read.Events[1].Session)
	assert.Equal(t, "blake3:aa11", read.Events[1].Digest)
	assert.Equal(t, "sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05", read.Events[2].Digest, "the canonical digest wins over the v2 hash")
	assert.Equal(t, "0123456789abcdef", read.Events[2].EventID)
	for _, event := range read.Events {
		assert.Regexp(t, `^[0-9a-f]{16}$`, event.EventID)
	}
	again, err := ReadLogEvents(path)
	require.NoError(t, err)
	assert.Equal(t, read.Events, again.Events, "ids derived for old lines are stable")
}

func TestReadLogEvents_RefusesASymlinkedLog(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.jsonl")
	require.NoError(t, os.WriteFile(real, []byte(previewLog), 0o600))
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}

	_, err := ReadLogEvents(link)

	assert.Error(t, err)
}

func TestPlan_GoldenAndDeterministic(t *testing.T) {
	read, err := ReadLogEvents(writeLog(t, previewLog))
	require.NoError(t, err)
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "5.0.0"}

	first, err := en.Plan(read.Events, 0, true)
	require.NoError(t, err)
	second, err := en.Plan(read.Events, 0, true)
	require.NoError(t, err)

	require.Len(t, first, 2)
	assert.Equal(t, PathLogs, first[0].Path)
	assert.Equal(t, PathMetrics, first[1].Path)
	assert.Equal(t, 4, first[0].Events)
	assert.Positive(t, first[0].GzipBytes)
	assert.Equal(t, first, second)
	golden(t, "preview_logs.golden.json", first[0].Body)
	golden(t, "preview_metrics.golden.json", first[1].Body)
}

func TestPlan_BatchesAndSkipsMetricsOnRequest(t *testing.T) {
	events := sampleEvents()
	en := Encoder{}

	plan, err := en.Plan(events, 3, false)

	require.NoError(t, err)
	require.Len(t, plan, 2)
	assert.Equal(t, 3, plan[0].Events)
	assert.Equal(t, 1, plan[1].Events)
	for _, request := range plan {
		assert.Equal(t, PathLogs, request.Path)
	}
	empty, err := en.Plan(nil, 0, true)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// A line can carry any key; only allowlisted fields may reach an encoded body.
func TestPlan_NeverEmitsFieldsOutsideTheAllowlist(t *testing.T) {
	const secret = "TOPSECRETVALUE"
	log := `{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","invocation":"tool","harness":"claude","outcome":"loaded",` +
		`"prompt":"` + secret + `","cwd":"/home/` + secret + `","command":"` + secret + `","env":{"K":"` + secret + `"},"file_path":"/x/` + secret + `","tool_input":"` + secret + `"}
{"v":1,"event":"item_event","ts":"2026-10-04T08:00:00Z","event_id":"aaaaaaaaaaaaaaa1","kind":"rule","id":"r","source":"hook","outcome":"loaded","prompt":"` + secret + `","cwd":"` + secret + `"}
`
	read, err := ReadLogEvents(writeLog(t, log))
	require.NoError(t, err)
	require.Len(t, read.Events, 2)

	plan, err := (&Encoder{IncludePaths: true, IncludeSession: true}).Plan(read.Events, 0, true)

	require.NoError(t, err)
	for _, request := range plan {
		assert.NotContains(t, string(request.Body), secret, request.Path)
	}
}

func TestEncoderFields(t *testing.T) {
	closed := Encoder{}
	open := Encoder{IncludePaths: true, IncludeSession: true}

	exported, withheld := closed.Fields()
	allExported, noneWithheld := open.Fields()

	assert.Contains(t, exported, "event.name")
	assert.Contains(t, exported, "ai_rulez.item.id")
	assert.ElementsMatch(t, []string{"ai_rulez.item.path", "ai_rulez.session"}, withheld)
	assert.Contains(t, allExported, "ai_rulez.session")
	assert.Empty(t, noneWithheld)
	assert.Len(t, allExported, len(Allowlist)+1)
}

func TestDisplayURL(t *testing.T) {
	tests := []struct {
		name, endpoint, want string
	}{
		{"plain", "https://collector.example.org:4318", "https://collector.example.org:4318/v1/logs"},
		{"trailing slash and path", "https://c.example.org/otlp/", "https://c.example.org/otlp/v1/logs"},
		{"credentials and query are dropped", "https://user:pw@c.example.org/x?token=abc#f", "https://c.example.org/x/v1/logs"},
		{"not a url", "::::", ""},
		{"no host", "/just/a/path", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayURL(tt.endpoint, PathLogs)

			assert.Equal(t, tt.want, got)
			assert.False(t, strings.Contains(got, "pw") || strings.Contains(got, "token"))
		})
	}
}

func TestSettingsEncoderMatchesTheExporterWiring(t *testing.T) {
	s := Settings{ServiceName: "svc", Resource: map[string]string{"team": "x"}, IncludePaths: true}

	en := s.Encoder("5.0.0")

	assert.Equal(t, Encoder{ServiceName: "svc", ServiceVersion: "5.0.0", Resource: map[string]string{"team": "x"}, IncludePaths: true}, en)
}
