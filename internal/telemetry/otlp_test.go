package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pretty(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.Indent(&buf, raw, "", "  "))
	buf.WriteByte('\n')
	return buf.Bytes()
}

func TestEncodeLogsGolden(t *testing.T) {
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "4.30.0"}
	body, err := en.EncodeLogs(sampleEvents(), fixedNow)
	require.NoError(t, err)
	golden(t, "logs.golden.json", pretty(t, body))
}

func TestEncodeLogsGatedAttributes(t *testing.T) {
	events := sampleEvents()
	closed, err := (&Encoder{}).EncodeLogs(events, fixedNow)
	require.NoError(t, err)
	assert.NotContains(t, string(closed), "ai_rulez.item.path", "paths are off by default")
	assert.NotContains(t, string(closed), "ai_rulez.session", "the session hash is off by default")
	assert.NotContains(t, string(closed), ".claude/rules")
	assert.NotContains(t, string(closed), "5b1c0e9a7d3f2a64")

	open, err := (&Encoder{IncludePaths: true, IncludeSession: true}).EncodeLogs(events, fixedNow)
	require.NoError(t, err)
	assert.Contains(t, string(open), ".claude/rules/atomic-commits.md")
	assert.Contains(t, string(open), "5b1c0e9a7d3f2a64")
}

func TestEncodeMetricsGolden(t *testing.T) {
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "4.30.0", IncludeSession: true, IncludePaths: true}
	body, err := en.EncodeMetrics(sampleEvents(), fixedNow)
	require.NoError(t, err)
	golden(t, "metrics.golden.json", pretty(t, body))
	// Session and path never become metric labels, even when opened for logs.
	assert.NotContains(t, string(body), "5b1c0e9a7d3f2a64")
	assert.NotContains(t, string(body), ".claude/rules")
	// The full digest is a log attribute only; metrics carry the short form.
	assert.NotContains(t, string(body), "0699fc6b2f01aa11223344")
	assert.Contains(t, string(body), "0699fc6b2f01")
}

func TestEncodeMetrics_NilWhenNothingToCount(t *testing.T) {
	body, err := (&Encoder{}).EncodeMetrics(nil, fixedNow)
	require.NoError(t, err)
	assert.Nil(t, body)
}

func TestEncoders_NeverEmitUnlistedText(t *testing.T) {
	// Leak property: text put in fields the allowlist does not name cannot appear.
	e := sampleEvents()[0]
	e.Path = ".claude/rules/secret-prompt.md"
	logs, err := (&Encoder{}).EncodeLogs([]Event{e}, fixedNow)
	require.NoError(t, err)
	metrics, err := (&Encoder{}).EncodeMetrics([]Event{e}, fixedNow)
	require.NoError(t, err)
	for _, body := range [][]byte{logs, metrics} {
		assert.False(t, strings.Contains(string(body), "secret-prompt"))
	}
}

func TestSampled_IsPerSessionAndDeterministic(t *testing.T) {
	in, out := 0, 0
	for i := 0; i < 2000; i++ {
		e := Event{Session: HashForTest(i)}
		if Sampled(0.5, &e) {
			in++
			assert.True(t, Sampled(0.5, &e), "the same session must always decide the same way")
		} else {
			out++
		}
	}
	assert.InDelta(t, 1000, in, 150)
	assert.Positive(t, out)
	e := Event{EventID: "x"}
	assert.True(t, Sampled(1, &e))
	assert.False(t, Sampled(0, &e))
}

// HashForTest makes a 16-hex session for sampling tests.
func HashForTest(i int) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 16)
	v := uint64(i)*2654435761 + 12345
	for n := range out {
		out[n] = hexDigits[v&0xf]
		v = v>>4 ^ v*6364136223846793005 + 1442695040888963407
	}
	return string(out)
}

func TestEncoder_ResourceAttributesAreSortedAndAfterTheFixedOnes(t *testing.T) {
	// Arrange
	en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "5.0.0", Resource: map[string]string{"team": "platform", "deployment.environment": "prod"}}

	// Act
	body, err := en.EncodeLogs(sampleEvents()[:1], fixedNow)

	// Assert
	require.NoError(t, err)
	var doc struct {
		ResourceLogs []struct {
			Resource otlpResource `json:"resource"`
		} `json:"resourceLogs"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	var keys []string
	for _, a := range doc.ResourceLogs[0].Resource.Attributes {
		keys = append(keys, a.Key)
	}
	assert.Equal(t, []string{"service.name", "service.version", "ai_rulez.schema_version", "deployment.environment", "team"}, keys)
}

func TestEncodeLogs_NeverWritesAnOverflowedTimestamp(t *testing.T) {
	// An event without a time and no observation time (a log of old events, no clock).
	events := []Event{{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}}

	body, err := (&Encoder{}).EncodeLogs(events, time.Time{})

	require.NoError(t, err)
	var doc struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					Time     string `json:"timeUnixNano"`
					Observed string `json:"observedTimeUnixNano"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	record := doc.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	assert.Equal(t, "0", record.Time, "0 is OTLP's unknown time")
	assert.Equal(t, "0", record.Observed)
}
