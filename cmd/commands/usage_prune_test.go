package commands

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pruneCmdNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func resetPruneFlags(t *testing.T) {
	t.Helper()
	clear := func() { usagePruneKeepDays, usagePruneDryRun, usagePruneIgnoreCursor, usageLog = -1, false, false, "" }
	clear()
	t.Cleanup(clear)
}

func skillLine(daysAgo int, id string) string {
	ts := pruneCmdNow.AddDate(0, 0, -daysAgo).Format(time.RFC3339)
	return fmt.Sprintf(`{"v":3,"ts":%q,"event":"skill_invoked","skill":%q,"id":%q,"event_id":"%016x"}`+"\n", ts, id, id, daysAgo)
}

func TestUsagePrune_RemovesOldLinesAndReportsWhatItKept(t *testing.T) {
	resetPruneFlags(t)
	env := setupTelemetry(t, "", "")
	appendRawLog(t, env.log, skillLine(100, "old")+skillLine(2, "fresh"))
	usagePruneKeepDays = 30
	var out bytes.Buffer

	require.NoError(t, runUsagePrune(&out, pruneCmdNow))

	assert.Contains(t, out.String(), "removed 1 of 2 lines older than 30 days")
	data, err := os.ReadFile(env.log)
	require.NoError(t, err)
	assert.Equal(t, skillLine(2, "fresh"), string(data))
}

func TestUsagePrune_KeepsWhatTheCursorHasNotPassed(t *testing.T) {
	resetPruneFlags(t)
	env := setupTelemetry(t, "", "")
	appendRawLog(t, env.log, skillLine(100, "old")+skillLine(90, "older"))
	spool := &telemetry.Spool{Dir: telemetry.LocalDir(env.root, ".ai-rulez")}
	require.NoError(t, spool.PlaceCursor(env.log, true)) // nothing exported yet
	usagePruneKeepDays = 30
	var out bytes.Buffer

	require.NoError(t, runUsagePrune(&out, pruneCmdNow))

	assert.Contains(t, out.String(), "removed 0 of 2")
	assert.Contains(t, out.String(), "2 old ones not yet exported")
	data, err := os.ReadFile(env.log)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(data), "\n"))
}

func TestUsagePrune_DryRunAndBadInvocations(t *testing.T) {
	resetPruneFlags(t)
	env := setupTelemetry(t, "", "")
	appendRawLog(t, env.log, skillLine(100, "old"))
	usagePruneKeepDays, usagePruneDryRun = 30, true
	var out bytes.Buffer

	require.NoError(t, runUsagePrune(&out, pruneCmdNow))

	assert.Contains(t, out.String(), "would remove 1 of 1")
	assert.Equal(t, 1, strings.Count(readFileString(t, env.log), "\n"))

	usagePruneKeepDays = -5
	assert.Error(t, runUsagePrune(&out, pruneCmdNow), "a negative keep is refused")
	usagePruneKeepDays, usageLog = 30, env.root+"/nope.jsonl"
	assert.Error(t, runUsagePrune(&out, pruneCmdNow), "a missing log is an error")
}

func TestUsagePrune_AnotherLogIsPrunedByAgeAlone(t *testing.T) {
	resetPruneFlags(t)
	env := setupTelemetry(t, "", "")
	recordSkillLoads(t, env, "own")
	spool := &telemetry.Spool{Dir: telemetry.LocalDir(env.root, ".ai-rulez")}
	require.NoError(t, spool.PlaceCursor(env.log, false))
	other := env.root + "/other.jsonl"
	appendRawLog(t, other, skillLine(100, "old")+skillLine(1, "new"))
	usagePruneKeepDays, usageLog = 30, other

	require.NoError(t, runUsagePrune(&bytes.Buffer{}, pruneCmdNow))

	assert.Equal(t, skillLine(1, "new"), readFileString(t, other))
}
