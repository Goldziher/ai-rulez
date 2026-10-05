package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixedNow = time.Date(2026, 10, 5, 9, 12, 44, 0, time.UTC)

func fixedClock() time.Time { return fixedNow }

// golden compares got with testdata/<name>; UPDATE_GOLDEN=1 rewrites it.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(path, got, 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden %s (UPDATE_GOLDEN=1 go test)", name)
	assert.Equal(t, string(want), string(got))
}

func sampleEvents() []Event {
	return []Event{
		{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:40Z", EventID: "aaaaaaaaaaaaaaa1", Kind: KindRule, ID: "atomic-commits", Path: ".claude/rules/atomic-commits.md",
			Digest: "blake3:0699fc6b2f01aa11223344", Source: SourceHook, Harness: "claude", Role: "backend", Session: "5b1c0e9a7d3f2a64", Outcome: OutcomeLoaded, LoadReason: ReasonSessionStart, MemoryType: "Project"},
		{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:41Z", EventID: "aaaaaaaaaaaaaaa2", Kind: KindRule, ID: "atomic-commits", Source: SourceHook, Harness: "claude", Role: "backend", Session: "5b1c0e9a7d3f2a64", Outcome: OutcomeLoaded, LoadReason: ReasonPathGlob},
		{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:42Z", EventID: "aaaaaaaaaaaaaaa3", Kind: KindSkill, ID: "deploy-staging", Digest: "blake3:deadbeefdeadbeef00", Source: SourceMCP, Harness: "claude", Served: true, Outcome: OutcomeLoaded, LoadReason: ReasonRead},
		{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:43Z", EventID: "aaaaaaaaaaaaaaa4", Kind: KindAgent, ID: "code-reviewer", Source: SourceHook, Harness: "claude", Session: "5b1c0e9a7d3f2a64", Outcome: OutcomeUsed, LoadReason: ReasonSubagentStop, DurationMS: 2500},
	}
}
