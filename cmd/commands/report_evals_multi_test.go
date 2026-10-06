package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiLogProject has two skills with signed eval records at their canonical digests.
func multiLogProject(t *testing.T) (root string, lock map[string]string) {
	t.Helper()
	resetEnrichFlags(t)
	root = t.TempDir()
	t.Chdir(root)
	lock = map[string]string{}
	store := evals.NewStore()
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, ".ai-rulez", "skills", name)
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody\n"), 0o600))
		digest, err := contentlock.SkillDirDigest(dir)
		require.NoError(t, err)
		lock[name] = digest
		store.Put(evals.SkillRecord{ID: name, Digest: "sha256:" + name, LockDigest: digest, Passing: true, Score: evals.SkillScore{Scored: 4, PassRate: 1}})
	}
	signStore(t, store)
	require.NoError(t, store.Save(filepath.Join(root, ".ai-rulez", evals.StoreFileName)))
	return root, lock
}

func writeUsageLines(t *testing.T, path string, entries ...usage.Entry) {
	t.Helper()
	var sb strings.Builder
	for _, e := range entries {
		e.Time, e.Event, e.Skill = "2026-10-05T09:00:00Z", usage.EventSkillInvoked, e.ID
		data, err := json.Marshal(e)
		require.NoError(t, err)
		sb.Write(append(data, '\n'))
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(sb.String()), 0o600))
}

type evalsJSON struct {
	Skills  []evals.RankRow `json:"skills"`
	Sources map[string]int  `json:"usage_sources"`
}

func runEvalsJSON(t *testing.T) evalsJSON {
	t.Helper()
	reportEvalsFlags.json = true
	var out bytes.Buffer
	require.NoError(t, runReportEvals(&out))
	var doc evalsJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	return doc
}

func rowFor(t *testing.T, doc evalsJSON, id string) evals.RankRow {
	t.Helper()
	for _, row := range doc.Skills {
		if row.ID == id {
			return row
		}
	}
	require.Failf(t, "no row", "skill %s", id)
	return evals.RankRow{}
}

func TestReportEvals_MergesSeveralUsageLogsByEventID(t *testing.T) {
	// Arrange: log b repeats alpha's event e1 (a copy of the same machine's log) and adds e3.
	root, lock := multiLogProject(t)
	a, b := filepath.Join(root, "a", "usage.jsonl"), filepath.Join(root, "b", "usage.jsonl")
	canonical := func(skill, id string) usage.Entry {
		return usage.Entry{ID: skill, Digest: lock[skill], DigestScheme: usage.DigestSchemeSkill, EventID: id}
	}
	stale := usage.Entry{ID: "beta", Digest: "sha256:" + strings.Repeat("0", 64), DigestScheme: usage.DigestSchemeSkill, EventID: "00000000000000e2"}
	writeUsageLines(t, a, canonical("alpha", "00000000000000e1"), stale)
	writeUsageLines(t, b, canonical("alpha", "00000000000000e1"), canonical("alpha", "00000000000000e3"))
	reportEvalsFlags.usageLogs = []string{a, b}

	// Act
	doc := runEvalsJSON(t)

	// Assert
	assert.Equal(t, map[string]int{"logs": 2, "events": 3, "duplicates": 1}, doc.Sources)
	alpha, beta := rowFor(t, doc, "alpha"), rowFor(t, doc, "beta")
	require.NotNil(t, alpha.Uses)
	assert.Equal(t, 2, *alpha.Uses, "the repeated event counts once")
	assert.Equal(t, evals.JoinExact, alpha.Join)
	assert.Equal(t, evals.JoinStale, beta.Join)

	reportEvalsFlags.json = false
	var out bytes.Buffer
	require.NoError(t, runReportEvals(&out))
	assert.Contains(t, out.String(), "Usage: 2 logs, 3 events (1 duplicates removed by event id)")
}

func TestReportEvals_UsageLogAliasAndUsageCombine(t *testing.T) {
	root, lock := multiLogProject(t)
	a, b := filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")
	writeUsageLines(t, a, usage.Entry{ID: "alpha", Digest: lock["alpha"], DigestScheme: usage.DigestSchemeSkill, EventID: "00000000000000e1"})
	writeUsageLines(t, b, usage.Entry{ID: "alpha", EventID: "00000000000000e2"}, usage.Entry{ID: "alpha", Resource: true, EventID: "00000000000000e3"})
	reportEvalsFlags.usageLogs, reportEvalsFlags.usageLogsAlias = []string{a}, []string{b}

	doc := runEvalsJSON(t)

	assert.Equal(t, 2, *rowFor(t, doc, "alpha").Uses, "the supporting-file load is not a use")
	assert.Equal(t, evals.JoinExact, rowFor(t, doc, "alpha").Join, "one exact use is enough; the legacy one is by id only")
}

func TestReportEvals_FromOTLPReadsAnExportAndKeepsTheDigestScheme(t *testing.T) {
	// Arrange: an export holding a canonical-digest use of alpha and a served-digest use of beta.
	root, lock := multiLogProject(t)
	events := []telemetry.Event{
		{Time: "2026-10-05T09:00:00Z", EventID: "00000000000000e1", Kind: telemetry.KindSkill, ID: "alpha", Digest: lock["alpha"], DigestScheme: usage.DigestSchemeSkill, Source: telemetry.SourceHook, Outcome: telemetry.OutcomeLoaded},
		{Time: "2026-10-05T09:00:01Z", EventID: "00000000000000e2", Kind: telemetry.KindSkill, ID: "beta", Digest: lock["beta"], DigestScheme: usage.DigestSchemeServed, Source: telemetry.SourceMCP, Served: true, Outcome: telemetry.OutcomeLoaded},
	}
	for i := range events {
		require.NoError(t, events[i].Normalize())
	}
	file, err := (&telemetry.Encoder{}).EncodeFile(events)
	require.NoError(t, err)
	path := filepath.Join(root, "export.ndjson")
	require.NoError(t, os.WriteFile(path, file.Data, 0o600))
	reportEvalsFlags.usageLogs, reportEvalsFlags.fromOTLP = []string{path}, true

	// Act
	doc := runEvalsJSON(t)

	// Assert: the served digest has the same bytes as the lock digest here but another scheme,
	// so it joins by id only and never counts as exact.
	assert.Equal(t, evals.JoinExact, rowFor(t, doc, "alpha").Join)
	assert.Equal(t, evals.JoinLegacy, rowFor(t, doc, "beta").Join)
	assert.Equal(t, 2, doc.Sources["events"])
}

func TestReportEvals_ANamedUsageLogMustExistAndOTLPErrorsAreReported(t *testing.T) {
	root, _ := multiLogProject(t)
	reportEvalsFlags.usageLogs = []string{filepath.Join(root, "missing.jsonl")}
	assert.Error(t, runReportEvals(&bytes.Buffer{}))

	reportEvalsFlags.fromOTLP = true
	assert.Error(t, runReportEvals(&bytes.Buffer{}))
}
