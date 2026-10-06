package govview

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

func signalsDoc() *CatalogDocV2 {
	return &CatalogDocV2{Items: []CatalogItemV2{
		{Ref: "skill/-/deploy", Kind: "skill", ID: "deploy", Digest: "sha256:new"},
		{Ref: "skill/-/fresh", Kind: "skill", ID: "fresh", Digest: "sha256:same"},
		{Ref: "skill/a/dup", Kind: "skill", ID: "dup"},
		{Ref: "skill/b/dup", Kind: "skill", ID: "dup"},
		{Ref: "rule/-/deploy", Kind: "rule", ID: "deploy"},
	}}
}

func TestAttachSignals_EvalIsAllowlistedAndMarksStale(t *testing.T) {
	// Arrange
	delta := 0.5
	store := evals.NewStore()
	store.Put(evals.SkillRecord{ID: "deploy", LockDigest: "sha256:old", Date: "2026-10-01", Runner: "claude", Model: "secret-model",
		Passing: true, Score: evals.SkillScore{Scored: 4, PassRate: 0.75, AblationDelta: &delta}})
	store.Put(evals.SkillRecord{ID: "fresh", LockDigest: "sha256:same", Runner: "r", Passing: false, Score: evals.SkillScore{Scored: 2}})
	doc := signalsDoc()

	// Act
	attachSignals(doc, &CatalogOptions{WithEval: true, Eval: store})

	// Assert
	require.NotNil(t, doc.Items[0].Eval)
	assert.True(t, doc.Items[0].Eval.Stale, "the lock digest changed since the run")
	assert.Equal(t, 0.75, doc.Items[0].Eval.PassRate)
	assert.Equal(t, 4, doc.Items[0].Eval.Cases)
	require.NotNil(t, doc.Items[0].Eval.AblationDelta)
	assert.Equal(t, "2026-10-01", doc.Items[0].Eval.Date)
	require.NotNil(t, doc.Items[1].Eval)
	assert.False(t, doc.Items[1].Eval.Stale)
	assert.Nil(t, doc.Items[4].Eval, "only skills carry eval results")
	data, err := json.Marshal(doc.Items[0])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-model")
	assert.NotContains(t, string(data), "claude")
}

func TestAttachSignals_MissingEvalResultsAreSaidNotInvented(t *testing.T) {
	// Arrange
	doc := signalsDoc()

	// Act
	attachSignals(doc, &CatalogOptions{WithEval: true, EvalNote: "eval-results.json not found"})

	// Assert
	assert.Contains(t, doc.Notes, "eval-results.json not found: eval fields are omitted")
	for _, it := range doc.Items {
		assert.Nil(t, it.Eval)
	}
}

func TestAttachSignals_UsageIsCountedByIDAndAmbiguousIDsAreSkipped(t *testing.T) {
	// Arrange
	entries := []usage.Entry{
		{ID: "deploy", Time: "2026-10-01T10:00:00Z", Session: "abc", Harness: "claude", Outcome: "used"},
		{ID: "deploy", Time: "2026-10-04T09:00:00Z"},
		{ID: "deploy", Time: "2026-10-09T09:00:00Z", Resource: true},
		{ID: "dup", Time: "2026-10-02T00:00:00Z"},
		{ID: "gone", Time: "2026-10-02T00:00:00Z"},
	}
	doc := signalsDoc()

	// Act
	attachSignals(doc, &CatalogOptions{WithUsage: true, Usage: entries, UsageLoaded: true})

	// Assert
	require.NotNil(t, doc.Items[0].Usage)
	assert.Equal(t, ItemUsage{Invocations: 2, LastSeen: "2026-10-04"}, *doc.Items[0].Usage)
	assert.Nil(t, doc.Items[1].Usage, "an unused skill has no usage section")
	assert.Nil(t, doc.Items[2].Usage)
	assert.Nil(t, doc.Items[3].Usage)
	assert.Nil(t, doc.Items[4].Usage, "only skills carry usage")
	assert.Contains(t, doc.Notes[len(doc.Notes)-1], "share an id")
	data, err := json.Marshal(doc.Items[0])
	require.NoError(t, err)
	for _, leak := range []string{"abc", "claude", "used"} {
		assert.NotContains(t, string(data), leak)
	}
}

func TestAttachSignals_NothingRequestedChangesNothing(t *testing.T) {
	// Arrange
	doc := signalsDoc()

	// Act
	attachSignals(doc, &CatalogOptions{})

	// Assert
	assert.Empty(t, doc.Notes)
	assert.Nil(t, doc.Items[0].Eval)
	assert.Nil(t, doc.Items[0].Usage)
}
