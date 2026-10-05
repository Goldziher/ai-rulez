package evals

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rec(id string, score SkillScore, digest string, passing bool) SkillRecord {
	return SkillRecord{ID: id, Digest: digest, Passing: passing, Date: "2026-10-01", Score: score}
}

func TestRank_ActionsAndOrdering(t *testing.T) {
	store := NewStore()
	store.Put(rec("healthy", SkillScore{Scored: 5, PassRate: 1, TriggerPrecision: fp(1), TriggerRecall: fp(1), AblationDelta: fp(0.4)}, "sha256:h", true))
	store.Put(rec("low-pass", SkillScore{Scored: 5, PassRate: 0.4, TriggerPrecision: fp(1), TriggerRecall: fp(1), AblationDelta: fp(0.2)}, "sha256:l", false))
	store.Put(rec("noisy", SkillScore{Scored: 5, PassRate: 0.9, TriggerPrecision: fp(0.5), TriggerRecall: fp(0.6), AblationDelta: fp(0.2)}, "sha256:n", true))
	store.Put(rec("harmful", SkillScore{Scored: 5, PassRate: 0.9, AblationDelta: fp(-0.2)}, "sha256:x", true))
	store.Put(rec("edited", SkillScore{Scored: 5, PassRate: 1, AblationDelta: fp(0.3)}, "sha256:old", true))
	store.Put(rec("idle-useless", SkillScore{Scored: 5, PassRate: 1, AblationDelta: fp(0.0)}, "sha256:i", true))
	store.Put(rec("idle-valuable", SkillScore{Scored: 5, PassRate: 1, AblationDelta: fp(0.5)}, "sha256:v", true))

	rows := Rank(RankInput{
		Store: store,
		Skills: []RankSkill{
			{ID: "healthy", Digest: "sha256:h", SkillTokens: 100},
			{ID: "low-pass", Digest: "sha256:l", SkillTokens: 100},
			{ID: "noisy", Digest: "sha256:n", SkillTokens: 100},
			{ID: "harmful", Digest: "sha256:x"},
			{ID: "edited", Digest: "sha256:new"},
			{ID: "idle-useless", Digest: "sha256:i", SkillTokens: 900},
			{ID: "idle-valuable", Digest: "sha256:v"},
			{ID: "idle-unscored", SkillTokens: 300},
			{ID: "used-unscored"},
			{ID: "misled", Digest: ""},
		},
		Uses: map[string]int{"healthy": 9, "low-pass": 4, "noisy": 2, "harmful": 1, "edited": 3, "used-unscored": 5, "misled": 2},
		Feedback: map[string]map[string]int{
			"misled":  {"misled": 2, "wrong": 1, "great": 1},
			"healthy": {"misled": 1, "great": 2},
		},
	})

	actions := map[string]RankRow{}
	var order []string
	for _, r := range rows {
		actions[r.ID] = r
		order = append(order, r.ID)
	}
	assert.Equal(t, ActionKeep, actions["healthy"].Action, "one misled against two great does not flip it")
	assert.Equal(t, ActionRewrite, actions["low-pass"].Action)
	assert.Contains(t, actions["low-pass"].Reasons[0], "pass rate 40% is below 80%")
	assert.Equal(t, ActionRewrite, actions["noisy"].Action)
	assert.Len(t, actions["noisy"].Reasons, 2, "precision and recall")
	assert.Contains(t, actions["harmful"].Reasons[0], "worse with the skill")
	assert.True(t, actions["edited"].Stale)
	assert.Contains(t, actions["edited"].Reasons[0], "changed after its last passing eval (2026-10-01)")
	assert.Equal(t, ActionRewrite, actions["misled"].Action)
	assert.Contains(t, actions["misled"].Reasons[0], "3 misled/wrong/stale against 1 great")

	assert.Equal(t, ActionPrune, actions["idle-useless"].Action)
	assert.Contains(t, actions["idle-useless"].Reasons, "costs about 900 tokens when loaded")
	assert.Equal(t, ActionPrune, actions["idle-unscored"].Action)
	assert.Contains(t, actions["idle-unscored"].Reasons, "no ablation evidence that it helps")
	assert.Equal(t, ActionReview, actions["idle-valuable"].Action)
	assert.Equal(t, ActionReview, actions["used-unscored"].Action)
	assert.Contains(t, actions["used-unscored"].Reasons[0], "no eval results")

	// rewrite rows first, most reasons first; then prune (largest token cost first)
	assert.Equal(t, "noisy", order[0], "two reasons sorts before one")
	assert.Equal(t, []string{"idle-useless", "idle-unscored"}, order[5:7])
	assert.Equal(t, "healthy", order[len(order)-1])
	assert.Equal(t, "5 rewrite, 2 prune, 2 review, 1 keep", SummaryLine(rows))
}

func TestRank_NoUsageLogConcludesNothingAboutUse(t *testing.T) {
	rows := Rank(RankInput{Store: NewStore(), Skills: []RankSkill{{ID: "a"}}})
	require.Len(t, rows, 1)
	assert.Equal(t, ActionReview, rows[0].Action)
	assert.Nil(t, rows[0].Uses)
}

func TestRank_ThresholdsAreConfigurable(t *testing.T) {
	store := NewStore()
	store.Put(rec("a", SkillScore{Scored: 4, PassRate: 0.7}, "d", false))
	in := RankInput{Store: store, Skills: []RankSkill{{ID: "a", Digest: "d"}}, Uses: map[string]int{"a": 1}}
	assert.Equal(t, ActionRewrite, Rank(in)[0].Action)
	in.MinPassRate = 0.5
	assert.Equal(t, ActionKeep, Rank(in)[0].Action)
}
